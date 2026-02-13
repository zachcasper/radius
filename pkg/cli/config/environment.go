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

package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	// ProviderKindAWS indicates an AWS cloud provider.
	ProviderKindAWS = "aws"

	// ProviderKindAzure indicates an Azure cloud provider.
	ProviderKindAzure = "azure"
)

// Environment defines a deployment target environment.
// This is stored in .radius/env.<NAME>.yaml.
type Environment struct {
	// Name is the environment name (e.g., "default", "dev", "production").
	Name string `yaml:"name"`

	// Kind specifies the cloud provider: "aws" or "azure".
	Kind string `yaml:"kind"`

	// Recipes is the path to the recipes manifest file (e.g., ".radius/recipes.yaml").
	Recipes string `yaml:"recipes"`

	// RecipeParameters holds optional recipe-specific parameters.
	RecipeParameters map[string]map[string]any `yaml:"recipeParameters,omitempty"`

	// Provider holds cloud provider-specific configuration.
	Provider ProviderConfig `yaml:"provider"`
}

// ProviderConfig holds cloud provider-specific configuration.
type ProviderConfig struct {
	AWS   *AWSProviderConfig   `yaml:"aws,omitempty"`
	Azure *AzureProviderConfig `yaml:"azure,omitempty"`
}

// AWSProviderConfig holds AWS-specific environment configuration.
type AWSProviderConfig struct {
	// AccountID is the AWS account ID.
	AccountID string `yaml:"accountId"`

	// Region is the AWS region for deployments.
	Region string `yaml:"region"`

	// OIDCRoleARN is the IAM role ARN for GitHub Actions OIDC authentication.
	OIDCRoleARN string `yaml:"oidcRoleARN"`

	// EKSClusterName is the name of the EKS cluster for Kubernetes deployments.
	EKSClusterName string `yaml:"eksClusterName"`

	// KubernetesNamespace is the Kubernetes namespace for deployments.
	KubernetesNamespace string `yaml:"kubernetesNamespace"`

	// StateBackend holds the Terraform state backend configuration.
	StateBackend *AWSStateBackend `yaml:"stateBackend,omitempty"`
}

// AWSStateBackend holds AWS S3 state backend configuration.
type AWSStateBackend struct {
	// Bucket is the S3 bucket name for Terraform state.
	Bucket string `yaml:"bucket"`

	// Region is the AWS region for the state bucket.
	Region string `yaml:"region"`

	// DynamoDBTable is the DynamoDB table name for state locking.
	DynamoDBTable string `yaml:"dynamoDBTable"`
}

// AzureProviderConfig holds Azure-specific environment configuration.
type AzureProviderConfig struct {
	// SubscriptionID is the Azure subscription ID.
	SubscriptionID string `yaml:"subscriptionId"`

	// TenantID is the Azure AD tenant ID.
	TenantID string `yaml:"tenantId"`

	// ClientID is the Azure AD application (client) ID.
	ClientID string `yaml:"clientId"`

	// ResourceGroupName is the Azure resource group name.
	ResourceGroupName string `yaml:"resourceGroupName"`

	// OIDCEnabled indicates whether OIDC authentication is configured.
	OIDCEnabled bool `yaml:"oidcEnabled"`

	// AKSClusterName is the name of the AKS cluster for Kubernetes deployments.
	AKSClusterName string `yaml:"aksClusterName"`

	// KubernetesNamespace is the Kubernetes namespace for deployments.
	KubernetesNamespace string `yaml:"kubernetesNamespace"`

	// StateBackend holds the Terraform state backend configuration.
	StateBackend *AzureStateBackend `yaml:"stateBackend,omitempty"`
}

// AzureStateBackend holds Azure Storage state backend configuration.
type AzureStateBackend struct {
	// StorageAccountName is the Azure Storage account name for Terraform state.
	StorageAccountName string `yaml:"storageAccountName"`

	// ContainerName is the blob container name for Terraform state.
	ContainerName string `yaml:"containerName"`
}

// LoadEnvironment loads an Environment from a file.
func LoadEnvironment(filepath string) (*Environment, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read environment file: %w", err)
	}

	var env Environment
	if err := yaml.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("failed to parse environment file: %w", err)
	}

	return &env, nil
}

// SaveEnvironment saves an Environment to a file.
func SaveEnvironment(filepath string, env *Environment) error {
	data, err := yaml.Marshal(env)
	if err != nil {
		return fmt.Errorf("failed to marshal environment: %w", err)
	}

	if err := os.WriteFile(filepath, data, 0644); err != nil {
		return fmt.Errorf("failed to write environment file: %w", err)
	}

	return nil
}

// Validate checks that the environment configuration is valid.
func (e *Environment) Validate() error {
	if e.Name == "" {
		return fmt.Errorf("environment name is required")
	}

	switch e.Kind {
	case ProviderKindAWS:
		if e.Provider.AWS == nil {
			return fmt.Errorf("AWS provider configuration is required for kind 'aws'")
		}
	case ProviderKindAzure:
		if e.Provider.Azure == nil {
			return fmt.Errorf("Azure provider configuration is required for kind 'azure'")
		}
	default:
		return fmt.Errorf("kind must be 'aws' or 'azure', got: %s", e.Kind)
	}

	return nil
}

// IsOIDCConfigured returns true if OIDC authentication is configured for the environment.
func (e *Environment) IsOIDCConfigured() bool {
	switch e.Kind {
	case ProviderKindAWS:
		return e.Provider.AWS != nil && e.Provider.AWS.OIDCRoleARN != ""
	case ProviderKindAzure:
		return e.Provider.Azure != nil && e.Provider.Azure.OIDCEnabled
	default:
		return false
	}
}
