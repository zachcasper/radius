/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package connect

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/config"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/github"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/workspaces"
)

// NewCommand creates an instance of the command and runner for the `rad environment connect` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "connect",
		Short: "Configure OIDC authentication for a cloud provider",
		Long: `Configure OIDC authentication for a cloud provider (AWS or Azure).

This command sets up OpenID Connect (OIDC) authentication between GitHub Actions
and your cloud provider, enabling secure deployments without storing long-lived credentials.

For AWS, this will:
- Create or use an existing IAM OIDC identity provider for GitHub
- Create or use an existing IAM role with trust policy for GitHub Actions
- Create an S3 bucket and DynamoDB table for Terraform state management
- Update the environment file with AWS configuration

For Azure, this will:
- Create or use an existing Azure AD application with federated credentials
- Create an Azure Storage account for Terraform state management
- Update the environment file with Azure configuration`,
		Example: `# Connect the default environment
rad environment connect

# Connect a specific environment
rad environment connect --environment production`,
		Args: cobra.NoArgs,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddEnvironmentNameFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad environment connect` command.
type Runner struct {
	ConfigHolder        *framework.ConfigHolder
	Output              output.Interface
	Workspace           *workspaces.Workspace
	EnvironmentName     string
	InputPrompter       prompt.Interface
	ConfigFileInterface framework.ConfigFileInterface
	CommandRunner       github.CommandRunner
}

// NewRunner creates a new instance of the `rad environment connect` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:        factory.GetConfigHolder(),
		Output:              factory.GetOutput(),
		ConfigFileInterface: factory.GetConfigFileInterface(),
		InputPrompter:       factory.GetPrompter(),
		CommandRunner:       github.NewCommandRunner(),
	}
}

// Validate runs validation for the `rad environment connect` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	// Get workspace - required for GitHub mode
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	// FR-017: Validate the current workspace is a GitHub workspace
	if workspace.Connection == nil {
		return clierrors.Message("This command requires a GitHub workspace. Run 'rad init --github' first.")
	}

	kind, ok := workspace.Connection["kind"].(string)
	if !ok || kind != workspaces.KindGitHub {
		return clierrors.Message("This command requires a GitHub workspace. Current workspace kind: %v", kind)
	}

	// FR-016: Get environment name from flag or use default
	r.EnvironmentName, err = cmd.Flags().GetString("environment")
	if err != nil {
		return err
	}
	if r.EnvironmentName == "" {
		r.EnvironmentName = "default"
	}

	return nil
}

// Run runs the `rad environment connect` command.
func (r *Runner) Run(ctx context.Context) error {
	// Find the current directory and git root
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	repoPath, err := findGitRoot(cwd)
	if err != nil {
		return clierrors.Message("Current directory is not a git repository: %v", err)
	}

	radiusDir := filepath.Join(repoPath, ".radius")
	envFile := filepath.Join(radiusDir, fmt.Sprintf("env.%s.yaml", r.EnvironmentName))

	// Check if environment file exists
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		return clierrors.Message("Environment file not found: %s. Run 'rad init --github' to create the environment.", envFile)
	}

	// Load the environment file
	env, err := config.LoadEnvironment(envFile)
	if err != nil {
		return fmt.Errorf("failed to load environment: %w", err)
	}

	r.Output.LogInfo("Configuring OIDC authentication for environment '%s'...", r.EnvironmentName)
	r.Output.LogInfo("")

	// Route to provider-specific setup
	switch env.Kind {
	case config.ProviderKindAWS:
		if err := r.connectAWS(ctx, env, radiusDir); err != nil {
			return err
		}
	case config.ProviderKindAzure:
		if err := r.connectAzure(ctx, env, radiusDir); err != nil {
			return err
		}
	default:
		return clierrors.Message("Unsupported provider kind: %s. Expected 'aws' or 'azure'.", env.Kind)
	}

	// FR-031-A: Generate authentication test workflow
	if err := r.generateAuthTestWorkflow(repoPath, env.Kind); err != nil {
		return err
	}

	// FR-031: Commit changes with trailer
	r.Output.LogInfo("Committing environment changes...")
	if err := r.commitChanges(ctx, repoPath); err != nil {
		return err
	}

	// FR-031-F: Push changes and wait for workflow
	r.Output.LogInfo("Pushing changes to GitHub...")
	if err := r.pushChanges(ctx, repoPath); err != nil {
		r.Output.LogInfo("Warning: could not push changes: %v", err)
		r.Output.LogInfo("")
		r.Output.LogInfo("✅ OIDC authentication configured locally!")
		r.Output.LogInfo("")
		r.Output.LogInfo("Next steps:")
		r.Output.LogInfo("  1. Run 'git push' to push the changes to GitHub")
		r.Output.LogInfo("  2. The authentication test workflow will run automatically to verify OIDC setup")
		r.Output.LogInfo("  3. Run 'rad model' to create an application model")
		return nil
	}

	// FR-031-F/G/H: Wait for auth test workflow to complete
	providerName := "AWS"
	if env.Kind == config.ProviderKindAzure {
		providerName = "Azure"
	}

	if err := r.waitForAuthWorkflow(ctx, repoPath, providerName); err != nil {
		return err
	}

	r.Output.LogInfo("")
	r.Output.LogInfo("✅ OIDC authentication configured and verified successfully!")
	r.Output.LogInfo("")
	r.Output.LogInfo("Next steps:")
	r.Output.LogInfo("  1. Run 'rad model' to create an application model")
	r.Output.LogInfo("  2. Run 'rad pr create --environment %s' to create a deployment PR", r.EnvironmentName)

	return nil
}

// findGitRoot walks up the directory tree to find the .git directory.
func findGitRoot(startPath string) (string, error) {
	path := startPath
	for {
		gitPath := filepath.Join(path, ".git")
		if info, err := os.Stat(gitPath); err == nil && info.IsDir() {
			return path, nil
		}

		parent := filepath.Dir(path)
		if parent == path {
			return "", fmt.Errorf("not a git repository")
		}
		path = parent
	}
}

// connectAWS configures AWS OIDC authentication.
func (r *Runner) connectAWS(ctx context.Context, env *config.Environment, radiusDir string) error {
	r.Output.LogInfo("Setting up AWS OIDC authentication...")
	r.Output.LogInfo("")

	// FR-021: Verify AWS CLI authentication
	r.Output.LogInfo("Verifying AWS CLI authentication...")
	callerIdentity, err := r.CommandRunner.RunCommand(ctx, "aws", "sts", "get-caller-identity", "--output", "json")
	if err != nil {
		return clierrors.Message("AWS CLI authentication failed. Run 'aws configure' to set up credentials.\nError: %v", err)
	}
	r.Output.LogInfo("AWS CLI authenticated.")

	// FR-018: Prompt for account ID (default from caller identity)
	defaultAccountID := extractJSONField(callerIdentity, "Account")

	accountID, err := r.InputPrompter.GetTextInput("AWS Account ID", prompt.TextInputOptions{
		Default: defaultAccountID,
	})
	if err != nil {
		return err
	}

	// FR-019: Prompt for region
	defaultRegion, _ := r.CommandRunner.RunCommand(ctx, "aws", "configure", "get", "region")
	defaultRegion = strings.TrimSpace(defaultRegion)
	region, err := r.InputPrompter.GetTextInput("AWS Region", prompt.TextInputOptions{
		Default: defaultRegion,
	})
	if err != nil {
		return err
	}

	// FR-020: Prompt whether to create new role or use existing
	createNewRole, err := r.InputPrompter.GetListInput([]string{"Create new IAM role", "Use existing IAM role ARN"}, "OIDC IAM Role")
	if err != nil {
		return err
	}

	var roleARN string
	if createNewRole == "Use existing IAM role ARN" {
		roleARN, err = r.InputPrompter.GetTextInput("IAM Role ARN", prompt.TextInputOptions{})
		if err != nil {
			return err
		}
	} else {
		// Create new IAM role
		roleARN, err = r.createAWSRole(ctx, accountID, region)
		if err != nil {
			return err
		}
	}

	// Create state backend (S3 bucket + DynamoDB table)
	stateBackend, err := r.createAWSStateBackend(ctx, accountID, region)
	if err != nil {
		return err
	}

	// FR-067-C: Prompt for EKS cluster selection
	eksClusterName, err := r.promptForEKSCluster(ctx, region)
	if err != nil {
		return err
	}

	// FR-067-C: Prompt for Kubernetes namespace (press Enter for 'default')
	kubernetesNamespace, err := r.InputPrompter.GetTextInput("Kubernetes namespace (press Enter for 'default')", prompt.TextInputOptions{
		Default: "default",
	})
	if err != nil {
		return err
	}

	// FR-023: Update environment file with AWS configuration
	env.Provider.AWS = &config.AWSProviderConfig{
		AccountID:           accountID,
		Region:              region,
		OIDCRoleARN:         roleARN,
		EKSClusterName:      eksClusterName,
		KubernetesNamespace: kubernetesNamespace,
		StateBackend:        stateBackend,
	}

	if err := config.WriteEnvironmentWithHeader(radiusDir, env); err != nil {
		return fmt.Errorf("failed to update environment file: %w", err)
	}

	r.Output.LogInfo("AWS OIDC configuration saved to environment file.")

	// FR-030-A, FR-030-B: Set GitHub secrets for AWS workflow authentication
	r.Output.LogInfo("")
	r.Output.LogInfo("Setting GitHub repository secrets...")
	ghClient := github.NewClient()

	if err := ghClient.SetSecret("AWS_OIDC_ROLE_ARN", roleARN); err != nil {
		return fmt.Errorf("failed to set AWS_OIDC_ROLE_ARN secret: %w", err)
	}
	r.Output.LogInfo("  Set AWS_OIDC_ROLE_ARN")

	if err := ghClient.SetSecret("AWS_REGION", region); err != nil {
		return fmt.Errorf("failed to set AWS_REGION secret: %w", err)
	}
	r.Output.LogInfo("  Set AWS_REGION")

	return nil
}

// connectAzure configures Azure OIDC authentication.
func (r *Runner) connectAzure(ctx context.Context, env *config.Environment, radiusDir string) error {
	r.Output.LogInfo("Setting up Azure OIDC authentication...")
	r.Output.LogInfo("")

	// FR-027: Verify Azure CLI authentication
	r.Output.LogInfo("Verifying Azure CLI authentication...")
	accountInfo, err := r.CommandRunner.RunCommand(ctx, "az", "account", "show", "--output", "json")
	if err != nil {
		return clierrors.Message("Azure CLI authentication failed. Run 'az login' to sign in.\nError: %v", err)
	}
	r.Output.LogInfo("Azure CLI authenticated.")

	// Extract tenant ID from current account
	defaultTenantID := extractJSONField(accountInfo, "tenantId")

	// FR-024: Prompt for subscription from list
	subscriptionID, tenantID, err := r.promptForAzureSubscription(ctx, defaultTenantID)
	if err != nil {
		return err
	}

	// FR-025: Prompt for resource group - list existing or create new
	resourceGroupName, err := r.promptForAzureResourceGroup(ctx, subscriptionID)
	if err != nil {
		return err
	}

	// FR-026: Prompt whether to create new app or use existing
	createNewApp, err := r.InputPrompter.GetListInput([]string{"Create new Azure AD application", "Use existing Azure AD application"}, "Azure AD Application")
	if err != nil {
		return err
	}

	var clientID string
	if createNewApp == "Use existing Azure AD application" {
		// FR-026: List existing applications for selection
		clientID, err = r.promptForExistingAzureApp(ctx)
		if err != nil {
			return err
		}
	} else {
		// Set the subscription context to ensure we're in the correct tenant
		// This is required because Azure AD app creation uses the current tenant context
		r.Output.LogInfo("Setting subscription context...")
		_, err = r.CommandRunner.RunCommand(ctx, "az", "account", "set",
			"--subscription", subscriptionID,
		)
		if err != nil {
			return fmt.Errorf("failed to set subscription context: %w", err)
		}

		// Create new Azure AD app with federated credentials
		clientID, err = r.createAzureApp(ctx, subscriptionID, resourceGroupName)
		if err != nil {
			return err
		}
	}

	// Create state backend (Azure Storage)
	stateBackend, err := r.createAzureStateBackend(ctx, subscriptionID, resourceGroupName)
	if err != nil {
		return err
	}

	// FR-067-C: Prompt for AKS cluster selection
	aksClusterName, err := r.promptForAKSCluster(ctx, subscriptionID, resourceGroupName)
	if err != nil {
		return err
	}

	// FR-067-C: Prompt for Kubernetes namespace (press Enter for 'default')
	kubernetesNamespace, err := r.InputPrompter.GetTextInput("Kubernetes namespace (press Enter for 'default')", prompt.TextInputOptions{
		Default: "default",
	})
	if err != nil {
		return err
	}

	// FR-029: Update environment file with Azure configuration
	env.Provider.Azure = &config.AzureProviderConfig{
		SubscriptionID:      subscriptionID,
		ResourceGroupName:   resourceGroupName,
		TenantID:            tenantID,
		ClientID:            clientID,
		OIDCEnabled:         true,
		AKSClusterName:      aksClusterName,
		KubernetesNamespace: kubernetesNamespace,
		StateBackend:        stateBackend,
	}

	if err := config.WriteEnvironmentWithHeader(radiusDir, env); err != nil {
		return fmt.Errorf("failed to update environment file: %w", err)
	}

	r.Output.LogInfo("Azure OIDC configuration saved to environment file.")

	// FR-030-A, FR-030-C: Set GitHub secrets for Azure workflow authentication
	r.Output.LogInfo("")
	r.Output.LogInfo("Setting GitHub repository secrets...")
	ghClient := github.NewClient()

	if err := ghClient.SetSecret("AZURE_CLIENT_ID", clientID); err != nil {
		return fmt.Errorf("failed to set AZURE_CLIENT_ID secret: %w", err)
	}
	r.Output.LogInfo("  Set AZURE_CLIENT_ID")

	if err := ghClient.SetSecret("AZURE_TENANT_ID", tenantID); err != nil {
		return fmt.Errorf("failed to set AZURE_TENANT_ID secret: %w", err)
	}
	r.Output.LogInfo("  Set AZURE_TENANT_ID")

	if err := ghClient.SetSecret("AZURE_SUBSCRIPTION_ID", subscriptionID); err != nil {
		return fmt.Errorf("failed to set AZURE_SUBSCRIPTION_ID secret: %w", err)
	}
	r.Output.LogInfo("  Set AZURE_SUBSCRIPTION_ID")

	return nil
}

// promptForAzureSubscription prompts the user to select from available Azure subscriptions.
// Implements FR-024: prompt for subscription from az account list.
func (r *Runner) promptForAzureSubscription(ctx context.Context, defaultTenantID string) (subscriptionID, tenantID string, err error) {
	// Get list of subscriptions
	r.Output.LogInfo("Fetching available subscriptions...")
	subscriptionsJSON, err := r.CommandRunner.RunCommand(ctx, "az", "account", "list", "--output", "json")
	if err != nil {
		return "", "", fmt.Errorf("failed to list Azure subscriptions: %w", err)
	}

	// Parse subscriptions
	subscriptions, err := parseAzureSubscriptions(subscriptionsJSON)
	if err != nil {
		return "", "", err
	}

	if len(subscriptions) == 0 {
		return "", "", clierrors.Message("No Azure subscriptions found. Run 'az login' to sign in with access to subscriptions.")
	}

	// Build list of options for prompt
	options := make([]string, len(subscriptions))
	for i, sub := range subscriptions {
		options[i] = fmt.Sprintf("%s (%s)", sub.Name, sub.ID)
	}

	// Prompt user to select
	selected, err := r.InputPrompter.GetListInput(options, "Select Azure Subscription")
	if err != nil {
		return "", "", err
	}

	// Find the selected subscription
	for _, sub := range subscriptions {
		displayName := fmt.Sprintf("%s (%s)", sub.Name, sub.ID)
		if selected == displayName {
			return sub.ID, sub.TenantID, nil
		}
	}

	// Fallback - should not reach here
	return "", defaultTenantID, clierrors.Message("Failed to match selected subscription")
}

// azureSubscription represents an Azure subscription from az account list.
type azureSubscription struct {
	ID        string
	Name      string
	TenantID  string
	IsDefault bool
}

// parseAzureSubscriptions parses the JSON output of az account list.
func parseAzureSubscriptions(jsonStr string) ([]azureSubscription, error) {
	var rawSubs []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &rawSubs); err != nil {
		return nil, fmt.Errorf("failed to parse subscription list: %w", err)
	}

	subscriptions := make([]azureSubscription, 0, len(rawSubs))
	for _, raw := range rawSubs {
		sub := azureSubscription{}

		if id, ok := raw["id"].(string); ok {
			sub.ID = id
		}
		if name, ok := raw["name"].(string); ok {
			sub.Name = name
		}
		if tenantID, ok := raw["tenantId"].(string); ok {
			sub.TenantID = tenantID
		}
		if isDefault, ok := raw["isDefault"].(bool); ok {
			sub.IsDefault = isDefault
		}

		// Only include enabled subscriptions
		if state, ok := raw["state"].(string); ok && state == "Enabled" {
			subscriptions = append(subscriptions, sub)
		}
	}

	return subscriptions, nil
}

// promptForAzureResourceGroup prompts the user to select from existing resource groups or create new.
// Implements FR-025: prompt for resource group from az group list with option to create new.
func (r *Runner) promptForAzureResourceGroup(ctx context.Context, subscriptionID string) (string, error) {
	// Get list of resource groups
	r.Output.LogInfo("Fetching existing resource groups...")
	rgJSON, err := r.CommandRunner.RunCommand(ctx, "az", "group", "list",
		"--subscription", subscriptionID,
		"--output", "json",
	)
	if err != nil {
		// If we can't list resource groups, fall back to text input
		r.Output.LogInfo("Warning: could not list resource groups, prompting for name")
		return r.InputPrompter.GetTextInput("Azure Resource Group Name", prompt.TextInputOptions{
			Default: "radius-rg",
		})
	}

	// Parse resource groups
	resourceGroups, err := parseAzureResourceGroups(rgJSON)
	if err != nil {
		return r.InputPrompter.GetTextInput("Azure Resource Group Name", prompt.TextInputOptions{
			Default: "radius-rg",
		})
	}

	// Build options list: existing groups + create new option
	createNewOption := "Create new resource group"
	options := make([]string, 0, len(resourceGroups)+1)
	options = append(options, createNewOption)
	for _, rg := range resourceGroups {
		options = append(options, fmt.Sprintf("%s (%s)", rg.Name, rg.Location))
	}

	// Prompt user to select
	selected, err := r.InputPrompter.GetListInput(options, "Select Azure Resource Group")
	if err != nil {
		return "", err
	}

	if selected == createNewOption {
		// User wants to create a new resource group
		return r.InputPrompter.GetTextInput("New Resource Group Name", prompt.TextInputOptions{
			Default: "radius-rg",
		})
	}

	// Extract the resource group name from the selection
	for _, rg := range resourceGroups {
		displayName := fmt.Sprintf("%s (%s)", rg.Name, rg.Location)
		if selected == displayName {
			return rg.Name, nil
		}
	}

	// Fallback
	return "", clierrors.Message("Failed to match selected resource group")
}

// azureResourceGroup represents an Azure resource group from az group list.
type azureResourceGroup struct {
	Name     string
	Location string
}

// parseAzureResourceGroups parses the JSON output of az group list.
func parseAzureResourceGroups(jsonStr string) ([]azureResourceGroup, error) {
	var rawGroups []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &rawGroups); err != nil {
		return nil, fmt.Errorf("failed to parse resource group list: %w", err)
	}

	groups := make([]azureResourceGroup, 0, len(rawGroups))
	for _, raw := range rawGroups {
		rg := azureResourceGroup{}

		if name, ok := raw["name"].(string); ok {
			rg.Name = name
		}
		if location, ok := raw["location"].(string); ok {
			rg.Location = location
		}

		if rg.Name != "" {
			groups = append(groups, rg)
		}
	}

	return groups, nil
}

// promptForExistingAzureApp prompts the user to select from existing Azure AD applications.
// Implements FR-026: display list of applications from az ad app list.
func (r *Runner) promptForExistingAzureApp(ctx context.Context) (string, error) {
	// Get list of applications - use --all to include apps user has access to but doesn't own
	// Limit to 50 results to avoid loading thousands of apps in enterprise environments
	r.Output.LogInfo("Fetching Azure AD applications...")
	appsJSON, err := r.CommandRunner.RunCommand(ctx, "az", "ad", "app", "list",
		"--all",
		"--query", "[?starts_with(displayName, 'radius-')]", // Filter for radius apps first
		"--output", "json",
	)

	var apps []azureApplication
	if err == nil {
		apps, _ = parseAzureApplications(appsJSON)
	}

	// If no radius-prefixed apps found, try listing all recent apps
	if len(apps) == 0 {
		appsJSON, err = r.CommandRunner.RunCommand(ctx, "az", "ad", "app", "list",
			"--all",
			"--top", "50",
			"--output", "json",
		)
		if err == nil {
			apps, _ = parseAzureApplications(appsJSON)
		}
	}

	if len(apps) == 0 {
		// If we still can't list apps, fall back to text input
		r.Output.LogInfo("No applications found, prompting for ID")
		return r.InputPrompter.GetTextInput("Azure AD Application (Client) ID", prompt.TextInputOptions{})
	}

	// Build options list
	enterManualOption := "Enter application ID manually"
	options := make([]string, 0, len(apps)+1)
	for _, app := range apps {
		options = append(options, fmt.Sprintf("%s (%s)", app.DisplayName, app.AppID))
	}
	options = append(options, enterManualOption)

	// Prompt user to select
	selected, err := r.InputPrompter.GetListInput(options, "Select Azure AD Application")
	if err != nil {
		return "", err
	}

	if selected == enterManualOption {
		return r.InputPrompter.GetTextInput("Azure AD Application (Client) ID", prompt.TextInputOptions{})
	}

	// Extract the application ID from the selection
	for _, app := range apps {
		displayName := fmt.Sprintf("%s (%s)", app.DisplayName, app.AppID)
		if selected == displayName {
			return app.AppID, nil
		}
	}

	// Fallback
	return "", clierrors.Message("Failed to match selected application")
}

// azureApplication represents an Azure AD application from az ad app list.
type azureApplication struct {
	AppID       string
	DisplayName string
}

// parseAzureApplications parses the JSON output of az ad app list.
func parseAzureApplications(jsonStr string) ([]azureApplication, error) {
	var rawApps []map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &rawApps); err != nil {
		return nil, fmt.Errorf("failed to parse application list: %w", err)
	}

	apps := make([]azureApplication, 0, len(rawApps))
	for _, raw := range rawApps {
		app := azureApplication{}

		if appID, ok := raw["appId"].(string); ok {
			app.AppID = appID
		}
		if displayName, ok := raw["displayName"].(string); ok {
			app.DisplayName = displayName
		}

		if app.AppID != "" {
			apps = append(apps, app)
		}
	}

	return apps, nil
}

// createAWSRole creates an IAM role with GitHub OIDC trust policy.
func (r *Runner) createAWSRole(ctx context.Context, accountID, region string) (string, error) {
	// Get repo info from workspace
	repoURL, _ := r.Workspace.Connection["url"].(string)
	owner, repo := parseGitHubURL(repoURL)

	roleName := fmt.Sprintf("radius-%s-%s", owner, repo)

	r.Output.LogInfo("")
	r.Output.LogInfo("The following AWS resources will be created:")
	r.Output.LogInfo("  - IAM OIDC provider for GitHub Actions (if not exists)")
	r.Output.LogInfo("  - IAM role: %s", roleName)
	r.Output.LogInfo("")

	// FR-022: Request user confirmation
	confirm, err := r.InputPrompter.GetListInput([]string{"Yes, create these resources", "No, cancel"}, "Confirm")
	if err != nil {
		return "", err
	}
	if confirm != "Yes, create these resources" {
		return "", clierrors.Message("Operation cancelled by user.")
	}

	// Create OIDC provider if not exists
	r.Output.LogInfo("Creating IAM OIDC provider for GitHub...")
	_, err = r.CommandRunner.RunCommand(ctx, "aws", "iam", "create-open-id-connect-provider",
		"--url", "https://token.actions.githubusercontent.com",
		"--client-id-list", "sts.amazonaws.com",
		"--thumbprint-list", "6938fd4d98bab03faadb97b34396831e3780aea1",
	)
	// Ignore error if already exists
	if err != nil && !strings.Contains(err.Error(), "EntityAlreadyExists") {
		return "", fmt.Errorf("failed to create OIDC provider: %w", err)
	}

	// Create trust policy document
	trustPolicy := fmt.Sprintf(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::%s:oidc-provider/token.actions.githubusercontent.com"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "token.actions.githubusercontent.com:aud": "sts.amazonaws.com"
        },
        "StringLike": {
          "token.actions.githubusercontent.com:sub": "repo:%s/%s:*"
        }
      }
    }
  ]
}`, accountID, owner, repo)

	// Create the IAM role
	r.Output.LogInfo("Creating IAM role: %s...", roleName)
	_, err = r.CommandRunner.RunCommand(ctx, "aws", "iam", "create-role",
		"--role-name", roleName,
		"--assume-role-policy-document", trustPolicy,
		"--description", "Radius deployment role for GitHub Actions",
	)
	if err != nil && !strings.Contains(err.Error(), "EntityAlreadyExists") {
		return "", fmt.Errorf("failed to create IAM role: %w", err)
	}

	// Attach necessary policies
	r.Output.LogInfo("Attaching policies to role...")
	policies := []string{
		"arn:aws:iam::aws:policy/AmazonEC2FullAccess",
		"arn:aws:iam::aws:policy/AmazonS3FullAccess",
		"arn:aws:iam::aws:policy/AmazonDynamoDBFullAccess",
	}
	for _, policy := range policies {
		_, err = r.CommandRunner.RunCommand(ctx, "aws", "iam", "attach-role-policy",
			"--role-name", roleName,
			"--policy-arn", policy,
		)
		if err != nil {
			// Ignore if already attached
			r.Output.LogInfo("Warning: could not attach policy %s: %v", policy, err)
		}
	}

	roleARN := fmt.Sprintf("arn:aws:iam::%s:role/%s", accountID, roleName)
	r.Output.LogInfo("IAM role created: %s", roleARN)

	return roleARN, nil
}

// createAWSStateBackend creates S3 bucket and DynamoDB table for Terraform state.
func (r *Runner) createAWSStateBackend(ctx context.Context, accountID, region string) (*config.AWSStateBackend, error) {
	repoURL, _ := r.Workspace.Connection["url"].(string)
	owner, repo := parseGitHubURL(repoURL)

	// FR-097-B: S3 bucket naming convention
	bucketName := fmt.Sprintf("tfstate-%s-%s", owner, repo)
	tableName := fmt.Sprintf("tfstate-lock-%s-%s", owner, repo)

	r.Output.LogInfo("")
	r.Output.LogInfo("Creating Terraform state backend...")

	// FR-047, FR-093: Create S3 bucket for state storage
	r.Output.LogInfo("Creating S3 bucket: %s...", bucketName)
	_, err := r.CommandRunner.RunCommand(ctx, "aws", "s3api", "create-bucket",
		"--bucket", bucketName,
		"--region", region,
		"--create-bucket-configuration", fmt.Sprintf("LocationConstraint=%s", region),
	)
	if err != nil && !strings.Contains(err.Error(), "BucketAlreadyOwnedByYou") {
		return nil, fmt.Errorf("failed to create S3 bucket: %w", err)
	}

	// Enable versioning
	_, err = r.CommandRunner.RunCommand(ctx, "aws", "s3api", "put-bucket-versioning",
		"--bucket", bucketName,
		"--versioning-configuration", "Status=Enabled",
	)
	if err != nil {
		r.Output.LogInfo("Warning: could not enable bucket versioning: %v", err)
	}

	// FR-096: Create DynamoDB table for state locking
	r.Output.LogInfo("Creating DynamoDB table for state locking: %s...", tableName)
	_, err = r.CommandRunner.RunCommand(ctx, "aws", "dynamodb", "create-table",
		"--table-name", tableName,
		"--attribute-definitions", "AttributeName=LockID,AttributeType=S",
		"--key-schema", "AttributeName=LockID,KeyType=HASH",
		"--billing-mode", "PAY_PER_REQUEST",
		"--region", region,
	)
	if err != nil && !strings.Contains(err.Error(), "ResourceInUseException") {
		return nil, fmt.Errorf("failed to create DynamoDB table: %w", err)
	}

	r.Output.LogInfo("Terraform state backend created.")

	// FR-097: Return state backend configuration
	return &config.AWSStateBackend{
		Bucket:        bucketName,
		Region:        region,
		DynamoDBTable: tableName,
	}, nil
}

// createAzureApp creates an Azure AD application with federated credentials.
func (r *Runner) createAzureApp(ctx context.Context, subscriptionID, resourceGroupName string) (string, error) {
	repoURL, _ := r.Workspace.Connection["url"].(string)
	owner, repo := parseGitHubURL(repoURL)

	appName := fmt.Sprintf("radius-%s-%s", owner, repo)

	r.Output.LogInfo("")
	r.Output.LogInfo("The following Azure resources will be created:")
	r.Output.LogInfo("  - Azure AD application: %s", appName)
	r.Output.LogInfo("  - Federated credential for GitHub Actions")
	r.Output.LogInfo("  - Resource group: %s (if not exists)", resourceGroupName)
	r.Output.LogInfo("")

	// FR-028: Request user confirmation
	confirm, err := r.InputPrompter.GetListInput([]string{"Yes, create these resources", "No, cancel"}, "Confirm")
	if err != nil {
		return "", err
	}
	if confirm != "Yes, create these resources" {
		return "", clierrors.Message("Operation cancelled by user.")
	}

	// Create resource group if not exists
	r.Output.LogInfo("Creating resource group: %s...", resourceGroupName)
	_, err = r.CommandRunner.RunCommand(ctx, "az", "group", "create",
		"--name", resourceGroupName,
		"--location", "eastus",
		"--subscription", subscriptionID,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create resource group: %w", err)
	}

	// Create Azure AD application
	// First check if an app with this name already exists to avoid duplicates
	r.Output.LogInfo("Creating Azure AD application: %s...", appName)

	existingAppOutput, _ := r.CommandRunner.RunCommand(ctx, "az", "ad", "app", "list",
		"--display-name", appName,
		"--output", "json",
	)

	var clientID, objectID string

	// Check if app already exists
	var existingApps []map[string]interface{}
	if err := json.Unmarshal([]byte(existingAppOutput), &existingApps); err == nil && len(existingApps) > 0 {
		// Use existing app
		if appID, ok := existingApps[0]["appId"].(string); ok {
			clientID = appID
		}
		if objID, ok := existingApps[0]["id"].(string); ok {
			objectID = objID
		}
		r.Output.LogInfo("Using existing Azure AD application: %s", clientID)
	} else {
		// Create new app
		appOutput, err := r.CommandRunner.RunCommand(ctx, "az", "ad", "app", "create",
			"--display-name", appName,
			"--output", "json",
		)
		if err != nil {
			return "", fmt.Errorf("failed to create Azure AD application: %w", err)
		}

		clientID = extractJSONField(appOutput, "appId")
		objectID = extractJSONField(appOutput, "id")
	}

	// Create service principal
	r.Output.LogInfo("Creating service principal...")
	_, err = r.CommandRunner.RunCommand(ctx, "az", "ad", "sp", "create",
		"--id", clientID,
	)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return "", fmt.Errorf("failed to create service principal: %w", err)
	}

	// Assign Contributor role to the service principal at resource group level
	r.Output.LogInfo("Assigning Contributor role to resource group...")
	_, err = r.CommandRunner.RunCommand(ctx, "az", "role", "assignment", "create",
		"--assignee", clientID,
		"--role", "Contributor",
		"--scope", fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", subscriptionID, resourceGroupName),
	)
	if err != nil {
		r.Output.LogInfo("Warning: could not assign Contributor role to resource group: %v", err)
	}

	// Also assign Reader role at subscription level so azure/login can access the subscription
	r.Output.LogInfo("Assigning Reader role to subscription...")
	_, err = r.CommandRunner.RunCommand(ctx, "az", "role", "assignment", "create",
		"--assignee", clientID,
		"--role", "Reader",
		"--scope", fmt.Sprintf("/subscriptions/%s", subscriptionID),
	)
	if err != nil {
		r.Output.LogInfo("Warning: could not assign Reader role to subscription: %v", err)
	}

	// FR-052: Create federated credential for GitHub Actions
	r.Output.LogInfo("Creating federated credential for GitHub Actions...")
	fedCredParams := fmt.Sprintf(`{
  "name": "github-actions",
  "issuer": "https://token.actions.githubusercontent.com",
  "subject": "repo:%s/%s:ref:refs/heads/main",
  "audiences": ["api://AzureADTokenExchange"],
  "description": "GitHub Actions OIDC"
}`, owner, repo)

	_, err = r.CommandRunner.RunCommand(ctx, "az", "ad", "app", "federated-credential", "create",
		"--id", objectID,
		"--parameters", fedCredParams,
	)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return "", fmt.Errorf("failed to create federated credential: %w", err)
	}

	// Verify the app exists and get tenant info for debugging
	accountOutput, err := r.CommandRunner.RunCommand(ctx, "az", "account", "show", "--output", "json")
	if err == nil {
		currentTenant := extractJSONField(accountOutput, "tenantId")
		r.Output.LogInfo("Azure AD application created in tenant: %s", currentTenant)
	}

	r.Output.LogInfo("Azure AD application created: %s", clientID)
	r.Output.LogInfo("Client ID: %s", clientID)

	return clientID, nil
}

// createAzureStateBackend creates Azure Storage for Terraform state.
func (r *Runner) createAzureStateBackend(ctx context.Context, subscriptionID, resourceGroupName string) (*config.AzureStateBackend, error) {
	repoURL, _ := r.Workspace.Connection["url"].(string)
	owner, repo := parseGitHubURL(repoURL)

	// FR-097-A: Azure Storage account naming convention
	// Storage account names must be 3-24 chars, lowercase alphanumeric only
	storageAccountName := sanitizeStorageAccountName(fmt.Sprintf("tfstateradius%s%s", owner, repo))
	containerName := "tfstate"

	r.Output.LogInfo("")
	r.Output.LogInfo("Creating Terraform state backend...")

	// Create storage account
	r.Output.LogInfo("Creating storage account: %s...", storageAccountName)
	_, err := r.CommandRunner.RunCommand(ctx, "az", "storage", "account", "create",
		"--name", storageAccountName,
		"--resource-group", resourceGroupName,
		"--subscription", subscriptionID,
		"--sku", "Standard_LRS",
		"--kind", "StorageV2",
		"--min-tls-version", "TLS1_2",
	)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return nil, fmt.Errorf("failed to create storage account: %w", err)
	}

	// Create blob container
	r.Output.LogInfo("Creating blob container: %s...", containerName)
	_, err = r.CommandRunner.RunCommand(ctx, "az", "storage", "container", "create",
		"--name", containerName,
		"--account-name", storageAccountName,
		"--auth-mode", "login",
	)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return nil, fmt.Errorf("failed to create storage container: %w", err)
	}

	r.Output.LogInfo("Terraform state backend created.")

	return &config.AzureStateBackend{
		StorageAccountName: storageAccountName,
		ContainerName:      containerName,
	}, nil
}

// generateAuthTestWorkflow creates the OIDC authentication test workflow.
// FR-031-A: Creates .github/workflows/radius-auth-test.yaml
func (r *Runner) generateAuthTestWorkflow(repoPath string, provider string) error {
	r.Output.LogInfo("")
	r.Output.LogInfo("Creating authentication test workflow...")

	workflowsDir := filepath.Join(repoPath, ".github", "workflows")
	if err := os.MkdirAll(workflowsDir, 0755); err != nil {
		return fmt.Errorf("failed to create workflows directory: %w", err)
	}

	workflow := github.GenerateAuthTestWorkflow(provider)
	workflowPath := filepath.Join(workflowsDir, "radius-auth-test.yaml")

	if err := github.SaveWorkflow(workflowPath, workflow); err != nil {
		return fmt.Errorf("failed to save auth test workflow: %w", err)
	}

	r.Output.LogInfo("Created authentication test workflow: .github/workflows/radius-auth-test.yaml")
	return nil
}

// commitChanges commits the environment changes with the appropriate trailer.
func (r *Runner) commitChanges(ctx context.Context, repoPath string) error {
	gitHelper, err := github.NewGitHelper(repoPath)
	if err != nil {
		return fmt.Errorf("failed to access git repository: %w", err)
	}

	// Stage the environment file (.radius directory)
	if err := gitHelper.Add(".radius"); err != nil {
		return fmt.Errorf("failed to stage environment file: %w", err)
	}

	// Stage the auth test workflow (.github directory)
	if err := gitHelper.Add(".github"); err != nil {
		return fmt.Errorf("failed to stage workflow file: %w", err)
	}

	// FR-031: Commit with trailer
	_, err = gitHelper.CommitWithTrailer(
		fmt.Sprintf("Configure OIDC authentication for %s environment", r.EnvironmentName),
		"environment-connect",
	)
	if err != nil {
		return fmt.Errorf("failed to commit changes: %w", err)
	}

	return nil
}

// pushChanges pushes the committed changes to the remote.
func (r *Runner) pushChanges(ctx context.Context, repoPath string) error {
	gitHelper, err := github.NewGitHelper(repoPath)
	if err != nil {
		return fmt.Errorf("failed to access git repository: %w", err)
	}

	if err := gitHelper.Push(); err != nil {
		return fmt.Errorf("failed to push changes: %w", err)
	}

	return nil
}

// waitForAuthWorkflow waits for the auth test workflow to complete and reports the result.
// Implements FR-031-F, FR-031-G, FR-031-H.
func (r *Runner) waitForAuthWorkflow(ctx context.Context, repoPath string, providerName string) error {
	ghClient := github.NewClient()

	// Wait a bit for GitHub to process the push and trigger the workflow
	r.Output.LogInfo("")
	r.Output.LogInfo("Waiting for authentication test workflow to start...")
	time.Sleep(5 * time.Second)

	// Look for a PENDING (queued or in_progress) workflow run
	// D025 fix: Don't use GetLatestWorkflowRun as it may return a completed run from a previous execution
	var run *github.WorkflowRun
	var err error

	// Try to find a pending workflow run (may take a few seconds to appear)
	// Timeout after 2 minutes of waiting for a run to appear
	timeoutAt := time.Now().Add(2 * time.Minute)
	for time.Now().Before(timeoutAt) {
		run, err = ghClient.GetPendingWorkflowRun("radius-auth-test.yaml")
		if err == nil && run != nil {
			break
		}
		time.Sleep(3 * time.Second)
	}

	if run == nil {
		r.Output.LogInfo("Warning: could not find pending workflow run. The workflow may not have triggered.")
		r.Output.LogInfo("Check GitHub Actions at: https://github.com/<owner>/<repo>/actions")
		return nil
	}

	// FR-031-F: Display animated progress with fast spinner
	r.Output.LogInfo("")
	spinChars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinIdx := 0
	done := make(chan struct{})

	// Run spinner animation in background (100ms interval for smooth animation)
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				spinner := spinChars[spinIdx%len(spinChars)]
				spinIdx++
				fmt.Printf("\r%s Verifying access to %s...", spinner, providerName)
			}
		}
	}()

	finalRun, err := ghClient.WatchWorkflowRun(run.ID, nil)
	close(done)
	fmt.Printf("\r                                                    \r") // Clear spinner line

	if err != nil {
		return fmt.Errorf("failed to watch workflow: %w", err)
	}

	// FR-031-G/H: Report success or failure
	if finalRun.Conclusion == "success" {
		r.Output.LogInfo("✅ Authentication verified successfully!")
		return nil
	}

	// FR-031-H: Show error logs on failure
	r.Output.LogInfo("❌ Authentication verification failed!")
	r.Output.LogInfo("")
	r.Output.LogInfo("Workflow logs:")
	logs, err := ghClient.GetWorkflowRunLogs(finalRun.ID)
	if err == nil && logs != "" {
		// Truncate logs if too long
		if len(logs) > 2000 {
			logs = logs[len(logs)-2000:]
		}
		r.Output.LogInfo(logs)
	}
	r.Output.LogInfo("")
	r.Output.LogInfo("For full details, see: %s", finalRun.URL)

	return clierrors.Message("Authentication test failed. Please check the workflow logs and fix any issues.")
}

// Helper functions

func extractJSONField(jsonStr, field string) string {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return ""
	}
	if val, ok := data[field]; ok {
		return fmt.Sprintf("%v", val)
	}
	return ""
}

func parseGitHubURL(url string) (owner, repo string) {
	// Parse https://github.com/owner/repo format
	// Remove trailing .git if present
	url = strings.TrimSuffix(url, ".git")
	parts := strings.Split(url, "/")
	if len(parts) >= 2 {
		repo = parts[len(parts)-1]
		owner = parts[len(parts)-2]
	}
	return
}

func sanitizeStorageAccountName(name string) string {
	// Storage account names: 3-24 chars, lowercase letters and numbers only
	result := strings.ToLower(name)
	var sanitized strings.Builder
	for _, c := range result {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			sanitized.WriteRune(c)
		}
	}
	s := sanitized.String()
	if len(s) > 24 {
		s = s[:24]
	}
	return s
}

// promptForEKSCluster prompts the user to select from available EKS clusters.
// Implements FR-067-C.
func (r *Runner) promptForEKSCluster(ctx context.Context, region string) (string, error) {
	r.Output.LogInfo("Fetching EKS clusters in region %s...", region)

	// List EKS clusters
	clustersJSON, err := r.CommandRunner.RunCommand(ctx, "aws", "eks", "list-clusters", "--region", region, "--output", "json")
	if err != nil {
		// If listing fails, fall back to manual entry
		r.Output.LogInfo("Could not list EKS clusters, please enter cluster name manually.")
		return r.InputPrompter.GetTextInput("EKS Cluster Name", prompt.TextInputOptions{})
	}

	// Parse clusters
	clusters := parseEKSClusters(clustersJSON)
	if len(clusters) == 0 {
		r.Output.LogInfo("No EKS clusters found in region %s, please enter cluster name manually.", region)
		return r.InputPrompter.GetTextInput("EKS Cluster Name", prompt.TextInputOptions{})
	}

	// Add "Enter manually" option
	options := append(clusters, "Enter cluster name manually")

	selection, err := r.InputPrompter.GetListInput(options, "Select EKS cluster")
	if err != nil {
		return "", err
	}

	if selection == "Enter cluster name manually" {
		return r.InputPrompter.GetTextInput("EKS Cluster Name", prompt.TextInputOptions{})
	}

	return selection, nil
}

// parseEKSClusters parses EKS cluster names from AWS CLI JSON output.
func parseEKSClusters(jsonStr string) []string {
	var result struct {
		Clusters []string `json:"clusters"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil
	}
	return result.Clusters
}

// promptForAKSCluster prompts the user to select from available AKS clusters.
// Implements FR-067-C.
func (r *Runner) promptForAKSCluster(ctx context.Context, subscriptionID, resourceGroup string) (string, error) {
	r.Output.LogInfo("Fetching AKS clusters...")

	// List AKS clusters - try resource group first, then subscription-wide
	var clustersJSON string
	var err error

	// First try within the resource group
	clustersJSON, err = r.CommandRunner.RunCommand(ctx, "az", "aks", "list",
		"--resource-group", resourceGroup,
		"--subscription", subscriptionID,
		"--output", "json")

	if err != nil || clustersJSON == "[]" {
		// Fall back to subscription-wide listing
		clustersJSON, err = r.CommandRunner.RunCommand(ctx, "az", "aks", "list",
			"--subscription", subscriptionID,
			"--output", "json")
	}

	if err != nil {
		// If listing fails, fall back to manual entry
		r.Output.LogInfo("Could not list AKS clusters, please enter cluster name manually.")
		return r.InputPrompter.GetTextInput("AKS Cluster Name", prompt.TextInputOptions{})
	}

	// Parse clusters
	clusters := parseAKSClusters(clustersJSON)
	if len(clusters) == 0 {
		r.Output.LogInfo("No AKS clusters found, please enter cluster name manually.")
		return r.InputPrompter.GetTextInput("AKS Cluster Name", prompt.TextInputOptions{})
	}

	// Add "Enter manually" option
	options := append(clusters, "Enter cluster name manually")

	selection, err := r.InputPrompter.GetListInput(options, "Select AKS cluster")
	if err != nil {
		return "", err
	}

	if selection == "Enter cluster name manually" {
		return r.InputPrompter.GetTextInput("AKS Cluster Name", prompt.TextInputOptions{})
	}

	return selection, nil
}

// parseAKSClusters parses AKS cluster names from Azure CLI JSON output.
func parseAKSClusters(jsonStr string) []string {
	var clusters []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &clusters); err != nil {
		return nil
	}

	names := make([]string, 0, len(clusters))
	for _, c := range clusters {
		names = append(names, c.Name)
	}
	return names
}
