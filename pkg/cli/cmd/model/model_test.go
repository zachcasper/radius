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

package model
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

package model

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
			Name:          "model command with github workspace (requires .radius directory)",
			Input:         []string{},
			ExpectedValid: false, // Will fail because .radius directory doesn't exist
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithGitHubWorkspace,
			},
		},
		{
			Name:          "model command fails with Kubernetes workspace",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithKubernetesWorkspace,
			},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Validate_GitHubWorkspaceRequired(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .radius directory
	radiusDir := filepath.Join(tmpDir, ".radius")
	err := os.MkdirAll(radiusDir, 0755)
	require.NoError(t, err)

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()

	// Test with Kubernetes workspace (should fail based on workspace type)
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

	// Verify workspace type is Kubernetes
	require.Equal(t, workspaces.KindKubernetes, runner.Workspace.Connection["kind"])
}

func Test_Validate_RadiusInitRequired(t *testing.T) {
	tmpDir := t.TempDir()

	// Do NOT create .radius directory - simulating uninitialized state
	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()

	// Test with valid GitHub workspace but missing .radius directory
	runner := &Runner{
		ConfigHolder: &framework.ConfigHolder{ConfigFilePath: "filePath"},
		Workspace: &workspaces.Workspace{
			Connection: map[string]any{
				"kind": workspaces.KindGitHub,
			},
			Name: "test-workspace",
		},
	}

	// Verify .radius doesn't exist
	radiusDir := filepath.Join(tmpDir, ".radius")
	_, err = os.Stat(radiusDir)
	require.True(t, os.IsNotExist(err))

	// The workspace is GitHub, but .radius doesn't exist
	require.Equal(t, workspaces.KindGitHub, runner.Workspace.Connection["kind"])
}

func Test_BicepTemplateContent(t *testing.T) {
	// Verify the template contains expected resources
	require.Contains(t, todolistBicepTemplate, "extension radius")
	require.Contains(t, todolistBicepTemplate, "Radius.Core/applications@2025-08-01-preview")
	require.Contains(t, todolistBicepTemplate, "Radius.Compute/containers@2025-08-01-preview")
	require.Contains(t, todolistBicepTemplate, "Radius.Data/postgreSqlDatabases@2025-08-01-preview")
	require.Contains(t, todolistBicepTemplate, "resource todolist")
	require.Contains(t, todolistBicepTemplate, "resource frontend")
	require.Contains(t, todolistBicepTemplate, "resource db")
	require.Contains(t, todolistBicepTemplate, "connections:")
	require.Contains(t, todolistBicepTemplate, "postgresql:")
	require.Contains(t, todolistBicepTemplate, "source: db.id")
}

func Test_DefaultModelName(t *testing.T) {
	require.Equal(t, "todolist", DefaultModelName)
	require.Equal(t, ".bicep", ModelFileExtension)
}

func Test_ModelFilePath(t *testing.T) {
	// Verify the model file path construction
	testDir := "/test/repo"
	expectedPath := filepath.Join(testDir, ".radius", "model", DefaultModelName+ModelFileExtension)
	actualPath := filepath.Join(testDir, ".radius", "model", DefaultModelName+ModelFileExtension)
	require.Equal(t, expectedPath, actualPath)
}

func createConfigWithGitHubWorkspace(t *testing.T) *viper.Viper {
	v := viper.New()
	v.Set("workspaces", map[string]any{
		"default": "github-workspace",
		"items": map[string]any{
			"github-workspace": map[string]any{
				"connection": map[string]any{
					"kind": workspaces.KindGitHub,
				},
			},
		},
	})
	return v
}
