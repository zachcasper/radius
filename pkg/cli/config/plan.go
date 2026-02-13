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

	"gopkg.in/yaml.v3"
)

const (
	// StepStatusPlanned indicates the step is planned but not yet executed.
	StepStatusPlanned = "planned"

	// StepStatusExecuting indicates the step is currently executing.
	StepStatusExecuting = "executing"

	// StepStatusSucceeded indicates the step completed successfully.
	StepStatusSucceeded = "succeeded"

	// StepStatusFailed indicates the step failed during execution.
	StepStatusFailed = "failed"

	// StepStatusSkipped indicates the step was skipped (e.g., dependency failed).
	StepStatusSkipped = "skipped"
)

// DeploymentPlan defines the deployment plan for an application.
// This is stored in .radius/plan/<APP>/<ENV>/plan.yaml.
type DeploymentPlan struct {
	// Application is the name of the application being deployed.
	Application string `yaml:"application"`

	// ApplicationModelFile is the path to the application model file.
	ApplicationModelFile string `yaml:"applicationModelFile"`

	// Environment is the target environment name.
	Environment string `yaml:"environment"`

	// Steps contains the ordered deployment steps.
	Steps []DeploymentStep `yaml:"steps"`

	// Summary contains aggregated plan statistics.
	Summary PlanSummary `yaml:"summary"`
}

// DeploymentStep defines a single step in the deployment plan.
type DeploymentStep struct {
	// Sequence is the step execution order (1-based).
	Sequence int `yaml:"sequence"`

	// Resource identifies the resource being deployed.
	Resource ResourceReference `yaml:"resource"`

	// Recipe identifies the recipe used for deployment.
	Recipe RecipeReference `yaml:"recipe"`

	// DeploymentArtifacts is the directory path containing deployment artifacts.
	DeploymentArtifacts string `yaml:"deploymentArtifacts"`

	// ExpectedChanges summarizes expected resource changes.
	ExpectedChanges ChangeCount `yaml:"expectedChanges"`

	// Status is the current step status.
	Status string `yaml:"status"`
}

// ResourceReference identifies the resource being deployed.
type ResourceReference struct {
	// Name is the resource name.
	Name string `yaml:"name"`

	// Type is the resource type (e.g., "Radius.Data/postgreSqlDatabases").
	Type string `yaml:"type"`

	// Properties contains the resource properties.
	Properties map[string]any `yaml:"properties"`
}

// RecipeReference identifies the recipe used for deployment.
type RecipeReference struct {
	// Name is the recipe name (matches resource type).
	Name string `yaml:"name"`

	// Kind is the recipe type: "terraform" or "bicep".
	Kind string `yaml:"kind"`

	// Location is the recipe source location (git URL or OCI registry).
	Location string `yaml:"location"`
}

// ChangeCount summarizes expected or actual resource changes.
type ChangeCount struct {
	// Add is the number of resources to add.
	Add int `yaml:"add" json:"add"`

	// Change is the number of resources to modify.
	Change int `yaml:"change" json:"change"`

	// Destroy is the number of resources to destroy.
	Destroy int `yaml:"destroy" json:"destroy"`
}

// PlanSummary aggregates plan statistics.
type PlanSummary struct {
	// TotalSteps is the total number of deployment steps.
	TotalSteps int `yaml:"totalSteps"`

	// TerraformSteps is the number of Terraform-based steps.
	TerraformSteps int `yaml:"terraformSteps"`

	// BicepSteps is the number of Bicep-based steps.
	BicepSteps int `yaml:"bicepSteps"`

	// TotalAdd is the total resources to add across all steps.
	TotalAdd int `yaml:"totalAdd"`

	// TotalChange is the total resources to modify across all steps.
	TotalChange int `yaml:"totalChange"`

	// TotalDestroy is the total resources to destroy across all steps.
	TotalDestroy int `yaml:"totalDestroy"`

	// AllVersionsPinned indicates whether all recipe versions are pinned.
	AllVersionsPinned bool `yaml:"allVersionsPinned"`
}

// LoadDeploymentPlan loads a DeploymentPlan from a file.
func LoadDeploymentPlan(filepath string) (*DeploymentPlan, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read deployment plan: %w", err)
	}

	var plan DeploymentPlan
	if err := yaml.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("failed to parse deployment plan: %w", err)
	}

	return &plan, nil
}

// SaveDeploymentPlan saves a DeploymentPlan to a file.
func SaveDeploymentPlan(filepath string, plan *DeploymentPlan) error {
	data, err := yaml.Marshal(plan)
	if err != nil {
		return fmt.Errorf("failed to marshal deployment plan: %w", err)
	}

	if err := os.WriteFile(filepath, data, 0644); err != nil {
		return fmt.Errorf("failed to write deployment plan: %w", err)
	}

	return nil
}

// Validate checks that the deployment plan is valid.
func (p *DeploymentPlan) Validate() error {
	if p.Application == "" {
		return fmt.Errorf("deployment plan application name is required")
	}

	if p.Environment == "" {
		return fmt.Errorf("deployment plan environment is required")
	}

	for i, step := range p.Steps {
		if step.Sequence != i+1 {
			return fmt.Errorf("step %d has incorrect sequence number %d", i+1, step.Sequence)
		}

		if step.Resource.Name == "" {
			return fmt.Errorf("step %d: resource name is required", step.Sequence)
		}

		if step.Recipe.Kind != RecipeKindTerraform && step.Recipe.Kind != RecipeKindBicep {
			return fmt.Errorf("step %d: recipe kind must be 'terraform' or 'bicep'", step.Sequence)
		}
	}

	return nil
}

// CalculateSummary recalculates the plan summary based on the steps.
func (p *DeploymentPlan) CalculateSummary() {
	p.Summary = PlanSummary{
		TotalSteps:        len(p.Steps),
		AllVersionsPinned: true,
	}

	for _, step := range p.Steps {
		switch step.Recipe.Kind {
		case RecipeKindTerraform:
			p.Summary.TerraformSteps++
		case RecipeKindBicep:
			p.Summary.BicepSteps++
		}

		p.Summary.TotalAdd += step.ExpectedChanges.Add
		p.Summary.TotalChange += step.ExpectedChanges.Change
		p.Summary.TotalDestroy += step.ExpectedChanges.Destroy

		// Check if version is pinned (contains ?ref=)
		if step.Recipe.Kind == RecipeKindTerraform {
			if !containsVersionRef(step.Recipe.Location) {
				p.Summary.AllVersionsPinned = false
			}
		}
	}
}

func containsVersionRef(location string) bool {
	return len(location) > 0 && (contains(location, "?ref=") || contains(location, "&ref="))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
