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
	"fmt"
	"strings"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

var _ Backend = (*azureStorageBackend)(nil)

const (
	BackendAzureRM = "azurerm"

	// AzureStateContainerName is the default container name for Terraform state files.
	AzureStateContainerName = "radius-tfstate"

	// AzureStateBlobPrefix is the prefix used for Terraform state file blobs.
	AzureStateBlobPrefix = "tfstate/"
)

// AzureStorageBackendConfig holds configuration for the Azure Storage backend.
type AzureStorageBackendConfig struct {
	// ResourceGroupName is the name of the resource group containing the storage account.
	ResourceGroupName string

	// StorageAccountName is the name of the Azure Storage account.
	StorageAccountName string

	// ContainerName is the name of the blob container for storing state.
	ContainerName string

	// SubscriptionID is the Azure subscription ID.
	SubscriptionID string

	// UseOIDC enables OIDC authentication for the backend.
	UseOIDC bool
}

// azureStorageBackend implements the Backend interface for Azure Storage.
type azureStorageBackend struct {
	config     AzureStorageBackendConfig
	blobClient BlobClient
}

// BlobClient is an interface for Azure Blob operations used by the Azure Storage backend.
// This interface allows for testing with mock implementations.
type BlobClient interface {
	GetProperties(ctx context.Context, containerName, blobName string) error
}

// NewAzureStorageBackend creates a new Azure Storage backend with the given configuration.
func NewAzureStorageBackend(config AzureStorageBackendConfig, client BlobClient) Backend {
	// Use default container name if not specified
	if config.ContainerName == "" {
		config.ContainerName = AzureStateContainerName
	}

	return &azureStorageBackend{
		config:     config,
		blobClient: client,
	}
}

// BuildBackend generates the Terraform backend configuration for Azure Storage backend.
// https://developer.hashicorp.com/terraform/language/settings/backends/azurerm
func (b *azureStorageBackend) BuildBackend(resourceRecipe *recipes.ResourceMetadata) (map[string]any, error) {
	stateKey, err := b.generateStateKey(resourceRecipe)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Azure state key: %w", err)
	}

	backendConfig := map[string]any{
		"resource_group_name":  b.config.ResourceGroupName,
		"storage_account_name": b.config.StorageAccountName,
		"container_name":       b.config.ContainerName,
		"key":                  stateKey,
	}

	// Add subscription ID if specified
	if b.config.SubscriptionID != "" {
		backendConfig["subscription_id"] = b.config.SubscriptionID
	}

	// Enable OIDC authentication if configured
	if b.config.UseOIDC {
		backendConfig["use_oidc"] = true
	}

	return map[string]any{
		BackendAzureRM: backendConfig,
	}, nil
}

// ValidateBackendExists checks if the Terraform state file exists in Azure Blob Storage.
// name is the blob name that was generated during terraform apply.
func (b *azureStorageBackend) ValidateBackendExists(ctx context.Context, name string) (bool, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	err := b.blobClient.GetProperties(ctx, b.config.ContainerName, name)
	if err != nil {
		// Check if the error indicates the blob doesn't exist
		errStr := err.Error()
		if strings.Contains(errStr, "BlobNotFound") || strings.Contains(errStr, "404") {
			logger.Info(fmt.Sprintf("Azure blob %q does not exist in container %q", name, b.config.ContainerName))
			return false, nil
		}
		return false, fmt.Errorf("failed to check Azure blob state: %w", err)
	}

	return true, nil
}

// generateStateKey returns a unique blob name for the terraform state file
// based on resourceID, environmentID, and applicationID.
func (b *azureStorageBackend) generateStateKey(resourceRecipe *recipes.ResourceMetadata) (string, error) {
	parsedResourceID, err := resources.Parse(resourceRecipe.ResourceID)
	if err != nil {
		return "", fmt.Errorf("failed to parse resource ID: %w", err)
	}

	parsedEnvID, err := resources.Parse(resourceRecipe.EnvironmentID)
	if err != nil {
		return "", fmt.Errorf("failed to parse environment ID: %w", err)
	}

	parsedAppID, err := resources.Parse(resourceRecipe.ApplicationID)
	if err != nil {
		return "", fmt.Errorf("failed to parse application ID: %w", err)
	}

	// Create a unique identifier for the state file
	inputString := strings.ToLower(fmt.Sprintf("%s-%s-%s", parsedEnvID.Name(), parsedAppID.Name(), parsedResourceID.String()))

	hasher := sha1.New()
	_, err = hasher.Write([]byte(inputString))
	if err != nil {
		return "", fmt.Errorf("failed to generate hash: %w", err)
	}
	hash := hasher.Sum(nil)
	hashStr := fmt.Sprintf("%x", hash)

	// Format: tfstate/{env}/{app}/{hash}.tfstate
	return fmt.Sprintf("%s%s/%s/%s.tfstate", AzureStateBlobPrefix, parsedEnvID.Name(), parsedAppID.Name(), hashStr), nil
}

// GetStateKeyName returns the full state key name for use with ValidateBackendExists.
// This is a helper function to get the key that was generated for a given resource.
func (b *azureStorageBackend) GetStateKeyName(resourceRecipe *recipes.ResourceMetadata) (string, error) {
	return b.generateStateKey(resourceRecipe)
}
