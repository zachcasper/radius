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

package github

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewOIDCSetup(t *testing.T) {
	t.Parallel()

	setup := NewOIDCSetup(nil, nil, nil, "owner", "repo")
	require.NotNil(t, setup)
	require.Equal(t, "owner", setup.Owner)
	require.Equal(t, "repo", setup.Repo)
}

func TestOIDCSetup_Structure(t *testing.T) {
	t.Parallel()

	setup := &OIDCSetup{
		Output:    nil,
		Prompter:  nil,
		CmdRunner: nil,
		Owner:     "my-org",
		Repo:      "my-repo",
	}

	require.Equal(t, "my-org", setup.Owner)
	require.Equal(t, "my-repo", setup.Repo)
}

func TestAWSOIDCResult_Structure(t *testing.T) {
	t.Parallel()

	result := &AWSOIDCResult{
		AccountID:           "123456789012",
		Region:              "us-west-2",
		RoleARN:             "arn:aws:iam::123456789012:role/radius-role",
		EKSClusterName:      "my-cluster",
		KubernetesNamespace: "radius-system",
		StateBackend: &AWSStateBackend{
			Bucket:        "my-state-bucket",
			Region:        "us-west-2",
			DynamoDBTable: "terraform-locks",
		},
	}

	require.Equal(t, "123456789012", result.AccountID)
	require.Equal(t, "us-west-2", result.Region)
	require.Equal(t, "arn:aws:iam::123456789012:role/radius-role", result.RoleARN)
	require.Equal(t, "my-cluster", result.EKSClusterName)
	require.Equal(t, "radius-system", result.KubernetesNamespace)
	require.NotNil(t, result.StateBackend)
	require.Equal(t, "my-state-bucket", result.StateBackend.Bucket)
}

func TestAzureOIDCResult_Structure(t *testing.T) {
	t.Parallel()

	result := &AzureOIDCResult{
		SubscriptionID:      "00000000-0000-0000-0000-000000000000",
		TenantID:            "11111111-1111-1111-1111-111111111111",
		ClientID:            "22222222-2222-2222-2222-222222222222",
		ResourceGroupName:   "my-resource-group",
		AKSClusterName:      "my-aks-cluster",
		KubernetesNamespace: "radius-system",
		StateBackend: &AzureStateBackend{
			StorageAccountName: "mystorageaccount",
			ContainerName:      "tfstate",
		},
	}

	require.Equal(t, "00000000-0000-0000-0000-000000000000", result.SubscriptionID)
	require.Equal(t, "11111111-1111-1111-1111-111111111111", result.TenantID)
	require.Equal(t, "22222222-2222-2222-2222-222222222222", result.ClientID)
	require.Equal(t, "my-resource-group", result.ResourceGroupName)
	require.Equal(t, "my-aks-cluster", result.AKSClusterName)
	require.NotNil(t, result.StateBackend)
	require.Equal(t, "mystorageaccount", result.StateBackend.StorageAccountName)
}

func TestExtractJSONField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		jsonStr  string
		field    string
		expected string
	}{
		{
			name:     "valid field",
			jsonStr:  `{"name":"test","value":123}`,
			field:    "name",
			expected: "test",
		},
		{
			name:     "numeric field",
			jsonStr:  `{"name":"test","value":123}`,
			field:    "value",
			expected: "123",
		},
		{
			name:     "missing field",
			jsonStr:  `{"name":"test"}`,
			field:    "missing",
			expected: "",
		},
		{
			name:     "invalid json",
			jsonStr:  "not valid json",
			field:    "name",
			expected: "",
		},
		{
			name:     "empty json",
			jsonStr:  "{}",
			field:    "name",
			expected: "",
		},
		{
			name:     "null value",
			jsonStr:  `{"name":null}`,
			field:    "name",
			expected: "<nil>",
		},
		{
			name:     "boolean value",
			jsonStr:  `{"active":true}`,
			field:    "active",
			expected: "true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractJSONField(tt.jsonStr, tt.field)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeStorageAccountName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "already valid",
			input:    "mystorageaccount",
			expected: "mystorageaccount",
		},
		{
			name:     "uppercase to lowercase",
			input:    "MyStorageAccount",
			expected: "mystorageaccount",
		},
		{
			name:     "removes special characters",
			input:    "my-storage_account!",
			expected: "mystorageaccount",
		},
		{
			name:     "removes spaces",
			input:    "my storage account",
			expected: "mystorageaccount",
		},
		{
			name:     "truncates to 24 chars",
			input:    "thisstorageaccountnameiswaytoolongtobevalid",
			expected: "thisstorageaccountnameis",
		},
		{
			name:     "preserves numbers",
			input:    "storage123account456",
			expected: "storage123account456",
		},
		{
			name:     "handles empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "handles only special chars",
			input:    "!!!---___",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeStorageAccountName(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestAWSStateBackend_Structure(t *testing.T) {
	t.Parallel()

	backend := &AWSStateBackend{
		Bucket:        "my-bucket",
		Region:        "us-east-1",
		DynamoDBTable: "locks",
	}

	require.Equal(t, "my-bucket", backend.Bucket)
	require.Equal(t, "us-east-1", backend.Region)
	require.Equal(t, "locks", backend.DynamoDBTable)
}

func TestAzureStateBackend_Structure(t *testing.T) {
	t.Parallel()

	backend := &AzureStateBackend{
		StorageAccountName: "mystorageaccount",
		ContainerName:      "tfstate",
	}

	require.Equal(t, "mystorageaccount", backend.StorageAccountName)
	require.Equal(t, "tfstate", backend.ContainerName)
}
