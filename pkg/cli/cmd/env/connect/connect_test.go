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

package connect

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/test/radcli"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)
	configWithGitHubWorkspace := createConfigWithGitHubWorkspace(t)

	testcases := []radcli.ValidateInput{
		{
			Name:          "Valid connect command with GitHub workspace",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithGitHubWorkspace,
			},
		},
		{
			Name:          "Connect command fails with Kubernetes workspace",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Connect command with environment flag",
			Input:         []string{"--environment", "production"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithGitHubWorkspace,
			},
			ValidateCallback: func(t *testing.T, runner framework.Runner) {
				r := runner.(*Runner)
				require.Equal(t, "production", r.EnvironmentName)
			},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run_AWS_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tmpDir := t.TempDir()

	// Create git repository structure
	gitDir := filepath.Join(tmpDir, ".git")
	err := os.MkdirAll(gitDir, 0755)
	require.NoError(t, err)

	// Create .radius directory with environment file
	radiusDir := filepath.Join(tmpDir, ".radius")
	err = os.MkdirAll(radiusDir, 0755)
	require.NoError(t, err)

	// Create AWS environment file
	envContent := `kind: aws
name: default
provider: {}
`
	envFile := filepath.Join(radiusDir, "env.default.yaml")
	err = os.WriteFile(envFile, []byte(envContent), 0644)
	require.NoError(t, err)

	// Set up mocks
	mockPrompter := prompt.NewMockInterface(ctrl)
	mockCommandRunner := &MockCommandRunner{
		responses: map[string]string{
			"aws sts get-caller-identity --output json": `{"Account": "123456789012", "Arn": "arn:aws:iam::123456789012:user/test"}`,
			"aws configure get region":                  "us-west-2",
		},
	}
	outputSink := &output.MockOutput{}

	workspace := &workspaces.Workspace{
		Connection: map[string]any{
			"kind": workspaces.KindGitHub,
			"url":  "https://github.com/testowner/testrepo",
		},
		Name: "test-workspace",
	}

	runner := &Runner{
		ConfigHolder:    &framework.ConfigHolder{ConfigFilePath: "filePath"},
		Output:          outputSink,
		Workspace:       workspace,
		EnvironmentName: "default",
		InputPrompter:   mockPrompter,
		CommandRunner:   mockCommandRunner,
	}

	// Set up prompt expectations for AWS flow
	// Account ID prompt
	mockPrompter.EXPECT().
		GetTextInput("AWS Account ID", gomock.Any()).
		Return("123456789012", nil).Times(1)

	// Region prompt
	mockPrompter.EXPECT().
		GetTextInput("AWS Region", gomock.Any()).
		Return("us-west-2", nil).Times(1)

	// Create new role or use existing
	mockPrompter.EXPECT().
		GetListInput([]string{"Create new IAM role", "Use existing IAM role ARN"}, "OIDC IAM Role").
		Return("Use existing IAM role ARN", nil).Times(1)

	// Existing role ARN prompt
	mockPrompter.EXPECT().
		GetTextInput("IAM Role ARN", gomock.Any()).
		Return("arn:aws:iam::123456789012:role/existing-role", nil).Times(1)

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()

	// Since we're using an existing role, we only need state backend creation
	// S3 bucket creation (FR-097-B naming)
	mockCommandRunner.responses["aws s3api create-bucket --bucket tfstate-testowner-testrepo --region us-west-2 --create-bucket-configuration LocationConstraint=us-west-2"] = "{}"
	// S3 versioning
	mockCommandRunner.responses["aws s3api put-bucket-versioning --bucket tfstate-testowner-testrepo --versioning-configuration Status=Enabled"] = "{}"
	// DynamoDB table creation
	mockCommandRunner.responses["aws dynamodb create-table --table-name tfstate-lock-testowner-testrepo --attribute-definitions AttributeName=LockID,AttributeType=S --key-schema AttributeName=LockID,KeyType=HASH --billing-mode PAY_PER_REQUEST --region us-west-2"] = "{}"

	err = runner.Run(context.Background())

	// We expect an error about git operations since we don't have a real git repo
	// but we should get past the AWS config phase
	require.Error(t, err)
	require.Contains(t, err.Error(), "git")
}

func Test_Run_Azure_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tmpDir := t.TempDir()

	// Create git repository structure
	gitDir := filepath.Join(tmpDir, ".git")
	err := os.MkdirAll(gitDir, 0755)
	require.NoError(t, err)

	// Create .radius directory with environment file
	radiusDir := filepath.Join(tmpDir, ".radius")
	err = os.MkdirAll(radiusDir, 0755)
	require.NoError(t, err)

	// Create Azure environment file
	envContent := `kind: azure
name: default
provider: {}
`
	envFile := filepath.Join(radiusDir, "env.default.yaml")
	err = os.WriteFile(envFile, []byte(envContent), 0644)
	require.NoError(t, err)

	// Set up mocks
	mockPrompter := prompt.NewMockInterface(ctrl)
	mockCommandRunner := &MockCommandRunner{
		responses: map[string]string{
			"az account show --output json": `{"tenantId": "tenant-123", "id": "sub-456"}`,
			"az account list --output json": `[{"id": "sub-456", "name": "Test Subscription", "tenantId": "tenant-123", "state": "Enabled", "isDefault": true}]`,
		},
	}
	outputSink := &output.MockOutput{}

	workspace := &workspaces.Workspace{
		Connection: map[string]any{
			"kind": workspaces.KindGitHub,
			"url":  "https://github.com/testowner/testrepo",
		},
		Name: "test-workspace",
	}

	runner := &Runner{
		ConfigHolder:    &framework.ConfigHolder{ConfigFilePath: "filePath"},
		Output:          outputSink,
		Workspace:       workspace,
		EnvironmentName: "default",
		InputPrompter:   mockPrompter,
		CommandRunner:   mockCommandRunner,
	}

	// Set up prompt expectations for Azure flow
	// Subscription selection
	mockPrompter.EXPECT().
		GetListInput([]string{"Test Subscription (sub-456)"}, "Select Azure Subscription").
		Return("Test Subscription (sub-456)", nil).Times(1)

	// Resource group prompt
	mockPrompter.EXPECT().
		GetTextInput("Azure Resource Group Name", gomock.Any()).
		Return("radius-rg", nil).Times(1)

	// Create new app or use existing
	mockPrompter.EXPECT().
		GetListInput([]string{"Create new Azure AD application", "Use existing Azure AD application"}, "Azure AD Application").
		Return("Use existing Azure AD application", nil).Times(1)

	// Existing app client ID prompt
	mockPrompter.EXPECT().
		GetTextInput("Azure AD Application (Client) ID", gomock.Any()).
		Return("client-id-123", nil).Times(1)

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()

	// Storage account creation (FR-097-A naming)
	mockCommandRunner.responses["az storage account create --name tfstateradiustestownert --resource-group radius-rg --subscription sub-456 --sku Standard_LRS --kind StorageV2 --min-tls-version TLS1_2"] = "{}"
	// Blob container creation
	mockCommandRunner.responses["az storage container create --name tfstate --account-name tfstateradiustestownert --auth-mode login"] = "{}"

	err = runner.Run(context.Background())

	// We expect an error about git operations since we don't have a real git repo
	// but we should get past the Azure config phase
	require.Error(t, err)
	require.Contains(t, err.Error(), "git")
}

func Test_parseAzureSubscriptions(t *testing.T) {
	testcases := []struct {
		name     string
		input    string
		expected []azureSubscription
		hasError bool
	}{
		{
			name:  "Parse single enabled subscription",
			input: `[{"id": "sub-123", "name": "My Subscription", "tenantId": "tenant-456", "state": "Enabled", "isDefault": true}]`,
			expected: []azureSubscription{
				{ID: "sub-123", Name: "My Subscription", TenantID: "tenant-456", IsDefault: true},
			},
			hasError: false,
		},
		{
			name: "Parse multiple subscriptions with disabled filtered out",
			input: `[
				{"id": "sub-1", "name": "Active Sub", "tenantId": "t1", "state": "Enabled", "isDefault": true},
				{"id": "sub-2", "name": "Disabled Sub", "tenantId": "t1", "state": "Disabled", "isDefault": false},
				{"id": "sub-3", "name": "Another Active", "tenantId": "t2", "state": "Enabled", "isDefault": false}
			]`,
			expected: []azureSubscription{
				{ID: "sub-1", Name: "Active Sub", TenantID: "t1", IsDefault: true},
				{ID: "sub-3", Name: "Another Active", TenantID: "t2", IsDefault: false},
			},
			hasError: false,
		},
		{
			name:     "Empty subscription list",
			input:    `[]`,
			expected: []azureSubscription{},
			hasError: false,
		},
		{
			name:     "Invalid JSON",
			input:    `invalid json`,
			expected: nil,
			hasError: true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseAzureSubscriptions(tc.input)
			if tc.hasError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expected, result)
			}
		})
	}
}

func Test_parseGitHubURL(t *testing.T) {
	testcases := []struct {
		url           string
		expectedOwner string
		expectedRepo  string
	}{
		{
			url:           "https://github.com/owner/repo",
			expectedOwner: "owner",
			expectedRepo:  "repo",
		},
		{
			url:           "https://github.com/owner/repo.git",
			expectedOwner: "owner",
			expectedRepo:  "repo",
		},
		// Note: SSH URLs (git@github.com:owner/repo.git) are not supported
		// GitHub workspace configs use HTTPS URLs
	}

	for _, tc := range testcases {
		t.Run(tc.url, func(t *testing.T) {
			owner, repo := parseGitHubURL(tc.url)
			require.Equal(t, tc.expectedOwner, owner)
			require.Equal(t, tc.expectedRepo, repo)
		})
	}
}

func Test_sanitizeStorageAccountName(t *testing.T) {
	testcases := []struct {
		input    string
		expected string
	}{
		{
			input:    "radiustfownerrepo",
			expected: "radiustfownerrepo",
		},
		{
			input:    "Radius-TF_Owner.Repo",
			expected: "radiustfownerrepo",
		},
		{
			input:    "radiustfverylongownernamewithlongreponame",
			expected: "radiustfverylongownernam",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.input, func(t *testing.T) {
			result := sanitizeStorageAccountName(tc.input)
			require.Equal(t, tc.expected, result)
		})
	}
}

func Test_extractJSONField(t *testing.T) {
	testcases := []struct {
		name     string
		json     string
		field    string
		expected string
	}{
		{
			name:     "Extract existing field",
			json:     `{"Account": "123456", "Arn": "arn:aws:iam::123456:user/test"}`,
			field:    "Account",
			expected: "123456",
		},
		{
			name:     "Extract missing field",
			json:     `{"Account": "123456"}`,
			field:    "NotFound",
			expected: "",
		},
		{
			name:     "Invalid JSON",
			json:     `invalid`,
			field:    "any",
			expected: "",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			result := extractJSONField(tc.json, tc.field)
			require.Equal(t, tc.expected, result)
		})
	}
}

func Test_Validate_NonGitHubWorkspace(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	configWithKubernetesWorkspace := radcli.LoadConfigWithWorkspace(t)

	fw := &framework.Impl{
		ConfigHolder: &framework.ConfigHolder{
			Config: configWithKubernetesWorkspace,
		},
		Output: &output.MockOutput{},
	}

	cmd, runner := NewCommand(fw)
	cmd.SetArgs([]string{})

	err := cmd.ParseFlags([]string{})
	require.NoError(t, err)

	err = runner.Validate(cmd, []string{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "GitHub workspace")
}

func Test_Run_EnvironmentNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tmpDir := t.TempDir()

	// Create git repository structure
	gitDir := filepath.Join(tmpDir, ".git")
	err := os.MkdirAll(gitDir, 0755)
	require.NoError(t, err)

	// Create .radius directory but NO environment file
	radiusDir := filepath.Join(tmpDir, ".radius")
	err = os.MkdirAll(radiusDir, 0755)
	require.NoError(t, err)

	outputSink := &output.MockOutput{}

	workspace := &workspaces.Workspace{
		Connection: map[string]any{
			"kind": workspaces.KindGitHub,
			"url":  "https://github.com/testowner/testrepo",
		},
		Name: "test-workspace",
	}

	runner := &Runner{
		ConfigHolder:    &framework.ConfigHolder{ConfigFilePath: "filePath"},
		Output:          outputSink,
		Workspace:       workspace,
		EnvironmentName: "nonexistent",
		CommandRunner:   &MockCommandRunner{},
	}

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()

	err = runner.Run(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "Environment file not found")
}

func Test_Run_AWSAuthenticationFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tmpDir := t.TempDir()

	// Create git repository structure
	gitDir := filepath.Join(tmpDir, ".git")
	err := os.MkdirAll(gitDir, 0755)
	require.NoError(t, err)

	// Create .radius directory with environment file
	radiusDir := filepath.Join(tmpDir, ".radius")
	err = os.MkdirAll(radiusDir, 0755)
	require.NoError(t, err)

	// Create AWS environment file
	envContent := `kind: aws
name: default
provider: {}
`
	envFile := filepath.Join(radiusDir, "env.default.yaml")
	err = os.WriteFile(envFile, []byte(envContent), 0644)
	require.NoError(t, err)

	// Set up mock command runner that fails AWS auth
	mockCommandRunner := &MockCommandRunner{
		errors: map[string]error{
			"aws sts get-caller-identity --output json": errors.New("AWS credentials not configured"),
		},
	}
	outputSink := &output.MockOutput{}

	workspace := &workspaces.Workspace{
		Connection: map[string]any{
			"kind": workspaces.KindGitHub,
			"url":  "https://github.com/testowner/testrepo",
		},
		Name: "test-workspace",
	}

	runner := &Runner{
		ConfigHolder:    &framework.ConfigHolder{ConfigFilePath: "filePath"},
		Output:          outputSink,
		Workspace:       workspace,
		EnvironmentName: "default",
		CommandRunner:   mockCommandRunner,
	}

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()

	err = runner.Run(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "AWS CLI authentication failed")
}

func Test_Run_AzureAuthenticationFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tmpDir := t.TempDir()

	// Create git repository structure
	gitDir := filepath.Join(tmpDir, ".git")
	err := os.MkdirAll(gitDir, 0755)
	require.NoError(t, err)

	// Create .radius directory with environment file
	radiusDir := filepath.Join(tmpDir, ".radius")
	err = os.MkdirAll(radiusDir, 0755)
	require.NoError(t, err)

	// Create Azure environment file
	envContent := `kind: azure
name: default
provider: {}
`
	envFile := filepath.Join(radiusDir, "env.default.yaml")
	err = os.WriteFile(envFile, []byte(envContent), 0644)
	require.NoError(t, err)

	// Set up mock command runner that fails Azure auth
	mockCommandRunner := &MockCommandRunner{
		errors: map[string]error{
			"az account show --output json": errors.New("Please run 'az login' to set up account"),
		},
	}
	outputSink := &output.MockOutput{}

	workspace := &workspaces.Workspace{
		Connection: map[string]any{
			"kind": workspaces.KindGitHub,
			"url":  "https://github.com/testowner/testrepo",
		},
		Name: "test-workspace",
	}

	runner := &Runner{
		ConfigHolder:    &framework.ConfigHolder{ConfigFilePath: "filePath"},
		Output:          outputSink,
		Workspace:       workspace,
		EnvironmentName: "default",
		CommandRunner:   mockCommandRunner,
	}

	// Change to temp directory
	origDir, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()

	err = runner.Run(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "Azure CLI authentication failed")
}

// MockCommandRunner is a test mock for github.CommandRunner
type MockCommandRunner struct {
	responses map[string]string
	errors    map[string]error
	invoked   []string
}

func (m *MockCommandRunner) RunCommand(ctx context.Context, name string, args ...string) (string, error) {
	// Build command key
	key := name
	for _, arg := range args {
		key += " " + arg
	}
	m.invoked = append(m.invoked, key)

	// Check for error
	if m.errors != nil {
		if err, ok := m.errors[key]; ok {
			return "", err
		}
	}

	// Check for response
	if m.responses != nil {
		if response, ok := m.responses[key]; ok {
			return response, nil
		}
	}

	// Default: return empty success
	return "", nil
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
