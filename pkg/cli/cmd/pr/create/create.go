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
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/github"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
)

const (
	// PlanWorkflowFile is the name of the plan workflow file
	PlanWorkflowFile = "radius-plan.yaml"
)

// NewCommand creates an instance of the command and runner for the `rad pr create` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a deployment pull request",
		Long: `Create a deployment pull request for a Radius application.

This command triggers a GitHub Actions workflow that:
1. Creates a deployment branch named deploy/<app>/<env>-<timestamp>
2. Sets up a k3d cluster with Radius installed
3. Generates a deployment plan with Terraform artifacts
4. Creates a pull request for review

The pull request contains the deployment plan and artifacts that can be reviewed
before merging to execute the actual deployment.`,
		Example: `# Create a deployment PR (auto-selects application if only one exists)
rad pr create

# Create a deployment PR for a specific application
rad pr create --application frontend

# Create a deployment PR for a specific application in a specific environment
rad pr create --environment production --application frontend`,
		Args: cobra.NoArgs,
		RunE: framework.RunCommand(runner),
	}

	cmd.Flags().StringP("environment", "e", "default", "The target environment name (defaults to 'default')")
	cmd.Flags().StringP("application", "a", "", "The application to deploy (required if multiple applications exist)")
	commonflags.AddWorkspaceFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad pr create` command.
type Runner struct {
	ConfigHolder    *framework.ConfigHolder
	Output          output.Interface
	Workspace       *workspaces.Workspace
	EnvironmentName string
	ApplicationName string
	GitHubClient    *github.Client
}

// NewRunner creates a new instance of the `rad pr create` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
		GitHubClient: github.NewClient(),
	}
}

// Validate runs validation for the `rad pr create` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	// Load workspace
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}

	// Validate that this is a GitHub workspace
	if workspace.Connection == nil {
		return clierrors.Message("Current workspace is not configured. Run 'rad init --github' to configure a GitHub workspace.")
	}

	kind, ok := workspace.Connection["kind"].(string)
	if !ok || kind != workspaces.KindGitHub {
		return clierrors.Message("This command requires a GitHub workspace. Current workspace kind is '%s'. Run 'rad init --github' to configure a GitHub workspace.", kind)
	}

	r.Workspace = workspace

	// Get environment name (defaults to "default")
	r.EnvironmentName, err = cmd.Flags().GetString("environment")
	if err != nil {
		return err
	}

	// Get application name from flag
	r.ApplicationName, _ = cmd.Flags().GetString("application")

	// Verify we're in a git repository
	cwd, err := os.Getwd()
	if err != nil {
		return clierrors.Message("Failed to get current directory: %v", err)
	}

	if _, err := os.Stat(filepath.Join(cwd, ".git")); os.IsNotExist(err) {
		return clierrors.Message("Not in a git repository. Please run this command from the root of your repository.")
	}

	// Detect applications in .radius/model/ if not specified
	if r.ApplicationName == "" {
		apps, err := r.detectApplications(cwd)
		if err != nil {
			return err
		}
		if len(apps) == 0 {
			return clierrors.Message("No applications found in '.radius/model/'. Create an application model first.")
		} else if len(apps) == 1 {
			r.ApplicationName = apps[0]
		} else {
			return clierrors.Message("Multiple applications found. Please specify one with --application:\n  %s", strings.Join(apps, "\n  "))
		}
	}

	// Verify gh CLI is available
	if err := r.GitHubClient.AuthStatus(); err != nil {
		return clierrors.Message("GitHub CLI (gh) is not authenticated. Run 'gh auth login' to authenticate.")
	}

	// Verify environment file exists
	envFile := filepath.Join(cwd, ".radius", fmt.Sprintf("env.%s.yaml", r.EnvironmentName))
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		return clierrors.Message("Environment '%s' not found. Create it with 'rad environment connect --environment %s'.", r.EnvironmentName, r.EnvironmentName)
	}

	// Verify plan workflow exists
	workflowFile := filepath.Join(cwd, ".github", "workflows", PlanWorkflowFile)
	if _, err := os.Stat(workflowFile); os.IsNotExist(err) {
		return clierrors.Message("Plan workflow not found at '%s'. Run 'rad init --github' to create the required workflow files.", workflowFile)
	}

	return nil
}

// Run executes the `rad pr create` command.
func (r *Runner) Run(ctx context.Context) error {
	r.Output.LogInfo("Creating deployment pull request...")
	r.Output.LogInfo("")

	// Build workflow inputs - application is always set (either from flag or auto-detected)
	inputs := map[string]string{
		"environment": r.EnvironmentName,
		"application": r.ApplicationName,
	}

	// Trigger the plan workflow
	r.Output.LogInfo("Triggering plan workflow for application '%s' in environment '%s'...", r.ApplicationName, r.EnvironmentName)

	// Get the current branch to use as the ref
	gitHelper, err := github.NewGitHelper(".")
	if err != nil {
		return clierrors.Message("Failed to initialize git: %v", err)
	}

	branch, err := gitHelper.GetCurrentBranch()
	if err != nil {
		branch = "main" // fallback to main
	}

	err = r.GitHubClient.RunWorkflow(PlanWorkflowFile, branch, inputs)
	if err != nil {
		return clierrors.Message("Failed to trigger plan workflow: %v", err)
	}

	r.Output.LogInfo("")
	r.Output.LogInfo("Plan workflow triggered successfully!")
	r.Output.LogInfo("")

	// Wait for workflow to start and display progress
	if err := r.waitForWorkflow(ctx); err != nil {
		return err
	}

	return nil
}

// waitForWorkflow waits for the plan workflow to complete and displays progress.
func (r *Runner) waitForWorkflow(ctx context.Context) error {
	r.Output.LogInfo("Waiting for workflow to start...")
	time.Sleep(3 * time.Second)

	// Try to find the workflow run
	var run *github.WorkflowRun
	var err error

	for i := 0; i < 10; i++ {
		run, err = r.GitHubClient.GetLatestWorkflowRun(PlanWorkflowFile)
		if err == nil && run != nil {
			break
		}
		time.Sleep(2 * time.Second)
	}

	if run == nil {
		r.Output.LogInfo("Could not find workflow run. Check GitHub Actions for status.")
		r.Output.LogInfo("You will be notified when the PR is created.")
		return nil
	}

	// Display animated progress
	spinChars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinIdx := 0
	done := make(chan struct{})

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
				fmt.Printf("\r%s Generating deployment plan...", spinner)
			}
		}
	}()

	finalRun, err := r.GitHubClient.WatchWorkflowRun(run.ID, nil)
	close(done)
	fmt.Printf("\r                                          \r") // Clear spinner line

	if err != nil {
		return clierrors.Message("Failed to watch workflow: %v", err)
	}

	r.Output.LogInfo("")

	if finalRun.Conclusion == "success" {
		r.Output.LogInfo("✓ Deployment plan generated successfully!")
		r.Output.LogInfo("")
		r.Output.LogInfo("A pull request has been created for review.")
		r.Output.LogInfo("View the PR at: %s", getPRURL(finalRun.URL))
		r.Output.LogInfo("")
		r.Output.LogInfo("Next steps:")
		r.Output.LogInfo("  1. Review the deployment plan in the pull request")
		r.Output.LogInfo("  2. Run 'rad pr merge' to deploy the application")
		return nil
	}

	// Handle failure
	r.Output.LogInfo("✗ Deployment plan generation failed!")
	r.Output.LogInfo("")
	logs, _ := r.GitHubClient.GetWorkflowRunLogs(finalRun.ID)
	if logs != "" {
		if len(logs) > 2000 {
			logs = logs[len(logs)-2000:]
		}
		r.Output.LogInfo("Workflow logs:")
		r.Output.LogInfo(logs)
	}
	r.Output.LogInfo("")
	r.Output.LogInfo("For full details, see: %s", finalRun.URL)

	return clierrors.Message("Deployment plan generation failed. Please check the workflow logs.")
}

// getPRURL converts a workflow run URL to the likely PR URL.
// Workflow URL format: https://github.com/owner/repo/actions/runs/12345
// PR URL format: https://github.com/owner/repo/pulls
func getPRURL(workflowURL string) string {
	// Extract base repository URL
	parts := strings.Split(workflowURL, "/actions/runs/")
	if len(parts) > 0 {
		return parts[0] + "/pulls"
	}
	return workflowURL
}

// detectApplications finds all application directories in .radius/model/
func (r *Runner) detectApplications(cwd string) ([]string, error) {
	modelDir := filepath.Join(cwd, ".radius", "model")
	entries, err := os.ReadDir(modelDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, clierrors.Message("Failed to read application models: %v", err)
	}

	var apps []string
	for _, entry := range entries {
		// D027 fix: Look for .bicep files, not directories
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".bicep") {
			// Extract application name from filename (e.g., "todolist.bicep" -> "todolist")
			appName := strings.TrimSuffix(entry.Name(), ".bicep")
			if appName != "" {
				apps = append(apps, appName)
			}
		}
	}
	return apps, nil
}
