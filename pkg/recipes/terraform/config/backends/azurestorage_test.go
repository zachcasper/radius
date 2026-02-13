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

package backends

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/stretchr/testify/require"
)

type mockBlobClient struct {
	existingBlobs map[string]bool
	returnError   error
}

func (m *mockBlobClient) GetProperties(ctx context.Context, containerName, blobName string) error {
	if m.returnError != nil {
		return m.returnError
	}

	key := containerName + "/" + blobName
	if m.existingBlobs[key] {
		return nil
	}

	return errors.New("BlobNotFound: The specified blob does not exist")
}

func getAzureTestInputs() recipes.ResourceMetadata {
	return recipes.ResourceMetadata{
		Name:          "test-recipe",
		Parameters:    map[string]any{},
		EnvironmentID: "/planes/radius/local/resourceGroups/test-group/providers/Applications.Environments/testEnv/env",
		ApplicationID: "/planes/radius/local/resourceGroups/test-group/providers/Applications.Applications/testApp/app",
		ResourceID:    "/planes/radius/local/resourceGroups/test-group/providers/Applications.Datastores/redisCaches/redis",
	}
}

func Test_AzureStorageBackend_BuildBackend(t *testing.T) {
	t.Parallel()

	resourceRecipe := getAzureTestInputs()
	config := AzureStorageBackendConfig{
		ResourceGroupName:  "my-rg",
		StorageAccountName: "mystorageaccount",
		ContainerName:      "tfstate",
	}

	backend := NewAzureStorageBackend(config, &mockBlobClient{})
	result, err := backend.BuildBackend(&resourceRecipe)
	require.NoError(t, err)
	require.NotNil(t, result)

	azConfig, ok := result[BackendAzureRM].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "my-rg", azConfig["resource_group_name"])
	require.Equal(t, "mystorageaccount", azConfig["storage_account_name"])
	require.Equal(t, "tfstate", azConfig["container_name"])
	require.Contains(t, azConfig["key"], AzureStateBlobPrefix)
}

func Test_AzureStorageBackend_BuildBackend_DefaultContainer(t *testing.T) {
	t.Parallel()

	resourceRecipe := getAzureTestInputs()
	config := AzureStorageBackendConfig{
		ResourceGroupName:  "my-rg",
		StorageAccountName: "mystorageaccount",
		// ContainerName not set - should use default
	}

	backend := NewAzureStorageBackend(config, &mockBlobClient{})
	result, err := backend.BuildBackend(&resourceRecipe)
	require.NoError(t, err)
	require.NotNil(t, result)

	azConfig, ok := result[BackendAzureRM].(map[string]any)
	require.True(t, ok)
	require.Equal(t, AzureStateContainerName, azConfig["container_name"])
}

func Test_AzureStorageBackend_BuildBackend_WithOIDC(t *testing.T) {
	t.Parallel()

	resourceRecipe := getAzureTestInputs()
	config := AzureStorageBackendConfig{
		ResourceGroupName:  "my-rg",
		StorageAccountName: "mystorageaccount",
		ContainerName:      "tfstate",
		UseOIDC:            true,
	}

	backend := NewAzureStorageBackend(config, &mockBlobClient{})
	result, err := backend.BuildBackend(&resourceRecipe)
	require.NoError(t, err)
	require.NotNil(t, result)

	azConfig, ok := result[BackendAzureRM].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, azConfig["use_oidc"])
}

func Test_AzureStorageBackend_BuildBackend_WithSubscriptionID(t *testing.T) {
	t.Parallel()

	resourceRecipe := getAzureTestInputs()
	config := AzureStorageBackendConfig{
		ResourceGroupName:  "my-rg",
		StorageAccountName: "mystorageaccount",
		ContainerName:      "tfstate",
		SubscriptionID:     "00000000-0000-0000-0000-000000000000",
	}

	backend := NewAzureStorageBackend(config, &mockBlobClient{})
	result, err := backend.BuildBackend(&resourceRecipe)
	require.NoError(t, err)
	require.NotNil(t, result)

	azConfig, ok := result[BackendAzureRM].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "00000000-0000-0000-0000-000000000000", azConfig["subscription_id"])
}

func Test_AzureStorageBackend_BuildBackend_InvalidResourceID(t *testing.T) {
	t.Parallel()

	resourceRecipe := getAzureTestInputs()
	resourceRecipe.ResourceID = "invalid"

	config := AzureStorageBackendConfig{
		ResourceGroupName:  "my-rg",
		StorageAccountName: "mystorageaccount",
	}

	backend := NewAzureStorageBackend(config, &mockBlobClient{})
	_, err := backend.BuildBackend(&resourceRecipe)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to generate Azure state key")
}

func Test_AzureStorageBackend_BuildBackend_InvalidEnvID(t *testing.T) {
	t.Parallel()

	resourceRecipe := getAzureTestInputs()
	resourceRecipe.EnvironmentID = "invalid"

	config := AzureStorageBackendConfig{
		ResourceGroupName:  "my-rg",
		StorageAccountName: "mystorageaccount",
	}

	backend := NewAzureStorageBackend(config, &mockBlobClient{})
	_, err := backend.BuildBackend(&resourceRecipe)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to generate Azure state key")
}

func Test_AzureStorageBackend_BuildBackend_InvalidAppID(t *testing.T) {
	t.Parallel()

	resourceRecipe := getAzureTestInputs()
	resourceRecipe.ApplicationID = "invalid"

	config := AzureStorageBackendConfig{
		ResourceGroupName:  "my-rg",
		StorageAccountName: "mystorageaccount",
	}

	backend := NewAzureStorageBackend(config, &mockBlobClient{})
	_, err := backend.BuildBackend(&resourceRecipe)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to generate Azure state key")
}

func Test_AzureStorageBackend_ValidateBackendExists_Found(t *testing.T) {
	t.Parallel()

	resourceRecipe := getAzureTestInputs()

	// Generate expected key
	hasher := sha1.New()
	inputString := strings.ToLower(fmt.Sprintf("%s-%s-%s", "env", "app", resourceRecipe.ResourceID))
	_, err := hasher.Write([]byte(inputString))
	require.NoError(t, err)
	hashStr := fmt.Sprintf("%x", hasher.Sum(nil))
	expectedKey := fmt.Sprintf("%senv/app/%s.tfstate", AzureStateBlobPrefix, hashStr)

	mockClient := &mockBlobClient{
		existingBlobs: map[string]bool{
			AzureStateContainerName + "/" + expectedKey: true,
		},
	}

	config := AzureStorageBackendConfig{
		ResourceGroupName:  "my-rg",
		StorageAccountName: "mystorageaccount",
	}

	backend := NewAzureStorageBackend(config, mockClient)
	exists, err := backend.ValidateBackendExists(context.Background(), expectedKey)
	require.NoError(t, err)
	require.True(t, exists)
}

func Test_AzureStorageBackend_ValidateBackendExists_NotFound(t *testing.T) {
	t.Parallel()

	mockClient := &mockBlobClient{
		existingBlobs: map[string]bool{},
	}

	config := AzureStorageBackendConfig{
		ResourceGroupName:  "my-rg",
		StorageAccountName: "mystorageaccount",
	}

	backend := NewAzureStorageBackend(config, mockClient)
	exists, err := backend.ValidateBackendExists(context.Background(), "nonexistent-blob")
	require.NoError(t, err)
	require.False(t, exists)
}

func Test_AzureStorageBackend_ValidateBackendExists_Error(t *testing.T) {
	t.Parallel()

	mockClient := &mockBlobClient{
		returnError: errors.New("access denied"),
	}

	config := AzureStorageBackendConfig{
		ResourceGroupName:  "my-rg",
		StorageAccountName: "mystorageaccount",
	}

	backend := NewAzureStorageBackend(config, mockClient)
	_, err := backend.ValidateBackendExists(context.Background(), "some-blob")
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to check Azure blob state")
}

func Test_AzureStorageBackend_GenerateStateKey(t *testing.T) {
	t.Parallel()

	resourceRecipe := getAzureTestInputs()
	config := AzureStorageBackendConfig{
		ResourceGroupName:  "my-rg",
		StorageAccountName: "mystorageaccount",
	}

	backend := NewAzureStorageBackend(config, &mockBlobClient{}).(*azureStorageBackend)
	key, err := backend.generateStateKey(&resourceRecipe)
	require.NoError(t, err)

	// Verify key format
	require.True(t, strings.HasPrefix(key, AzureStateBlobPrefix))
	require.Contains(t, key, "env/")
	require.Contains(t, key, "app/")
	require.True(t, strings.HasSuffix(key, ".tfstate"))

	// Verify same inputs produce same key
	key2, err := backend.generateStateKey(&resourceRecipe)
	require.NoError(t, err)
	require.Equal(t, key, key2)
}
