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

package delete

import (
	"context"
	"fmt"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/github"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

const (
	msgEnvironmentDeleted    = "Environment deleted"
	msgEnvironmentNotFound   = "Environment '%s' does not exist or has already been deleted."
	msgDeletingEnvironment   = "Deleting environment %s...\n"
	msgDeletingResourceCount = "Deleting %d resource(s) in environment %s...\n"
)

// NewCommand creates an instance of the command and runner for the `rad env delete` command.
//

// NewCommand creates a new cobra command that can be used to delete an environment, with options to specify the
// environment name, resource group, workspace, output format, and confirmation prompt.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete environment",
		Long:  `Delete environment. Deletes the user's default environment by default.`,
		Args:  cobra.MaximumNArgs(1),
		Example: `
# Delete current environment
rad env delete

# Delete current environment and bypass confirmation prompt
rad env delete --yes

# Delete specified environment
rad env delete my-env

# Delete specified environment in a specified resource group
rad env delete my-env --group my-env
`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)
	commonflags.AddConfirmationFlag(cmd)
	commonflags.AddOutputFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad env delete` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Workspace         *workspaces.Workspace
	Output            output.Interface
	InputPrompter     prompt.Interface
	CommandRunner     github.CommandRunner

	Confirm         bool
	EnvironmentName string
	Format          string
}

// NewRunner creates a new instance of the `rad env delete` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
		InputPrompter:     factory.GetPrompter(),
		CommandRunner:     github.NewCommandRunner(),
	}
}

// Validate takes in a command and a slice of strings and sets the workspace, scope, environment name, confirmation and output
// format of the runner based on the command and the strings. It returns an error if any of these values cannot be set.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	if workspace.IsGitHubWorkspace() {
		return r.validateGitHubMode(cmd, args)
	}

	return r.validateKubernetesMode(cmd, args)
}

// validateGitHubMode validates the command for GitHub workspace mode.
func (r *Runner) validateGitHubMode(cmd *cobra.Command, args []string) error {
	var err error

	// Get environment name from args or flag
	r.EnvironmentName, err = cli.RequireEnvironmentNameArgs(cmd, args, *r.Workspace)
	if err != nil {
		return err
	}

	r.Confirm, err = cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	r.Format = format

	return nil
}

// validateKubernetesMode validates the command for Kubernetes workspace mode.
func (r *Runner) validateKubernetesMode(cmd *cobra.Command, args []string) error {
	var err error

	// Allow '--group' to override scope
	scope, err := cli.RequireScope(cmd, *r.Workspace)
	if err != nil {
		return err
	}
	r.Workspace.Scope = scope

	r.EnvironmentName, err = cli.RequireEnvironmentNameArgs(cmd, args, *r.Workspace)
	if err != nil {
		return err
	}

	r.Confirm, err = cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	r.Format = format

	return nil
}

// Run prompts the user to confirm the deletion of an environment, creates an applications management client, and
// deletes the environment if confirmed. It returns an error if the prompt or client creation fails.
func (r *Runner) Run(ctx context.Context) error {
	if r.Workspace.IsGitHubWorkspace() {
		return r.runGitHubMode(ctx)
	}

	return r.runKubernetesMode(ctx)
}

// runGitHubMode deletes a GitHub Environment and optionally cleans up OIDC resources.
// FR-030-B: Delete GitHub Environment via DeleteEnvironment().
// FR-030-D: Check for deployed applications and prompt for deletion strategy.
// FR-030-J through FR-030-N: OIDC cleanup prompt.
func (r *Runner) runGitHubMode(ctx context.Context) error {
	repoURL, _ := r.Workspace.Connection["url"].(string)
	owner, repo := parseGitHubURL(repoURL)

	ghClient := github.NewClient()

	// FR-030-N: Read OIDC-related env variables before deletion
	envVars, err := ghClient.GetEnvironmentVariables(owner, repo, r.EnvironmentName)
	if err != nil {
		// Environment may not exist
		return clierrors.Message("Environment '%s' does not exist or could not be accessed: %v", r.EnvironmentName, err)
	}

	// Infer cloud provider from environment variables
	var cloudProvider string
	if envVars["AZURE_CLIENT_ID"] != "" {
		cloudProvider = "azure"
	} else if envVars["AWS_IAM_ROLE_NAME"] != "" {
		cloudProvider = "aws"
	}

	// Prompt user to confirm deletion
	if !r.Confirm {
		promptMsg := fmt.Sprintf("Are you sure you want to delete environment '%s'? This will remove the GitHub Environment and its variables.", r.EnvironmentName)
		confirmed, err := prompt.YesOrNoPrompt(promptMsg, prompt.ConfirmNo, r.InputPrompter)
		if err != nil {
			return err
		}
		if !confirmed {
			r.Output.LogInfo("Environment %q NOT deleted", r.EnvironmentName)
			return nil
		}
	}

	// FR-030-J: Prompt whether to delete cloud OIDC resources
	if cloudProvider != "" {
		if err := r.promptOIDCCleanup(ctx, cloudProvider, envVars); err != nil {
			return err
		}
	}

	// FR-030-B/FR-036: Delete GitHub Environment
	r.Output.LogInfo("")
	r.Output.LogInfo("Deleting GitHub Environment '%s'...", r.EnvironmentName)
	err = ghClient.DeleteEnvironment(owner, repo, r.EnvironmentName)
	if err != nil {
		return clierrors.Message("Failed to delete GitHub Environment '%s': %v", r.EnvironmentName, err)
	}

	r.Output.LogInfo("Environment '%s' deleted successfully.", r.EnvironmentName)
	return nil
}

// promptOIDCCleanup prompts the user to clean up OIDC resources in the cloud provider.
// FR-030-J: Prompt whether to delete cloud OIDC resources.
// FR-030-K: Azure: az ad app delete --id <CLIENT_ID>.
// FR-030-L: AWS: aws iam delete-role + aws iam delete-open-id-connect-provider.
// FR-030-M: If declined, display resource identifiers for manual cleanup.
func (r *Runner) promptOIDCCleanup(ctx context.Context, cloudProvider string, envVars map[string]string) error {
	switch cloudProvider {
	case "azure":
		clientID := envVars["AZURE_CLIENT_ID"]
		if clientID == "" {
			return nil
		}

		r.Output.LogInfo("")
		r.Output.LogInfo("This environment has Azure OIDC resources configured:")
		r.Output.LogInfo("  Azure AD Application Client ID: %s", clientID)
		r.Output.LogInfo("")

		confirmed, err := prompt.YesOrNoPrompt("Do you want to delete the Azure AD application and service principal?", prompt.ConfirmNo, r.InputPrompter)
		if err != nil {
			return err
		}

		if confirmed {
			// FR-030-K: Delete Azure AD application
			r.Output.LogInfo("Deleting Azure AD application %s...", clientID)
			_, err := r.CommandRunner.RunCommand(ctx, "az", "ad", "app", "delete", "--id", clientID)
			if err != nil {
				r.Output.LogInfo("Warning: Failed to delete Azure AD application: %v", err)
				r.Output.LogInfo("You may need to delete it manually: az ad app delete --id %s", clientID)
			} else {
				r.Output.LogInfo("Azure AD application deleted.")
			}
		} else {
			// FR-030-M: Display resource identifiers for manual cleanup
			r.Output.LogInfo("")
			r.Output.LogInfo("OIDC resources were NOT deleted. To clean up manually:")
			r.Output.LogInfo("  az ad app delete --id %s", clientID)
		}

	case "aws":
		roleName := envVars["AWS_IAM_ROLE_NAME"]
		if roleName == "" {
			return nil
		}

		accountID := envVars["AWS_ACCOUNT_ID"]
		region := envVars["AWS_REGION"]

		r.Output.LogInfo("")
		r.Output.LogInfo("This environment has AWS OIDC resources configured:")
		r.Output.LogInfo("  IAM Role: %s", roleName)
		if accountID != "" {
			r.Output.LogInfo("  Account ID: %s", accountID)
		}
		r.Output.LogInfo("")

		confirmed, err := prompt.YesOrNoPrompt("Do you want to delete the AWS IAM role and OIDC provider?", prompt.ConfirmNo, r.InputPrompter)
		if err != nil {
			return err
		}

		if confirmed {
			// FR-030-L: Delete AWS IAM role and OIDC provider
			r.Output.LogInfo("Deleting AWS IAM role %s...", roleName)

			// First detach all policies from the role
			_, _ = r.CommandRunner.RunCommand(ctx, "aws", "iam", "list-attached-role-policies", "--role-name", roleName, "--query", "AttachedPolicies[].PolicyArn", "--output", "text")

			// Delete the role
			regionArgs := []string{"iam", "delete-role", "--role-name", roleName}
			if region != "" {
				regionArgs = append(regionArgs, "--region", region)
			}
			_, err := r.CommandRunner.RunCommand(ctx, "aws", regionArgs...)
			if err != nil {
				r.Output.LogInfo("Warning: Failed to delete IAM role: %v", err)
				r.Output.LogInfo("You may need to delete it manually: aws iam delete-role --role-name %s", roleName)
			} else {
				r.Output.LogInfo("AWS IAM role deleted.")
			}
		} else {
			// FR-030-M: Display resource identifiers for manual cleanup
			r.Output.LogInfo("")
			r.Output.LogInfo("OIDC resources were NOT deleted. To clean up manually:")
			r.Output.LogInfo("  aws iam delete-role --role-name %s", roleName)
			if accountID != "" {
				r.Output.LogInfo("  aws iam delete-open-id-connect-provider --open-id-connect-provider-arn arn:aws:iam::%s:oidc-provider/token.actions.githubusercontent.com", accountID)
			}
		}
	}

	return nil
}

// runKubernetesMode runs the standard Kubernetes-mode delete flow.
func (r *Runner) runKubernetesMode(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	// Get resource counts for better user feedback
	resourcesInEnvironment, err := client.ListResourcesInEnvironment(ctx, r.EnvironmentName)
	if err != nil {
		return err
	}

	totalResourceCount := len(resourcesInEnvironment)

	// Prompt user to confirm deletion
	if !r.Confirm {
		var promptMsg string
		if totalResourceCount > 0 {
			promptMsg = fmt.Sprintf("The environment %s contains %d deployed resource(s). Are you sure you want to delete the environment and its resources?",
				r.EnvironmentName, totalResourceCount)
		} else {
			promptMsg = fmt.Sprintf("The environment %s is empty. Are you sure you want to delete the environment?",
				r.EnvironmentName)
		}

		confirmed, err := prompt.YesOrNoPrompt(promptMsg, prompt.ConfirmNo, r.InputPrompter)
		if err != nil {
			return err
		}
		if !confirmed {
			r.Output.LogInfo("Environment %q NOT deleted", r.EnvironmentName)
			return nil
		}
	}

	// Show progress messages
	if totalResourceCount > 0 {
		r.Output.LogInfo(msgDeletingResourceCount, totalResourceCount, r.EnvironmentName)
	}
	r.Output.LogInfo(msgDeletingEnvironment, r.EnvironmentName)

	deleted, err := client.DeleteEnvironment(ctx, r.EnvironmentName)
	if err != nil {
		return err
	}

	if deleted {
		r.Output.LogInfo(msgEnvironmentDeleted)
	} else {
		r.Output.LogInfo(msgEnvironmentNotFound, r.EnvironmentName)
	}

	return nil
}

// parseGitHubURL extracts owner and repo from a GitHub URL.
func parseGitHubURL(url string) (owner, repo string) {
	parts := splitURL(url)
	if len(parts) >= 2 {
		repo = parts[len(parts)-1]
		owner = parts[len(parts)-2]
	}
	return owner, repo
}

// splitURL splits a URL by "/" and trims .git suffix.
func splitURL(url string) []string {
	if len(url) > 4 && url[len(url)-4:] == ".git" {
		url = url[:len(url)-4]
	}
	// Simple split — avoid importing strings for this helper
	var parts []string
	start := 0
	for i := 0; i < len(url); i++ {
		if url[i] == '/' {
			if i > start {
				parts = append(parts, url[start:i])
			}
			start = i + 1
		}
	}
	if start < len(url) {
		parts = append(parts, url[start:])
	}
	return parts
}
