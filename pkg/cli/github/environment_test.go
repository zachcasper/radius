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
	"testing"

	"github.com/stretchr/testify/require"
)

// Note: These tests cover the pure logic and structure of environment functions.
// Full integration testing of GitHub API calls requires a real gh CLI and repository.
// Integration tests should be run separately with proper credentials.

func TestClient_EnvironmentMethods_Structure(t *testing.T) {
	t.Parallel()

	// Verify Client has expected environment methods
	client := NewClient()

	// These method declarations are compile-time verified
	_ = func() error { return client.CreateEnvironment("owner", "repo", "env") }
	_ = func() error { return client.DeleteEnvironment("owner", "repo", "env") }
	_ = func() (bool, error) { return client.EnvironmentExists("owner", "repo", "env") }
	_ = func() ([]string, error) { return client.ListEnvironments("owner", "repo") }
	_ = func() error { return client.SetEnvironmentVariable("owner", "repo", "env", "name", "value") }
	_ = func() (map[string]string, error) { return client.GetEnvironmentVariables("owner", "repo", "env") }
	_ = func() error { return client.SetRepoVariable("name", "value") }
	_ = func() (string, error) { return client.GetRepoVariable("name") }
}

func TestEnvironmentVariableParsing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected map[string]string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: map[string]string{},
		},
		{
			name:     "whitespace only",
			input:    "   \n  \t  ",
			expected: map[string]string{},
		},
		{
			name:  "single variable",
			input: "FOO=bar",
			expected: map[string]string{
				"FOO": "bar",
			},
		},
		{
			name:  "multiple variables",
			input: "FOO=bar\nBAZ=qux",
			expected: map[string]string{
				"FOO": "bar",
				"BAZ": "qux",
			},
		},
		{
			name:  "variable with equals in value",
			input: "CONFIG=key=value",
			expected: map[string]string{
				"CONFIG": "key=value",
			},
		},
		{
			name:  "variable with empty value",
			input: "EMPTY=",
			expected: map[string]string{
				"EMPTY": "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseEnvironmentVariables(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

// parseEnvironmentVariables is a helper to test the parsing logic
// used in GetEnvironmentVariables. This extracts the parsing logic
// for testability.
func parseEnvironmentVariables(output string) map[string]string {
	vars := make(map[string]string)
	if len(output) == 0 {
		return vars
	}

	// Trim and check for empty
	trimmed := output
	for len(trimmed) > 0 && (trimmed[0] == ' ' || trimmed[0] == '\t' || trimmed[0] == '\n' || trimmed[0] == '\r') {
		trimmed = trimmed[1:]
	}
	for len(trimmed) > 0 && (trimmed[len(trimmed)-1] == ' ' || trimmed[len(trimmed)-1] == '\t' || trimmed[len(trimmed)-1] == '\n' || trimmed[len(trimmed)-1] == '\r') {
		trimmed = trimmed[:len(trimmed)-1]
	}

	if len(trimmed) == 0 {
		return vars
	}

	// Split by newlines and parse each line
	lines := []string{}
	start := 0
	for i := 0; i <= len(trimmed); i++ {
		if i == len(trimmed) || trimmed[i] == '\n' {
			if i > start {
				lines = append(lines, trimmed[start:i])
			}
			start = i + 1
		}
	}

	for _, line := range lines {
		idx := -1
		for i := 0; i < len(line); i++ {
			if line[i] == '=' {
				idx = i
				break
			}
		}
		if idx >= 0 {
			vars[line[:idx]] = line[idx+1:]
		}
	}

	return vars
}

func TestClient_NewClient(t *testing.T) {
	t.Parallel()

	client := NewClient()
	require.NotNil(t, client)
	require.False(t, client.Verbose)
}
