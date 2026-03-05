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
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/cli/github"
	"github.com/stretchr/testify/require"
)

func Test_initializeGitHubWorkflows(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("creates workflow files", func(t *testing.T) {
		testDir := filepath.Join(tempDir, "test-repo")
		err := os.MkdirAll(testDir, 0755)
		require.NoError(t, err)

		err = initializeGitHubWorkflows(testDir)
		require.NoError(t, err)

		// Verify .github/workflows directory was created
		workflowDir := filepath.Join(testDir, ".github", "workflows")
		info, err := os.Stat(workflowDir)
		require.NoError(t, err)
		require.True(t, info.IsDir())

		// Verify rad-deploy.yaml was created
		deployFile := filepath.Join(workflowDir, github.DeployWorkflowFile)
		_, err = os.Stat(deployFile)
		require.NoError(t, err)

		// Verify rad-app-delete.yaml was created
		appDeleteFile := filepath.Join(workflowDir, github.AppDeleteWorkflowFile)
		_, err = os.Stat(appDeleteFile)
		require.NoError(t, err)

		// Verify rad-auth-test.yaml was created
		authTestFile := filepath.Join(workflowDir, github.AuthTestWorkflowFile)
		_, err = os.Stat(authTestFile)
		require.NoError(t, err)
	})
}

func Test_findGitRoot(t *testing.T) {
	t.Run("finds git root from subdirectory", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create a fake .git directory
		gitDir := filepath.Join(tempDir, ".git")
		err := os.MkdirAll(gitDir, 0755)
		require.NoError(t, err)

		// Create a subdirectory
		subDir := filepath.Join(tempDir, "src", "pkg")
		err = os.MkdirAll(subDir, 0755)
		require.NoError(t, err)

		// Find root from subdirectory
		root, err := findGitRoot(subDir)
		require.NoError(t, err)
		require.Equal(t, tempDir, root)
	})

	t.Run("finds git root from root", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create a fake .git directory
		gitDir := filepath.Join(tempDir, ".git")
		err := os.MkdirAll(gitDir, 0755)
		require.NoError(t, err)

		// Find root from root
		root, err := findGitRoot(tempDir)
		require.NoError(t, err)
		require.Equal(t, tempDir, root)
	})

	t.Run("returns error if not a git repo", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create a subdirectory without .git
		subDir := filepath.Join(tempDir, "src")
		err := os.MkdirAll(subDir, 0755)
		require.NoError(t, err)

		// Should fail when no .git directory exists
		_, err = findGitRoot(subDir)
		require.Error(t, err)
		require.Contains(t, err.Error(), "not a git repository")
	})
}

func Test_githubInitOptions_Structure(t *testing.T) {
	opts := &githubInitOptions{
		ResourceTypesRepo: "https://github.com/example/resource-types",
		RepoPath:          "/path/to/repo",
		Owner:             "myorg",
		Repo:              "myrepo",
	}

	require.Equal(t, "https://github.com/example/resource-types", opts.ResourceTypesRepo)
	require.Equal(t, "/path/to/repo", opts.RepoPath)
	require.Equal(t, "myorg", opts.Owner)
	require.Equal(t, "myrepo", opts.Repo)
}

func Test_DefaultResourceTypesRepoURL(t *testing.T) {
	// Verify the constant is set
	require.NotEmpty(t, DefaultResourceTypesRepoURL)
	require.Contains(t, DefaultResourceTypesRepoURL, "github.com")
}
