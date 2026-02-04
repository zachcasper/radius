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

package diff

// Exit codes for rad diff command per spec FR-063
const (
	// ExitCodeNoDiff indicates no differences were detected.
	ExitCodeNoDiff = 0

	// ExitCodeDiffFound indicates differences were detected.
	ExitCodeDiffFound = 1

	// ExitCodeValidationError indicates a validation error occurred.
	ExitCodeValidationError = 2

	// ExitCodeAuthError indicates an authentication error occurred.
	ExitCodeAuthError = 3
)

// DiffResult contains the result of a diff operation.
type DiffResult struct {
	// HasDiff indicates whether differences were found.
	HasDiff bool

	// Source describes the source of the comparison.
	Source string

	// Target describes the target of the comparison.
	Target string

	// Application is the application name.
	Application string

	// Environment is the environment name.
	Environment string

	// PlanDiffs contains differences in plan.yaml files.
	PlanDiffs []PropertyDiff

	// DeploymentDiffs contains differences in deployment records.
	DeploymentDiffs []PropertyDiff

	// ManifestDiffs contains differences in Kubernetes manifests.
	ManifestDiffs []ManifestDiff

	// DyffOutput contains the raw dyff output if available.
	DyffOutput string
}

// PropertyDiff represents a difference in a property value.
type PropertyDiff struct {
	// Path is the JSON path to the property.
	Path string

	// Change is the type of change (added, removed, modified).
	Change string

	// OldValue is the previous value.
	OldValue any

	// NewValue is the new value.
	NewValue any
}

// ManifestDiff represents a difference in a Kubernetes manifest.
type ManifestDiff struct {
	// Kind is the Kubernetes resource kind.
	Kind string

	// Name is the resource name.
	Name string

	// Namespace is the resource namespace.
	Namespace string

	// Change is the type of change (added, removed, modified).
	Change string

	// PropertyDiffs contains the property-level differences.
	PropertyDiffs []PropertyDiff

	// DyffOutput contains the dyff output for this manifest.
	DyffOutput string
}

// DiffChange constants
const (
	ChangeAdded    = "added"
	ChangeRemoved  = "removed"
	ChangeModified = "modified"
)

// ArtifactType represents the type of artifact being compared.
type ArtifactType string

const (
	// ArtifactTypeModel represents a Bicep model file.
	ArtifactTypeModel ArtifactType = "model"

	// ArtifactTypePlan represents a plan.yaml file.
	ArtifactTypePlan ArtifactType = "plan"

	// ArtifactTypeDeployment represents a deployment record.
	ArtifactTypeDeployment ArtifactType = "deployment"

	// ArtifactTypeLive represents live cloud state.
	ArtifactTypeLive ArtifactType = "live"
)

// DiffOptions contains options for a diff operation.
type DiffOptions struct {
	// Application is the application to diff.
	Application string

	// Environment is the environment to diff.
	Environment string

	// Source is the source commit, deployment, or "live".
	Source string

	// Target is the target commit, deployment, or "live".
	Target string

	// PlanOnly indicates to only diff plan.yaml files.
	PlanOnly bool

	// AllEnvironments indicates to diff all environments.
	AllEnvironments bool

	// OutputJSON indicates to output in JSON format.
	OutputJSON bool
}
