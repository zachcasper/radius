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

package github

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateDeployWorkflow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		provider        string
		expectAWSAuth   bool
		expectAzureAuth bool
	}{
		{
			name:            "aws provider",
			provider:        "aws",
			expectAWSAuth:   true,
			expectAzureAuth: false,
		},
		{
			name:            "azure provider",
			provider:        "azure",
			expectAWSAuth:   false,
			expectAzureAuth: true,
		},
		{
			name:            "no provider",
			provider:        "",
			expectAWSAuth:   false,
			expectAzureAuth: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			workflow := GenerateDeployWorkflow(tt.provider)

			assert.Equal(t, "Radius Deploy", workflow.Name)
			assert.NotNil(t, workflow.On.PullRequest)
			assert.Contains(t, workflow.On.PullRequest.Branches, "main")
			assert.Contains(t, workflow.On.PullRequest.Types, "closed")

			assert.NotNil(t, workflow.Permissions)
			assert.Equal(t, "write", workflow.Permissions["id-token"])
			assert.Equal(t, "write", workflow.Permissions["contents"])

			assert.NotNil(t, workflow.Concurrency)
			assert.Contains(t, workflow.Concurrency.Group, "radius-deploy")

			job, exists := workflow.Jobs["deploy"]
			require.True(t, exists)
			assert.Equal(t, "ubuntu-latest", job.RunsOn)
			assert.Contains(t, job.If, "merged == true")
			assert.Contains(t, job.If, "deploy/")

			// Check for cloud auth steps
			hasAWSAuth := false
			hasAzureAuth := false
			for _, step := range job.Steps {
				if step.Uses == "aws-actions/configure-aws-credentials@v4" {
					hasAWSAuth = true
				}
				if step.Uses == "azure/login@v2" {
					hasAzureAuth = true
				}
			}
			assert.Equal(t, tt.expectAWSAuth, hasAWSAuth)
			assert.Equal(t, tt.expectAzureAuth, hasAzureAuth)
		})
	}
}

func TestGenerateDestroyWorkflow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		provider        string
		expectAWSAuth   bool
		expectAzureAuth bool
	}{
		{
			name:            "aws provider",
			provider:        "aws",
			expectAWSAuth:   true,
			expectAzureAuth: false,
		},
		{
			name:            "azure provider",
			provider:        "azure",
			expectAWSAuth:   false,
			expectAzureAuth: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			workflow := GenerateDestroyWorkflow(tt.provider)

			assert.Equal(t, "Radius Destroy", workflow.Name)
			assert.NotNil(t, workflow.On.PullRequest)

			job, exists := workflow.Jobs["destroy"]
			require.True(t, exists)
			assert.Contains(t, job.If, "merged == true")
			assert.Contains(t, job.If, "destroy/")

			// Check for cloud auth steps
			hasAWSAuth := false
			hasAzureAuth := false
			for _, step := range job.Steps {
				if step.Uses == "aws-actions/configure-aws-credentials@v4" {
					hasAWSAuth = true
				}
				if step.Uses == "azure/login@v2" {
					hasAzureAuth = true
				}
			}
			assert.Equal(t, tt.expectAWSAuth, hasAWSAuth)
			assert.Equal(t, tt.expectAzureAuth, hasAzureAuth)
		})
	}
}

func TestGeneratePlanWorkflow(t *testing.T) {
	t.Parallel()

	workflow := GeneratePlanWorkflow("aws")

	assert.Equal(t, "Radius Plan", workflow.Name)
	assert.NotNil(t, workflow.On.WorkflowDispatch)

	// Check inputs
	inputs := workflow.On.WorkflowDispatch.Inputs
	envInput, exists := inputs["environment"]
	require.True(t, exists)
	assert.True(t, envInput.Required)
	assert.Equal(t, "string", envInput.Type)

	appInput, exists := inputs["application"]
	require.True(t, exists)
	assert.False(t, appInput.Required)

	// Check permissions
	assert.Equal(t, "write", workflow.Permissions["pull-requests"])

	// Check job
	job, exists := workflow.Jobs["plan"]
	require.True(t, exists)
	assert.Equal(t, "ubuntu-latest", job.RunsOn)

	// Verify steps include PR creation
	hasPRCreation := false
	for _, step := range job.Steps {
		if step.Run != "" && contains(step.Run, "gh pr create") {
			hasPRCreation = true
		}
	}
	assert.True(t, hasPRCreation)
}

func TestSaveAndLoadWorkflow(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	workflowPath := filepath.Join(tmpDir, "test-workflow.yaml")

	// Create a workflow
	workflow := &Workflow{
		Name: "Test Workflow",
		On: WorkflowTrigger{
			Push: &PushTrigger{
				Branches: []string{"main"},
			},
		},
		Jobs: map[string]WorkflowJob{
			"test": {
				Name:   "Test Job",
				RunsOn: "ubuntu-latest",
				Steps: []WorkflowStep{
					{
						Name: "Checkout",
						Uses: "actions/checkout@v4",
					},
				},
			},
		},
	}

	// Save
	err := SaveWorkflow(workflowPath, workflow)
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(workflowPath)
	require.NoError(t, err)

	// Load
	loaded, err := LoadWorkflow(workflowPath)
	require.NoError(t, err)

	// Verify loaded content matches
	assert.Equal(t, workflow.Name, loaded.Name)
	assert.Equal(t, workflow.On.Push.Branches, loaded.On.Push.Branches)
	assert.Len(t, loaded.Jobs, 1)

	job := loaded.Jobs["test"]
	assert.Equal(t, "Test Job", job.Name)
	assert.Len(t, job.Steps, 1)
}

func TestSaveWorkflow_AddsHeader(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	workflowPath := filepath.Join(tmpDir, "header-test.yaml")

	workflow := &Workflow{
		Name: "Header Test",
		On: WorkflowTrigger{
			Push: &PushTrigger{
				Branches: []string{"main"},
			},
		},
		Jobs: map[string]WorkflowJob{},
	}

	err := SaveWorkflow(workflowPath, workflow)
	require.NoError(t, err)

	content, err := os.ReadFile(workflowPath)
	require.NoError(t, err)

	assert.Contains(t, string(content), "# Radius deployment workflow")
	assert.Contains(t, string(content), "# Generated by rad init")
}

func TestLoadWorkflow_InvalidFile(t *testing.T) {
	t.Parallel()

	t.Run("file not found", func(t *testing.T) {
		t.Parallel()
		_, err := LoadWorkflow("/nonexistent/path/workflow.yaml")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read workflow")
	})

	t.Run("invalid yaml", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		invalidPath := filepath.Join(tmpDir, "invalid.yaml")
		err := os.WriteFile(invalidPath, []byte("not: valid: yaml: content:"), 0644)
		require.NoError(t, err)

		_, err = LoadWorkflow(invalidPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse workflow")
	})
}

func TestWorkflowJob_StepOrdering(t *testing.T) {
	t.Parallel()

	workflow := GenerateDeployWorkflow("aws")
	job := workflow.Jobs["deploy"]

	// Verify step ordering makes sense
	stepNames := make([]string, len(job.Steps))
	for i, step := range job.Steps {
		stepNames[i] = step.Name
	}

	// Checkout should be first
	assert.Equal(t, "Checkout", stepNames[0])

	// Find indices
	checkoutIdx := -1
	authIdx := -1
	deployIdx := -1
	commitIdx := -1

	for i, name := range stepNames {
		switch name {
		case "Checkout":
			checkoutIdx = i
		case "Configure AWS credentials":
			authIdx = i
		case "Run deployment":
			deployIdx = i
		case "Commit deployment results":
			commitIdx = i
		}
	}

	// Order should be: checkout < auth < deploy < commit
	assert.True(t, checkoutIdx < authIdx, "checkout should be before auth")
	assert.True(t, authIdx < deployIdx, "auth should be before deploy")
	assert.True(t, deployIdx < commitIdx, "deploy should be before commit")
}

func TestWorkflowConcurrency(t *testing.T) {
	t.Parallel()

	workflow := GenerateDeployWorkflow("aws")

	require.NotNil(t, workflow.Concurrency)
	assert.Contains(t, workflow.Concurrency.Group, "radius-deploy")
	assert.Contains(t, workflow.Concurrency.Group, "github.event.pull_request.head.ref")
	assert.False(t, workflow.Concurrency.CancelInProgress, "should not cancel in-progress deployments")
}

// Helper function
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

func TestGenerateAuthTestWorkflow_AWS(t *testing.T) {
	t.Parallel()

	workflow := GenerateAuthTestWorkflow("aws")

	// FR-031-A: Workflow name and structure
	assert.Equal(t, "Radius Auth Test", workflow.Name)

	// FR-031-B: Triggered by push to env files
	require.NotNil(t, workflow.On.Push)
	assert.Contains(t, workflow.On.Push.Paths, ".radius/env.*.yaml")

	// Must have id-token write permission for OIDC
	assert.Equal(t, "write", workflow.Permissions["id-token"])

	// FR-031-C: AWS auth test job
	job, exists := workflow.Jobs["test-aws-auth"]
	require.True(t, exists, "should have test-aws-auth job")
	assert.Equal(t, "Test AWS OIDC Authentication", job.Name)
	assert.Equal(t, "ubuntu-latest", job.RunsOn)

	// Should have step to parse env file
	hasCheckout := false
	hasEnvParse := false
	hasAWSAuth := false
	hasIdentityCheck := false
	for _, step := range job.Steps {
		if step.Uses == "actions/checkout@v4" {
			hasCheckout = true
			// D011 fix: Need fetch-depth for git diff
			assert.Equal(t, "2", step.With["fetch-depth"])
		}
		if step.ID == "env" && step.Name == "Parse environment config" {
			hasEnvParse = true
			assert.Contains(t, step.Run, "yq")
			assert.Contains(t, step.Run, ".provider.aws.oidcRoleARN")
			// D011 fix: Should validate required fields
			assert.Contains(t, step.Run, "Error: oidcRoleARN not found")
		}
		if step.Uses == "aws-actions/configure-aws-credentials@v4" {
			hasAWSAuth = true
			// Should use env outputs, not secrets
			assert.Equal(t, "${{ steps.env.outputs.role-arn }}", step.With["role-to-assume"])
			assert.Equal(t, "${{ steps.env.outputs.region }}", step.With["aws-region"])
		}
		if step.Name == "Verify AWS authentication" {
			hasIdentityCheck = true
			assert.Contains(t, step.Run, "aws sts get-caller-identity")
		}
	}
	assert.True(t, hasCheckout, "should have checkout step")
	assert.True(t, hasEnvParse, "should parse env file")
	assert.True(t, hasAWSAuth, "should have AWS credentials step")
	assert.True(t, hasIdentityCheck, "should verify AWS identity")
}

func TestGenerateAuthTestWorkflow_Azure(t *testing.T) {
	t.Parallel()

	workflow := GenerateAuthTestWorkflow("azure")

	// FR-031-A: Workflow name and structure
	assert.Equal(t, "Radius Auth Test", workflow.Name)

	// FR-031-B: Triggered by push to env files
	require.NotNil(t, workflow.On.Push)
	assert.Contains(t, workflow.On.Push.Paths, ".radius/env.*.yaml")

	// Must have id-token write permission for OIDC
	assert.Equal(t, "write", workflow.Permissions["id-token"])

	// FR-031-D: Azure auth test job
	job, exists := workflow.Jobs["test-azure-auth"]
	require.True(t, exists, "should have test-azure-auth job")
	assert.Equal(t, "Test Azure OIDC Authentication", job.Name)
	assert.Equal(t, "ubuntu-latest", job.RunsOn)

	// Should have step to parse env file
	hasCheckout := false
	hasEnvParse := false
	hasAzureAuth := false
	hasAccountCheck := false
	for _, step := range job.Steps {
		if step.Uses == "actions/checkout@v4" {
			hasCheckout = true
			// D011 fix: Need fetch-depth for git diff
			assert.Equal(t, "2", step.With["fetch-depth"])
		}
		if step.ID == "env" && step.Name == "Parse environment config" {
			hasEnvParse = true
			assert.Contains(t, step.Run, "yq")
			assert.Contains(t, step.Run, ".provider.azure.clientId")
			// D011 fix: Should validate required fields
			assert.Contains(t, step.Run, "Error: clientId not found")
			assert.Contains(t, step.Run, "Error: tenantId not found")
		}
		if step.Uses == "azure/login@v2" {
			hasAzureAuth = true
			// D011 fix: Should use env outputs, not secrets
			assert.Equal(t, "${{ steps.env.outputs.client-id }}", step.With["client-id"])
			assert.Equal(t, "${{ steps.env.outputs.tenant-id }}", step.With["tenant-id"])
			assert.Equal(t, "${{ steps.env.outputs.subscription-id }}", step.With["subscription-id"])
		}
		if step.Name == "Verify Azure authentication" {
			hasAccountCheck = true
			assert.Contains(t, step.Run, "az account show")
		}
	}
	assert.True(t, hasCheckout, "should have checkout step")
	assert.True(t, hasEnvParse, "should parse env file")
	assert.True(t, hasAzureAuth, "should have Azure login step")
	assert.True(t, hasAccountCheck, "should verify Azure account")
}
