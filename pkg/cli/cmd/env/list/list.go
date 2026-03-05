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

package list

import (
	"context"
	"strings"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/github"
	"github.com/radius-project/radius/pkg/cli/objectformats"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad env list` command.
//

// NewCommand creates a new Cobra command and a Runner to list environments using the current or specified workspace,
// with optional flags for resource group and output.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List environments",
		Long:  `List environments using the current, or specified workspace.`,
		Args:  cobra.NoArgs,
		Example: `
# List environments
rad env list
`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddOutputFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad env list` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Workspace         *workspaces.Workspace
	Output            output.Interface

	Format string
}

// NewRunner creates a new instance of the `rad env list` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad env list` command.
//

// Validate checks the workspace, scope, and output format of the command and sets them in the Runner struct,
// returning an error if any of these checks fail.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	// GitHub mode doesn't need scope validation
	if !workspace.IsGitHubWorkspace() {
		// Allow '--group' to override scope
		scope, err := cli.RequireScope(cmd, *r.Workspace)
		if err != nil {
			return err
		}
		r.Workspace.Scope = scope
	}

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	r.Format = format

	return nil
}

// Run runs the `rad env list` command.
//

// Run creates an ApplicationsManagementClient using the provided context and workspace, then lists the environments in the
// resource group and writes the formatted output to the Output. If any of these steps fail, an error is returned.
func (r *Runner) Run(ctx context.Context) error {
	// FR-074-A: Branch on workspace kind
	if r.Workspace.IsGitHubWorkspace() {
		return r.runGitHubMode(ctx)
	}

	return r.runKubernetesMode(ctx)
}

// runGitHubMode lists environments from GitHub via the GitHub API.
// FR-074-A: Query GitHub API and display environments with Name and Provider columns.
func (r *Runner) runGitHubMode(ctx context.Context) error {
	ghClient := github.NewClient()

	// Get repo info from workspace URL
	repoURL, _ := r.Workspace.Connection["url"].(string)
	owner, repo := parseGitHubURL(repoURL)

	// List environments from GitHub
	envNames, err := ghClient.ListEnvironments(owner, repo)
	if err != nil {
		return err
	}

	if len(envNames) == 0 {
		r.Output.LogInfo("No environments found. Run 'rad env create <name> --provider <aws|azure>' to create one.")
		return nil
	}

	// Build output with provider information
	type envRow struct {
		Name     string `json:"name"`
		Provider string `json:"provider"`
	}

	var envs []envRow
	for _, name := range envNames {
		// Get environment variables to determine provider
		vars, err := ghClient.GetEnvironmentVariables(owner, repo, name)
		provider := "unknown"
		if err == nil {
			if p, ok := vars["RADIUS_CLOUD_PROVIDER"]; ok {
				provider = p
			} else if _, ok := vars["AWS_ACCOUNT_ID"]; ok {
				provider = "aws"
			} else if _, ok := vars["AZURE_SUBSCRIPTION_ID"]; ok {
				provider = "azure"
			}
		}
		envs = append(envs, envRow{Name: name, Provider: provider})
	}

	// FR-074-D: Support --output json
	return r.Output.WriteFormatted(r.Format, envs, output.FormatterOptions{
		Columns: []output.Column{
			{Heading: "NAME", JSONPath: "{ .name }"},
			{Heading: "PROVIDER", JSONPath: "{ .provider }"},
		},
	})
}

// runKubernetesMode lists environments from the Kubernetes control plane.
func (r *Runner) runKubernetesMode(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	environments, err := client.ListEnvironments(ctx)
	if err != nil {
		return err
	}

	return r.Output.WriteFormatted(r.Format, environments, objectformats.GetResourceTableFormat())
}

// parseGitHubURL extracts owner and repo from a GitHub URL.
func parseGitHubURL(url string) (owner, repo string) {
	url = strings.TrimSuffix(url, ".git")
	parts := strings.Split(url, "/")
	if len(parts) >= 2 {
		repo = parts[len(parts)-1]
		owner = parts[len(parts)-2]
	}
	return
}
