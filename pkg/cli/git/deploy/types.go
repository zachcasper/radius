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
	"time"
)

// DeploymentRecord captures the result of a deployment for Git workspace mode.
// This record is stored in .radius/deploy/{app}/{env}/deployment-{commit}.json
type DeploymentRecord struct {
	// Application is the name of the application that was deployed.
	Application string `json:"application"`

	// Environment contains information about the deployment environment.
	Environment *EnvironmentInfo `json:"environment"`

	// StartedAt is when the deployment started.
	StartedAt time.Time `json:"startedAt"`

	// CompletedAt is when the deployment completed.
	CompletedAt time.Time `json:"completedAt,omitempty"`

	// Status is the overall deployment status (in-progress, succeeded, failed).
	Status string `json:"status"`

	// Git contains information about the Git state at deployment time.
	Git *GitInfo `json:"git"`

	// Plan contains reference information about the plan that was deployed.
	Plan *PlanReference `json:"plan"`

	// Steps contains the results of each deployment step.
	Steps []StepResult `json:"steps"`

	// Resources contains a list of all resources deployed (for backward compatibility).
	Resources []string `json:"resources"`

	// Summary contains summary statistics for the deployment.
	Summary *DeploymentSummary `json:"summary"`

	// Error contains error information if the deployment failed.
	Error string `json:"error,omitempty"`
}

// EnvironmentInfo contains information about the deployment environment.
type EnvironmentInfo struct {
	// Name is the environment name (e.g., "default", "production").
	Name string `json:"name"`

	// EnvironmentFile is the path to the environment file.
	EnvironmentFile string `json:"environmentFile"`

	// KubernetesContext is the Kubernetes context used for deployment.
	KubernetesContext string `json:"kubernetesContext,omitempty"`

	// KubernetesNamespace is the Kubernetes namespace used for deployment.
	KubernetesNamespace string `json:"kubernetesNamespace,omitempty"`
}

// PlanReference contains reference information about the plan.
type PlanReference struct {
	// PlanFile is the path to the plan.yaml file.
	PlanFile string `json:"planFile"`

	// PlanCommit is the Git commit SHA where the plan was generated.
	PlanCommit string `json:"planCommit"`

	// GeneratedAt is when the plan was generated.
	GeneratedAt string `json:"generatedAt"`
}

// GitInfo contains information about the Git state at deployment time.
type GitInfo struct {
	// Commit is the full Git commit SHA.
	Commit string `json:"commit"`

	// CommitShort is the short (8 character) Git commit SHA.
	CommitShort string `json:"commitShort"`

	// Branch is the Git branch name.
	Branch string `json:"branch"`

	// IsDirty indicates whether the working directory had uncommitted changes.
	IsDirty bool `json:"isDirty"`
}

// StepResult captures the result of a single deployment step.
type StepResult struct {
	// Sequence is the step sequence number.
	Sequence int `json:"sequence"`

	// Name is the resource name.
	Name string `json:"name"`

	// ResourceType is the type of the resource.
	ResourceType string `json:"resourceType"`

	// Tool is the deployment tool (terraform, bicep).
	Tool string `json:"tool"`

	// Status is the step status (pending, in-progress, succeeded, failed, skipped).
	Status string `json:"status"`

	// StartedAt is when the step started.
	StartedAt time.Time `json:"startedAt,omitempty"`

	// CompletedAt is when the step completed.
	CompletedAt time.Time `json:"completedAt,omitempty"`

	// Duration is the step duration in nanoseconds.
	Duration int64 `json:"duration,omitempty"`

	// Changes contains the summary of changes made by this step.
	Changes *ChangesSummary `json:"changes,omitempty"`

	// Outputs contains the terraform outputs for this step.
	Outputs map[string]any `json:"outputs,omitempty"`

	// CapturedResources contains Kubernetes manifests captured after deployment.
	CapturedResources []CapturedResource `json:"capturedResources,omitempty"`

	// Error contains error information if the step failed.
	Error string `json:"error,omitempty"`
}

// ChangesSummary contains a summary of changes made by a deployment step.
type ChangesSummary struct {
	// Add is the number of resources added.
	Add int `json:"add"`

	// Change is the number of resources changed.
	Change int `json:"change"`

	// Destroy is the number of resources destroyed.
	Destroy int `json:"destroy"`
}

// CapturedResource represents a Kubernetes resource captured after deployment.
type CapturedResource struct {
	// ResourceID is the unique identifier for this resource (namespace/type/name).
	ResourceID string `json:"resourceId"`

	// ResourceType is the Kubernetes resource type (deployment, service, etc.).
	ResourceType string `json:"resourceType"`

	// Provider is the cloud provider (kubernetes, azure, aws, gcp).
	Provider string `json:"provider"`

	// Name is the resource name.
	Name string `json:"name"`

	// Namespace is the Kubernetes namespace.
	Namespace string `json:"namespace"`

	// RadiusResourceType is the Radius resource type that created this.
	RadiusResourceType string `json:"radiusResourceType"`

	// DeploymentStep is the step number that created this resource.
	DeploymentStep int `json:"deploymentStep"`

	// RawManifest is the captured manifest as a structured JSON object.
	RawManifest any `json:"rawManifest"`
}

// DeploymentSummary contains summary statistics for the deployment.
type DeploymentSummary struct {
	// TotalSteps is the total number of deployment steps.
	TotalSteps int `json:"totalSteps"`

	// SucceededSteps is the number of steps that succeeded.
	SucceededSteps int `json:"succeededSteps"`

	// FailedSteps is the number of steps that failed.
	FailedSteps int `json:"failedSteps"`

	// SkippedSteps is the number of steps that were skipped.
	SkippedSteps int `json:"skippedSteps"`

	// TotalResources is the total number of cloud resources affected.
	TotalResources int `json:"totalResources"`

	// ResourcesAdded is the number of resources added.
	ResourcesAdded int `json:"resourcesAdded"`

	// ResourcesChanged is the number of resources changed.
	ResourcesChanged int `json:"resourcesChanged"`

	// ResourcesDestroyed is the number of resources destroyed.
	ResourcesDestroyed int `json:"resourcesDestroyed"`
}

// DeploymentStatus constants
const (
	StatusPending    = "pending"
	StatusInProgress = "in-progress"
	StatusSucceeded  = "succeeded"
	StatusFailed     = "failed"
	StatusSkipped    = "skipped"
	// For backward compatibility
	StatusCompleted = "completed"
)

// NewDeploymentRecord creates a new DeploymentRecord.
func NewDeploymentRecord(application string, environment *EnvironmentInfo, planRef *PlanReference, gitInfo *GitInfo) *DeploymentRecord {
	return &DeploymentRecord{
		Application: application,
		Environment: environment,
		StartedAt:   time.Now(),
		Status:      StatusInProgress,
		Git:         gitInfo,
		Plan:        planRef,
		Steps:       []StepResult{},
		Resources:   []string{},
		Summary:     &DeploymentSummary{},
	}
}

// AddStepResult adds a step result to the deployment record.
func (r *DeploymentRecord) AddStepResult(result StepResult) {
	r.Steps = append(r.Steps, result)
}

// Complete marks the deployment as succeeded and builds summary.
func (r *DeploymentRecord) Complete() {
	r.CompletedAt = time.Now()
	r.Status = StatusSucceeded

	// Build summary
	r.Summary.TotalSteps = len(r.Steps)
	for _, step := range r.Steps {
		switch step.Status {
		case StatusSucceeded, StatusCompleted:
			r.Summary.SucceededSteps++
		case StatusFailed:
			r.Summary.FailedSteps++
		case StatusSkipped:
			r.Summary.SkippedSteps++
		}
		if step.Changes != nil {
			r.Summary.ResourcesAdded += step.Changes.Add
			r.Summary.ResourcesChanged += step.Changes.Change
			r.Summary.ResourcesDestroyed += step.Changes.Destroy
		}
	}
	// TotalResources refers to cloud resources changed, not captured K8s manifests
	r.Summary.TotalResources = r.Summary.ResourcesAdded + r.Summary.ResourcesChanged + r.Summary.ResourcesDestroyed
}

// Fail marks the deployment as failed with the given error.
func (r *DeploymentRecord) Fail(err error) {
	r.CompletedAt = time.Now()
	r.Status = StatusFailed
	if err != nil {
		r.Error = err.Error()
	}
	// Build partial summary
	r.Summary.TotalSteps = len(r.Steps)
	for _, step := range r.Steps {
		switch step.Status {
		case StatusSucceeded, StatusCompleted:
			r.Summary.SucceededSteps++
		case StatusFailed:
			r.Summary.FailedSteps++
		case StatusSkipped:
			r.Summary.SkippedSteps++
		}
	}
}

// Duration returns the duration of the deployment.
func (r *DeploymentRecord) Duration() time.Duration {
	if r.CompletedAt.IsZero() {
		return time.Since(r.StartedAt)
	}
	return r.CompletedAt.Sub(r.StartedAt)
}
