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
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashicorp/terraform-exec/tfexec"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/radius-project/radius/pkg/cli/git/deploy"
)

// TerraformExecutor executes Terraform operations for a deployment step.
type TerraformExecutor struct {
	// StepDir is the directory containing the Terraform configuration.
	StepDir string

	// ResourceName is the name of the resource being deployed.
	ResourceName string

	// ResourceType is the type of the resource being deployed (with API version).
	ResourceType string

	// RecipeName is the name of the recipe.
	RecipeName string

	// RecipeSource is the source of the recipe.
	RecipeSource string

	// KubeNamespace is the Kubernetes namespace for the deployment.
	KubeNamespace string

	// KubeContext is the Kubernetes context.
	KubeContext string

	// Sequence is the step sequence number.
	Sequence int

	// DeployDir is the directory where deployment artifacts are saved.
	DeployDir string
}

// NewTerraformExecutor creates a new TerraformExecutor.
func NewTerraformExecutor(stepDir string) *TerraformExecutor {
	return &TerraformExecutor{
		StepDir: stepDir,
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

// WithDeployDir sets the deployment directory for saving resource manifests.
func (e *TerraformExecutor) WithDeployDir(dir string) *TerraformExecutor {
	e.DeployDir = dir
	return e
}

// Execute runs terraform init and apply for the step.
func (e *TerraformExecutor) Execute(ctx context.Context) (*deploy.StepResult, error) {
	startTime := time.Now()
	result := &deploy.StepResult{
		Sequence:     e.Sequence,
		Name:         e.ResourceName,
		ResourceType: e.ResourceType,
		Tool:         "terraform",
		Status:       deploy.StatusInProgress,
		StartedAt:    startTime,
	}

	tf, err := tfexec.NewTerraform(e.StepDir, "terraform")
	if err != nil {
		result.Status = deploy.StatusFailed
		result.Error = fmt.Sprintf("failed to create terraform executor: %v", err)
		result.CompletedAt = time.Now()
		result.Duration = time.Since(startTime).Nanoseconds()
		return result, fmt.Errorf("failed to create terraform executor: %w", err)
	}

	// Run terraform init
	if err := tf.Init(ctx, tfexec.Upgrade(true)); err != nil {
		result.Status = deploy.StatusFailed
		result.Error = fmt.Sprintf("terraform init failed: %v", err)
		result.CompletedAt = time.Now()
		result.Duration = time.Since(startTime).Nanoseconds()
		return result, fmt.Errorf("terraform init failed in %s: %w", e.StepDir, err)
	}

	// Run terraform plan to get change counts
	hasChanges, err := tf.Plan(ctx)
	if err != nil {
		result.Status = deploy.StatusFailed
		result.Error = fmt.Sprintf("terraform plan failed: %v", err)
		result.CompletedAt = time.Now()
		result.Duration = time.Since(startTime).Nanoseconds()
		return result, fmt.Errorf("terraform plan failed in %s: %w", e.StepDir, err)
	}

	// Parse plan output for change counts
	result.Changes = &deploy.ChangesSummary{
		Add:     0,
		Change:  0,
		Destroy: 0,
	}
	if hasChanges {
		// Show the plan to get detailed change info
		planFile := filepath.Join(e.StepDir, "tfplan")
		_, err = tf.Plan(ctx, tfexec.Out(planFile))
		if err == nil {
			plan, err := tf.ShowPlanFile(ctx, planFile)
			if err == nil && plan != nil && plan.ResourceChanges != nil {
				for _, change := range plan.ResourceChanges {
					if change.Change != nil {
						for _, action := range change.Change.Actions {
							switch action {
							case tfjson.ActionCreate:
								result.Changes.Add++
							case tfjson.ActionUpdate:
								result.Changes.Change++
							case tfjson.ActionDelete:
								result.Changes.Destroy++
							}
						}
					}
				}
			}
		}
	}

	// Run terraform apply
	if err := tf.Apply(ctx); err != nil {
		result.Status = deploy.StatusFailed
		result.Error = fmt.Sprintf("terraform apply failed: %v", err)
		result.CompletedAt = time.Now()
		result.Duration = time.Since(startTime).Nanoseconds()
		return result, fmt.Errorf("terraform apply failed in %s: %w", e.StepDir, err)
	}

	// Get terraform outputs
	outputs, err := tf.Output(ctx)
	if err == nil && len(outputs) > 0 {
		result.Outputs = make(map[string]any)
		for name, output := range outputs {
			var value any
			if err := json.Unmarshal(output.Value, &value); err == nil {
				result.Outputs[name] = value
			}
		}
	}

	// Parse terraform state for Kubernetes resources and capture them
	state, err := tf.Show(ctx)
	if err == nil && state != nil && state.Values != nil {
		// Collect all resources from root module and child modules
		var allResources []*tfjson.StateResource
		if state.Values.RootModule != nil {
			allResources = append(allResources, state.Values.RootModule.Resources...)
			allResources = append(allResources, collectChildResources(state.Values.RootModule.ChildModules)...)
		}

		for _, resource := range allResources {
			provider := parseProvider(resource.Type)
			if provider == "kubernetes" {
				captured := e.captureKubernetesResource(ctx, resource)
				if captured != nil {
					result.CapturedResources = append(result.CapturedResources, *captured)
				}
			}
		}
	}

	result.Status = deploy.StatusSucceeded
	result.CompletedAt = time.Now()
	result.Duration = time.Since(startTime).Nanoseconds()

	return result, nil
}

// captureKubernetesResource captures a Kubernetes resource from the cluster and saves it to a YAML file.
func (e *TerraformExecutor) captureKubernetesResource(ctx context.Context, resource *tfjson.StateResource) *deploy.CapturedResource {
	// Extract resource type from terraform type (e.g., kubernetes_deployment -> deployment)
	resourceType := strings.TrimPrefix(resource.Type, "kubernetes_")

	// Get name and namespace from metadata (kubernetes provider stores these under metadata[0])
	var name, namespace string
	if metadata, ok := resource.AttributeValues["metadata"].([]interface{}); ok && len(metadata) > 0 {
		if meta, ok := metadata[0].(map[string]interface{}); ok {
			name, _ = meta["name"].(string)
			namespace, _ = meta["namespace"].(string)
		}
	}
	if namespace == "" {
		namespace = e.KubeNamespace
	}
	if name == "" || namespace == "" {
		return nil
	}

	// Build kubectl command to get resource as YAML
	args := []string{"get", resourceType, name, "-n", namespace, "-o", "yaml"}
	if e.KubeContext != "" {
		args = append([]string{"--context", e.KubeContext}, args...)
	}

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	yamlOutput, err := cmd.Output()
	if err != nil {
		return nil
	}

	// Generate filename: <resource-type>-<name>.yaml
	filename := fmt.Sprintf("%s-%s.yaml", resourceType, name)

	// Save YAML to file in deploy directory if specified
	if e.DeployDir != "" {
		filePath := filepath.Join(e.DeployDir, filename)
		if err := os.WriteFile(filePath, yamlOutput, 0644); err != nil {
			return nil
		}
	}

	resourceID := fmt.Sprintf("%s/%s/%s", namespace, resourceType, name)

	return &deploy.CapturedResource{
		ResourceID:             resourceID,
		ResourceDefinitionFile: filename,
	}
}

// Destroy runs terraform destroy for the step.
func (e *TerraformExecutor) Destroy(ctx context.Context) (*deploy.StepResult, error) {
	startTime := time.Now()
	result := &deploy.StepResult{
		Sequence:     e.Sequence,
		Name:         e.ResourceName,
		ResourceType: e.ResourceType,
		Tool:         "terraform",
		Status:       deploy.StatusInProgress,
		StartedAt:    startTime,
	}

	tf, err := tfexec.NewTerraform(e.StepDir, "terraform")
	if err != nil {
		result.Status = deploy.StatusFailed
		result.Error = fmt.Sprintf("failed to create terraform executor: %v", err)
		result.CompletedAt = time.Now()
		result.Duration = time.Since(startTime).Nanoseconds()
		return result, fmt.Errorf("failed to create terraform executor: %w", err)
	}

	// Run terraform init (required before destroy)
	if err := tf.Init(ctx, tfexec.Upgrade(true)); err != nil {
		result.Status = deploy.StatusFailed
		result.Error = fmt.Sprintf("terraform init failed: %v", err)
		result.CompletedAt = time.Now()
		result.Duration = time.Since(startTime).Nanoseconds()
		return result, fmt.Errorf("terraform init failed in %s: %w", e.StepDir, err)
	}

	// Run terraform destroy
	if err := tf.Destroy(ctx); err != nil {
		result.Status = deploy.StatusFailed
		result.Error = fmt.Sprintf("terraform destroy failed: %v", err)
		result.CompletedAt = time.Now()
		result.Duration = time.Since(startTime).Nanoseconds()
		return result, fmt.Errorf("terraform destroy failed in %s: %w", e.StepDir, err)
	}

	result.Status = deploy.StatusSucceeded
	result.CompletedAt = time.Now()
	result.Duration = time.Since(startTime).Nanoseconds()

	return result, nil
}

// parseProvider extracts the provider name from a Terraform resource type.
func parseProvider(resourceType string) string {
	if len(resourceType) == 0 {
		return ""
	}

	// Common providers
	providers := []string{"kubernetes", "azurerm", "aws", "google", "helm", "null", "random", "local", "time"}

	for _, p := range providers {
		if len(resourceType) > len(p) && resourceType[:len(p)] == p && resourceType[len(p)] == '_' {
			return p
		}
	}

	return ""
}

// collectChildResources recursively collects resources from child modules.
func collectChildResources(modules []*tfjson.StateModule) []*tfjson.StateResource {
	var resources []*tfjson.StateResource
	for _, mod := range modules {
		if mod == nil {
			continue
		}
		resources = append(resources, mod.Resources...)
		resources = append(resources, collectChildResources(mod.ChildModules)...)
	}
	return resources
}
