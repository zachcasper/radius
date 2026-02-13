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
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// RecipeKindTerraform indicates a Terraform-based recipe.
	RecipeKindTerraform = "terraform"

	// RecipeKindBicep indicates a Bicep-based recipe.
	RecipeKindBicep = "bicep"
)

// RecipesManifest defines recipes available for resource provisioning.
// This is stored in .radius/recipes.yaml or referenced from environment files.
type RecipesManifest struct {
	Recipes map[string]RecipeEntry `yaml:"recipes"`
}

// RecipeEntry defines a single recipe reference.
type RecipeEntry struct {
	// RecipeKind specifies the recipe type: "terraform" or "bicep".
	RecipeKind string `yaml:"recipeKind"`

	// RecipeLocation is the location of the recipe.
	// For Terraform: git:: URL format
	// For Bicep: OCI registry URL (https://...)
	RecipeLocation string `yaml:"recipeLocation"`
}

// LoadRecipesManifest loads a RecipesManifest from a file.
func LoadRecipesManifest(filepath string) (*RecipesManifest, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read recipes manifest: %w", err)
	}

	var manifest RecipesManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse recipes manifest: %w", err)
	}

	return &manifest, nil
}

// SaveRecipesManifest saves a RecipesManifest to a file.
func SaveRecipesManifest(filepath string, manifest *RecipesManifest) error {
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("failed to marshal recipes manifest: %w", err)
	}

	if err := os.WriteFile(filepath, data, 0644); err != nil {
		return fmt.Errorf("failed to write recipes manifest: %w", err)
	}

	return nil
}

// Validate checks that all recipe entries are valid.
func (m *RecipesManifest) Validate() error {
	for name, entry := range m.Recipes {
		switch entry.RecipeKind {
		case RecipeKindTerraform:
			if !strings.HasPrefix(entry.RecipeLocation, "git::") {
				return fmt.Errorf("recipe %s: Terraform recipe location must start with 'git::'", name)
			}
		case RecipeKindBicep:
			if !strings.HasPrefix(entry.RecipeLocation, "https://") {
				return fmt.Errorf("recipe %s: Bicep recipe location must be an HTTPS OCI registry URL", name)
			}
		default:
			return fmt.Errorf("recipe %s: recipeKind must be 'terraform' or 'bicep', got: %s", name, entry.RecipeKind)
		}

		// Validate resource type name format (Namespace/Type)
		parts := strings.Split(name, "/")
		if len(parts) != 2 {
			return fmt.Errorf("recipe name must be in format 'Namespace/Type', got: %s", name)
		}
	}

	return nil
}
