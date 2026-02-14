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

package merge

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
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/workspaces"
)

const (
	// DeployBranchPrefix is the prefix for deployment branches created by rad pr create
	DeployBranchPrefix = "deploy/"

	// DeployWorkflowFile is the name of the deploy workflow file
	DeployWorkflowFile = "radius-deploy.yaml"
)

// NewCommand creates an instance of the command and runner for the `rad pr merge` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "merge",
		Short: "Merge a deployment pull request to execute deployment",
		Long: `Merge a deployment pull request to execute the deployment.

This command merges an approved deployment pull request, which triggers a GitHub 
Actions workflow to execute the deployment using the pre-generated plan and 
Terraform artifacts.

By default, this command finds the latest deployment PR created by 'rad pr create'.
You can specify a specific PR number using the --pr flag.`,
		Example: `# Merge the latest deployment PR
rad pr merge

# Merge a specific PR
rad pr merge --pr 42

# Merge without confirmation
rad pr merge --yes`,
		Args: cobra.NoArgs,
		RunE: framework.RunCommand(runner),
	}

	cmd.Flags().IntP("pr", "p", 0, "The PR number to merge (defaults to latest deployment PR)")
	cmd.Flags().BoolP("yes", "y", false, "Merge without confirmation")
	commonflags.AddWorkspaceFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad pr merge` command.
type Runner struct {
	ConfigHolder  *framework.ConfigHolder
	Output        output.Interface
	Workspace     *workspaces.Workspace
	PRNumber      int
	SkipConfirm   bool
	GitHubClient  *github.Client
	InputPrompter prompt.Interface
}

// NewRunner creates a new instance of the `rad pr merge` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:  factory.GetConfigHolder(),
		Output:        factory.GetOutput(),
		GitHubClient:  github.NewClient(),
		InputPrompter: factory.GetPrompter(),
	}
}

// Validate runs validation for the `rad pr merge` command.
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

	// Get optional PR number
	r.PRNumber, err = cmd.Flags().GetInt("pr")
	if err != nil {
		return err
	}

	// Get skip confirmation flag
	r.SkipConfirm, err = cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}

	// Verify gh CLI is available
	if err := r.GitHubClient.AuthStatus(); err != nil {
		return clierrors.Message("GitHub CLI (gh) is not authenticated. Run 'gh auth login' to authenticate.")
	}

	// Verify we're in a git repository
	cwd, err := os.Getwd()
	if err != nil {
		return clierrors.Message("Failed to get current directory: %v", err)
	}

	if _, err := os.Stat(filepath.Join(cwd, ".git")); os.IsNotExist(err) {
		return clierrors.Message("Not in a git repository. Please run this command from the root of your repository.")
	}

	// Verify deploy workflow exists
	workflowFile := filepath.Join(cwd, ".github", "workflows", DeployWorkflowFile)
	if _, err := os.Stat(workflowFile); os.IsNotExist(err) {
		return clierrors.Message("Deploy workflow not found at '%s'. Run 'rad init --github' to create the required workflow files.", workflowFile)
	}

	return nil
}

// Run executes the `rad pr merge` command.
func (r *Runner) Run(ctx context.Context) error {
	var pr *github.PullRequest
	var err error

	if r.PRNumber > 0 {
		// Get specific PR
		r.Output.LogInfo("Fetching PR #%d...", r.PRNumber)
		pr, err = r.GitHubClient.GetPR(r.PRNumber)
		if err != nil {
			return clierrors.Message("Failed to get PR #%d: %v", r.PRNumber, err)
		}
	} else {
		// Find latest deployment PR
		r.Output.LogInfo("Finding latest deployment PR...")
		pr, err = r.findLatestDeploymentPR()
		if err != nil {
			return err
		}
	}

	if pr == nil {
		return clierrors.Message("No deployment PR found. Run 'rad pr create' to create a deployment PR first.")
	}

	// Display PR info
	r.Output.LogInfo("")
	r.Output.LogInfo("Found deployment PR:")
	r.Output.LogInfo("  #%d: %s", pr.Number, pr.Title)
	r.Output.LogInfo("  Branch: %s -> %s", pr.HeadRefName, pr.BaseRefName)
	r.Output.LogInfo("  URL: %s", pr.URL)
	r.Output.LogInfo("")

	// Check PR state
	if pr.State != "OPEN" && pr.State != "open" {
		return clierrors.Message("PR #%d is not open (state: %s). Cannot merge.", pr.Number, pr.State)
	}

	// Confirm merge unless --yes flag is set
	if !r.SkipConfirm {
		confirmed, err := r.confirmMerge(pr)
		if err != nil {
			return err
		}
		if !confirmed {
			r.Output.LogInfo("Merge cancelled.")
			return nil
		}
	}

	// Merge the PR
	r.Output.LogInfo("Merging PR #%d...", pr.Number)
	err = r.GitHubClient.MergePR(pr.Number, "squash", true)
	if err != nil {
		return clierrors.Message("Failed to merge PR: %v", err)
	}

	r.Output.LogInfo("")
	r.Output.LogInfo("PR #%d merged successfully!", pr.Number)
	r.Output.LogInfo("")

	// Wait for deploy workflow to start
	if err := r.waitForDeployWorkflow(ctx); err != nil {
		// Don't fail if we can't track the workflow - the merge succeeded
		r.Output.LogInfo("Note: %v", err)
	}

	r.Output.LogInfo("")
	r.Output.LogInfo("Next steps:")
	r.Output.LogInfo("  - Monitor the deployment workflow in GitHub Actions")
	r.Output.LogInfo("  - Deployment records will be stored in .radius/deploy/")

	return nil
}

// findLatestDeploymentPR finds the most recent open deployment PR.
func (r *Runner) findLatestDeploymentPR() (*github.PullRequest, error) {
	// List all open PRs with deploy/ branch prefix
	prs, err := r.GitHubClient.ListPRs("open", "")
	if err != nil {
		return nil, clierrors.Message("Failed to list PRs: %v", err)
	}

	// Find PRs with deploy/ branch prefix
	var deployPRs []github.PullRequest
	for _, pr := range prs {
		if strings.HasPrefix(pr.HeadRefName, DeployBranchPrefix) {
			deployPRs = append(deployPRs, pr)
		}
	}

	if len(deployPRs) == 0 {
		return nil, nil
	}

	// Return the first one (most recently created)
	return &deployPRs[0], nil
}

// confirmMerge prompts the user to confirm the merge.
func (r *Runner) confirmMerge(pr *github.PullRequest) (bool, error) {
	if r.InputPrompter == nil {
		// No prompter available, assume yes
		return true, nil
	}

	options := []string{
		fmt.Sprintf("Yes, merge PR #%d and deploy", pr.Number),
		"No, cancel",
	}

	result, err := r.InputPrompter.GetListInput(options, "Merge and deploy?")
	if err != nil {
		return false, clierrors.Message("Prompt failed: %v", err)
	}

	return strings.HasPrefix(result, "Yes"), nil
}

// waitForDeployWorkflow waits for the deploy workflow to start and displays progress.
func (r *Runner) waitForDeployWorkflow(ctx context.Context) error {
	r.Output.LogInfo("Waiting for deploy workflow to start...")
	time.Sleep(3 * time.Second)

	// Try to find the workflow run
	var run *github.WorkflowRun
	var err error

	for i := 0; i < 10; i++ {
		run, err = r.GitHubClient.GetLatestWorkflowRun(DeployWorkflowFile)
		if err == nil && run != nil {
			break
		}
		time.Sleep(2 * time.Second)
	}

	if run == nil {
		return fmt.Errorf("could not find deploy workflow run. Check GitHub Actions for status")
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
				fmt.Printf("\r%s Deploying resources...", spinner)
			}
		}
	}()

	finalRun, err := r.GitHubClient.WatchWorkflowRun(run.ID, nil)
	close(done)
	fmt.Printf("\r                                          \r") // Clear spinner line

	if err != nil {
		return fmt.Errorf("failed to watch workflow: %v", err)
	}

	r.Output.LogInfo("")

	if finalRun.Conclusion == "success" {
		r.Output.LogInfo("Deployment completed successfully!")
		return nil
	}

	// Handle failure
	r.Output.LogInfo("Deployment failed!")
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

	return fmt.Errorf("deployment failed. Please check the workflow logs")
}
