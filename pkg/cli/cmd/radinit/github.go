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

package radinit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/radius-project/radius/pkg/cli/github"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// githubInitOptions holds options specific to GitHub mode initialization.
type githubInitOptions struct {
	// ResourceTypesRepo is the URL to the resource types repository.
	ResourceTypesRepo string

	// RepoPath is the path to the git repository root.
	RepoPath string

	// Owner is the GitHub repository owner.
	Owner string

	// Repo is the GitHub repository name.
	Repo string
}

// validateGitHubPrerequisites checks that the current directory is a valid
// GitHub repository and that the user is authenticated with gh CLI.
func validateGitHubPrerequisites(ctx context.Context, output output.Interface) (*githubInitOptions, error) {
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current directory: %w", err)
	}

	// Find git repository root
	repoPath, err := findGitRoot(cwd)
	if err != nil {
		return nil, fmt.Errorf("current directory is not a git repository: %w", err)
	}

	// Check if this is a GitHub repository
	gitHelper, err := github.NewGitHelper(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to access git repository: %w", err)
	}

	isGitHub, err := gitHelper.IsGitHubRemote()
	if err != nil {
		return nil, fmt.Errorf("failed to check remote URL: %w", err)
	}
	if !isGitHub {
		return nil, fmt.Errorf("remote origin is not a GitHub repository")
	}

	owner, repo, err := gitHelper.GetOwnerRepo()
	if err != nil {
		return nil, fmt.Errorf("failed to parse GitHub repository info: %w", err)
	}

	// Check GitHub CLI authentication
	ghClient := github.NewClient()
	if err := ghClient.AuthStatus(); err != nil {
		output.LogInfo("GitHub CLI authentication required. Run 'gh auth login' to authenticate.")
		return nil, err
	}

	return &githubInitOptions{
		RepoPath: repoPath,
		Owner:    owner,
		Repo:     repo,
	}, nil
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

// DefaultResourceTypesRepoURL is the default URL for the resource types repository.
const DefaultResourceTypesRepoURL = "https://github.com/zachcasper/resource-types-contrib/tree/github-radius"

// initializeGitHubWorkflows creates the .github/workflows/ directory
// and generates the 4 workflow files for the two-phase deployment model.
// FR-112: Generates radius-deployment-create.yaml, radius-deployment-apply.yaml,
// radius-destroy.yaml, and radius-auth-test.yaml
func initializeGitHubWorkflows(repoPath string) error {
	workflowsDir := filepath.Join(repoPath, ".github", "workflows")

	// Create workflows directory
	if err := os.MkdirAll(workflowsDir, 0755); err != nil {
		return fmt.Errorf("failed to create workflows directory: %w", err)
	}

	// Generate deployment create workflow
	createWorkflow := github.GenerateDeploymentCreateWorkflow()
	createPath := filepath.Join(workflowsDir, github.DeploymentCreateWorkflowFile)
	if err := github.SaveWorkflow(createPath, createWorkflow); err != nil {
		return fmt.Errorf("failed to save deployment create workflow: %w", err)
	}

	// Generate deployment apply workflow
	applyWorkflow := github.GenerateDeploymentApplyWorkflow()
	applyPath := filepath.Join(workflowsDir, github.DeploymentApplyWorkflowFile)
	if err := github.SaveWorkflow(applyPath, applyWorkflow); err != nil {
		return fmt.Errorf("failed to save deployment apply workflow: %w", err)
	}

	// Generate destroy workflow
	destroyWorkflow := github.GenerateDestroyWorkflowV2()
	destroyPath := filepath.Join(workflowsDir, github.DestroyWorkflowFile)
	if err := github.SaveWorkflow(destroyPath, destroyWorkflow); err != nil {
		return fmt.Errorf("failed to save destroy workflow: %w", err)
	}

	// Generate auth test workflow
	authTestWorkflow := github.GenerateAuthTestWorkflowV2()
	authTestPath := filepath.Join(workflowsDir, github.AuthTestWorkflowFile)
	if err := github.SaveWorkflow(authTestPath, authTestWorkflow); err != nil {
		return fmt.Errorf("failed to save auth test workflow: %w", err)
	}

	return nil
}

// commitRadiusInit commits workflow files with the Radius-Action: init trailer.
// Returns true if a commit was created, false if there were no changes to commit.
func commitRadiusInit(repoPath string) (bool, error) {
	gitHelper, err := github.NewGitHelper(repoPath)
	if err != nil {
		return false, fmt.Errorf("failed to access git repository: %w", err)
	}

	// Stage .github/workflows/ directory
	if err := gitHelper.Add(".github/workflows"); err != nil {
		// Non-fatal - workflows might not exist yet
	}

	// Create commit with Radius-Action trailer (returns empty hash if nothing to commit)
	hash, err := gitHelper.CommitWithTrailer("Initialize Radius configuration", "init")
	if err != nil {
		return false, fmt.Errorf("failed to create commit: %w", err)
	}

	return hash != "", nil
}

// validateGitHubMode validates the GitHub mode flags and prerequisites.
func (r *Runner) validateGitHubMode(cmd *cobra.Command) error {
	// Check GitHub prerequisites (FR-008: git repo, FR-009: GitHub remote, FR-010: gh auth)
	opts, err := validateGitHubPrerequisites(cmd.Context(), r.Output)
	if err != nil {
		return err
	}

	// Set resource types repo URL
	if r.ResourceTypesRepo != "" {
		opts.ResourceTypesRepo = r.ResourceTypesRepo
	} else {
		opts.ResourceTypesRepo = DefaultResourceTypesRepoURL
	}

	// Store in runner for access in Run
	r.githubOpts = opts

	return nil
}

// runGitHubInit executes the GitHub mode initialization.
// FR-001 through FR-014: Sets up repository, workflows, repo variable, workspace config.
// FR-002/FR-003/FR-004: Does NOT create types.yaml, recipes.yaml, or env files.
func (r *Runner) runGitHubInit(ctx context.Context) error {
	opts := r.githubOpts
	if opts == nil {
		return fmt.Errorf("GitHub options not initialized")
	}

	r.Output.LogInfo("Initializing Radius for GitHub repository: %s/%s", opts.Owner, opts.Repo)

	// T016: Check for re-initialization — if workflow files already exist, warn user
	workflowFile := filepath.Join(opts.RepoPath, ".github", "workflows", github.DeploymentCreateWorkflowFile)
	if _, err := os.Stat(workflowFile); err == nil {
		r.Output.LogInfo("")
		r.Output.LogInfo("Warning: Radius workflow files already exist in this repository.")
		r.Output.LogInfo("Reinitializing will overwrite existing workflow files.")
		r.Output.LogInfo("")

		confirmed, err := r.Prompter.GetListInput([]string{"Yes, reinitialize", "No, cancel"}, "Reinitialize Radius?")
		if err != nil {
			return err
		}
		if confirmed != "Yes, reinitialize" {
			r.Output.LogInfo("Initialization cancelled.")
			return nil
		}
	}

	// Step 1: Generate 4 GitHub Actions workflow files (FR-112)
	r.Output.LogInfo("Generating GitHub Actions workflows...")
	if err := initializeGitHubWorkflows(opts.RepoPath); err != nil {
		return err
	}

	// Step 2: Set RADIUS_RESOURCE_TYPES_REPO repo variable (FR-005, FR-006)
	r.Output.LogInfo("Setting repository variable RADIUS_RESOURCE_TYPES_REPO...")
	ghClient := github.NewClient()
	if err := ghClient.SetRepoVariable("RADIUS_RESOURCE_TYPES_REPO", opts.ResourceTypesRepo); err != nil {
		return fmt.Errorf("failed to set RADIUS_RESOURCE_TYPES_REPO: %w", err)
	}

	// Step 3: Commit and push (FR-013)
	r.Output.LogInfo("Committing changes...")
	committed, err := commitRadiusInit(opts.RepoPath)
	if err != nil {
		return err
	}

	if committed {
		r.Output.LogInfo("Pushing to GitHub...")
		gitHelper, err := github.NewGitHelper(opts.RepoPath)
		if err != nil {
			return fmt.Errorf("failed to access git repository: %w", err)
		}
		if err := gitHelper.Push(); err != nil {
			return fmt.Errorf("failed to push changes: %w", err)
		}
	} else {
		r.Output.LogInfo("No changes to commit.")
	}

	// Step 4: Update local workspace config (FR-011)
	r.Output.LogInfo("Updating workspace configuration...")
	if err := r.updateGitHubWorkspace(ctx, opts); err != nil {
		return err
	}

	r.Output.LogInfo("")
	r.Output.LogInfo("Radius initialized successfully!")
	r.Output.LogInfo("")
	r.Output.LogInfo("Next steps:")
	r.Output.LogInfo("  1. Run 'rad environment create <name> --provider <aws|azure>' to set up a cloud environment")
	r.Output.LogInfo("  2. Run 'rad app model' to create an application definition")
	r.Output.LogInfo("  3. Run 'rad deployment create' to generate a deployment plan")
	r.Output.LogInfo("  4. Run 'rad deployment apply' to execute the deployment")

	return nil
}

// updateGitHubWorkspace updates the local workspace configuration for GitHub mode.
func (r *Runner) updateGitHubWorkspace(ctx context.Context, opts *githubInitOptions) error {
	cfg := r.ConfigFileInterface.ConfigFromContext(ctx)

	// Create GitHub workspace — only connection with url and kind (FR-011)
	repoURL := fmt.Sprintf("https://github.com/%s/%s", opts.Owner, opts.Repo)
	ws := &workspaces.Workspace{
		Name: opts.Repo,
		Connection: map[string]any{
			"kind": workspaces.KindGitHub,
			"url":  repoURL,
		},
		// Note: GitHub workspaces do not have Environment or Scope properties.
		// Environment info is stored as GitHub Environment variables.
	}

	// Use EditWorkspaces to update the config file
	if err := r.ConfigFileInterface.EditWorkspaces(ctx, cfg, ws); err != nil {
		return fmt.Errorf("failed to save workspace config: %w", err)
	}

	// Set as the current workspace
	if err := r.ConfigFileInterface.SetDefaultWorkspace(ctx, cfg, ws.Name); err != nil {
		return fmt.Errorf("failed to set current workspace: %w", err)
	}

	return nil
}
