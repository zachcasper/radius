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

package apply

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/github"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"

	tea "github.com/charmbracelet/bubbletea"
)

// NewCommand creates an instance of the command and runner for the `rad deployment apply` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply a deployment plan",
		Long: `Apply a deployment plan by dispatching a GitHub Actions workflow.

The apply command dispatches a workflow that executes a previously generated deployment plan.
The plan is located at .radius/deploy/<app>/<env>/<commit>/ in the repository.

After execution, the deployment record is updated with status, timing, and captured resources.`,
		Example: `
# Apply the most recent deployment plan (auto-select app and env if unambiguous)
rad deployment apply

# Apply a specific deployment plan
rad deployment apply --application todolist --environment dev

# Apply a deployment plan for a specific commit
rad deployment apply --application todolist --environment dev --git-commit abc1234
`,
		Args: cobra.NoArgs,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	cmd.Flags().StringP("application", "a", "", "The application name")
	cmd.Flags().StringP("environment", "e", "", "The environment name")
	cmd.Flags().String("git-commit", "", "The git commit hash of the deployment plan to apply")

	return cmd, runner
}

// Runner is the runner implementation for the `rad deployment apply` command.
type Runner struct {
	ConfigHolder *framework.ConfigHolder
	Output       output.Interface
	Prompter     prompt.Interface

	Workspace   *workspaces.Workspace
	Application string
	Environment string
	GitCommit   string
}

// NewRunner creates a new instance of the `rad deployment apply` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
		Prompter:     factory.GetPrompter(),
	}
}

// Validate runs validation for the `rad deployment apply` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}

	r.Workspace = workspace

	// FR-054: Require GitHub workspace
	if !workspace.IsGitHubWorkspace() {
		return clierrors.Message("The 'rad deployment apply' command requires a GitHub workspace. Use 'rad init --github' to initialize a GitHub workspace.")
	}

	r.Application, err = cmd.Flags().GetString("application")
	if err != nil {
		return err
	}

	r.Environment, err = cmd.Flags().GetString("environment")
	if err != nil {
		return err
	}

	r.GitCommit, err = cmd.Flags().GetString("git-commit")
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return clierrors.Message("Failed to get current directory: %v", err)
	}

	// FR-049: Auto-select application if not specified
	if r.Application == "" {
		app, err := autoSelectApplication(cwd)
		if err != nil {
			return err
		}
		r.Application = app
	}

	// FR-049: Auto-select environment if not specified
	if r.Environment == "" {
		repoURL, _ := workspace.Connection["url"].(string)
		owner, repo := parseGitHubURL(repoURL)
		env, err := autoSelectEnvironment(owner, repo)
		if err != nil {
			return err
		}
		r.Environment = env
	}

	// FR-050: Resolve deployment plan
	if r.GitCommit == "" {
		commit, err := resolveMostRecentPlan(cwd, r.Application, r.Environment)
		if err != nil {
			return err
		}
		r.GitCommit = commit
	} else {
		// Verify the specified plan exists
		planDir := filepath.Join(cwd, ".radius", "deploy", r.Application, r.Environment, r.GitCommit)
		if _, err := os.Stat(planDir); os.IsNotExist(err) {
			return clierrors.Message("No deployment plan found at .radius/deploy/%s/%s/%s/. Run 'rad deployment create' first.", r.Application, r.Environment, r.GitCommit)
		}
	}

	return nil
}

// Run executes the `rad deployment apply` command.
// FR-051: Dispatches the deployment apply workflow and shows progress.
func (r *Runner) Run(ctx context.Context) error {
	r.Output.LogInfo("Applying deployment plan for %s in %s...", r.Application, r.Environment)
	r.Output.LogInfo("Commit: %s", r.GitCommit)
	r.Output.LogInfo("")

	ghClient := github.NewClient()

	inputs := map[string]string{
		"application": r.Application,
		"environment": r.Environment,
		"commit":      r.GitCommit,
	}

	// FR-051: Dispatch workflow and watch for run
	runID, runURL, err := ghClient.DispatchAndWatch(
		github.DeploymentApplyWorkflowFile,
		"main",
		inputs,
		func() {
			r.Output.LogInfo("Workflow queued, waiting for runner...")
		},
	)
	if err != nil {
		return fmt.Errorf("failed to dispatch deployment apply workflow: %w", err)
	}

	// FR-089-D through FR-089-G: Show animated progress
	pollFunc := ghClient.CreatePollFunc(runID)
	model := github.NewProgressModel("Applying deployment", pollFunc)
	model.RunURL = runURL
	model.StepPollFunc = ghClient.CreateStepPollFunc(runID)

	p := tea.NewProgram(model)
	finalModel, err := r.Prompter.RunProgram(p)
	if err != nil {
		return fmt.Errorf("progress display error: %w", err)
	}

	// Check final state
	if pm, ok := finalModel.(github.ProgressModel); ok {
		if pm.State == github.ProgressStateFailed {
			r.Output.LogInfo("")
			r.Output.LogInfo("Deployment apply failed.")
			r.Output.LogInfo("See workflow logs: %s", runURL)
			return clierrors.Message("Deployment apply failed. Check the workflow logs for details.")
		}
	}

	r.Output.LogInfo("")
	r.Output.LogInfo("Deployment applied successfully!")
	r.Output.LogInfo("Application %s is now deployed to %s.", r.Application, r.Environment)

	return nil
}

// resolveMostRecentPlan finds the most recent deployment plan under .radius/deploy/<app>/<env>/.
// FR-050: If no --git-commit specified, use the most recent plan directory.
func resolveMostRecentPlan(cwd, application, environment string) (string, error) {
	planBaseDir := filepath.Join(cwd, ".radius", "deploy", application, environment)
	entries, err := os.ReadDir(planBaseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", clierrors.Message("No deployment plans found for %s in %s. Run 'rad deployment create' first.", application, environment)
		}
		return "", clierrors.Message("Failed to read deployment plans: %v", err)
	}

	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry.Name())
		}
	}

	if len(dirs) == 0 {
		return "", clierrors.Message("No deployment plans found for %s in %s. Run 'rad deployment create' first.", application, environment)
	}

	// Use the last directory (most recently created, directories are commit hashes)
	return dirs[len(dirs)-1], nil
}

// autoSelectApplication auto-selects the application when unambiguous.
// FR-040: If exactly one .bicep file exists in .radius/applications/, use it.
// FR-041: If multiple exist, require --application flag.
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
// FR-043: If exactly one GitHub Environment exists, use it.
// FR-044: If multiple exist, require --environment flag.
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
