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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

// ResourceTypeDefinition represents a resource type definition loaded from config.
type ResourceTypeDefinition struct {
	// Type is the fully qualified resource type (e.g., "Radius.Compute/containers").
	Type string `yaml:"type" json:"type"`

	// Description is a human-readable description of the resource type.
	Description string `yaml:"description" json:"description"`

	// Schema contains the JSON schema for the resource properties.
	Schema *ResourceSchema `yaml:"schema" json:"schema"`
}

// ResourceSchema contains the JSON schema for resource properties.
type ResourceSchema struct {
	// Type is the JSON schema type (typically "object").
	Type string `yaml:"type" json:"type"`

	// Properties contains the property definitions.
	Properties map[string]*PropertySchema `yaml:"properties" json:"properties"`

	// Required lists the required property names.
	Required []string `yaml:"required" json:"required"`
}

// PropertySchema contains the schema for a single property.
type PropertySchema struct {
	// Type is the property type (string, number, boolean, object, array).
	Type string `yaml:"type" json:"type"`

	// Description is a human-readable description.
	Description string `yaml:"description" json:"description"`

	// Properties contains nested property definitions for objects.
	Properties map[string]*PropertySchema `yaml:"properties" json:"properties"`

	// Items contains the schema for array items.
	Items *PropertySchema `yaml:"items" json:"items"`

	// Required lists required nested properties.
	Required []string `yaml:"required" json:"required"`

	// Enum lists allowed values for the property.
	Enum []any `yaml:"enum" json:"enum"`

	// Default is the default value.
	Default any `yaml:"default" json:"default"`
}

// ResourceTypeRegistry holds loaded resource type definitions.
type ResourceTypeRegistry struct {
	// Types maps resource type names to their definitions.
	Types map[string]*ResourceTypeDefinition
}

// NewResourceTypeRegistry creates a new empty registry.
func NewResourceTypeRegistry() *ResourceTypeRegistry {
	return &ResourceTypeRegistry{
		Types: make(map[string]*ResourceTypeDefinition),
	}
}

// LoadResourceTypes loads resource type definitions from a directory.
func LoadResourceTypes(configDir string) (*ResourceTypeRegistry, error) {
	registry := NewResourceTypeRegistry()

	typesDir := filepath.Join(configDir, "types")
	if _, err := os.Stat(typesDir); os.IsNotExist(err) {
		return registry, nil // No types directory, return empty registry
	}

	entries, err := os.ReadDir(typesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read types directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml")) {
			continue
		}

		filePath := filepath.Join(typesDir, entry.Name())
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read type file %s: %w", filePath, err)
		}

		var typeDef ResourceTypeDefinition
		if err := yaml.Unmarshal(content, &typeDef); err != nil {
			return nil, fmt.Errorf("failed to parse type file %s: %w", filePath, err)
		}

		if typeDef.Type != "" {
			registry.Types[typeDef.Type] = &typeDef
		}
	}

	return registry, nil
}

// GetType returns the definition for a resource type.
func (r *ResourceTypeRegistry) GetType(resourceType string) *ResourceTypeDefinition {
	return r.Types[resourceType]
}

// ValidateResourceProperties validates resource properties against the schema.
func (r *ResourceTypeRegistry) ValidateResourceProperties(resourceType string, properties map[string]any) []ValidationError {
	typeDef := r.GetType(resourceType)
	if typeDef == nil || typeDef.Schema == nil {
		return nil // No schema to validate against
	}

	return validateAgainstSchema("properties", properties, typeDef.Schema)
}

// ValidationError represents a validation error.
type ValidationError struct {
	// Path is the JSON path to the invalid property.
	Path string

	// Message is the error message.
	Message string

	// Suggestion is a suggested fix, if available.
	Suggestion string
}

// Error implements the error interface.
func (e ValidationError) Error() string {
	if e.Suggestion != "" {
		return fmt.Sprintf("%s: %s (suggestion: %s)", e.Path, e.Message, e.Suggestion)
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// validateAgainstSchema validates a value against a schema.
func validateAgainstSchema(path string, value any, schema *ResourceSchema) []ValidationError {
	var errors []ValidationError

	if schema == nil {
		return errors
	}

	// Check if value is a map
	valueMap, ok := value.(map[string]any)
	if !ok {
		// Check for syntax errors
		if strVal, isStr := value.(string); isStr && (strVal == "[" || strings.HasPrefix(strVal, "[")) {
			errors = append(errors, ValidationError{
				Path:       path,
				Message:    "invalid syntax - found '[' (check your Bicep braces: use { } for objects/maps, not [ ])",
				Suggestion: "Use { } instead of [ ] for map/object values",
			})
		}
		return errors
	}

	// Check for required properties
	for _, required := range schema.Required {
		if _, ok := valueMap[required]; !ok {
			// Find similar property names for suggestion
			suggestion := findSimilarProperty(required, valueMap)
			errors = append(errors, ValidationError{
				Path:       path + "." + required,
				Message:    "required property is missing",
				Suggestion: suggestion,
			})
		}
	}

	// Check for unknown properties and validate nested properties
	for propName, propValue := range valueMap {
		propPath := path + "." + propName

		propSchema, ok := schema.Properties[propName]
		if !ok {
			// Find similar property names
			suggestion := findSimilarPropertyInSchema(propName, schema.Properties)
			errors = append(errors, ValidationError{
				Path:       propPath,
				Message:    fmt.Sprintf("unknown property '%s'", propName),
				Suggestion: suggestion,
			})
			continue
		}

		// Validate nested objects
		if propSchema.Type == "object" && propSchema.Properties != nil {
			nestedSchema := &ResourceSchema{
				Type:       "object",
				Properties: propSchema.Properties,
				Required:   propSchema.Required,
			}
			errors = append(errors, validateAgainstSchema(propPath, propValue, nestedSchema)...)
		}

		// Check type mismatches
		errors = append(errors, validatePropertyType(propPath, propValue, propSchema)...)
	}

	return errors
}

// validatePropertyType validates that a value matches the expected type.
func validatePropertyType(path string, value any, schema *PropertySchema) []ValidationError {
	var errors []ValidationError

	if schema == nil {
		return errors
	}

	switch schema.Type {
	case "string":
		if _, ok := value.(string); !ok && value != nil {
			errors = append(errors, ValidationError{
				Path:    path,
				Message: fmt.Sprintf("expected string, got %T", value),
			})
		}
	case "number", "integer":
		switch value.(type) {
		case int, int32, int64, float32, float64, json.Number:
			// OK
		default:
			if value != nil {
				errors = append(errors, ValidationError{
					Path:    path,
					Message: fmt.Sprintf("expected number, got %T", value),
				})
			}
		}
	case "boolean":
		if _, ok := value.(bool); !ok && value != nil {
			errors = append(errors, ValidationError{
				Path:    path,
				Message: fmt.Sprintf("expected boolean, got %T", value),
			})
		}
	case "array":
		if _, ok := value.([]any); !ok && value != nil {
			errors = append(errors, ValidationError{
				Path:    path,
				Message: fmt.Sprintf("expected array, got %T", value),
			})
		}
	case "object":
		if _, ok := value.(map[string]any); !ok && value != nil {
			// Check for common syntax error
			if strVal, isStr := value.(string); isStr && strings.HasPrefix(strVal, "[") {
				errors = append(errors, ValidationError{
					Path:       path,
					Message:    "expected object but found array syntax",
					Suggestion: "Use { } for objects/maps, not [ ]",
				})
			} else {
				errors = append(errors, ValidationError{
					Path:    path,
					Message: fmt.Sprintf("expected object, got %T", value),
				})
			}
		}
	}

	return errors
}

// findSimilarProperty finds a similar property name in a map.
func findSimilarProperty(target string, props map[string]any) string {
	for propName := range props {
		if isSimilar(target, propName) {
			return fmt.Sprintf("did you mean '%s'?", propName)
		}
	}
	return ""
}

// findSimilarPropertyInSchema finds a similar property name in schema properties.
func findSimilarPropertyInSchema(target string, props map[string]*PropertySchema) string {
	for propName := range props {
		if isSimilar(target, propName) {
			return fmt.Sprintf("did you mean '%s'?", propName)
		}
	}
	return ""
}

// isSimilar checks if two strings are similar (simple Levenshtein-like check).
func isSimilar(a, b string) bool {
	a = strings.ToLower(a)
	b = strings.ToLower(b)

	// Check for common variations
	if a == b+"s" || b == a+"s" { // plural/singular
		return true
	}
	if strings.Contains(a, b) || strings.Contains(b, a) {
		return true
	}

	// Simple edit distance check (within 2 edits)
	if len(a) > 0 && len(b) > 0 {
		if a[0] == b[0] && len(a) == len(b) {
			diffs := 0
			for i := 0; i < len(a); i++ {
				if a[i] != b[i] {
					diffs++
				}
			}
			return diffs <= 2
		}
	}

	return false
}
