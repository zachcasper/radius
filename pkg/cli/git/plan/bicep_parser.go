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

package plan

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// BicepModel represents a parsed Bicep model containing resources and their relationships.
type BicepModel struct {
	// FilePath is the path to the Bicep file.
	FilePath string

	// Resources contains the parsed resources indexed by symbolic name.
	Resources map[string]*BicepResource

	// Parameters contains the model parameters.
	Parameters map[string]*BicepParameter

	// Variables contains the model variables.
	Variables map[string]string
}

// BicepResource represents a resource defined in a Bicep model.
type BicepResource struct {
	// SymbolicName is the name used to reference this resource in Bicep.
	SymbolicName string

	// Type is the fully qualified resource type (e.g., "Radius.Compute/containers").
	Type string

	// Name is the resource name from properties.
	Name string

	// Properties contains the resource properties.
	Properties map[string]any

	// Connections contains the connected resources, resolved from symbolic references.
	Connections map[string]*ConnectedResource

	// Recipe contains recipe configuration if the resource uses a recipe.
	Recipe *BicepRecipe

	// DependsOn contains the symbolic names of resources this resource depends on.
	DependsOn []string

	// RawContent contains the raw Bicep content for this resource.
	RawContent string

	// StartLine is the starting line number in the Bicep file.
	StartLine int

	// EndLine is the ending line number in the Bicep file.
	EndLine int
}

// BicepRecipe represents recipe configuration for a resource.
type BicepRecipe struct {
	// Name is the recipe name.
	Name string

	// Kind is the recipe kind (terraform, bicep).
	Kind string

	// Source is the recipe source (e.g., OCI registry path).
	Source string

	// Parameters contains the recipe parameters.
	Parameters map[string]any
}

// BicepParameter represents a parameter in a Bicep model.
type BicepParameter struct {
	// Name is the parameter name.
	Name string

	// Type is the parameter type.
	Type string

	// DefaultValue is the default value, if specified.
	DefaultValue any

	// Description is the parameter description.
	Description string
}

// ParseBicepFile parses a Bicep file and returns a BicepModel.
func ParseBicepFile(filePath string) (*BicepModel, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read Bicep file: %w", err)
	}

	return ParseBicepContent(filePath, string(content))
}

// ParseBicepContent parses Bicep content and returns a BicepModel.
func ParseBicepContent(filePath, content string) (*BicepModel, error) {
	model := &BicepModel{
		FilePath:   filePath,
		Resources:  make(map[string]*BicepResource),
		Parameters: make(map[string]*BicepParameter),
		Variables:  make(map[string]string),
	}

	lines := strings.Split(content, "\n")
	
	// Parse resources
	resourcePattern := regexp.MustCompile(`^resource\s+(\w+)\s+'([^']+)'`)
	
	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Check for resource declaration
		if matches := resourcePattern.FindStringSubmatch(trimmed); matches != nil {
			symbolicName := matches[1]
			resourceType := matches[2]
			
			resource, endLine, err := parseResourceBlock(lines, i, symbolicName, resourceType)
			if err != nil {
				return nil, fmt.Errorf("failed to parse resource %s: %w", symbolicName, err)
			}
			
			model.Resources[symbolicName] = resource
			i = endLine + 1
			continue
		}

		// Check for parameter declaration
		if strings.HasPrefix(trimmed, "param ") {
			param := parseParameter(trimmed)
			if param != nil {
				model.Parameters[param.Name] = param
			}
		}

		// Check for variable declaration
		if strings.HasPrefix(trimmed, "var ") {
			name, value := parseVariable(trimmed)
			if name != "" {
				model.Variables[name] = value
			}
		}

		i++
	}

	return model, nil
}

// parseResourceBlock parses a resource block starting at the given line.
func parseResourceBlock(lines []string, startLine int, symbolicName, resourceType string) (*BicepResource, int, error) {
	resource := &BicepResource{
		SymbolicName: symbolicName,
		Type:         resourceType,
		Properties:   make(map[string]any),
		Connections:  make(map[string]*ConnectedResource),
		DependsOn:    []string{},
		StartLine:    startLine + 1, // 1-indexed
	}

	// Find the opening brace and collect content
	braceCount := 0
	foundOpen := false
	var contentLines []string
	endLine := startLine

	for i := startLine; i < len(lines); i++ {
		line := lines[i]
		contentLines = append(contentLines, line)

		for _, ch := range line {
			if ch == '{' {
				braceCount++
				foundOpen = true
			} else if ch == '}' {
				braceCount--
			}
		}

		if foundOpen && braceCount == 0 {
			endLine = i
			break
		}
	}

	resource.EndLine = endLine + 1 // 1-indexed
	resource.RawContent = strings.Join(contentLines, "\n")

	// Parse properties from the content
	parseResourceProperties(resource, contentLines)

	return resource, endLine, nil
}

// parseResourceProperties extracts properties from resource content lines.
func parseResourceProperties(resource *BicepResource, lines []string) {
	content := strings.Join(lines, "\n")

	// Extract name property
	namePattern := regexp.MustCompile(`name:\s*'([^']+)'`)
	if matches := namePattern.FindStringSubmatch(content); matches != nil {
		resource.Name = matches[1]
	}

	// Extract properties block
	propsPattern := regexp.MustCompile(`(?s)properties:\s*\{([^}]*(?:\{[^}]*\}[^}]*)*)\}`)
	if matches := propsPattern.FindStringSubmatch(content); matches != nil {
		resource.Properties = parsePropertiesBlock(matches[1])
	}

	// Extract connections block (may be inside properties or at root level)
	connPattern := regexp.MustCompile(`(?s)connections:\s*\{([^}]*(?:\{[^}]*\}[^}]*)*)\}`)
	if matches := connPattern.FindStringSubmatch(content); matches != nil {
		resource.Connections = parseConnectionsBlock(matches[1], resource.SymbolicName)
		// Also add connections to properties map for plan.yaml output
		if resource.Properties != nil {
			connectionsMap := make(map[string]any)
			for name, conn := range resource.Connections {
				// Reconstruct the symbolic reference (e.g., "db.id")
				connectionsMap[name] = map[string]any{
					"source": conn.Name + ".id",
				}
			}
			resource.Properties["connections"] = connectionsMap
		}
	}

	// Extract recipe configuration
	recipePattern := regexp.MustCompile(`recipe:\s*\{[^}]*name:\s*'([^']+)'`)
	if matches := recipePattern.FindStringSubmatch(content); matches != nil {
		resource.Recipe = &BicepRecipe{
			Name: matches[1],
		}
	}
}

// parsePropertiesBlock parses a properties block into a map, handling nested objects.
func parsePropertiesBlock(content string) map[string]any {
	props := make(map[string]any)
	lines := strings.Split(content, "\n")
	parsePropertiesRecursive(lines, props, 0)
	return props
}

// parsePropertiesRecursive recursively parses property lines into a map.
// Returns the number of lines consumed.
func parsePropertiesRecursive(lines []string, props map[string]any, startIdx int) int {
	i := startIdx
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "//") {
			i++
			continue
		}

		// Check for closing brace - end of this object
		if line == "}" || strings.HasPrefix(line, "}") {
			return i - startIdx + 1
		}

		// Parse key: value pairs
		colonIdx := strings.Index(line, ":")
		if colonIdx <= 0 {
			i++
			continue
		}

		key := strings.TrimSpace(line[:colonIdx])
		value := strings.TrimSpace(line[colonIdx+1:])

		// Remove trailing comma
		value = strings.TrimSuffix(value, ",")

		// Check if value is a nested object (starts with {)
		if value == "{" || strings.HasPrefix(value, "{") {
			// Find the matching closing brace
			nestedProps := make(map[string]any)
			consumed := parseNestedObject(lines, nestedProps, i+1)
			props[key] = nestedProps
			i += consumed + 1
			continue
		}

		// Handle inline object like: web: { containerPort: 3000 }
		if strings.HasPrefix(value, "{") && strings.HasSuffix(value, "}") {
			// Parse inline object
			innerContent := strings.TrimPrefix(value, "{")
			innerContent = strings.TrimSuffix(innerContent, "}")
			nestedProps := parseInlineObject(innerContent)
			props[key] = nestedProps
			i++
			continue
		}

		// Simple value - remove quotes
		if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
			value = value[1 : len(value)-1]
		}

		// Try to parse as number
		if numVal, err := parseNumber(value); err == nil {
			props[key] = numVal
		} else if key != "" && value != "" {
			props[key] = value
		}

		i++
	}

	return i - startIdx
}

// parseNestedObject parses a nested object starting after the opening brace.
func parseNestedObject(lines []string, props map[string]any, startIdx int) int {
	braceCount := 1
	i := startIdx

	for i < len(lines) && braceCount > 0 {
		line := strings.TrimSpace(lines[i])

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "//") {
			i++
			continue
		}

		// Count braces
		for _, ch := range line {
			if ch == '{' {
				braceCount++
			} else if ch == '}' {
				braceCount--
				if braceCount == 0 {
					break
				}
			}
		}

		if braceCount == 0 {
			break
		}

		// Parse key: value
		colonIdx := strings.Index(line, ":")
		if colonIdx <= 0 {
			i++
			continue
		}

		key := strings.TrimSpace(line[:colonIdx])
		value := strings.TrimSpace(line[colonIdx+1:])
		value = strings.TrimSuffix(value, ",")

		// Check for nested object
		if value == "{" {
			nestedProps := make(map[string]any)
			consumed := parseNestedObject(lines, nestedProps, i+1)
			props[key] = nestedProps
			i += consumed + 1
			continue
		}

		// Handle inline object
		if strings.HasPrefix(value, "{") && strings.Contains(value, "}") {
			innerContent := extractInlineObject(value)
			nestedProps := parseInlineObject(innerContent)
			props[key] = nestedProps
			i++
			continue
		}

		// Simple value
		if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
			value = value[1 : len(value)-1]
		}

		if numVal, err := parseNumber(value); err == nil {
			props[key] = numVal
		} else if key != "" && value != "" {
			props[key] = value
		}

		i++
	}

	return i - startIdx
}

// parseInlineObject parses an inline object like "containerPort: 3000".
func parseInlineObject(content string) map[string]any {
	props := make(map[string]any)
	parts := strings.Split(content, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		colonIdx := strings.Index(part, ":")
		if colonIdx <= 0 {
			continue
		}

		key := strings.TrimSpace(part[:colonIdx])
		value := strings.TrimSpace(part[colonIdx+1:])

		// Remove quotes
		if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
			value = value[1 : len(value)-1]
		}

		// Try to parse as number
		if numVal, err := parseNumber(value); err == nil {
			props[key] = numVal
		} else if key != "" && value != "" {
			props[key] = value
		}
	}

	return props
}

// extractInlineObject extracts content between first { and last }.
func extractInlineObject(value string) string {
	start := strings.Index(value, "{")
	end := strings.LastIndex(value, "}")
	if start >= 0 && end > start {
		return value[start+1 : end]
	}
	return value
}

// parseNumber attempts to parse a string as a number.
func parseNumber(value string) (any, error) {
	// Try integer first
	if !strings.Contains(value, ".") {
		var intVal int64
		_, err := fmt.Sscanf(value, "%d", &intVal)
		if err == nil {
			return intVal, nil
		}
	}

	// Try float
	var floatVal float64
	_, err := fmt.Sscanf(value, "%f", &floatVal)
	if err == nil {
		return floatVal, nil
	}

	return nil, fmt.Errorf("not a number")
}

// parseConnectionsBlock parses a connections block.
func parseConnectionsBlock(content, resourceName string) map[string]*ConnectedResource {
	connections := make(map[string]*ConnectedResource)
	
	// Parse connection entries like: postgresql: { source: database.id }
	connEntryPattern := regexp.MustCompile(`(\w+):\s*\{[^}]*source:\s*(\w+)\.(\w+)`)
	matches := connEntryPattern.FindAllStringSubmatch(content, -1)
	
	for _, match := range matches {
		connName := match[1]
		targetResource := match[2]
		targetProperty := match[3]
		
		connections[connName] = &ConnectedResource{
			Name: targetResource,
			// ID and Type will be resolved later by ResolveConnections
		}
		_ = targetProperty // Used for reference resolution
	}

	return connections
}

// parseParameter parses a parameter declaration.
func parseParameter(line string) *BicepParameter {
	// param name type = defaultValue
	pattern := regexp.MustCompile(`param\s+(\w+)\s+(\w+)(?:\s*=\s*(.+))?`)
	matches := pattern.FindStringSubmatch(line)
	if matches == nil {
		return nil
	}

	param := &BicepParameter{
		Name: matches[1],
		Type: matches[2],
	}

	if len(matches) > 3 && matches[3] != "" {
		param.DefaultValue = strings.TrimSpace(matches[3])
	}

	return param
}

// parseVariable parses a variable declaration.
func parseVariable(line string) (string, string) {
	// var name = value
	pattern := regexp.MustCompile(`var\s+(\w+)\s*=\s*(.+)`)
	matches := pattern.FindStringSubmatch(line)
	if matches == nil {
		return "", ""
	}
	return matches[1], strings.TrimSpace(matches[2])
}

// ResolveConnections resolves connection references to full resource metadata.
func (m *BicepModel) ResolveConnections(resource *BicepResource) {
	if resource == nil || resource.Connections == nil {
		return
	}

	for connName, conn := range resource.Connections {
		// Look up the referenced resource by symbolic name
		if targetResource, ok := m.Resources[conn.Name]; ok {
			resource.Connections[connName] = &ConnectedResource{
				ID:         fmt.Sprintf("/planes/radius/local/resourceGroups/default/providers/%s/%s", targetResource.Type, targetResource.Name),
				Name:       targetResource.Name,
				Type:       targetResource.Type,
				Properties: targetResource.Properties,
			}
		}
	}
}

// ResolveAllConnections resolves connections for all resources in the model.
func (m *BicepModel) ResolveAllConnections() {
	for _, resource := range m.Resources {
		m.ResolveConnections(resource)
	}
}

// GetResourceByName returns a resource by its symbolic name.
func (m *BicepModel) GetResourceByName(name string) *BicepResource {
	return m.Resources[name]
}

// GetResourcesByType returns all resources of a given type.
func (m *BicepModel) GetResourcesByType(resourceType string) []*BicepResource {
	var resources []*BicepResource
	for _, r := range m.Resources {
		if r.Type == resourceType {
			resources = append(resources, r)
		}
	}
	return resources
}

// extractSymbolicName extracts the symbolic name from a reference like "database.id".
func extractSymbolicName(reference string) string {
	parts := strings.Split(reference, ".")
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// GetOrderedResources returns resources in dependency order using topological sort.
// Dependencies come before the resources that depend on them.
func (m *BicepModel) GetOrderedResources() []*BicepResource {
	// Build dependency graph
	var resources []*BicepResource
	visited := make(map[string]bool)
	inProgress := make(map[string]bool)

	// Helper function for DFS topological sort
	var visit func(name string) bool
	visit = func(name string) bool {
		if inProgress[name] {
			// Cycle detected - skip to avoid infinite loop
			return false
		}
		if visited[name] {
			return true
		}

		inProgress[name] = true
		resource, ok := m.Resources[name]
		if !ok {
			inProgress[name] = false
			return true
		}

		// Visit dependencies first (from connections)
		for _, conn := range resource.Connections {
			if conn.Name != "" {
				visit(conn.Name)
			}
		}

		// Visit explicit dependencies
		for _, dep := range resource.DependsOn {
			visit(dep)
		}

		inProgress[name] = false
		visited[name] = true
		resources = append(resources, resource)
		return true
	}

	// Visit all resources
	for name := range m.Resources {
		visit(name)
	}

	return resources
}

// FindBicepFiles finds all .bicep files in a directory.
func FindBicepFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".bicep") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
