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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

var _ Backend = (*s3Backend)(nil)

const (
	BackendS3 = "s3"

	// S3StateKeyPrefix is the prefix used for Terraform state file keys in S3.
	S3StateKeyPrefix = "radius-tfstate/"
)

// S3BackendConfig holds configuration for the S3 backend.
type S3BackendConfig struct {
	// Bucket is the name of the S3 bucket to store Terraform state.
	Bucket string

	// Region is the AWS region where the bucket is located.
	Region string

	// DynamoDBTable is the optional DynamoDB table name for state locking.
	DynamoDBTable string

	// Encrypt enables server-side encryption of the state file.
	Encrypt bool
}

// s3Backend implements the Backend interface for AWS S3.
type s3Backend struct {
	config   S3BackendConfig
	s3Client S3Client
}

// S3Client is an interface for S3 operations used by the S3 backend.
// This interface allows for testing with mock implementations.
type S3Client interface {
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
}

// NewS3Backend creates a new S3 backend with the given configuration.
func NewS3Backend(config S3BackendConfig, client S3Client) Backend {
	return &s3Backend{
		config:   config,
		s3Client: client,
	}
}

// BuildBackend generates the Terraform backend configuration for S3 backend.
// https://developer.hashicorp.com/terraform/language/settings/backends/s3
func (b *s3Backend) BuildBackend(resourceRecipe *recipes.ResourceMetadata) (map[string]any, error) {
	stateKey, err := b.generateStateKey(resourceRecipe)
	if err != nil {
		return nil, fmt.Errorf("failed to generate S3 state key: %w", err)
	}

	backendConfig := map[string]any{
		"bucket":  b.config.Bucket,
		"key":     stateKey,
		"region":  b.config.Region,
		"encrypt": b.config.Encrypt,
	}

	// Add optional DynamoDB table for state locking
	if b.config.DynamoDBTable != "" {
		backendConfig["dynamodb_table"] = b.config.DynamoDBTable
	}

	return map[string]any{
		BackendS3: backendConfig,
	}, nil
}

// ValidateBackendExists checks if the Terraform state file exists in the S3 bucket.
// name is the state file key that was generated during terraform apply.
func (b *s3Backend) ValidateBackendExists(ctx context.Context, name string) (bool, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	_, err := b.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.config.Bucket),
		Key:    aws.String(name),
	})
	if err != nil {
		// Check if the error indicates the object doesn't exist
		// S3 returns a specific error for non-existent objects
		errStr := err.Error()
		if strings.Contains(errStr, "NotFound") || strings.Contains(errStr, "404") {
			logger.Info(fmt.Sprintf("S3 state file %q does not exist in bucket %q", name, b.config.Bucket))
			return false, nil
		}
		return false, fmt.Errorf("failed to check S3 state file: %w", err)
	}

	return true, nil
}

// generateStateKey returns a unique S3 key for the terraform state file
// based on resourceID, environmentID, and applicationID.
func (b *s3Backend) generateStateKey(resourceRecipe *recipes.ResourceMetadata) (string, error) {
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

	// Format: radius-tfstate/{env}/{app}/{hash}.tfstate
	return fmt.Sprintf("%s%s/%s/%s.tfstate", S3StateKeyPrefix, parsedEnvID.Name(), parsedAppID.Name(), hashStr), nil
}

// GetStateKeyName returns the full state key name for use with ValidateBackendExists.
// This is a helper function to get the key that was generated for a given resource.
func (b *s3Backend) GetStateKeyName(resourceRecipe *recipes.ResourceMetadata) (string, error) {
	return b.generateStateKey(resourceRecipe)
}
