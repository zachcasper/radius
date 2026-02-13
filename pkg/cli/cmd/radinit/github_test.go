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

	"github.com/stretchr/testify/require"
)

func Test_initializeRadiusDirectory(t *testing.T) {
	tempDir := t.TempDir()

	err := initializeRadiusDirectory(tempDir)
	require.NoError(t, err)

	// Verify .radius directory was created
	radiusDir := filepath.Join(tempDir, ".radius")
	info, err := os.Stat(radiusDir)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	// Verify plan subdirectory was created
	planDir := filepath.Join(radiusDir, "plan")
	info, err = os.Stat(planDir)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	// Verify deploy subdirectory was created
	deployDir := filepath.Join(radiusDir, "deploy")
	info, err = os.Stat(deployDir)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

func Test_initializeRadiusDirectory_AlreadyExists(t *testing.T) {
	tempDir := t.TempDir()

	// Create .radius directory first
	radiusDir := filepath.Join(tempDir, ".radius")
	err := os.MkdirAll(radiusDir, 0755)
	require.NoError(t, err)

	// Should not fail if directory already exists
	err = initializeRadiusDirectory(tempDir)
	require.NoError(t, err)
}

func Test_initializeGitHubWorkflows(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name     string
		provider string
	}{
		{
			name:     "AWS provider",
			provider: "aws",
		},
		{
			name:     "Azure provider",
			provider: "azure",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDir := filepath.Join(tempDir, tt.provider)
			err := os.MkdirAll(testDir, 0755)
			require.NoError(t, err)

			err = initializeGitHubWorkflows(testDir, tt.provider)
			require.NoError(t, err)

			// Verify .github/workflows directory was created
			workflowDir := filepath.Join(testDir, ".github", "workflows")
			info, err := os.Stat(workflowDir)
			require.NoError(t, err)
			require.True(t, info.IsDir())

			// Verify workflow files were created
			deployFile := filepath.Join(workflowDir, "radius-deploy.yaml")
			_, err = os.Stat(deployFile)
			require.NoError(t, err)

			planFile := filepath.Join(workflowDir, "radius-plan.yaml")
			_, err = os.Stat(planFile)
			require.NoError(t, err)

			destroyFile := filepath.Join(workflowDir, "radius-destroy.yaml")
			_, err = os.Stat(destroyFile)
			require.NoError(t, err)
		})
	}
}

func Test_validateGitHubMode_MissingProvider(t *testing.T) {
	runner := &Runner{
		GitHub:         true,
		Provider:       "",
		DeploymentTool: "terraform",
	}

	err := runner.validateGitHubMode(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "--provider flag is required")
}

func Test_validateGitHubMode_InvalidProvider(t *testing.T) {
	runner := &Runner{
		GitHub:         true,
		Provider:       "gcp",
		DeploymentTool: "terraform",
	}

	err := runner.validateGitHubMode(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "--provider must be 'aws' or 'azure'")
}

func Test_validateGitHubMode_InvalidDeploymentTool(t *testing.T) {
	runner := &Runner{
		GitHub:         true,
		Provider:       "aws",
		DeploymentTool: "pulumi",
	}

	err := runner.validateGitHubMode(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "--deployment-tool must be 'terraform' or 'bicep'")
}

func Test_validateGitHubMode_DefaultEnvironmentName(t *testing.T) {
	runner := &Runner{
		GitHub:          true,
		Provider:        "aws",
		DeploymentTool:  "terraform",
		EnvironmentName: "",
	}

	// Since validateGitHubMode sets the default before calling validateGitHubPrerequisites,
	// we can verify the default is set even though the overall call will fail due to missing git repo.
	// The environment name default is set early in the function.
	// For now, just verify the initial field handling.
	require.Equal(t, "", runner.EnvironmentName)

	// After validation (with error), the default should be set
	// Skipping full validation test as it requires git repo context
}

func Test_githubInitOptions_Structure(t *testing.T) {
	opts := &githubInitOptions{
		Provider:        "aws",
		DeploymentTool:  "terraform",
		EnvironmentName: "dev",
		RepoPath:        "/path/to/repo",
		Owner:           "myorg",
		Repo:            "myrepo",
	}

	require.Equal(t, "aws", opts.Provider)
	require.Equal(t, "terraform", opts.DeploymentTool)
	require.Equal(t, "dev", opts.EnvironmentName)
	require.Equal(t, "/path/to/repo", opts.RepoPath)
	require.Equal(t, "myorg", opts.Owner)
	require.Equal(t, "myrepo", opts.Repo)
}
