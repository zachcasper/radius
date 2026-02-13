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
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const (
	// RecordStatusSucceeded indicates the deployment completed successfully.
	RecordStatusSucceeded = "succeeded"

	// RecordStatusFailed indicates the deployment failed.
	RecordStatusFailed = "failed"

	// RecordStatusPartial indicates partial deployment (some steps succeeded, some failed).
	RecordStatusPartial = "partial"
)

// DeploymentRecord captures the full deployment execution audit.
// This is stored in .radius/deploy/<APP>/<ENV>/<COMMIT>/deploy-<COMMIT>.json.
type DeploymentRecord struct {
	// Application is the name of the deployed application.
	Application string `json:"application"`

	// Environment provides environment context for the deployment.
	Environment EnvironmentInfo `json:"environment"`

	// StartedAt is the deployment start time.
	StartedAt time.Time `json:"startedAt"`

	// CompletedAt is the deployment completion time.
	CompletedAt time.Time `json:"completedAt"`

	// Status is the overall deployment status.
	Status string `json:"status"`

	// Git provides git information at deployment time.
	Git GitContext `json:"git"`

	// Plan links back to the plan used.
	Plan PlanReference `json:"plan"`

	// Steps contains execution details for each deployment step.
	Steps []ExecutedStep `json:"steps"`

	// Resources lists all deployed resources.
	Resources []ResourceInfo `json:"resources"`

	// Summary aggregates execution statistics.
	Summary ExecutionSummary `json:"summary"`
}

// EnvironmentInfo provides environment context for the deployment.
type EnvironmentInfo struct {
	// Name is the environment name.
	Name string `json:"name"`

	// EnvironmentFile is the path to the environment configuration file.
	EnvironmentFile string `json:"environmentFile"`

	// KubernetesContext is the Kubernetes context used (if applicable).
	KubernetesContext string `json:"kubernetesContext,omitempty"`

	// KubernetesNamespace is the Kubernetes namespace used (if applicable).
	KubernetesNamespace string `json:"kubernetesNamespace,omitempty"`
}

// GitContext provides git information at deployment time.
type GitContext struct {
	// Commit is the full git commit SHA.
	Commit string `json:"commit"`

	// CommitShort is the short git commit SHA.
	CommitShort string `json:"commitShort"`

	// Branch is the git branch name.
	Branch string `json:"branch"`

	// IsDirty indicates whether the working tree has uncommitted changes.
	IsDirty bool `json:"isDirty"`
}

// PlanReference links back to the plan used.
type PlanReference struct {
	// PlanFile is the path to the plan file.
	PlanFile string `json:"planFile"`

	// PlanCommit is the git commit where the plan was generated.
	PlanCommit string `json:"planCommit"`

	// GeneratedAt is when the plan was generated.
	GeneratedAt time.Time `json:"generatedAt"`
}

// ExecutedStep records execution details for a deployment step.
type ExecutedStep struct {
	// Sequence is the step execution order.
	Sequence int `json:"sequence"`

	// Name is the resource name.
	Name string `json:"name"`

	// ResourceType is the resource type with API version.
	ResourceType string `json:"resourceType"`

	// Tool is the deployment tool used ("terraform" or "bicep").
	Tool string `json:"tool"`

	// Status is the step execution status.
	Status string `json:"status"`

	// StartedAt is when the step started.
	StartedAt time.Time `json:"startedAt"`

	// CompletedAt is when the step completed.
	CompletedAt time.Time `json:"completedAt"`

	// Duration is the step execution duration.
	Duration time.Duration `json:"duration"`

	// Changes summarizes actual resource changes.
	Changes ChangeCount `json:"changes"`

	// Outputs contains step outputs (e.g., connection strings).
	Outputs map[string]any `json:"outputs,omitempty"`

	// CapturedResources lists resources captured during deployment.
	CapturedResources []CapturedResource `json:"capturedResources,omitempty"`

	// Error contains error details if the step failed.
	Error *ErrorInfo `json:"error,omitempty"`
}

// CapturedResource links to the captured resource definition.
type CapturedResource struct {
	// ResourceID is the resource identifier.
	ResourceID string `json:"resourceId"`

	// ResourceDefinitionFile is the filename containing the captured definition.
	ResourceDefinitionFile string `json:"resourceDefinitionFile"`
}

// ErrorInfo captures error details when a step fails.
type ErrorInfo struct {
	// Message is the error message.
	Message string `json:"message"`

	// Details provides additional error context.
	Details string `json:"details,omitempty"`

	// LogFile is the path to the error log file.
	LogFile string `json:"logFile,omitempty"`
}

// ResourceInfo provides information about a deployed resource.
type ResourceInfo struct {
	// ID is the resource identifier.
	ID string `json:"id"`

	// Type is the resource type.
	Type string `json:"type"`

	// Name is the resource name.
	Name string `json:"name"`
}

// ExecutionSummary aggregates execution statistics.
type ExecutionSummary struct {
	// TotalSteps is the total number of steps.
	TotalSteps int `json:"totalSteps"`

	// SucceededSteps is the number of successful steps.
	SucceededSteps int `json:"succeededSteps"`

	// FailedSteps is the number of failed steps.
	FailedSteps int `json:"failedSteps"`

	// SkippedSteps is the number of skipped steps.
	SkippedSteps int `json:"skippedSteps"`

	// TotalResources is the total number of resources affected.
	TotalResources int `json:"totalResources"`

	// ResourcesAdded is the number of resources added.
	ResourcesAdded int `json:"resourcesAdded"`

	// ResourcesChanged is the number of resources changed.
	ResourcesChanged int `json:"resourcesChanged"`

	// ResourcesDestroyed is the number of resources destroyed.
	ResourcesDestroyed int `json:"resourcesDestroyed"`
}

// LoadDeploymentRecord loads a DeploymentRecord from a file.
func LoadDeploymentRecord(filepath string) (*DeploymentRecord, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read deployment record: %w", err)
	}

	var record DeploymentRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("failed to parse deployment record: %w", err)
	}

	return &record, nil
}

// SaveDeploymentRecord saves a DeploymentRecord to a file.
func SaveDeploymentRecord(filepath string, record *DeploymentRecord) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal deployment record: %w", err)
	}

	if err := os.WriteFile(filepath, data, 0644); err != nil {
		return fmt.Errorf("failed to write deployment record: %w", err)
	}

	return nil
}

// CalculateSummary recalculates the record summary based on the steps.
func (r *DeploymentRecord) CalculateSummary() {
	r.Summary = ExecutionSummary{
		TotalSteps: len(r.Steps),
	}

	for _, step := range r.Steps {
		switch step.Status {
		case RecordStatusSucceeded:
			r.Summary.SucceededSteps++
		case RecordStatusFailed:
			r.Summary.FailedSteps++
		case StepStatusSkipped:
			r.Summary.SkippedSteps++
		}

		r.Summary.ResourcesAdded += step.Changes.Add
		r.Summary.ResourcesChanged += step.Changes.Change
		r.Summary.ResourcesDestroyed += step.Changes.Destroy
	}

	r.Summary.TotalResources = r.Summary.ResourcesAdded + r.Summary.ResourcesChanged + r.Summary.ResourcesDestroyed

	// Determine overall status
	if r.Summary.FailedSteps == 0 {
		r.Status = RecordStatusSucceeded
	} else if r.Summary.SucceededSteps > 0 {
		r.Status = RecordStatusPartial
	} else {
		r.Status = RecordStatusFailed
	}
}

// Duration returns the total deployment duration.
func (r *DeploymentRecord) Duration() time.Duration {
	return r.CompletedAt.Sub(r.StartedAt)
}
