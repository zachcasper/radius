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

// ResourceTypesManifest defines resource types available in the repository.
// This is stored in .radius/types.yaml.
type ResourceTypesManifest struct {
	Types map[string]ResourceTypeEntry `yaml:"types"`
}

// ResourceTypeEntry defines a single resource type reference.
type ResourceTypeEntry struct {
	// DefinitionLocation is the git URL to the resource type definition.
	// Format: git::https://github.com/<owner>/<repo>.git//<path>?ref=<version>
	DefinitionLocation string `yaml:"definitionLocation"`
}

// LoadResourceTypesManifest loads a ResourceTypesManifest from a file.
func LoadResourceTypesManifest(filepath string) (*ResourceTypesManifest, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read resource types manifest: %w", err)
	}

	var manifest ResourceTypesManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse resource types manifest: %w", err)
	}

	return &manifest, nil
}

// SaveResourceTypesManifest saves a ResourceTypesManifest to a file.
func SaveResourceTypesManifest(filepath string, manifest *ResourceTypesManifest) error {
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("failed to marshal resource types manifest: %w", err)
	}

	if err := os.WriteFile(filepath, data, 0644); err != nil {
		return fmt.Errorf("failed to write resource types manifest: %w", err)
	}

	return nil
}

// Validate checks that all resource type entries are valid.
func (m *ResourceTypesManifest) Validate() error {
	for name, entry := range m.Types {
		if !strings.HasPrefix(entry.DefinitionLocation, "git::") {
			return fmt.Errorf("resource type %s: definitionLocation must start with 'git::'", name)
		}

		if !strings.Contains(entry.DefinitionLocation, "?ref=") {
			return fmt.Errorf("resource type %s: definitionLocation must include version ref (?ref=)", name)
		}

		// Validate resource type name format (Namespace/Type)
		parts := strings.Split(name, "/")
		if len(parts) != 2 {
			return fmt.Errorf("resource type name must be in format 'Namespace/Type', got: %s", name)
		}
	}

	return nil
}
