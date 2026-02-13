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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_extractTypeBasePath(t *testing.T) {
	tests := []struct {
		name               string
		definitionLocation string
		expected           string
	}{
		{
			name:               "containers type",
			definitionLocation: "git::https://github.com/radius-project/resource-types-contrib.git//Compute/containers/container.yaml",
			expected:           "Compute/containers",
		},
		{
			name:               "postgreSqlDatabases type",
			definitionLocation: "git::https://github.com/radius-project/resource-types-contrib.git//Data/postgreSqlDatabases/postgreSqlDatabases.yaml",
			expected:           "Data/postgreSqlDatabases",
		},
		{
			name:               "applications type",
			definitionLocation: "git::https://github.com/radius-project/resource-types-contrib.git//Core/applications/applications.yaml",
			expected:           "Core/applications",
		},
		{
			name:               "empty string",
			definitionLocation: "",
			expected:           "",
		},
		{
			name:               "no git separator",
			definitionLocation: "file:///path/to/file.yaml",
			expected:           "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTypeBasePath(tt.definitionLocation)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func Test_extractTypeShortName(t *testing.T) {
	tests := []struct {
		name     string
		typeName string
		expected string
	}{
		{
			name:     "containers",
			typeName: "Radius.Compute/containers",
			expected: "containers",
		},
		{
			name:     "postgreSqlDatabases",
			typeName: "Radius.Data/postgreSqlDatabases",
			expected: "postgreSqlDatabases",
		},
		{
			name:     "no slash",
			typeName: "containers",
			expected: "containers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTypeShortName(tt.typeName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func Test_RecipesManifestFromTypes_Terraform_AWS(t *testing.T) {
	types := DefaultTypesManifest()
	recipes := RecipesManifestFromTypes(types, "aws", "terraform")

	require.NotNil(t, recipes)
	require.NotNil(t, recipes.Recipes)

	// Should have recipes for types with recipe directories (not Radius.Core/applications)
	assert.NotContains(t, recipes.Recipes, "Radius.Core/applications")

	// Check containers recipe
	containersRecipe, ok := recipes.Recipes["Radius.Compute/containers"]
	require.True(t, ok, "should have containers recipe")
	assert.Equal(t, RecipeKindTerraform, containersRecipe.RecipeKind)
	assert.Contains(t, containersRecipe.RecipeLocation, "Compute/containers/recipes/aws/terraform")
	// Note: Version tags are a future enhancement - verify no version tag present
	assert.NotContains(t, containersRecipe.RecipeLocation, "?ref=")

	// Check postgreSqlDatabases recipe
	pgRecipe, ok := recipes.Recipes["Radius.Data/postgreSqlDatabases"]
	require.True(t, ok, "should have postgreSqlDatabases recipe")
	assert.Equal(t, RecipeKindTerraform, pgRecipe.RecipeKind)
	assert.Contains(t, pgRecipe.RecipeLocation, "Data/postgreSqlDatabases/recipes/aws/terraform")
}

func Test_RecipesManifestFromTypes_Terraform_Azure(t *testing.T) {
	types := DefaultTypesManifest()
	recipes := RecipesManifestFromTypes(types, "azure", "terraform")

	require.NotNil(t, recipes)

	// Check containers recipe points to azure
	containersRecipe, ok := recipes.Recipes["Radius.Compute/containers"]
	require.True(t, ok)
	assert.Contains(t, containersRecipe.RecipeLocation, "recipes/azure/terraform")
}

func Test_RecipesManifestFromTypes_Bicep_Azure(t *testing.T) {
	types := DefaultTypesManifest()
	recipes := RecipesManifestFromTypes(types, "azure", "bicep")

	require.NotNil(t, recipes)

	// Check containers recipe uses OCI format
	containersRecipe, ok := recipes.Recipes["Radius.Compute/containers"]
	require.True(t, ok)
	assert.Equal(t, RecipeKindBicep, containersRecipe.RecipeKind)
	assert.Contains(t, containersRecipe.RecipeLocation, "ghcr.io/radius-project/recipes/azure/containers")
}

func Test_DefaultRecipesManifest_UsesFR009A(t *testing.T) {
	// DefaultRecipesManifest should now use RecipesManifestFromTypes internally
	recipes := DefaultRecipesManifest("aws", "terraform")

	require.NotNil(t, recipes)
	require.NotNil(t, recipes.Recipes)

	// Should have recipes matching types from DefaultTypesManifest (excluding metadata-only types)
	// At minimum, should have containers
	_, hasContainers := recipes.Recipes["Radius.Compute/containers"]
	assert.True(t, hasContainers, "should have containers recipe")

	// Should NOT have the old Applications.Datastores types
	_, hasOldMongo := recipes.Recipes["Applications.Datastores/mongoDatabases"]
	assert.False(t, hasOldMongo, "should not have old Applications.Datastores types")
}
