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

package create

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	corerp "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/to"
	"github.com/spf13/cobra"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/cmd/env/namespace"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/github"
	"github.com/radius-project/radius/pkg/cli/kubernetes"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_radius "github.com/radius-project/radius/pkg/ucp/resources/radius"
)

// NewCommand creates an instance of the command and runner for the `rad env create` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "create [envName]",
		Short: "Create a new Radius Environment",
		Long: `Create a new Radius Environment
Radius Environments are prepared "landing zones" for Radius Applications.
Applications deployed to an environment will inherit the container runtime, configuration, and other settings from the environment.

In GitHub mode, this command:
- Creates a GitHub Environment
- Sets up OIDC authentication with your cloud provider (AWS or Azure)
- Stores configuration as GitHub Environment variables
- Dispatches an auth test workflow to verify connectivity`,
		Args: cobra.ExactArgs(1),
		Example: `# Create an environment (Kubernetes mode)
rad env create myenv

# Create an environment with cloud provider (GitHub mode)
rad env create dev --provider azure

# Create an environment with custom recipes manifest
rad env create prod --provider aws --recipes https://example.com/recipes.yaml`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddEnvironmentNameFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddNamespaceFlag(cmd)

	// GitHub mode flags
	cmd.Flags().String("provider", "", "Cloud provider for the environment (aws or azure)")
	cmd.Flags().String("recipes", "", "URL to a custom recipes manifest")

	return cmd, runner
}

// Runner is the runner implementation for the `rad env create` command.
type Runner struct {
	ConfigHolder        *framework.ConfigHolder
	Output              output.Interface
	Workspace           *workspaces.Workspace
	EnvironmentName     string
	ResourceGroupName   string
	Namespace           string
	ConnectionFactory   connections.Factory
	ConfigFileInterface framework.ConfigFileInterface
	KubernetesInterface kubernetes.Interface
	NamespaceInterface  namespace.Interface
	Prompter            prompt.Interface
	CommandRunner       github.CommandRunner

	// GitHub mode fields
	Provider string
	Recipes  string
}

// NewRunner creates a new instance of the `rad env create` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:        factory.GetConfigHolder(),
		Output:              factory.GetOutput(),
		ConnectionFactory:   factory.GetConnectionFactory(),
		ConfigFileInterface: factory.GetConfigFileInterface(),
		KubernetesInterface: factory.GetKubernetesInterface(),
		NamespaceInterface:  factory.GetNamespaceInterface(),
		Prompter:            factory.GetPrompter(),
		CommandRunner:       github.NewCommandRunner(),
	}
}

// Validate runs validation for the `rad env create` command.
// Branches on workspace kind: GitHub mode validates provider flag,
// Kubernetes mode validates scope and namespace.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	r.EnvironmentName = args[0]

	// Branch on workspace kind
	if r.Workspace.IsGitHubWorkspace() {
		return r.validateGitHubMode(cmd)
	}

	return r.validateKubernetesMode(cmd)
}

// validateGitHubMode validates GitHub mode specific flags.
func (r *Runner) validateGitHubMode(cmd *cobra.Command) error {
	// --provider is required in GitHub mode
	provider, err := cmd.Flags().GetString("provider")
	if err != nil {
		return err
	}
	if provider == "" {
		return clierrors.Message("--provider flag is required for GitHub mode (use 'aws' or 'azure')")
	}
	if provider != "aws" && provider != "azure" {
		return clierrors.Message("--provider must be 'aws' or 'azure', got: %s", provider)
	}
	r.Provider = provider

	// --recipes is optional
	r.Recipes, err = cmd.Flags().GetString("recipes")
	if err != nil {
		return err
	}

	return nil
}

// validateKubernetesMode validates Kubernetes mode specific flags.
func (r *Runner) validateKubernetesMode(cmd *cobra.Command) error {
	var err error

	r.EnvironmentName, err = cli.RequireEnvironmentNameArgs(cmd, []string{r.EnvironmentName}, *r.Workspace)
	if err != nil {
		return err
	}

	r.Workspace.Scope, err = cli.RequireScope(cmd, *r.Workspace)
	if err != nil {
		return err
	}

	r.Namespace, err = cmd.Flags().GetString("namespace")
	if err != nil {
		return err
	} else if r.Namespace == "" {
		r.Namespace = r.EnvironmentName
	}

	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(cmd.Context(), *r.Workspace)
	if err != nil {
		return err
	}

	scopeId, err := resources.Parse(r.Workspace.Scope)
	if err != nil {
		return err
	}
	r.ResourceGroupName = scopeId.FindScope(resources_radius.ScopeResourceGroups)

	_, err = client.GetResourceGroup(cmd.Context(), "local", r.ResourceGroupName)
	if clients.Is404Error(err) {
		return clierrors.Message("Resource group %q could not be found.", r.ResourceGroupName)
	} else if err != nil {
		return err
	}

	return nil
}

// Run runs the `rad env create` command.
// Branches on workspace kind for GitHub vs Kubernetes mode.
func (r *Runner) Run(ctx context.Context) error {
	if r.Workspace.IsGitHubWorkspace() {
		return r.runGitHubMode(ctx)
	}

	return r.runKubernetesMode(ctx)
}

// runKubernetesMode creates an environment via UCP (original behavior).
func (r *Runner) runKubernetesMode(ctx context.Context) error {
	r.Output.LogInfo("Creating Environment...")

	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	resource := &corerp.EnvironmentResource{
		Location: to.Ptr(v1.LocationGlobal),
		Properties: &corerp.EnvironmentProperties{
			Compute: &corerp.KubernetesCompute{
				Namespace: to.Ptr(r.Namespace),
			},
		},
	}

	err = client.CreateOrUpdateEnvironment(ctx, r.EnvironmentName, resource)
	if err != nil {
		return err
	}
	r.Output.LogInfo("Successfully created environment %q in resource group %q", r.EnvironmentName, r.ResourceGroupName)

	return nil
}

// runGitHubMode creates a GitHub Environment, sets up OIDC, stores env variables,
// and dispatches an auth test workflow.
// FR-022 through FR-030: Full GitHub environment creation flow.
func (r *Runner) runGitHubMode(ctx context.Context) error {
	// Extract owner/repo from workspace URL
	repoURL, _ := r.Workspace.Connection["url"].(string)
	owner, repo := parseGitHubURL(repoURL)
	if owner == "" || repo == "" {
		return clierrors.Message("Could not parse GitHub repository from workspace URL: %s", repoURL)
	}

	r.Output.LogInfo("Creating environment %q for %s/%s...", r.EnvironmentName, owner, repo)

	// Step 1: Create GitHub Environment (FR-022)
	r.Output.LogInfo("Creating GitHub Environment...")
	ghClient := github.NewClient()
	if err := ghClient.CreateEnvironment(owner, repo, r.EnvironmentName); err != nil {
		return fmt.Errorf("failed to create GitHub Environment: %w", err)
	}
	r.Output.LogInfo("GitHub Environment %q created.", r.EnvironmentName)

	// Step 2: Run OIDC setup based on provider (FR-024/FR-028)
	oidcSetup := github.NewOIDCSetup(r.Output, r.Prompter, r.CommandRunner, owner, repo)

	r.Output.LogInfo("")

	switch r.Provider {
	case "aws":
		result, err := oidcSetup.SetupAWSOIDC(ctx)
		if err != nil {
			return err
		}

		// Step 3: Set environment variables (FR-025)
		r.Output.LogInfo("")
		r.Output.LogInfo("Setting GitHub Environment variables for '%s'...", r.EnvironmentName)
		if err := oidcSetup.SetAWSEnvironmentVariables(r.EnvironmentName, result); err != nil {
			return err
		}

	case "azure":
		result, err := oidcSetup.SetupAzureOIDC(ctx, r.EnvironmentName)
		if err != nil {
			return err
		}

		// Step 3: Set environment variables (FR-029)
		r.Output.LogInfo("")
		r.Output.LogInfo("Setting GitHub Environment variables for '%s'...", r.EnvironmentName)
		if err := oidcSetup.SetAzureEnvironmentVariables(r.EnvironmentName, result); err != nil {
			return err
		}
	}

	// Step 4: Set RADIUS_RECIPES_MANIFEST env variable (FR-026, FR-027, FR-030)
	recipesManifest := r.Recipes
	if recipesManifest == "" {
		// Use default recipes manifest from resource-types-contrib repo based on provider
		// FR-026: Azure defaults to Bicep, FR-030: AWS defaults to Terraform
		switch r.Provider {
		case "aws":
			recipesManifest = "https://raw.githubusercontent.com/zachcasper/resource-types-contrib/refs/heads/github-radius/config/recipes-aws-terraform.yaml"
		case "azure":
			recipesManifest = "https://raw.githubusercontent.com/zachcasper/resource-types-contrib/refs/heads/github-radius/config/recipes-azure-bicep.yaml"
		default:
			recipesManifest = "https://raw.githubusercontent.com/zachcasper/resource-types-contrib/refs/heads/github-radius/config/recipes-azure-bicep.yaml"
		}
	}
	r.Output.LogInfo("  Setting RADIUS_RECIPES_MANIFEST...")
	if err := ghClient.SetEnvironmentVariable(owner, repo, r.EnvironmentName, "RADIUS_RECIPES_MANIFEST", recipesManifest); err != nil {
		return fmt.Errorf("failed to set RADIUS_RECIPES_MANIFEST: %w", err)
	}

	// Step 5: Dispatch auth test workflow (FR-030-E through FR-030-I)
	r.Output.LogInfo("")
	r.Output.LogInfo("Creating authentication test workflow...")
	if err := r.dispatchAuthTest(ctx, r.EnvironmentName, r.Provider); err != nil {
		// Don't fail the command if auth test dispatch fails
		r.Output.LogInfo("Warning: could not dispatch auth test: %v", err)
		r.Output.LogInfo("You can manually run the auth test workflow from GitHub Actions.")
	}

	r.Output.LogInfo("")
	r.Output.LogInfo("Environment %q created successfully!", r.EnvironmentName)
	r.Output.LogInfo("")
	r.Output.LogInfo("Next steps:")
	r.Output.LogInfo("  1. Run 'rad app model' to create an application definition")
	r.Output.LogInfo("  2. Run 'rad deploy <bicep-file> --environment %s' to deploy your application", r.EnvironmentName)

	return nil
}

// dispatchAuthTest dispatches the auth test workflow and shows animated progress.
// FR-030-E through FR-030-I: Dispatch, watch with animated progress and L key support,
// report success/failure with hints.
func (r *Runner) dispatchAuthTest(ctx context.Context, envName string, provider string) error {
	ghClient := github.NewClient()

	// Get the repository's default branch for dispatch
	repoInfo, err := ghClient.GetRepoInfo()
	if err != nil {
		return fmt.Errorf("failed to get repository info: %w", err)
	}
	defaultBranch := repoInfo.DefaultBranchRef.Name
	if defaultBranch == "" {
		defaultBranch = "main" // Fallback if not set
	}

	inputs := map[string]string{
		"environment": envName,
	}

	// FR-030-E/FR-030-F: Dispatch and get run ID
	runID, runURL, err := ghClient.DispatchAndWatch(
		github.AuthTestWorkflowFile,
		defaultBranch,
		inputs,
		func() {
			r.Output.LogInfo("Auth test workflow queued, waiting for runner...")
		},
	)
	if err != nil {
		return err
	}

	// FR-030-G: Animated progress with auto step display
	pollFunc := ghClient.CreatePollFunc(runID)
	model := github.NewProgressModel(fmt.Sprintf("Testing authentication to %s", provider), pollFunc)
	model.RunURL = runURL
	model.StepPollFunc = ghClient.CreateStepPollFunc(runID)

	p := tea.NewProgram(model)
	finalModel, err := r.Prompter.RunProgram(p)
	if err != nil {
		return fmt.Errorf("progress display error: %w", err)
	}

	// FR-030-H/FR-030-I: Report success or failure
	if pm, ok := finalModel.(github.ProgressModel); ok {
		if pm.State == github.ProgressStateFailed {
			r.Output.LogInfo("")
			r.Output.LogInfo("Authentication test failed.")
			r.Output.LogInfo("See workflow logs: %s", runURL)
			r.Output.LogInfo("")
			r.Output.LogInfo("Common causes:")
			r.Output.LogInfo("  - OIDC federation not configured correctly")
			r.Output.LogInfo("  - Incorrect client ID, tenant ID, or subscription ID")
			r.Output.LogInfo("  - IAM role trust policy does not allow the GitHub repository")
			r.Output.LogInfo("")
			r.Output.LogInfo("The environment and variables have been preserved so you can fix the issue and re-run.")
			r.Output.LogInfo("To re-test: gh workflow run %s -f environment=%s", github.AuthTestWorkflowFile, envName)
			return fmt.Errorf("authentication test failed")
		}

		// FR-030-I: Success
		r.Output.LogInfo("")
		r.Output.LogInfo("Authentication test passed! Environment %q is ready for deployments.", envName)
	}

	return nil
}

// parseGitHubURL extracts owner and repo from a GitHub URL.
func parseGitHubURL(url string) (owner, repo string) {
	url = strings.TrimSuffix(url, ".git")
	parts := strings.Split(url, "/")
	if len(parts) >= 2 {
		repo = parts[len(parts)-1]
		owner = parts[len(parts)-2]
	}
	return
}
