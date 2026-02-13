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

package workspaces

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/radius-project/radius/pkg/sdk"
)

var _ ConnectionConfig = (*GitHubConnectionConfig)(nil)

// GitHubConnectionConfig represents a connection to a GitHub-based Radius workspace.
type GitHubConnectionConfig struct {
	// Kind specifies the kind of connection. For GitHubConnectionConfig this is always 'github'.
	Kind string `json:"kind" mapstructure:"kind" yaml:"kind"`

	// URL is the GitHub repository URL for the workspace.
	URL string `json:"url" mapstructure:"url" yaml:"url"`
}

// String returns a string that describes the GitHub connection configuration.
func (c *GitHubConnectionConfig) String() string {
	return fmt.Sprintf("GitHub (url=%s)", c.URL)
}

// GetKind returns the string "github" for a GitHubConnectionConfig object.
func (c *GitHubConnectionConfig) GetKind() string {
	return KindGitHub
}

// Connect returns an error because GitHub workspaces do not connect to a Radius control plane.
// GitHub workspaces use ephemeral k3d clusters in GitHub Actions for deployments.
func (c *GitHubConnectionConfig) Connect() (sdk.Connection, error) {
	return nil, fmt.Errorf("GitHub workspaces do not support direct connections; deployments are executed via GitHub Actions")
}

// Validate checks that the GitHub repository URL is valid.
func (c *GitHubConnectionConfig) Validate() error {
	if c.URL == "" {
		return fmt.Errorf("GitHub connection URL is required")
	}

	parsed, err := url.Parse(c.URL)
	if err != nil {
		return fmt.Errorf("invalid GitHub repository URL: %w", err)
	}

	if parsed.Host != "github.com" && !strings.HasSuffix(parsed.Host, ".github.com") {
		return fmt.Errorf("URL must be a GitHub repository (got host: %s)", parsed.Host)
	}

	return nil
}

// ParseOwnerRepo extracts the owner and repository name from the GitHub URL.
func (c *GitHubConnectionConfig) ParseOwnerRepo() (owner, repo string, err error) {
	parsed, err := url.Parse(c.URL)
	if err != nil {
		return "", "", fmt.Errorf("invalid GitHub repository URL: %w", err)
	}

	// Remove leading slash and .git suffix
	path := strings.TrimPrefix(parsed.Path, "/")
	path = strings.TrimSuffix(path, ".git")

	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("GitHub URL must contain owner/repo (got: %s)", path)
	}

	return parts[0], parts[1], nil
}
