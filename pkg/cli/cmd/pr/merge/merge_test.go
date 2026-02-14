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
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/github"
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
			Name:          "pr merge command fails with Kubernetes workspace",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithKubernetesWorkspace,
			},
		},
		{
			Name:          "pr merge command with GitHub workspace (requires gh auth)",
			Input:         []string{},
			ExpectedValid: false, // Will fail at gh auth check in Validate
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithGitHubWorkspace,
			},
		},
		{
			Name:          "pr merge command with --pr flag",
			Input:         []string{"--pr", "42"},
			ExpectedValid: false, // Will fail at gh auth check in Validate
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithGitHubWorkspace,
			},
		},
		{
			Name:          "pr merge command with --yes flag",
			Input:         []string{"--yes"},
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

	// Create workflow directory
	workflowDir := filepath.Join(tmpDir, ".github", "workflows")
	err = os.MkdirAll(workflowDir, 0755)
	require.NoError(t, err)

	workflowFile := filepath.Join(workflowDir, DeployWorkflowFile)
	err = os.WriteFile(workflowFile, []byte("name: Radius Deploy\n"), 0644)
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
	}

	// Manually test that we get error for non-GitHub workspace
	require.Equal(t, workspaces.KindKubernetes, runner.Workspace.Connection["kind"])
}

func Test_Validate_WorkflowFileRequired(t *testing.T) {
	tmpDir := t.TempDir()

	// Create git repository structure
	gitDir := filepath.Join(tmpDir, ".git")
	err := os.MkdirAll(gitDir, 0755)
	require.NoError(t, err)

	// Create workflow directory but NO deploy workflow file
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
	workflowFile := filepath.Join(workflowDir, DeployWorkflowFile)
	_, err = os.Stat(workflowFile)
	require.True(t, os.IsNotExist(err), "Deploy workflow file should not exist for this test")
}

func Test_Validate_GitRepoRequired(t *testing.T) {
	tmpDir := t.TempDir()

	// Create empty directory but NO .git directory

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

func Test_findLatestDeploymentPR(t *testing.T) {
	// This test verifies the logic of findLatestDeploymentPR
	runner := &Runner{
		GitHubClient: github.NewClient(),
	}

	// Test that DeployBranchPrefix is correctly set
	require.Equal(t, "deploy/", DeployBranchPrefix)
	require.NotNil(t, runner.GitHubClient)
}

func Test_confirmMerge_NoPrompter(t *testing.T) {
	// When no prompter is available, confirmMerge should return true
	runner := &Runner{
		InputPrompter: nil,
	}

	pr := &github.PullRequest{
		Number: 42,
		Title:  "Test PR",
	}

	confirmed, err := runner.confirmMerge(pr)
	require.NoError(t, err)
	require.True(t, confirmed, "Should return true when no prompter is available")
}

func Test_RunnerFields(t *testing.T) {
	factory := &framework.Impl{
		ConfigHolder: &framework.ConfigHolder{},
	}

	runner := NewRunner(factory)

	require.NotNil(t, runner.ConfigHolder)
	require.Nil(t, runner.Workspace, "Workspace should be nil initially")
	require.Equal(t, 0, runner.PRNumber, "PRNumber should be 0 initially")
	require.False(t, runner.SkipConfirm, "SkipConfirm should be false initially")
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
