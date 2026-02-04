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
// This record is stored in .radius/deployments/{env}/{timestamp}.json
type DeploymentRecord struct {
	// Application is the name of the application that was deployed.
	Application string `json:"application"`

	// Environment contains information about the deployment environment.
	Environment *EnvironmentInfo `json:"environment"`

	// Plan contains reference information about the plan that was deployed.
	Plan *PlanReference `json:"plan"`

	// Git contains information about the Git state at deployment time.
	Git *GitInfo `json:"git"`

	// CI contains information about the CI/CD system, if applicable.
	CI *CIInfo `json:"ci,omitempty"`

	// Steps contains the results of each deployment step.
	Steps []StepResult `json:"steps"`

	// StartedAt is when the deployment started.
	StartedAt time.Time `json:"startedAt"`

	// CompletedAt is when the deployment completed.
	CompletedAt time.Time `json:"completedAt,omitempty"`

	// Status is the overall deployment status (in-progress, completed, failed).
	Status string `json:"status"`

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

	// AzureSubscriptionID is the Azure subscription ID, if using Azure.
	AzureSubscriptionID string `json:"azureSubscriptionId,omitempty"`

	// AzureResourceGroup is the Azure resource group, if using Azure.
	AzureResourceGroup string `json:"azureResourceGroup,omitempty"`

	// AWSAccountID is the AWS account ID, if using AWS.
	AWSAccountID string `json:"awsAccountId,omitempty"`

	// AWSRegion is the AWS region, if using AWS.
	AWSRegion string `json:"awsRegion,omitempty"`
}

// PlanReference contains reference information about the plan.
type PlanReference struct {
	// Commit is the Git commit SHA where the plan was generated.
	Commit string `json:"commit"`

	// Path is the path to the plan.yaml file.
	Path string `json:"path"`
}

// GitInfo contains information about the Git state at deployment time.
type GitInfo struct {
	// Commit is the full Git commit SHA.
	Commit string `json:"commit"`

	// CommitShort is the short (8 character) Git commit SHA.
	CommitShort string `json:"commitShort"`

	// Branch is the Git branch name.
	Branch string `json:"branch"`

	// Tag is the Git tag, if the commit is tagged.
	Tag string `json:"tag,omitempty"`

	// IsDirty indicates whether the working directory had uncommitted changes.
	// When true, the deployed code may differ from what's recorded in the commit.
	IsDirty bool `json:"isDirty"`
}

// CIInfo contains information about the CI/CD system.
type CIInfo struct {
	// System is the CI/CD system name (e.g., "github-actions", "azure-devops").
	System string `json:"system,omitempty"`

	// BuildID is the CI/CD build identifier.
	BuildID string `json:"buildId,omitempty"`

	// BuildURL is the URL to the CI/CD build.
	BuildURL string `json:"buildUrl,omitempty"`

	// PipelineName is the name of the CI/CD pipeline.
	PipelineName string `json:"pipelineName,omitempty"`
}

// StepResult captures the result of a single deployment step.
type StepResult struct {
	// Sequence is the step sequence number.
	Sequence int `json:"sequence"`

	// Resource contains information about the resource that was deployed.
	Resource *ResourceResult `json:"resource"`

	// Recipe contains information about the recipe that was used.
	Recipe *RecipeResult `json:"recipe"`

	// Status is the step status (pending, in-progress, completed, failed, skipped).
	Status string `json:"status"`

	// StartedAt is when the step started.
	StartedAt time.Time `json:"startedAt,omitempty"`

	// CompletedAt is when the step completed.
	CompletedAt time.Time `json:"completedAt,omitempty"`

	// Changes contains the summary of changes made by this step.
	Changes *ChangesSummary `json:"changes,omitempty"`

	// CloudResources contains information about cloud resources created/modified.
	CloudResources []CloudResource `json:"cloudResources,omitempty"`

	// CapturedResources contains Kubernetes manifests captured after deployment.
	CapturedResources []CapturedResource `json:"capturedResources,omitempty"`

	// Error contains error information if the step failed.
	Error string `json:"error,omitempty"`

	// Logs contains paths to log files for this step.
	Logs *StepLogs `json:"logs,omitempty"`
}

// ResourceResult contains information about a deployed resource.
type ResourceResult struct {
	// Name is the symbolic name of the resource.
	Name string `json:"name"`

	// Type is the resource type.
	Type string `json:"type"`
}

// RecipeResult contains information about the recipe used for deployment.
type RecipeResult struct {
	// Name is the recipe name.
	Name string `json:"name"`

	// Kind is the recipe kind (terraform, bicep).
	Kind string `json:"kind"`

	// Source is the recipe source location.
	Source string `json:"source"`

	// Version is the recipe version that was used.
	Version string `json:"version,omitempty"`
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

// CloudResource represents a cloud resource created or modified by deployment.
type CloudResource struct {
	// Provider is the cloud provider (kubernetes, azure, aws, gcp).
	Provider string `json:"provider"`

	// Type is the resource type (e.g., "kubernetes_deployment", "azurerm_storage_account").
	Type string `json:"type"`

	// Name is the resource name.
	Name string `json:"name"`

	// ID is the resource identifier.
	ID string `json:"id,omitempty"`

	// Namespace is the Kubernetes namespace, if applicable.
	Namespace string `json:"namespace,omitempty"`

	// RawManifest contains the raw Kubernetes manifest YAML, if applicable.
	// This is used for drift detection with `rad diff <commit>...live`.
	RawManifest string `json:"rawManifest,omitempty"`
}

// CapturedResource represents a Kubernetes resource captured after deployment.
type CapturedResource struct {
	// Kind is the Kubernetes resource kind (e.g., "Deployment", "Service").
	Kind string `json:"kind"`

	// Name is the resource name.
	Name string `json:"name"`

	// Namespace is the Kubernetes namespace.
	Namespace string `json:"namespace"`

	// Manifest is the captured YAML manifest.
	Manifest string `json:"manifest"`

	// CapturedAt is when the manifest was captured.
	CapturedAt time.Time `json:"capturedAt"`
}

// StepLogs contains paths to log files for a deployment step.
type StepLogs struct {
	// TerraformPlan is the path to the terraform plan output.
	TerraformPlan string `json:"terraformPlan,omitempty"`

	// TerraformApply is the path to the terraform apply output.
	TerraformApply string `json:"terraformApply,omitempty"`

	// Context is the path to the context file.
	Context string `json:"context,omitempty"`
}

// DeploymentStatus constants
const (
	StatusPending    = "pending"
	StatusInProgress = "in-progress"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
	StatusSkipped    = "skipped"
)

// NewDeploymentRecord creates a new DeploymentRecord.
func NewDeploymentRecord(application string, environment *EnvironmentInfo, planRef *PlanReference, gitInfo *GitInfo) *DeploymentRecord {
	return &DeploymentRecord{
		Application: application,
		Environment: environment,
		Plan:        planRef,
		Git:         gitInfo,
		Steps:       []StepResult{},
		StartedAt:   time.Now(),
		Status:      StatusInProgress,
	}
}

// AddStepResult adds a step result to the deployment record.
func (r *DeploymentRecord) AddStepResult(result StepResult) {
	r.Steps = append(r.Steps, result)
}

// Complete marks the deployment as completed.
func (r *DeploymentRecord) Complete() {
	r.CompletedAt = time.Now()
	r.Status = StatusCompleted
}

// Fail marks the deployment as failed with the given error.
func (r *DeploymentRecord) Fail(err error) {
	r.CompletedAt = time.Now()
	r.Status = StatusFailed
	if err != nil {
		r.Error = err.Error()
	}
}

// Duration returns the duration of the deployment.
func (r *DeploymentRecord) Duration() time.Duration {
	if r.CompletedAt.IsZero() {
		return time.Since(r.StartedAt)
	}
	return r.CompletedAt.Sub(r.StartedAt)
}
