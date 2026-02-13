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

	"github.com/radius-project/radius/pkg/cli/config"
	"github.com/radius-project/radius/pkg/cli/github"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// githubInitOptions holds options specific to GitHub mode initialization.
type githubInitOptions struct {
	// Provider specifies the cloud provider (aws or azure).
	Provider string

	// DeploymentTool specifies the infrastructure tool (terraform or bicep).
	DeploymentTool string

	// EnvironmentName specifies the name of the environment to create.
	EnvironmentName string

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

// initializeRadiusDirectory creates the .radius/ directory structure.
// FR-014-A: Creates model/, plan/, deploy/ subdirectories with .gitkeep files.
func initializeRadiusDirectory(repoPath string) error {
	radiusDir := filepath.Join(repoPath, ".radius")

	// Create .radius directory
	if err := os.MkdirAll(radiusDir, 0755); err != nil {
		return fmt.Errorf("failed to create .radius directory: %w", err)
	}

	// Create subdirectories with .gitkeep files (FR-014-A)
	subdirs := []string{"model", "plan", "deploy"}
	for _, dir := range subdirs {
		path := filepath.Join(radiusDir, dir)
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("failed to create %s directory: %w", dir, err)
		}
		// Create .gitkeep to ensure directory is tracked by Git
		gitkeepPath := filepath.Join(path, ".gitkeep")
		if err := os.WriteFile(gitkeepPath, []byte{}, 0644); err != nil {
			return fmt.Errorf("failed to create .gitkeep in %s: %w", dir, err)
		}
	}

	return nil
}

// initializeGitHubWorkflows creates the .github/workflows/ directory
// and generates the workflow files.
func initializeGitHubWorkflows(repoPath, provider string) error {
	workflowsDir := filepath.Join(repoPath, ".github", "workflows")

	// Create workflows directory
	if err := os.MkdirAll(workflowsDir, 0755); err != nil {
		return fmt.Errorf("failed to create workflows directory: %w", err)
	}

	// Generate deploy workflow
	deployWorkflow := github.GenerateDeployWorkflow(provider)
	deployPath := filepath.Join(workflowsDir, "radius-deploy.yaml")
	if err := github.SaveWorkflow(deployPath, deployWorkflow); err != nil {
		return fmt.Errorf("failed to save deploy workflow: %w", err)
	}

	// Generate destroy workflow
	destroyWorkflow := github.GenerateDestroyWorkflow(provider)
	destroyPath := filepath.Join(workflowsDir, "radius-destroy.yaml")
	if err := github.SaveWorkflow(destroyPath, destroyWorkflow); err != nil {
		return fmt.Errorf("failed to save destroy workflow: %w", err)
	}

	// Generate plan workflow
	planWorkflow := github.GeneratePlanWorkflow(provider)
	planPath := filepath.Join(workflowsDir, "radius-plan.yaml")
	if err := github.SaveWorkflow(planPath, planWorkflow); err != nil {
		return fmt.Errorf("failed to save plan workflow: %w", err)
	}

	return nil
}

// commitRadiusInit commits all .radius/ files with the Radius-Action: init trailer.
func commitRadiusInit(repoPath string) error {
	gitHelper, err := github.NewGitHelper(repoPath)
	if err != nil {
		return fmt.Errorf("failed to access git repository: %w", err)
	}

	// Stage .radius/ directory
	if err := gitHelper.Add(".radius"); err != nil {
		return fmt.Errorf("failed to stage .radius directory: %w", err)
	}

	// Stage .github/workflows/ directory
	if err := gitHelper.Add(".github/workflows"); err != nil {
		// Non-fatal - workflows might not exist yet
	}

	// Create commit with Radius-Action trailer
	_, err = gitHelper.CommitWithTrailer("Initialize Radius configuration", "init")
	if err != nil {
		return fmt.Errorf("failed to create commit: %w", err)
	}

	return nil
}

// validateGitHubMode validates the GitHub mode flags and prerequisites.
func (r *Runner) validateGitHubMode(cmd *cobra.Command) error {
	// Validate provider flag
	if r.Provider == "" {
		return fmt.Errorf("--provider flag is required for GitHub mode (use 'aws' or 'azure')")
	}
	if r.Provider != "aws" && r.Provider != "azure" {
		return fmt.Errorf("--provider must be 'aws' or 'azure', got: %s", r.Provider)
	}

	// Validate deployment tool flag
	if r.DeploymentTool != "terraform" && r.DeploymentTool != "bicep" {
		return fmt.Errorf("--deployment-tool must be 'terraform' or 'bicep', got: %s", r.DeploymentTool)
	}

	// Validate environment name
	if r.EnvironmentName == "" {
		r.EnvironmentName = "default"
	}

	// Check GitHub prerequisites
	opts, err := validateGitHubPrerequisites(cmd.Context(), r.Output)
	if err != nil {
		return err
	}

	// Store options for use in Run
	opts.Provider = r.Provider
	opts.DeploymentTool = r.DeploymentTool
	opts.EnvironmentName = r.EnvironmentName

	// Store in runner for access in Run
	r.githubOpts = opts

	return nil
}

// runGitHubInit executes the GitHub mode initialization.
func (r *Runner) runGitHubInit(ctx context.Context) error {
	opts := r.githubOpts
	if opts == nil {
		return fmt.Errorf("GitHub options not initialized")
	}

	r.Output.LogInfo("Initializing Radius for GitHub repository: %s/%s", opts.Owner, opts.Repo)

	// Step 1: Create .radius/ directory structure
	r.Output.LogInfo("Creating .radius/ directory structure...")
	if err := initializeRadiusDirectory(opts.RepoPath); err != nil {
		return err
	}

	radiusDir := filepath.Join(opts.RepoPath, ".radius")

	// Step 2: Generate types.yaml (FR-008: fetch from resource-types-contrib)
	r.Output.LogInfo("Fetching resource types from radius-project/resource-types-contrib...")
	typesManifest, err := r.fetchTypesManifest(ctx)
	if err != nil {
		r.Output.LogInfo("Warning: Failed to fetch resource types, using defaults: %v", err)
		typesManifest = config.DefaultTypesManifest()
	}
	r.Output.LogInfo("Generating types.yaml...")
	if err := config.WriteTypesManifestWithHeader(radiusDir, typesManifest); err != nil {
		return err
	}

	// Step 3: Generate recipes.yaml (FR-009-A: recipe for each type in types.yaml)
	r.Output.LogInfo("Generating recipes.yaml...")
	recipesManifest := config.RecipesManifestFromTypes(typesManifest, opts.Provider, opts.DeploymentTool)
	if err := config.WriteRecipesManifestWithHeader(radiusDir, recipesManifest); err != nil {
		return err
	}

	// Step 4: Generate environment file
	r.Output.LogInfo("Generating env.%s.yaml...", opts.EnvironmentName)
	env := config.DefaultEnvironment(opts.EnvironmentName, opts.Provider)
	if err := config.WriteEnvironmentWithHeader(radiusDir, env); err != nil {
		return err
	}

	// Step 5: Generate GitHub Actions workflows
	r.Output.LogInfo("Generating GitHub Actions workflows...")
	if err := initializeGitHubWorkflows(opts.RepoPath, opts.Provider); err != nil {
		return err
	}

	// Step 6: Commit changes
	r.Output.LogInfo("Committing changes...")
	if err := commitRadiusInit(opts.RepoPath); err != nil {
		return err
	}

	// Step 7: Update local workspace config
	r.Output.LogInfo("Updating workspace configuration...")
	if err := r.updateGitHubWorkspace(ctx, opts); err != nil {
		return err
	}

	r.Output.LogInfo("")
	r.Output.LogInfo("✅ Radius initialized successfully!")
	r.Output.LogInfo("")
	r.Output.LogInfo("Next steps:")
	r.Output.LogInfo("  1. Run 'rad environment connect' to configure OIDC authentication")
	r.Output.LogInfo("  2. Run 'git push' to push the changes to GitHub")
	r.Output.LogInfo("  3. Run 'rad model' to create an application model")

	return nil
}

// updateGitHubWorkspace updates the local workspace configuration for GitHub mode.
func (r *Runner) updateGitHubWorkspace(ctx context.Context, opts *githubInitOptions) error {
	config := r.ConfigFileInterface.ConfigFromContext(ctx)

	// Create GitHub workspace per Appendix C.4 - only connection with url and kind
	repoURL := fmt.Sprintf("https://github.com/%s/%s", opts.Owner, opts.Repo)
	ws := &workspaces.Workspace{
		Name: opts.Repo,
		Connection: map[string]any{
			"kind": workspaces.KindGitHub,
			"url":  repoURL,
		},
		// Note: GitHub workspaces do not have Environment or Scope properties.
		// Environment info is stored in .radius/env.<name>.yaml files.
	}

	// Use EditWorkspaces to update the config file
	if err := r.ConfigFileInterface.EditWorkspaces(ctx, config, ws); err != nil {
		return fmt.Errorf("failed to save workspace config: %w", err)
	}

	// Set as the default workspace
	if err := r.ConfigFileInterface.SetDefaultWorkspace(ctx, config, ws.Name); err != nil {
		return fmt.Errorf("failed to set default workspace: %w", err)
	}

	return nil
}

// fetchTypesManifest fetches resource types from radius-project/resource-types-contrib
// per FR-008 and converts them to a types manifest.
func (r *Runner) fetchTypesManifest(ctx context.Context) (*config.ResourceTypesManifest, error) {
	ghClient := github.NewClient()
	fetcher := github.NewResourceTypeFetcher(ghClient)

	// Fetch resource types using sparse checkout
	// Pass empty string for targetDir - we don't need schema files copied
	types, err := fetcher.FetchResourceTypes(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch resource types: %w", err)
	}

	if len(types) == 0 {
		return nil, fmt.Errorf("no resource types found in repository")
	}

	// Convert fetched types to the manifest format
	fetchedTypes := make([]config.FetchedResourceType, 0, len(types))
	for _, rt := range types {
		// Parse namespace from type (e.g., "Radius.Compute/containers" -> "Radius.Compute")
		namespace := ""
		name := rt.Name
		if idx := len(rt.Type) - len(rt.Name) - 1; idx > 0 && rt.Type != "" {
			namespace = rt.Type[:idx]
		}

		// Build relative path based on type structure
		// e.g., "Radius.Compute/containers" -> "Compute/containers/containers.yaml"
		relativePath := rt.SchemaPath
		if relativePath == "" {
			// Generate path from type name if not provided
			parts := []string{}
			if namespace != "" {
				// Extract category from namespace (e.g., "Radius.Compute" -> "Compute")
				if dotIdx := len("Radius."); dotIdx < len(namespace) {
					parts = append(parts, namespace[dotIdx:])
				}
			}
			if name != "" {
				parts = append(parts, name, name+".yaml")
			}
			if len(parts) > 0 {
				relativePath = filepath.Join(parts...)
			}
		}

		fetchedTypes = append(fetchedTypes, config.FetchedResourceType{
			Type:         rt.Type,
			Name:         rt.Name,
			Namespace:    namespace,
			RelativePath: relativePath,
		})
	}

	return config.TypesManifestFromFetched(fetchedTypes), nil
}
