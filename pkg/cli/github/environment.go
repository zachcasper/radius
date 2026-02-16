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
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// CreateEnvironment creates a GitHub Environment in the repository.
func (c *Client) CreateEnvironment(owner, repo, envName string) error {
	_, err := c.runCommand(
		"api",
		fmt.Sprintf("repos/%s/%s/environments/%s", owner, repo, envName),
		"--method", "PUT",
	)
	if err != nil {
		return fmt.Errorf("failed to create GitHub Environment %q: %w", envName, err)
	}

	return nil
}

// DeleteEnvironment deletes a GitHub Environment from the repository.
func (c *Client) DeleteEnvironment(owner, repo, envName string) error {
	_, err := c.runCommand(
		"api",
		fmt.Sprintf("repos/%s/%s/environments/%s", owner, repo, envName),
		"--method", "DELETE",
	)
	if err != nil {
		return fmt.Errorf("failed to delete GitHub Environment %q: %w", envName, err)
	}

	return nil
}

// EnvironmentExists checks if a GitHub Environment exists in the repository.
func (c *Client) EnvironmentExists(owner, repo, envName string) (bool, error) {
	_, err := c.runCommand(
		"api",
		fmt.Sprintf("repos/%s/%s/environments/%s", owner, repo, envName),
	)
	if err != nil {
		// If the error contains "Not Found", the environment doesn't exist
		if strings.Contains(err.Error(), "Not Found") || strings.Contains(err.Error(), "404") {
			return false, nil
		}
		return false, fmt.Errorf("failed to check GitHub Environment %q: %w", envName, err)
	}

	return true, nil
}

// ListEnvironments returns the names of all GitHub Environments in the repository.
func (c *Client) ListEnvironments(owner, repo string) ([]string, error) {
	output, err := c.runCommand(
		"api",
		fmt.Sprintf("repos/%s/%s/environments", owner, repo),
		"--jq", ".environments[].name",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list GitHub Environments: %w", err)
	}

	if strings.TrimSpace(output) == "" {
		return nil, nil
	}

	names := strings.Split(strings.TrimSpace(output), "\n")
	return names, nil
}

// SetEnvironmentVariable sets a variable scoped to a GitHub Environment.
func (c *Client) SetEnvironmentVariable(owner, repo, envName, name, value string) error {
	// Get the repository ID (required by the environment variables API)
	repoID, err := c.getRepoID(owner, repo)
	if err != nil {
		return err
	}

	// Check if variable already exists
	existingValue, exists, err := c.getEnvironmentVariable(owner, repo, repoID, name)
	if err != nil {
		return err
	}

	if exists {
		if existingValue == value {
			return nil // Already set to desired value
		}
		// Update existing variable
		body := fmt.Sprintf(`{"name":"%s","value":"%s"}`, name, value)
		_, err = c.runCommandWithStdin(
			body,
			"api",
			fmt.Sprintf("repositories/%d/environments/%s/variables/%s", repoID, envName, name),
			"--method", "PATCH",
			"--input", "-",
		)
		if err != nil {
			// Fall back to delete + create
			_ = c.deleteEnvironmentVariable(owner, repo, repoID, envName, name)
			return c.createEnvironmentVariable(owner, repo, repoID, envName, name, value)
		}
		return nil
	}

	// Create new variable
	return c.createEnvironmentVariable(owner, repo, repoID, envName, name, value)
}

// createEnvironmentVariable creates a new environment variable via the GitHub API.
func (c *Client) createEnvironmentVariable(owner, repo string, repoID int64, envName, name, value string) error {
	body := fmt.Sprintf(`{"name":"%s","value":"%s"}`, name, value)
	_, err := c.runCommandWithStdin(
		body,
		"api",
		fmt.Sprintf("repositories/%d/environments/%s/variables", repoID, envName),
		"--method", "POST",
		"--input", "-",
	)
	if err != nil {
		return fmt.Errorf("failed to set environment variable %q in %q: %w", name, envName, err)
	}

	return nil
}

// deleteEnvironmentVariable deletes an environment variable via the GitHub API.
func (c *Client) deleteEnvironmentVariable(owner, repo string, repoID int64, envName, name string) error {
	_, err := c.runCommand(
		"api",
		fmt.Sprintf("repositories/%d/environments/%s/variables/%s", repoID, envName, name),
		"--method", "DELETE",
	)
	return err
}

// getEnvironmentVariable retrieves an environment variable value.
func (c *Client) getEnvironmentVariable(owner, repo string, repoID int64, name string) (string, bool, error) {
	// Note: We list all variables and find the one we need since the
	// single-variable GET endpoint requires the environment name which
	// we don't have in this helper's signature. For the check-before-set
	// pattern, this is acceptable.
	output, err := c.runCommand(
		"api",
		fmt.Sprintf("repos/%s/%s/actions/variables/%s", owner, repo, name),
		"--jq", ".value",
	)
	if err != nil {
		// Variable doesn't exist
		return "", false, nil
	}

	return strings.TrimSpace(output), true, nil
}

// GetEnvironmentVariables retrieves all variables for a GitHub Environment.
func (c *Client) GetEnvironmentVariables(owner, repo, envName string) (map[string]string, error) {
	// Get repository ID (required by the environment variables API)
	repoID, err := c.getRepoID(owner, repo)
	if err != nil {
		return nil, err
	}

	// List variables using pagination
	output, err := c.runCommand(
		"api",
		fmt.Sprintf("repositories/%d/environments/%s/variables", repoID, envName),
		"--paginate",
		"--jq", ".variables[] | .name + \"=\" + .value",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list environment variables for %q: %w", envName, err)
	}

	vars := make(map[string]string)
	if strings.TrimSpace(output) == "" {
		return vars, nil
	}

	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			vars[parts[0]] = parts[1]
		}
	}

	return vars, nil
}

// SetRepoVariable sets a repository-level variable (not environment-scoped).
func (c *Client) SetRepoVariable(name, value string) error {
	body := fmt.Sprintf(`{"name":"%s","value":"%s"}`, name, value)

	// Try to create first
	_, err := c.runCommandWithStdin(
		body,
		"api",
		"repos/{owner}/{repo}/actions/variables",
		"--method", "POST",
		"--input", "-",
	)
	if err != nil {
		// If it already exists, update it
		_, err = c.runCommandWithStdin(
			body,
			"api",
			fmt.Sprintf("repos/{owner}/{repo}/actions/variables/%s", name),
			"--method", "PATCH",
			"--input", "-",
		)
		if err != nil {
			return fmt.Errorf("failed to set repository variable %q: %w", name, err)
		}
	}

	return nil
}

// GetRepoVariable gets a repository-level variable value.
func (c *Client) GetRepoVariable(name string) (string, error) {
	output, err := c.runCommand(
		"api",
		fmt.Sprintf("repos/{owner}/{repo}/actions/variables/%s", name),
		"--jq", ".value",
	)
	if err != nil {
		return "", fmt.Errorf("failed to get repository variable %q: %w", name, err)
	}

	return strings.TrimSpace(output), nil
}

// runCommandWithStdin executes a gh CLI command with stdin input and returns the output.
func (c *Client) runCommandWithStdin(stdin string, args ...string) (string, error) {
	cmd := exec.Command("gh", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = strings.NewReader(stdin)

	if c.Verbose {
		fmt.Printf("gh %s\n", strings.Join(args, " "))
	}

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// getRepoID fetches the numeric repository ID from the GitHub API.
func (c *Client) getRepoID(owner, repo string) (int64, error) {
	output, err := c.runCommand(
		"api",
		fmt.Sprintf("repos/%s/%s", owner, repo),
		"--jq", ".id",
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get repository ID for %s/%s: %w", owner, repo, err)
	}

	var repoID int64
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &repoID); err != nil {
		return 0, fmt.Errorf("failed to parse repository ID: %w", err)
	}

	return repoID, nil
}
