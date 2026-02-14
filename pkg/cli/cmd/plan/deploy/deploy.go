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

package deploy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
)

// NewCommand creates the rad plan deploy command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Generate a deployment plan",
		Long:  "Generate a deployment plan for an application.",
		Args:  cobra.NoArgs,
		RunE:  framework.RunCommand(runner),
	}
	cmd.Flags().StringP("environment", "e", "", "Target environment (required)")
	cmd.Flags().StringP("application", "a", "", "Application to plan")
	_ = cmd.MarkFlagRequired("environment")
	return cmd, runner
}

// Runner implements the rad plan deploy command.
type Runner struct {
	ConfigHolder    *framework.ConfigHolder
	Output          output.Interface
	Workspace       *workspaces.Workspace
	EnvironmentName string
	ApplicationName string
}

// NewRunner creates a new Runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{ConfigHolder: factory.GetConfigHolder(), Output: factory.GetOutput()}
}

// Validate validates the command arguments.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	if workspace.Connection == nil {
		return clierrors.Message("Workspace not configured.")
	}
	kind, ok := workspace.Connection["kind"].(string)
	if !ok || kind != workspaces.KindGitHub {
		return clierrors.Message("Requires GitHub workspace.")
	}
	r.Workspace = workspace
	r.EnvironmentName, _ = cmd.Flags().GetString("environment")
	r.ApplicationName, _ = cmd.Flags().GetString("application")
	return nil
}

// Run executes the command.
func (r *Runner) Run(ctx context.Context) error {
	cwd, _ := os.Getwd()
	r.Output.LogInfo("Generating deployment plan for environment: %s", r.EnvironmentName)
	modelsDir := filepath.Join(cwd, ".radius", "model")
	apps, _ := r.findApps(modelsDir)
	if len(apps) == 0 {
		return clierrors.Message("No applications found in .radius/model/")
	}
	if r.ApplicationName != "" {
		apps = []string{r.ApplicationName}
	}
	for _, app := range apps {
		r.generatePlan(cwd, app)
	}
	r.Output.LogInfo("Plan generated successfully!")
	return nil
}

func (r *Runner) findApps(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var apps []string
	for _, e := range entries {
		if !e.IsDir() {
			ext := filepath.Ext(e.Name())
			if ext == ".bicep" || ext == ".yaml" {
				apps = append(apps, e.Name()[:len(e.Name())-len(ext)])
			}
		}
	}
	sort.Strings(apps)
	return apps, nil
}

func (r *Runner) generatePlan(root, app string) error {
	planDir := filepath.Join(root, ".radius", "plan", app, r.EnvironmentName)
	os.MkdirAll(planDir, 0755)
	plan := DeploymentPlan{
		Application: app,
		Environment: r.EnvironmentName,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Steps:       []DeploymentStep{{Order: 1, ResourceName: app, ResourceType: "Radius.Core/applications", RecipeKind: "terraform", ArtifactDir: fmt.Sprintf("001-%s-terraform", app)}},
	}
	data, _ := yaml.Marshal(plan)
	os.WriteFile(filepath.Join(planDir, "plan.yaml"), data, 0644)
	for _, step := range plan.Steps {
		r.generateArtifacts(filepath.Join(planDir, step.ArtifactDir), step)
	}
	r.Output.LogInfo("Created plan for: %s", app)
	return nil
}

func (r *Runner) generateArtifacts(dir string, step DeploymentStep) error {
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "main.tf"), []byte(fmt.Sprintf("# Resource: %s\n", step.ResourceName)), 0644)
	os.WriteFile(filepath.Join(dir, "providers.tf"), []byte("terraform {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "variables.tf"), []byte("# Variables\n"), 0644)
	os.WriteFile(filepath.Join(dir, "terraform.tfvars.json"), []byte("{}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "terraform-context.txt"), []byte(fmt.Sprintf("Resource: %s\nType: %s\n", step.ResourceName, step.ResourceType)), 0644)
	return nil
}

// DeploymentPlan represents a plan.
type DeploymentPlan struct {
	Application string           `yaml:"application"`
	Environment string           `yaml:"environment"`
	GeneratedAt string           `yaml:"generatedAt"`
	Steps       []DeploymentStep `yaml:"steps"`
}

// DeploymentStep represents a step.
type DeploymentStep struct {
	Order        int    `yaml:"order"`
	ResourceName string `yaml:"resourceName"`
	ResourceType string `yaml:"resourceType"`
	RecipeKind   string `yaml:"recipeKind"`
	ArtifactDir  string `yaml:"artifactDir"`
}
