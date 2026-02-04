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

package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/radius-project/radius/pkg/cli/git/deploy"
)

// TerraformExecutor executes Terraform operations for a deployment step.
type TerraformExecutor struct {
	// StepDir is the directory containing the Terraform configuration.
	StepDir string

	// ResourceName is the name of the resource being deployed.
	ResourceName string

	// ResourceType is the type of the resource being deployed.
	ResourceType string

	// RecipeName is the name of the recipe.
	RecipeName string

	// RecipeKind is the kind of recipe (terraform).
	RecipeKind string

	// RecipeSource is the source of the recipe.
	RecipeSource string

	// KubeNamespace is the Kubernetes namespace for the deployment.
	KubeNamespace string

	// KubeContext is the Kubernetes context.
	KubeContext string

	// Sequence is the step sequence number.
	Sequence int
}

// NewTerraformExecutor creates a new TerraformExecutor.
func NewTerraformExecutor(stepDir string) *TerraformExecutor {
	return &TerraformExecutor{
		StepDir:    stepDir,
		RecipeKind: "terraform",
	}
}

// WithResource sets the resource information.
func (e *TerraformExecutor) WithResource(name, resourceType string) *TerraformExecutor {
	e.ResourceName = name
	e.ResourceType = resourceType
	return e
}

// WithRecipe sets the recipe information.
func (e *TerraformExecutor) WithRecipe(name, source string) *TerraformExecutor {
	e.RecipeName = name
	e.RecipeSource = source
	return e
}

// WithKubernetes sets the Kubernetes configuration.
func (e *TerraformExecutor) WithKubernetes(namespace, kubeContext string) *TerraformExecutor {
	e.KubeNamespace = namespace
	e.KubeContext = kubeContext
	return e
}

// WithSequence sets the step sequence number.
func (e *TerraformExecutor) WithSequence(seq int) *TerraformExecutor {
	e.Sequence = seq
	return e
}

// Execute runs terraform init and apply for the step.
func (e *TerraformExecutor) Execute(ctx context.Context) (*deploy.StepResult, error) {
	result := &deploy.StepResult{
		Sequence: e.Sequence,
		Resource: &deploy.ResourceResult{
			Name: e.ResourceName,
			Type: e.ResourceType,
		},
		Recipe: &deploy.RecipeResult{
			Name:   e.RecipeName,
			Kind:   e.RecipeKind,
			Source: e.RecipeSource,
		},
		Status:    deploy.StatusInProgress,
		StartedAt: time.Now(),
	}

	tf, err := tfexec.NewTerraform(e.StepDir, "terraform")
	if err != nil {
		result.Status = deploy.StatusFailed
		result.Error = fmt.Sprintf("failed to create terraform executor: %v", err)
		result.CompletedAt = time.Now()
		return result, fmt.Errorf("failed to create terraform executor: %w", err)
	}

	// Run terraform init
	if err := tf.Init(ctx, tfexec.Upgrade(true)); err != nil {
		result.Status = deploy.StatusFailed
		result.Error = fmt.Sprintf("terraform init failed: %v", err)
		result.CompletedAt = time.Now()
		return result, fmt.Errorf("terraform init failed in %s: %w", e.StepDir, err)
	}

	// Run terraform apply
	if err := tf.Apply(ctx); err != nil {
		result.Status = deploy.StatusFailed
		result.Error = fmt.Sprintf("terraform apply failed: %v", err)
		result.CompletedAt = time.Now()
		return result, fmt.Errorf("terraform apply failed in %s: %w", e.StepDir, err)
	}

	// Parse terraform state for cloud resources
	state, err := tf.Show(ctx)
	if err == nil && state != nil && state.Values != nil && state.Values.RootModule != nil {
		for _, resource := range state.Values.RootModule.Resources {
			cloudResource := deploy.CloudResource{
				Type: resource.Type,
				Name: resource.Name,
			}

			// Determine provider from resource type
			if provider := parseProvider(resource.Type); provider != "" {
				cloudResource.Provider = provider
			}

			// Extract namespace for Kubernetes resources
			if cloudResource.Provider == "kubernetes" {
				if ns, ok := resource.AttributeValues["namespace"].(string); ok {
					cloudResource.Namespace = ns
				}
			}

			result.CloudResources = append(result.CloudResources, cloudResource)
		}
	}

	// Get change summary from plan if available
	planFile := filepath.Join(e.StepDir, "tfplan.txt")
	if _, err := os.Stat(planFile); err == nil {
		result.Logs = &deploy.StepLogs{
			TerraformPlan: planFile,
		}
	}

	result.Status = deploy.StatusCompleted
	result.CompletedAt = time.Now()

	return result, nil
}

// Destroy runs terraform destroy for the step.
func (e *TerraformExecutor) Destroy(ctx context.Context) (*deploy.StepResult, error) {
	result := &deploy.StepResult{
		Sequence: e.Sequence,
		Resource: &deploy.ResourceResult{
			Name: e.ResourceName,
			Type: e.ResourceType,
		},
		Recipe: &deploy.RecipeResult{
			Name:   e.RecipeName,
			Kind:   e.RecipeKind,
			Source: e.RecipeSource,
		},
		Status:    deploy.StatusInProgress,
		StartedAt: time.Now(),
	}

	tf, err := tfexec.NewTerraform(e.StepDir, "terraform")
	if err != nil {
		result.Status = deploy.StatusFailed
		result.Error = fmt.Sprintf("failed to create terraform executor: %v", err)
		result.CompletedAt = time.Now()
		return result, fmt.Errorf("failed to create terraform executor: %w", err)
	}

	// Run terraform init (required before destroy)
	if err := tf.Init(ctx, tfexec.Upgrade(true)); err != nil {
		result.Status = deploy.StatusFailed
		result.Error = fmt.Sprintf("terraform init failed: %v", err)
		result.CompletedAt = time.Now()
		return result, fmt.Errorf("terraform init failed in %s: %w", e.StepDir, err)
	}

	// Run terraform destroy
	if err := tf.Destroy(ctx); err != nil {
		result.Status = deploy.StatusFailed
		result.Error = fmt.Sprintf("terraform destroy failed: %v", err)
		result.CompletedAt = time.Now()
		return result, fmt.Errorf("terraform destroy failed in %s: %w", e.StepDir, err)
	}

	result.Status = deploy.StatusCompleted
	result.CompletedAt = time.Now()

	return result, nil
}

// parseProvider extracts the provider name from a Terraform resource type.
func parseProvider(resourceType string) string {
	if len(resourceType) == 0 {
		return ""
	}

	// Resource types are formatted as provider_resource_type
	parts := splitResourceType(resourceType)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// splitResourceType splits a resource type into provider and resource parts.
func splitResourceType(resourceType string) []string {
	// Common providers
	providers := []string{"kubernetes", "azurerm", "aws", "google", "helm", "null", "random", "local", "time"}

	for _, p := range providers {
		if len(resourceType) > len(p) && resourceType[:len(p)] == p && resourceType[len(p)] == '_' {
			return []string{p, resourceType[len(p)+1:]}
		}
	}

	return []string{resourceType}
}
