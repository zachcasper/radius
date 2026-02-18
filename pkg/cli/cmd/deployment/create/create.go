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
	"os"
	"os/exec"
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

// NewCommand creates an instance of the command and runner for the `rad deployment create` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a deployment plan",
		Long: `Create a deployment plan by dispatching a GitHub Actions workflow.

The create command dispatches a workflow that generates a deployment plan for an application
in a specified environment. The plan is committed to the repository at
.radius/deploy/<app>/<env>/<commit>/.

You can specify the application and environment, or they will be auto-selected if unambiguous.`,
		Example: `
# Create a deployment plan (auto-select app and env if unambiguous)
rad deployment create

# Create a deployment plan for a specific application and environment
rad deployment create --application todolist --environment dev

# Create a deployment plan for a specific commit
rad deployment create --application todolist --environment dev --git-commit abc1234
`,
		Args: cobra.NoArgs,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	cmd.Flags().StringP("application", "a", "", "The application name")
	cmd.Flags().StringP("environment", "e", "", "The environment name")
	cmd.Flags().String("git-commit", "", "The git commit hash to use (defaults to HEAD)")

	return cmd, runner
}

// Runner is the runner implementation for the `rad deployment create` command.
type Runner struct {
	ConfigHolder *framework.ConfigHolder
	Output       output.Interface
	Prompter     prompt.Interface

	Workspace   *workspaces.Workspace
	Application string
	Environment string
	GitCommit   string
}

// NewRunner creates a new instance of the `rad deployment create` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
		Prompter:     factory.GetPrompter(),
	}
}

// Validate runs validation for the `rad deployment create` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}

	r.Workspace = workspace

	// FR-048: Require GitHub workspace
	if !workspace.IsGitHubWorkspace() {
		return clierrors.Message("The 'rad deployment create' command requires a GitHub workspace. Use 'rad init --github' to initialize a GitHub workspace.")
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

	// FR-045-A: Require clean worktree
	cwd, err := os.Getwd()
	if err != nil {
		return clierrors.Message("Failed to get current directory: %v", err)
	}

	gitHelper, err := github.NewGitHelper(cwd)
	if err != nil {
		return clierrors.Message("Failed to access git repository: %v", err)
	}

	dirty, err := gitHelper.IsDirty()
	if err != nil {
		return clierrors.Message("Failed to check worktree status: %v", err)
	}
	if dirty {
		return clierrors.Message("Working directory has uncommitted changes. Please commit or stash your changes before creating a deployment plan.")
	}

	// FR-045-B: Require all commits pushed
	hasUnpushed, err := gitHelper.HasUnpushedCommits()
	if err != nil {
		return clierrors.Message("Failed to check for unpushed commits: %v", err)
	}
	if hasUnpushed {
		return clierrors.Message("Local branch has unpushed commits. Please push your changes before creating a deployment plan.")
	}

	// FR-040/FR-041: Auto-select application if not specified
	if r.Application == "" {
		app, err := autoSelectApplication(cwd)
		if err != nil {
			return err
		}
		r.Application = app
	}

	// FR-043/FR-044: Auto-select environment if not specified
	if r.Environment == "" {
		repoURL, _ := workspace.Connection["url"].(string)
		owner, repo := parseGitHubURL(repoURL)
		env, err := autoSelectEnvironment(owner, repo)
		if err != nil {
			return err
		}
		r.Environment = env
	}

	// FR-045-C/FR-045-D: Resolve commit hash
	if r.GitCommit == "" {
		hash, err := gitHelper.GetCurrentCommit()
		if err != nil {
			return clierrors.Message("Failed to get HEAD commit hash: %v", err)
		}
		r.GitCommit = hash
	}

	return nil
}

// Run executes the `rad deployment create` command.
// FR-045: Dispatches the deployment create workflow and shows progress.
func (r *Runner) Run(ctx context.Context) error {
	r.Output.LogInfo("Creating deployment plan for %s in %s...", r.Application, r.Environment)
	r.Output.LogInfo("Commit: %s", r.GitCommit)
	r.Output.LogInfo("")

	ghClient := github.NewClient()

	// Use short commit hash (7 chars) for directory naming in log output
	shortCommit := r.GitCommit
	if len(shortCommit) > 7 {
		shortCommit = shortCommit[:7]
	}

	// Pass full commit hash to workflow (actions/checkout needs full SHA)
	inputs := map[string]string{
		"application": r.Application,
		"environment": r.Environment,
		"commit":      r.GitCommit,
	}

	// FR-089-C: Dispatch workflow and watch for run
	runID, runURL, err := ghClient.DispatchAndWatch(
		github.DeploymentCreateWorkflowFile,
		"main",
		inputs,
		func() {
			r.Output.LogInfo("Workflow queued, waiting for runner...")
		},
	)
	if err != nil {
		return fmt.Errorf("failed to dispatch deployment create workflow: %w", err)
	}

	// FR-089-D through FR-089-G: Show animated progress
	pollFunc := ghClient.CreatePollFunc(runID)
	model := github.NewProgressModel("Creating deployment plan", pollFunc)
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
			r.Output.LogInfo("Deployment plan creation failed.")
			r.Output.LogInfo("See workflow logs: %s", runURL)
			return clierrors.Message("Deployment plan creation failed. Check the workflow logs for details.")
		}
	}

	r.Output.LogInfo("")
	r.Output.LogInfo("Deployment plan created successfully!")

	// Pull the plan committed by the workflow so the local repo is up to date
	r.Output.LogInfo("Pulling deployment plan from remote...")
	pullCmd := exec.CommandContext(ctx, "git", "pull")
	pullCmd.Stdout = os.Stdout
	pullCmd.Stderr = os.Stderr
	if err := pullCmd.Run(); err != nil {
		r.Output.LogInfo("Warning: git pull failed: %v", err)
		r.Output.LogInfo("Run 'git pull' manually to fetch the deployment plan.")
	}

	r.Output.LogInfo("")
	r.Output.LogInfo("Plan location: .radius/deploy/%s/%s/%s/", r.Application, r.Environment, shortCommit)
	r.Output.LogInfo("")
	r.Output.LogInfo("Next step: Run 'rad deployment apply' to execute the plan")

	return nil
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
	return
}
