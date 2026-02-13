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

// Package github provides GitHub CLI integration for Radius on GitHub mode.
package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Client wraps the GitHub CLI (gh) for Radius operations.
type Client struct {
	// Verbose enables verbose output for debugging.
	Verbose bool
}

// NewClient creates a new GitHub CLI client.
func NewClient() *Client {
	return &Client{}
}

// AuthStatus checks if the user is authenticated with the GitHub CLI.
// Returns nil if authenticated, error otherwise.
func (c *Client) AuthStatus() error {
	_, err := c.runCommand("auth", "status")
	if err != nil {
		return fmt.Errorf("GitHub CLI authentication check failed: %w. Run 'gh auth login' to authenticate", err)
	}
	return nil
}

// CreatePR creates a pull request on GitHub.
func (c *Client) CreatePR(title, body, baseBranch, headBranch string) (*PullRequest, error) {
	args := []string{
		"pr", "create",
		"--title", title,
		"--body", body,
		"--base", baseBranch,
		"--head", headBranch,
		"--json", "number,url,title,state",
	}

	output, err := c.runCommand(args...)
	if err != nil {
		return nil, fmt.Errorf("failed to create pull request: %w", err)
	}

	var pr PullRequest
	if err := json.Unmarshal([]byte(output), &pr); err != nil {
		return nil, fmt.Errorf("failed to parse pull request response: %w", err)
	}

	return &pr, nil
}

// MergePR merges a pull request by number.
func (c *Client) MergePR(prNumber int, method string, deleteBranch bool) error {
	args := []string{
		"pr", "merge",
		fmt.Sprintf("%d", prNumber),
		"--" + method, // --merge, --squash, or --rebase
	}

	if deleteBranch {
		args = append(args, "--delete-branch")
	}

	_, err := c.runCommand(args...)
	if err != nil {
		return fmt.Errorf("failed to merge pull request #%d: %w", prNumber, err)
	}

	return nil
}

// GetPR retrieves information about a pull request.
func (c *Client) GetPR(prNumber int) (*PullRequest, error) {
	output, err := c.runCommand(
		"pr", "view",
		fmt.Sprintf("%d", prNumber),
		"--json", "number,url,title,state,headRefName,baseRefName",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get pull request #%d: %w", prNumber, err)
	}

	var pr PullRequest
	if err := json.Unmarshal([]byte(output), &pr); err != nil {
		return nil, fmt.Errorf("failed to parse pull request response: %w", err)
	}

	return &pr, nil
}

// ListPRs lists open pull requests matching a filter.
func (c *Client) ListPRs(state, headBranchPrefix string) ([]PullRequest, error) {
	args := []string{
		"pr", "list",
		"--state", state,
		"--json", "number,url,title,state,headRefName,baseRefName",
	}

	if headBranchPrefix != "" {
		args = append(args, "--head", headBranchPrefix)
	}

	output, err := c.runCommand(args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list pull requests: %w", err)
	}

	var prs []PullRequest
	if err := json.Unmarshal([]byte(output), &prs); err != nil {
		return nil, fmt.Errorf("failed to parse pull request list: %w", err)
	}

	return prs, nil
}

// RunWorkflow triggers a GitHub Actions workflow.
func (c *Client) RunWorkflow(workflowName string, ref string, inputs map[string]string) error {
	args := []string{
		"workflow", "run",
		workflowName,
		"--ref", ref,
	}

	for key, value := range inputs {
		args = append(args, "-f", fmt.Sprintf("%s=%s", key, value))
	}

	_, err := c.runCommand(args...)
	if err != nil {
		return fmt.Errorf("failed to run workflow %s: %w", workflowName, err)
	}

	return nil
}

// GetRepoInfo retrieves information about the current repository.
func (c *Client) GetRepoInfo() (*RepoInfo, error) {
	output, err := c.runCommand(
		"repo", "view",
		"--json", "name,owner,url,defaultBranchRef",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository info: %w", err)
	}

	var repo RepoInfo
	if err := json.Unmarshal([]byte(output), &repo); err != nil {
		return nil, fmt.Errorf("failed to parse repository info: %w", err)
	}

	return &repo, nil
}

// SetSecret sets a repository secret.
func (c *Client) SetSecret(name, value string) error {
	cmd := exec.Command("gh", "secret", "set", name)
	cmd.Stdin = strings.NewReader(value)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set secret %s: %w", name, err)
	}

	return nil
}

// runCommand executes a gh CLI command and returns the output.
func (c *Client) runCommand(args ...string) (string, error) {
	cmd := exec.Command("gh", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if c.Verbose {
		fmt.Printf("gh %s\n", strings.Join(args, " "))
	}

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// RunCommandInteractive runs a gh command with stdin/stdout attached for interactive use.
func (c *Client) RunCommandInteractive(args ...string) error {
	cmd := exec.Command("gh", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// PullRequest represents a GitHub pull request.
type PullRequest struct {
	Number      int    `json:"number"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	State       string `json:"state"`
	HeadRefName string `json:"headRefName"`
	BaseRefName string `json:"baseRefName"`
}

// RepoInfo represents basic repository information.
type RepoInfo struct {
	Name             string    `json:"name"`
	Owner            RepoOwner `json:"owner"`
	URL              string    `json:"url"`
	DefaultBranchRef BranchRef `json:"defaultBranchRef"`
}

// RepoOwner represents the repository owner.
type RepoOwner struct {
	Login string `json:"login"`
}

// BranchRef represents a branch reference.
type BranchRef struct {
	Name string `json:"name"`
}

// FullName returns the full repository name in owner/repo format.
func (r *RepoInfo) FullName() string {
	return fmt.Sprintf("%s/%s", r.Owner.Login, r.Name)
}

// WorkflowRun represents a GitHub Actions workflow run.
type WorkflowRun struct {
	ID         int64  `json:"databaseId"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	URL        string `json:"url"`
	HeadBranch string `json:"headBranch"`
	Event      string `json:"event"`
}

// GetLatestWorkflowRun gets the most recent workflow run for a given workflow file.
func (c *Client) GetLatestWorkflowRun(workflowFile string) (*WorkflowRun, error) {
	output, err := c.runCommand(
		"run", "list",
		"--workflow", workflowFile,
		"--limit", "1",
		"--json", "databaseId,name,status,conclusion,url,headBranch,event",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow runs: %w", err)
	}

	var runs []WorkflowRun
	if err := json.Unmarshal([]byte(output), &runs); err != nil {
		return nil, fmt.Errorf("failed to parse workflow runs: %w", err)
	}

	if len(runs) == 0 {
		return nil, nil
	}

	return &runs[0], nil
}

// WatchWorkflowRun waits for a workflow run to complete, calling the progress callback periodically.
// Returns the final workflow run status, or an error if watching fails.
func (c *Client) WatchWorkflowRun(runID int64, progressCallback func(status string)) (*WorkflowRun, error) {
	for {
		output, err := c.runCommand(
			"run", "view",
			fmt.Sprintf("%d", runID),
			"--json", "databaseId,name,status,conclusion,url,headBranch,event",
		)
		if err != nil {
			return nil, fmt.Errorf("failed to get workflow run status: %w", err)
		}

		var run WorkflowRun
		if err := json.Unmarshal([]byte(output), &run); err != nil {
			return nil, fmt.Errorf("failed to parse workflow run: %w", err)
		}

		if progressCallback != nil {
			progressCallback(run.Status)
		}

		// Check if the run has completed
		if run.Status == "completed" {
			return &run, nil
		}

		// Wait before checking again
		time.Sleep(3 * time.Second)
	}
}

// GetWorkflowRunLogs gets the logs for a failed workflow run.
func (c *Client) GetWorkflowRunLogs(runID int64) (string, error) {
	output, err := c.runCommand(
		"run", "view",
		fmt.Sprintf("%d", runID),
		"--log-failed",
	)
	if err != nil {
		// If --log-failed doesn't work, try getting all logs
		output, err = c.runCommand(
			"run", "view",
			fmt.Sprintf("%d", runID),
			"--log",
		)
		if err != nil {
			return "", fmt.Errorf("failed to get workflow logs: %w", err)
		}
	}

	return output, nil
}
