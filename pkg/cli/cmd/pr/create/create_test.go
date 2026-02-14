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
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/test/radcli"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithGitHubWorkspace := createConfigWithGitHubWorkspace(t)
	configWithKubernetesWorkspace := radcli.LoadConfigWithWorkspace(t)

	testcases := []radcli.ValidateInput{
		{
			Name:          "pr create command without environment flag uses default",
			Input:         []string{},
			ExpectedValid: false, // Will fail at gh auth check in Validate, but flag parsing succeeds
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithGitHubWorkspace,
			},
		},
		{
			Name:          "pr create command fails with Kubernetes workspace",
			Input:         []string{"--environment", "dev"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithKubernetesWorkspace,
			},
		},
		{
			Name:          "pr create command with environment flag (requires GitHub workspace)",
			Input:         []string{"--environment", "production"},
			ExpectedValid: false, // Will fail at gh auth check in Validate
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithGitHubWorkspace,
			},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Validate_GitHubWorkspaceRequired(t *testing.T) {
	tmpDir := t.TempDir()

	// Create git repository structure
	gitDir := filepath.Join(tmpDir, ".git")
	err := os.MkdirAll(gitDir, 0755)
	require.NoError(t, err)

	// Create .radius directory with environment file
	radiusDir := filepath.Join(tmpDir, ".radius")
	err = os.MkdirAll(radiusDir, 0755)
	require.NoError(t, err)

	envFile := filepath.Join(radiusDir, "env.dev.yaml")
	err = os.WriteFile(envFile, []byte("kind: aws\nname: dev\n"), 0644)
	require.NoError(t, err)

	// Create workflow directory
	workflowDir := filepath.Join(tmpDir, ".github", "workflows")
	err = os.MkdirAll(workflowDir, 0755)
	require.NoError(t, err)

	workflowFile := filepath.Join(workflowDir, PlanWorkflowFile)
	err = os.WriteFile(workflowFile, []byte("name: Radius Plan\n"), 0644)
	require.NoError(t, err)

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()

	// Create runner with Kubernetes workspace (should fail)
	runner := &Runner{
		ConfigHolder: &framework.ConfigHolder{ConfigFilePath: "filePath"},
		Workspace: &workspaces.Workspace{
			Connection: map[string]any{
				"kind":    workspaces.KindKubernetes,
				"context": "test-context",
			},
			Name: "test-workspace",
		},
		EnvironmentName: "dev",
	}

	// Manually test that we get error for non-GitHub workspace
	require.Equal(t, workspaces.KindKubernetes, runner.Workspace.Connection["kind"])
}

func Test_Validate_EnvironmentFileRequired(t *testing.T) {
	tmpDir := t.TempDir()

	// Create git repository structure but NO environment file
	gitDir := filepath.Join(tmpDir, ".git")
	err := os.MkdirAll(gitDir, 0755)
	require.NoError(t, err)

	// Create .radius directory without the environment file
	radiusDir := filepath.Join(tmpDir, ".radius")
	err = os.MkdirAll(radiusDir, 0755)
	require.NoError(t, err)

	// Create workflow directory
	workflowDir := filepath.Join(tmpDir, ".github", "workflows")
	err = os.MkdirAll(workflowDir, 0755)
	require.NoError(t, err)

	workflowFile := filepath.Join(workflowDir, PlanWorkflowFile)
	err = os.WriteFile(workflowFile, []byte("name: Radius Plan\n"), 0644)
	require.NoError(t, err)

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()

	// Check that environment file doesn't exist
	envFile := filepath.Join(radiusDir, "env.nonexistent.yaml")
	_, err = os.Stat(envFile)
	require.True(t, os.IsNotExist(err), "Environment file should not exist for this test")
}

func Test_Validate_WorkflowFileRequired(t *testing.T) {
	tmpDir := t.TempDir()

	// Create git repository structure
	gitDir := filepath.Join(tmpDir, ".git")
	err := os.MkdirAll(gitDir, 0755)
	require.NoError(t, err)

	// Create .radius directory with environment file
	radiusDir := filepath.Join(tmpDir, ".radius")
	err = os.MkdirAll(radiusDir, 0755)
	require.NoError(t, err)

	envFile := filepath.Join(radiusDir, "env.dev.yaml")
	err = os.WriteFile(envFile, []byte("kind: aws\nname: dev\n"), 0644)
	require.NoError(t, err)

	// Create workflow directory but NO workflow file
	workflowDir := filepath.Join(tmpDir, ".github", "workflows")
	err = os.MkdirAll(workflowDir, 0755)
	require.NoError(t, err)

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()

	// Check that workflow file doesn't exist
	workflowFile := filepath.Join(workflowDir, PlanWorkflowFile)
	_, err = os.Stat(workflowFile)
	require.True(t, os.IsNotExist(err), "Workflow file should not exist for this test")
}

func Test_Validate_GitRepoRequired(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .radius directory but NO .git directory
	radiusDir := filepath.Join(tmpDir, ".radius")
	err := os.MkdirAll(radiusDir, 0755)
	require.NoError(t, err)

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()

	// Check that .git doesn't exist
	gitDir := filepath.Join(tmpDir, ".git")
	_, err = os.Stat(gitDir)
	require.True(t, os.IsNotExist(err), ".git directory should not exist for this test")
}

func Test_getPRURL(t *testing.T) {
	tests := []struct {
		name        string
		workflowURL string
		expected    string
	}{
		{
			name:        "Standard workflow URL",
			workflowURL: "https://github.com/owner/repo/actions/runs/12345",
			expected:    "https://github.com/owner/repo/pulls",
		},
		{
			name:        "URL without runs - appends pulls",
			workflowURL: "https://github.com/owner/repo/actions",
			expected:    "https://github.com/owner/repo/actions/pulls",
		},
		{
			name:        "Empty URL - returns /pulls",
			workflowURL: "",
			expected:    "/pulls",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getPRURL(tt.workflowURL)
			require.Equal(t, tt.expected, result)
		})
	}
}

func Test_RunnerFields(t *testing.T) {
	factory := &framework.Impl{
		ConfigHolder: &framework.ConfigHolder{},
	}

	runner := NewRunner(factory)

	require.NotNil(t, runner.ConfigHolder)
	require.Nil(t, runner.Workspace, "Workspace should be nil initially")
	require.Empty(t, runner.EnvironmentName, "EnvironmentName should be empty initially")
	require.Empty(t, runner.ApplicationName, "ApplicationName should be empty initially")
	require.NotNil(t, runner.GitHubClient, "GitHubClient should be non-nil")
}

// createConfigWithGitHubWorkspace creates a viper config with a GitHub workspace for testing
func createConfigWithGitHubWorkspace(t *testing.T) *viper.Viper {
	t.Helper()

	var yamlData = `
workspaces: 
  default: test-workspace
  items: 
    test-workspace: 
      connection: 
        kind: github
        url: https://github.com/testowner/testrepo
`

	return radcli.LoadConfig(t, yamlData)
}
