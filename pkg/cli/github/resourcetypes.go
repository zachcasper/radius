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
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// ResourceTypesRepo is the GitHub repository containing community resource types.
	ResourceTypesRepo = "radius-project/resource-types-contrib"

	// ResourceTypesDefaultBranch is the default branch for the resource types repo.
	ResourceTypesDefaultBranch = "main"
)

// ResourceTypeNamespaces are the top-level directories in the resource-types-contrib repo.
var ResourceTypeNamespaces = []string{"Compute", "Data", "Security"}

// ResourceType represents a Radius resource type from the contrib repository.
type ResourceType struct {
	// Type is the fully qualified resource type name (e.g., "Radius.Data/mySqlDatabases").
	Type string `json:"type" yaml:"type"`

	// Name is the short name of the resource type.
	Name string `json:"name" yaml:"name"`

	// Description is a human-readable description of the resource type.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Provider indicates the cloud provider compatibility (aws, azure, or both).
	Provider string `json:"provider,omitempty" yaml:"provider,omitempty"`

	// SchemaPath is the relative path to the JSON schema file.
	SchemaPath string `json:"schemaPath,omitempty" yaml:"schemaPath,omitempty"`
}

// resourceTypeFile represents the actual YAML structure in resource-types-contrib.
// Example:
//
//	namespace: Radius.Data
//	types:
//	  mySqlDatabases:
//	    description: |
//	      ...
type resourceTypeFile struct {
	Namespace string                       `yaml:"namespace"`
	Types     map[string]resourceTypeEntry `yaml:"types"`
}

// resourceTypeEntry represents a single type entry in the types map.
type resourceTypeEntry struct {
	Description string `yaml:"description"`
}

// ResourceTypeFetcher fetches resource type definitions from the contrib repository.
type ResourceTypeFetcher struct {
	// ghClient is the GitHub CLI client for API operations.
	ghClient *Client
}

// NewResourceTypeFetcher creates a new resource type fetcher.
func NewResourceTypeFetcher(client *Client) *ResourceTypeFetcher {
	return &ResourceTypeFetcher{
		ghClient: client,
	}
}

// FetchResourceTypes fetches resource type definitions from the contrib repository.
// It uses sparse checkout to efficiently download only the types directory.
func (f *ResourceTypeFetcher) FetchResourceTypes(ctx context.Context, targetDir string) ([]ResourceType, error) {
	// Create a temporary directory for the sparse checkout
	tmpDir, err := os.MkdirTemp("", "radius-types-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Clone with sparse checkout
	err = f.sparseClone(ctx, tmpDir)
	if err != nil {
		return nil, fmt.Errorf("failed to clone resource types: %w", err)
	}

	// Parse the resource types from the cloned directory
	types, err := f.parseResourceTypes(tmpDir)
	if err != nil {
		return nil, fmt.Errorf("failed to parse resource types: %w", err)
	}

	// Copy relevant schema files to target directory if provided
	if targetDir != "" {
		schemasDir := filepath.Join(targetDir, "schemas")
		if err := os.MkdirAll(schemasDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create schemas directory: %w", err)
		}
		if err := f.copySchemas(tmpDir, schemasDir, types); err != nil {
			return nil, fmt.Errorf("failed to copy schemas: %w", err)
		}
	}

	return types, nil
}

// sparseClone performs a sparse checkout of the resource types repository.
func (f *ResourceTypeFetcher) sparseClone(ctx context.Context, targetDir string) error {
	// Initialize git repo with sparse checkout (use -q to suppress hints)
	initCommands := [][]string{
		{"git", "init", "-q"},
		{"git", "sparse-checkout", "init", "--cone"},
	}

	for _, args := range initCommands {
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.Dir = targetDir
		// Don't output stderr for init commands to suppress hints
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("command %q failed: %w", strings.Join(args, " "), err)
		}
	}

	// Set sparse checkout for namespace directories (Compute, Data, Security)
	sparseArgs := append([]string{"sparse-checkout", "set"}, ResourceTypeNamespaces...)
	cmd := exec.CommandContext(ctx, "git", sparseArgs...)
	cmd.Dir = targetDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command %q failed: %w", "git "+strings.Join(sparseArgs, " "), err)
	}

	// Fetch and checkout (use -q to suppress progress output)
	fetchCommands := [][]string{
		{"git", "remote", "add", "origin", fmt.Sprintf("https://github.com/%s.git", ResourceTypesRepo)},
		{"git", "fetch", "-q", "--depth=1", "origin", ResourceTypesDefaultBranch},
		{"git", "checkout", "-q", ResourceTypesDefaultBranch},
	}

	for _, args := range fetchCommands {
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.Dir = targetDir
		// Don't output stderr to suppress progress messages
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("command %q failed: %w", strings.Join(args, " "), err)
		}
	}

	return nil
}

// parseResourceTypes reads and parses resource type definitions from namespace directories.
// The repo structure is: <Namespace>/<resourceType>/<resourceType>.yaml
// YAML format:
//
//	namespace: Radius.Data
//	types:
//	  mySqlDatabases:
//	    description: |
//	      ...
func (f *ResourceTypeFetcher) parseResourceTypes(repoDir string) ([]ResourceType, error) {
	var types []ResourceType
	foundAny := false

	// Walk each namespace directory
	for _, namespace := range ResourceTypeNamespaces {
		namespaceDir := filepath.Join(repoDir, namespace)

		// Skip if namespace directory doesn't exist
		if _, err := os.Stat(namespaceDir); os.IsNotExist(err) {
			continue
		}
		foundAny = true

		// Walk the namespace directory
		err := filepath.Walk(namespaceDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Look for YAML files (excluding recipes directories)
			if !info.IsDir() && strings.HasSuffix(info.Name(), ".yaml") {
				// Skip recipe files (inside recipes/ directories)
				if strings.Contains(path, "/recipes/") {
					return nil
				}

				// Skip non-resource-type YAML files
				if info.Name() == "bicep.yaml" || info.Name() == "terraform.yaml" {
					return nil
				}

				data, err := os.ReadFile(path)
				if err != nil {
					return nil // Skip files we can't read
				}

				// Try parsing as resourceTypeFile format (namespace + types map)
				var rtFile resourceTypeFile
				if err := yaml.Unmarshal(data, &rtFile); err == nil && rtFile.Namespace != "" && len(rtFile.Types) > 0 {
					// Parse the types map
					for typeName, entry := range rtFile.Types {
						rt := ResourceType{
							Type:        fmt.Sprintf("%s/%s", rtFile.Namespace, typeName),
							Name:        typeName,
							Description: entry.Description,
						}
						types = append(types, rt)
					}
					return nil
				}

				// Fallback: try legacy format (direct type/name fields)
				var resourceType ResourceType
				if err := yaml.Unmarshal(data, &resourceType); err == nil {
					if resourceType.Type != "" {
						types = append(types, resourceType)
					} else if resourceType.Name != "" {
						resourceType.Type = fmt.Sprintf("Radius.%s/%s", namespace, resourceType.Name)
						types = append(types, resourceType)
					}
				}
			}

			return nil
		})

		if err != nil {
			return nil, err
		}
	}

	if !foundAny {
		return nil, fmt.Errorf("no resource type namespaces found in repository")
	}

	return types, nil
}

// copySchemas copies schema files for the discovered resource types.
func (f *ResourceTypeFetcher) copySchemas(repoDir, targetDir string, types []ResourceType) error {
	for _, rt := range types {
		if rt.SchemaPath == "" {
			continue
		}

		srcPath := filepath.Join(repoDir, rt.SchemaPath)
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			continue // Schema file doesn't exist
		}

		// Read schema file
		data, err := os.ReadFile(srcPath)
		if err != nil {
			continue // Skip files we can't read
		}

		// Create destination filename based on resource type
		destName := strings.ReplaceAll(rt.Type, "/", "-") + ".json"
		destPath := filepath.Join(targetDir, destName)

		if err := os.WriteFile(destPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write schema for %s: %w", rt.Type, err)
		}
	}

	return nil
}

// FilterByProvider filters resource types by cloud provider.
func FilterByProvider(types []ResourceType, provider string) []ResourceType {
	if provider == "" {
		return types
	}

	var filtered []ResourceType
	for _, rt := range types {
		// Include types that match the provider or have no provider specified (core types)
		if rt.Provider == "" || rt.Provider == provider || rt.Provider == "both" {
			filtered = append(filtered, rt)
		}
	}
	return filtered
}
