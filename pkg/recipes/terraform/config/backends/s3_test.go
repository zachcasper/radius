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

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/stretchr/testify/require"
)

type mockS3Client struct {
	existingKeys map[string]bool
	returnError  error
}

func (m *mockS3Client) HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	if m.returnError != nil {
		return nil, m.returnError
	}

	key := *params.Key
	if m.existingKeys[key] {
		return &s3.HeadObjectOutput{}, nil
	}

	return nil, errors.New("NotFound: The specified key does not exist")
}

func getS3TestInputs() recipes.ResourceMetadata {
	return recipes.ResourceMetadata{
		Name:          "test-recipe",
		Parameters:    map[string]any{},
		EnvironmentID: "/planes/radius/local/resourceGroups/test-group/providers/Applications.Environments/testEnv/env",
		ApplicationID: "/planes/radius/local/resourceGroups/test-group/providers/Applications.Applications/testApp/app",
		ResourceID:    "/planes/radius/local/resourceGroups/test-group/providers/Applications.Datastores/redisCaches/redis",
	}
}

func Test_S3Backend_BuildBackend(t *testing.T) {
	t.Parallel()

	resourceRecipe := getS3TestInputs()
	config := S3BackendConfig{
		Bucket:  "my-bucket",
		Region:  "us-west-2",
		Encrypt: true,
	}

	backend := NewS3Backend(config, &mockS3Client{})
	result, err := backend.BuildBackend(&resourceRecipe)
	require.NoError(t, err)
	require.NotNil(t, result)

	s3Config, ok := result[BackendS3].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "my-bucket", s3Config["bucket"])
	require.Equal(t, "us-west-2", s3Config["region"])
	require.Equal(t, true, s3Config["encrypt"])
	require.Contains(t, s3Config["key"], S3StateKeyPrefix)
}

func Test_S3Backend_BuildBackend_WithDynamoDB(t *testing.T) {
	t.Parallel()

	resourceRecipe := getS3TestInputs()
	config := S3BackendConfig{
		Bucket:        "my-bucket",
		Region:        "us-west-2",
		DynamoDBTable: "terraform-locks",
		Encrypt:       true,
	}

	backend := NewS3Backend(config, &mockS3Client{})
	result, err := backend.BuildBackend(&resourceRecipe)
	require.NoError(t, err)
	require.NotNil(t, result)

	s3Config, ok := result[BackendS3].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "terraform-locks", s3Config["dynamodb_table"])
}

func Test_S3Backend_BuildBackend_InvalidResourceID(t *testing.T) {
	t.Parallel()

	resourceRecipe := getS3TestInputs()
	resourceRecipe.ResourceID = "invalid"

	config := S3BackendConfig{
		Bucket: "my-bucket",
		Region: "us-west-2",
	}

	backend := NewS3Backend(config, &mockS3Client{})
	_, err := backend.BuildBackend(&resourceRecipe)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to generate S3 state key")
}

func Test_S3Backend_BuildBackend_InvalidEnvID(t *testing.T) {
	t.Parallel()

	resourceRecipe := getS3TestInputs()
	resourceRecipe.EnvironmentID = "invalid"

	config := S3BackendConfig{
		Bucket: "my-bucket",
		Region: "us-west-2",
	}

	backend := NewS3Backend(config, &mockS3Client{})
	_, err := backend.BuildBackend(&resourceRecipe)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to generate S3 state key")
}

func Test_S3Backend_BuildBackend_InvalidAppID(t *testing.T) {
	t.Parallel()

	resourceRecipe := getS3TestInputs()
	resourceRecipe.ApplicationID = "invalid"

	config := S3BackendConfig{
		Bucket: "my-bucket",
		Region: "us-west-2",
	}

	backend := NewS3Backend(config, &mockS3Client{})
	_, err := backend.BuildBackend(&resourceRecipe)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to generate S3 state key")
}

func Test_S3Backend_ValidateBackendExists_Found(t *testing.T) {
	t.Parallel()

	resourceRecipe := getS3TestInputs()

	// Generate expected key
	hasher := sha1.New()
	inputString := strings.ToLower(fmt.Sprintf("%s-%s-%s", "env", "app", resourceRecipe.ResourceID))
	_, err := hasher.Write([]byte(inputString))
	require.NoError(t, err)
	hashStr := fmt.Sprintf("%x", hasher.Sum(nil))
	expectedKey := fmt.Sprintf("%senv/app/%s.tfstate", S3StateKeyPrefix, hashStr)

	mockClient := &mockS3Client{
		existingKeys: map[string]bool{
			expectedKey: true,
		},
	}

	config := S3BackendConfig{
		Bucket: "my-bucket",
		Region: "us-west-2",
	}

	backend := NewS3Backend(config, mockClient)
	exists, err := backend.ValidateBackendExists(context.Background(), expectedKey)
	require.NoError(t, err)
	require.True(t, exists)
}

func Test_S3Backend_ValidateBackendExists_NotFound(t *testing.T) {
	t.Parallel()

	mockClient := &mockS3Client{
		existingKeys: map[string]bool{},
	}

	config := S3BackendConfig{
		Bucket: "my-bucket",
		Region: "us-west-2",
	}

	backend := NewS3Backend(config, mockClient)
	exists, err := backend.ValidateBackendExists(context.Background(), "nonexistent-key")
	require.NoError(t, err)
	require.False(t, exists)
}

func Test_S3Backend_ValidateBackendExists_Error(t *testing.T) {
	t.Parallel()

	mockClient := &mockS3Client{
		returnError: errors.New("access denied"),
	}

	config := S3BackendConfig{
		Bucket: "my-bucket",
		Region: "us-west-2",
	}

	backend := NewS3Backend(config, mockClient)
	_, err := backend.ValidateBackendExists(context.Background(), "some-key")
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to check S3 state file")
}

func Test_S3Backend_GenerateStateKey(t *testing.T) {
	t.Parallel()

	resourceRecipe := getS3TestInputs()
	config := S3BackendConfig{
		Bucket: "my-bucket",
		Region: "us-west-2",
	}

	backend := NewS3Backend(config, &mockS3Client{}).(*s3Backend)
	key, err := backend.generateStateKey(&resourceRecipe)
	require.NoError(t, err)

	// Verify key format
	require.True(t, strings.HasPrefix(key, S3StateKeyPrefix))
	require.Contains(t, key, "env/")
	require.Contains(t, key, "app/")
	require.True(t, strings.HasSuffix(key, ".tfstate"))

	// Verify same inputs produce same key
	key2, err := backend.generateStateKey(&resourceRecipe)
	require.NoError(t, err)
	require.Equal(t, key, key2)
}
