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

package show

import (
	"context"
	"strings"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/github"
	"github.com/radius-project/radius/pkg/cli/objectformats"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad env show` command.
//

// NewCommand creates a new Cobra command and a Runner object to show environment details, with flags for workspace,
// resource group, environment name and output.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show environment details",
		Long:  `Show environment details. Shows the user's default environment by default.`,
		Args:  cobra.MaximumNArgs(1),
		Example: `
# Show current environment
rad env show

# Show specified environment
rad env show my-env

# Show specified environment in a specified resource group
rad env show my-env --group my-env
`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)
	commonflags.AddOutputFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad env show` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Workspace         *workspaces.Workspace
	Output            output.Interface

	EnvironmentName string
	Format          string
}

// NewRunner creates a new instance of the `rad env show` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad env show` command.
//

// Validate checks the request object for a workspace, scope, environment name, and output format, and sets the
// corresponding fields in the Runner struct if they are found. If any of these fields are not found, an error is returned.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	// FR-074-B: Skip scope validation for GitHub workspaces
	if !workspace.IsGitHubWorkspace() {
		// Allow '--group' to override scope
		scope, err := cli.RequireScope(cmd, *r.Workspace)
		if err != nil {
			return err
		}
		r.Workspace.Scope = scope
	}

	r.EnvironmentName, err = cli.RequireEnvironmentNameArgs(cmd, args, *workspace)
	if err != nil {
		return err
	}

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}

	r.Format = format

	return nil
}

// Run runs the `rad env run` command.
//

// Run attempts to retrieve environment details from an ApplicationsManagementClient and write the details to an
// output in a specified format, returning an error if any of these operations fail.
func (r *Runner) Run(ctx context.Context) error {
	// FR-074-B: Branch on workspace kind
	if r.Workspace.IsGitHubWorkspace() {
		return r.runGitHubMode(ctx)
	}

	return r.runKubernetesMode(ctx)
}

// runGitHubMode shows environment details from GitHub.
// FR-074-B: Query GitHub API for environment variables and display formatted output.
func (r *Runner) runGitHubMode(ctx context.Context) error {
	ghClient := github.NewClient()

	// Get repo info from workspace URL
	repoURL, _ := r.Workspace.Connection["url"].(string)
	owner, repo := parseGitHubURL(repoURL)

	// Get environment variables from GitHub
	vars, err := ghClient.GetEnvironmentVariables(owner, repo, r.EnvironmentName)
	if err != nil {
		return clierrors.Message("The environment %q was not found or has been deleted.", r.EnvironmentName)
	}

	// FR-074-C: Determine provider from environment variables
	provider := "unknown"
	if p, ok := vars["RADIUS_CLOUD_PROVIDER"]; ok {
		provider = p
	} else if _, ok := vars["AWS_ACCOUNT_ID"]; ok {
		provider = "aws"
	} else if _, ok := vars["AZURE_SUBSCRIPTION_ID"]; ok {
		provider = "azure"
	}

	// Build output structure
	type envDetails struct {
		Repository string            `json:"repository"`
		Name       string            `json:"name"`
		Provider   string            `json:"provider"`
		Variables  map[string]string `json:"variables"`
	}

	details := envDetails{
		Repository: owner + "/" + repo,
		Name:       r.EnvironmentName,
		Provider:   provider,
		Variables:  vars,
	}

	// FR-074-D: Support --output json
	if r.Format == "json" {
		return r.Output.WriteFormatted(r.Format, details, output.FormatterOptions{})
	}

	// Default table format
	r.Output.LogInfo("Repository: %s", details.Repository)
	r.Output.LogInfo("Environment: %s", details.Name)
	r.Output.LogInfo("Provider: %s", details.Provider)
	r.Output.LogInfo("")
	r.Output.LogInfo("Variables:")

	type varRow struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}

	var varRows []varRow
	for k, v := range vars {
		varRows = append(varRows, varRow{Name: k, Value: v})
	}

	return r.Output.WriteFormatted("table", varRows, output.FormatterOptions{
		Columns: []output.Column{
			{Heading: "NAME", JSONPath: "{ .name }"},
			{Heading: "VALUE", JSONPath: "{ .value }"},
		},
	})
}

// runKubernetesMode shows environment details from the Kubernetes control plane.
func (r *Runner) runKubernetesMode(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	env, err := client.GetEnvironment(ctx, r.EnvironmentName)
	if clients.Is404Error(err) {
		return clierrors.Message("The environment %q was not found or has been deleted.", r.EnvironmentName)
	} else if err != nil {
		return err
	}

	return r.Output.WriteFormatted(r.Format, env, objectformats.GetResourceTableFormat())
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
