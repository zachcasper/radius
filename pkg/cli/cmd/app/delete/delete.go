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
	"os"
	"path/filepath"
	"strings"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/delete"

	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/github"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/spf13/cobra"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	deleteConfirmation = "Are you sure you want to delete application '%v' from environment '%v'?"
	bicepWarning       = "'%v' is a Bicep filename or path and not the name of a Radius Application. Specify the name of a valid application and try again"
)

// NewCommand creates an instance of the `rad app delete` command and runner.
//

// NewCommand creates a new Cobra command for deleting a Radius Application, with flags for workspace, resource group,
// application name and confirmation, and returns the command and a Runner object.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete Radius Application",
		Long:  "Delete the specified Radius Application deployed in the default environment",
		Example: `
# Delete current application
rad app delete

# Delete current application and bypass confirmation prompt
rad app delete --yes

# Delete specified application
rad app delete my-app

# Delete specified application in a specified resource group
rad app delete my-app --group my-group
`,
		Args: cobra.MaximumNArgs(1),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddApplicationNameFlag(cmd)
	commonflags.AddConfirmationFlag(cmd)

	return cmd, runner
}

// Runner is the Runner implementation for the `rad app delete` command.
type Runner struct {
	Delete            delete.Interface
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	InputPrompter     prompt.Interface
	Output            output.Interface

	ApplicationName string
	EnvironmentName string
	Scope           string
	Confirm         bool
	Workspace       *workspaces.Workspace
}

// NewRunner creates an instance of the runner for the `rad app delete` command.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		Delete:            factory.GetDelete(),
		ConfigHolder:      factory.GetConfigHolder(),
		ConnectionFactory: factory.GetConnectionFactory(),
		InputPrompter:     factory.GetPrompter(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad app delete` command.
//

// Validate checks the workspace, scope, application name, and confirm flag from the command line arguments and
// request object, and returns an error if any of these are invalid.
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
// FR-106-A: Auto-select application and environment when unambiguous.
func (r *Runner) validateGitHubMode(cmd *cobra.Command, args []string) error {
	var err error

	// Get application name from args or flag
	if len(args) > 0 {
		r.ApplicationName = args[0]
	} else {
		r.ApplicationName, err = cmd.Flags().GetString("application")
		if err != nil {
			return err
		}
	}

	// Auto-select application if not specified
	if r.ApplicationName == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return clierrors.Message("Failed to get current directory: %v", err)
		}

		app, err := autoSelectApplication(cwd)
		if err != nil {
			return err
		}
		r.ApplicationName = app
	}

	// Throw error if user specifies a Bicep filename
	if strings.HasSuffix(r.ApplicationName, ".bicep") {
		return clierrors.Message(bicepWarning, r.ApplicationName)
	}

	// Auto-select environment
	repoURL, _ := r.Workspace.Connection["url"].(string)
	owner, repo := parseGitHubURL(repoURL)
	if r.EnvironmentName == "" {
		env, err := autoSelectEnvironment(owner, repo)
		if err != nil {
			return err
		}
		r.EnvironmentName = env
	}

	r.Confirm, err = cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}

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

	r.ApplicationName, err = cli.RequireApplicationArgs(cmd, args, *r.Workspace)
	if err != nil {
		return err
	}

	// Lookup the environment name for use in the confirmation prompt
	if r.Workspace.Environment != "" {
		id, err := resources.ParseResource(r.Workspace.Environment)
		if err != nil {
			return err
		}

		r.EnvironmentName = id.Name()
	}

	// Throw error if user specifies a Bicep filename or path instead of an application name
	if strings.HasSuffix(r.ApplicationName, ".bicep") {
		return clierrors.Message(bicepWarning, r.ApplicationName)
	}

	r.Confirm, err = cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}

	return nil
}

// Run runs the `rad app delete` command.
//

// "Run" prompts the user to confirm the deletion of an application, creates a connection to the application management
// client, and deletes the application if it exists. If the application does not exist, it logs a message. It returns an
// error if there is an issue with the connection or the prompt.
func (r *Runner) Run(ctx context.Context) error {
	if r.Workspace.IsGitHubWorkspace() {
		return r.runGitHubMode(ctx)
	}

	return r.runKubernetesMode(ctx)
}

// runGitHubMode dispatches the destroy workflow for GitHub workspaces.
// FR-106-A: Dispatch radius-destroy.yaml workflow with application and environment inputs.
// FR-106-D: Show animated progress indicator.
func (r *Runner) runGitHubMode(ctx context.Context) error {
	// Prompt user to confirm deletion
	if !r.Confirm {
		confirmed, err := prompt.YesOrNoPrompt(fmt.Sprintf(deleteConfirmation, r.ApplicationName, r.EnvironmentName), prompt.ConfirmNo, r.InputPrompter)
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	}

	r.Output.LogInfo("Deleting application '%s' from environment '%s'...", r.ApplicationName, r.EnvironmentName)
	r.Output.LogInfo("")

	ghClient := github.NewClient()

	inputs := map[string]string{
		"application": r.ApplicationName,
		"environment": r.EnvironmentName,
	}

	// FR-106-A: Dispatch destroy workflow
	runID, runURL, err := ghClient.DispatchAndWatch(
		github.DestroyWorkflowFile,
		"main",
		inputs,
		func() {
			r.Output.LogInfo("Workflow queued, waiting for runner...")
		},
	)
	if err != nil {
		return fmt.Errorf("failed to dispatch destroy workflow: %w", err)
	}

	r.Output.LogInfo("Workflow started: %s", runURL)
	r.Output.LogInfo("")

	// FR-106-D: Show animated progress with L key log streaming
	pollFunc := ghClient.CreatePollFunc(runID)
	model := github.NewProgressModel("Destroying application", pollFunc)
	model.RunURL = runURL

	p := tea.NewProgram(model)
	finalModel, err := r.InputPrompter.RunProgram(p)
	if err != nil {
		return fmt.Errorf("progress display error: %w", err)
	}

	// Check final state
	if pm, ok := finalModel.(github.ProgressModel); ok {
		if pm.State == github.ProgressStateFailed {
			r.Output.LogInfo("")
			r.Output.LogInfo("Application deletion failed.")
			r.Output.LogInfo("See workflow logs: %s", runURL)
			return clierrors.Message("Application deletion failed. Check the workflow logs for details.")
		}
	}

	r.Output.LogInfo("")
	r.Output.LogInfo("Application '%s' deleted successfully from environment '%s'.", r.ApplicationName, r.EnvironmentName)

	return nil
}

// runKubernetesMode runs the standard Kubernetes-mode delete flow.
func (r *Runner) runKubernetesMode(ctx context.Context) error {
	// Prompt user to confirm deletion
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	app, err := client.GetApplication(ctx, r.ApplicationName)
	if clients.Is404Error(err) {
		r.Output.LogInfo("Application '%s' does not exist or has already been deleted.", r.ApplicationName)
		return nil
	} else if err != nil {
		return err
	}

	var environmentID resources.ID

	environmentID, err = resources.ParseResource(*app.Properties.Environment)
	if err != nil {
		return err
	}
	if !r.Confirm {
		confirmed, err := prompt.YesOrNoPrompt(fmt.Sprintf(deleteConfirmation, r.ApplicationName, environmentID.Name()), prompt.ConfirmNo, r.InputPrompter)
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	}
	r.EnvironmentName = environmentID.Name()

	progressText := fmt.Sprintf("Deleting application '%s' from environment '%s'...", r.ApplicationName, r.EnvironmentName)

	deleted, err := r.Delete.DeleteApplicationWithProgress(ctx, client, clients.DeleteOptions{
		ApplicationNameOrID: r.ApplicationName,
		ProgressText:        progressText,
	})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			r.Output.LogInfo("Application '%s' does not exist or has already been deleted.", r.ApplicationName)
			return nil
		}
		return clierrors.Message("Failed to delete application '%s': %v", r.ApplicationName, err)
	}
	if deleted {
		r.Output.LogInfo("Application %s deleted successfully", r.ApplicationName)
	} else {
		r.Output.LogInfo("Application '%s' does not exist or has already been deleted.", r.ApplicationName)
		return nil
	}

	return nil
}

// autoSelectApplication auto-selects the application when unambiguous.
// FR-107: If exactly one .bicep file exists in .radius/applications/, use it.
func autoSelectApplication(cwd string) (string, error) {
	appsDir := filepath.Join(cwd, ".radius", "applications")
	entries, err := os.ReadDir(appsDir)
	if err != nil {
		return "", clierrors.Message("No applications found. Run 'rad app model' to create an application definition.")
	}

	var bicepFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".bicep") {
			name := strings.TrimSuffix(entry.Name(), ".bicep")
			bicepFiles = append(bicepFiles, name)
		}
	}

	switch len(bicepFiles) {
	case 0:
		return "", clierrors.Message("No application definitions found in .radius/applications/. Run 'rad app model' to create one.")
	case 1:
		return bicepFiles[0], nil
	default:
		return "", clierrors.Message("Multiple applications found: %s. Use --application to specify which one.", strings.Join(bicepFiles, ", "))
	}
}

// autoSelectEnvironment auto-selects the environment when unambiguous.
func autoSelectEnvironment(owner, repo string) (string, error) {
	ghClient := github.NewClient()
	envs, err := ghClient.ListEnvironments(owner, repo)
	if err != nil {
		return "", clierrors.Message("Failed to list GitHub Environments: %v", err)
	}

	switch len(envs) {
	case 0:
		return "", clierrors.Message("No environments found. Run 'rad environment create' to set up an environment.")
	case 1:
		return envs[0], nil
	default:
		return "", clierrors.Message("Multiple environments found: %s. Use --environment to specify which one.", strings.Join(envs, ", "))
	}
}

// parseGitHubURL extracts owner and repo from a GitHub URL.
func parseGitHubURL(url string) (owner, repo string) {
	url = strings.TrimSuffix(url, ".git")
	parts := strings.Split(url, "/")
	if len(parts) >= 2 {
		repo = parts[len(parts)-1]
		owner = parts[len(parts)-2]
	}
	return owner, repo
}
