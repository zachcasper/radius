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

// Plan represents a deployment plan for Git workspace mode.
// It contains the application, environment, and ordered deployment steps.
type Plan struct {
	// Application is the name of the application being deployed.
	Application string `yaml:"application" json:"application"`

	// EnvironmentFile is the path to the environment file (e.g., ".env", ".env.production").
	EnvironmentFile string `yaml:"environmentFile" json:"environmentFile"`

	// Summary provides a high-level overview of the plan.
	Summary *PlanSummary `yaml:"summary,omitempty" json:"summary,omitempty"`

	// Steps is the ordered list of deployment steps.
	Steps []DeploymentStep `yaml:"steps" json:"steps"`
}

// PlanSummary provides summary information about a deployment plan.
type PlanSummary struct {
	// TotalSteps is the number of deployment steps.
	TotalSteps int `yaml:"totalSteps" json:"totalSteps"`

	// RecipeCount is the number of recipes to be deployed.
	RecipeCount int `yaml:"recipeCount" json:"recipeCount"`

	// AllVersionsPinned indicates whether all recipe versions are pinned.
	AllVersionsPinned bool `yaml:"allVersionsPinned" json:"allVersionsPinned"`
}

// DeploymentStep represents a single deployment step in a plan.
type DeploymentStep struct {
	// Sequence is the order of this step (1-indexed).
	Sequence int `yaml:"sequence" json:"sequence"`

	// Resource contains information about the resource being deployed.
	Resource ResourceInfo `yaml:"resource" json:"resource"`

	// Recipe contains information about the recipe being used.
	Recipe RecipeReference `yaml:"recipe" json:"recipe"`

	// Status is the current status of the step (pending, in-progress, completed, failed).
	Status string `yaml:"status,omitempty" json:"status,omitempty"`
}

// ResourceInfo contains information about a Bicep resource.
type ResourceInfo struct {
	// Name is the symbolic name of the resource in the Bicep model.
	Name string `yaml:"name" json:"name"`

	// Type is the fully qualified resource type (e.g., "Radius.Compute/containers").
	Type string `yaml:"type" json:"type"`

	// Properties contains the resource properties from the Bicep model.
	Properties map[string]any `yaml:"properties,omitempty" json:"properties,omitempty"`
}

// RecipeReference contains information about a recipe used by a resource.
type RecipeReference struct {
	// Name is the name of the recipe.
	Name string `yaml:"name" json:"name"`

	// Kind is the type of recipe (terraform, bicep).
	Kind string `yaml:"kind" json:"kind"`

	// Source is the source location of the recipe (e.g., OCI registry path).
	Source string `yaml:"source" json:"source"`

	// Version is the pinned version of the recipe, if specified.
	Version *RecipeVersion `yaml:"version,omitempty" json:"version,omitempty"`
}

// RecipeVersion represents a pinned recipe version.
type RecipeVersion struct {
	// Tag is the OCI tag or version string.
	Tag string `yaml:"tag,omitempty" json:"tag,omitempty"`

	// Digest is the OCI content digest for immutable references.
	Digest string `yaml:"digest,omitempty" json:"digest,omitempty"`
}

// ConnectedResource represents a resource that is connected to another resource.
// This mirrors the control plane's ConnectedResource in pkg/recipes/types.go.
type ConnectedResource struct {
	// ID is the fully qualified resource ID.
	ID string `json:"id,omitempty"`

	// Name is the resource name.
	Name string `json:"name,omitempty"`

	// Type is the resource type (e.g., "Radius.Datastores/postgreSQLDatabases").
	Type string `json:"type,omitempty"`

	// Properties contains the resource properties.
	Properties map[string]any `json:"properties,omitempty"`
}

// ContextVariable represents the context passed to Terraform/Bicep recipes.
// This mirrors the control plane's RecipeContext structure.
type ContextVariable struct {
	// Resource contains information about the resource being deployed.
	Resource *ContextResource `json:"resource,omitempty"`

	// Application contains information about the application.
	Application *ContextApplication `json:"application,omitempty"`

	// Environment contains information about the environment.
	Environment *ContextEnvironment `json:"environment,omitempty"`

	// Runtime contains runtime-specific configuration.
	Runtime *ContextRuntime `json:"runtime,omitempty"`

	// Azure contains Azure-specific configuration, if applicable.
	Azure *ContextAzure `json:"azure,omitempty"`

	// AWS contains AWS-specific configuration, if applicable.
	AWS *ContextAWS `json:"aws,omitempty"`
}

// ContextResource contains resource information for the context variable.
type ContextResource struct {
	// Name is the resource name.
	Name string `json:"name"`

	// Type is the resource type.
	Type string `json:"type"`

	// Properties contains the resource properties.
	Properties map[string]any `json:"properties,omitempty"`

	// Connections contains resolved connected resources.
	Connections map[string]*ConnectedResource `json:"connections,omitempty"`
}

// ContextApplication contains application information for the context variable.
type ContextApplication struct {
	// Name is the application name.
	Name string `json:"name"`
}

// ContextEnvironment contains environment information for the context variable.
type ContextEnvironment struct {
	// Name is the environment name.
	Name string `json:"name"`
}

// ContextRuntime contains runtime configuration for the context variable.
type ContextRuntime struct {
	// Kubernetes contains Kubernetes runtime configuration.
	Kubernetes *ContextKubernetes `json:"kubernetes,omitempty"`
}

// ContextKubernetes contains Kubernetes-specific runtime configuration.
type ContextKubernetes struct {
	// Namespace is the Kubernetes namespace.
	Namespace string `json:"namespace,omitempty"`

	// EnvironmentNamespace is the namespace for the environment.
	EnvironmentNamespace string `json:"environmentNamespace,omitempty"`
}

// ContextAzure contains Azure-specific configuration for the context variable.
type ContextAzure struct {
	// ResourceGroup is the Azure resource group.
	ResourceGroup string `json:"resourceGroup,omitempty"`

	// SubscriptionID is the Azure subscription ID.
	SubscriptionID string `json:"subscriptionId,omitempty"`
}

// ContextAWS contains AWS-specific configuration for the context variable.
type ContextAWS struct {
	// Region is the AWS region.
	Region string `json:"region,omitempty"`

	// AccountID is the AWS account ID.
	AccountID string `json:"accountId,omitempty"`
}

// NewPlan creates a new Plan with the given application and environment file.
func NewPlan(application, environmentFile string) *Plan {
	return &Plan{
		Application:     application,
		EnvironmentFile: environmentFile,
		Steps:           []DeploymentStep{},
	}
}

// AddStep adds a deployment step to the plan.
func (p *Plan) AddStep(step DeploymentStep) {
	step.Sequence = len(p.Steps) + 1
	p.Steps = append(p.Steps, step)
}

// UpdateSummary recalculates the plan summary.
func (p *Plan) UpdateSummary() {
	summary := &PlanSummary{
		TotalSteps:        len(p.Steps),
		RecipeCount:       0,
		AllVersionsPinned: true,
	}

	for _, step := range p.Steps {
		if step.Recipe.Name != "" {
			summary.RecipeCount++
			if step.Recipe.Version == nil || (step.Recipe.Version.Tag == "" && step.Recipe.Version.Digest == "") {
				summary.AllVersionsPinned = false
			}
		}
	}

	p.Summary = summary
}
