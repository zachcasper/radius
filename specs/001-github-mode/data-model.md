# Data Model: Radius on GitHub

**Phase**: 1 (Design & Contracts)  
**Date**: 2026-02-12  
**Plan**: [plan.md](plan.md)

## Overview

This document defines the data model for "Radius on GitHub" feature, extracting entities from the feature specification and mapping to Go types following existing Radius patterns.

---

## Entities

### 1. Workspace (Enhanced)

**Purpose**: Represents a user's working context, now supporting both Kubernetes and GitHub connection types.

**Location**: `~/.rad/config.yaml`

**Structure**:
```go
// WorkspaceSection holds all workspaces (existing, modify property name)
type WorkspaceSection struct {
    Current string                          `yaml:"current"` // Renamed from "default"
    Items   map[string]Workspace            `yaml:"items"`
}

// Workspace represents a workspace entry (existing, add GitHub connection type)
type Workspace struct {
    Source      Source                    // How it was loaded
    Name        string                    // Workspace name
    Connection  Connection                // Connection details
    Environment string                    // Default environment
    Scope       string                    // Default scope (Kubernetes only)
}

// Connection represents workspace connection configuration
type Connection struct {
    Kind    string `yaml:"kind"`    // "kubernetes" or "github"
    // For Kubernetes:
    Context string `yaml:"context,omitempty"`
    // For GitHub:
    URL     string `yaml:"url,omitempty"`
}
```

**Validation Rules**:
- `Kind` must be either `"kubernetes"` or `"github"`
- If `Kind == "github"`, `URL` must be a valid GitHub repository URL
- If `Kind == "kubernetes"`, `Context` must be specified
- `Scope` only applies to Kubernetes workspaces

---

### 2. ResourceTypesManifest

**Purpose**: Declares available resource types in the repository.

**Location**: `.radius/types.yaml`

**Structure**:
```go
// ResourceTypesManifest defines resource types available in the repository
type ResourceTypesManifest struct {
    Types map[string]ResourceTypeEntry `yaml:"types"`
}

// ResourceTypeEntry defines a single resource type reference
type ResourceTypeEntry struct {
    DefinitionLocation string `yaml:"definitionLocation"` // git:: URL format
}
```

**Example**:
```yaml
types:
  Radius.Core/applications:
    definitionLocation: git::https://github.com/radius-project/resource-types-contrib.git//Core/applications/applications.yaml?ref=v0.54.0
  Radius.Compute/containers:
    definitionLocation: git::https://github.com/radius-project/resource-types-contrib.git//Compute/containers/container.yaml?ref=v0.54.0
```

**Validation Rules**:
- `definitionLocation` must be a valid git URL with `git::` prefix
- Must include `?ref=` version specifier for reproducibility
- Resource type names must follow `<Namespace>/<Type>` format

---

### 3. RecipesManifest

**Purpose**: Declares recipes (implementation templates) for provisioning resources.

**Location**: `.radius/recipes.yaml` or referenced file

**Structure**:
```go
// RecipesManifest defines recipes available for resource provisioning
type RecipesManifest struct {
    Recipes map[string]RecipeEntry `yaml:"recipes"`
}

// RecipeEntry defines a single recipe reference
type RecipeEntry struct {
    RecipeKind     string `yaml:"recipeKind"`     // "terraform" or "bicep"
    RecipeLocation string `yaml:"recipeLocation"` // git:: URL or OCI registry URL
}
```

**Example**:
```yaml
recipes:
  Radius.Compute/containers:
    recipeKind: terraform
    recipeLocation: git::https://github.com/radius-project/resource-types-contrib.git//Compute/containers/recipes/aws/terraform?ref=v0.54.0
```

**Validation Rules**:
- `recipeKind` must be `"terraform"` or `"bicep"`
- For Terraform: `recipeLocation` must be a `git::` URL
- For Bicep: `recipeLocation` must be an OCI registry URL (https://...)

---

### 4. Environment

**Purpose**: Defines a target deployment context with cloud provider configuration.

**Location**: `.radius/env.<NAME>.yaml`

**Structure**:
```go
// Environment defines a deployment target environment
type Environment struct {
    Name             string                       `yaml:"name"`
    Kind             string                       `yaml:"kind"`             // "aws" or "azure"
    Recipes          string                       `yaml:"recipes"`          // Path to recipes file
    RecipeParameters map[string]map[string]any   `yaml:"recipeParameters,omitempty"`
    Provider         ProviderConfig               `yaml:"provider"`
}

// ProviderConfig holds cloud provider-specific configuration
type ProviderConfig struct {
    AWS   *AWSProviderConfig   `yaml:"aws,omitempty"`
    Azure *AzureProviderConfig `yaml:"azure,omitempty"`
}

// AWSProviderConfig holds AWS-specific environment configuration
type AWSProviderConfig struct {
    AccountID    string `yaml:"accountId"`
    Region       string `yaml:"region"`
    OIDCRoleARN  string `yaml:"oidcRoleARN"`
    // Terraform state backend
    StateBackend *AWSStateBackend `yaml:"stateBackend,omitempty"`
}

// AWSStateBackend holds AWS S3 state backend configuration
type AWSStateBackend struct {
    Bucket        string `yaml:"bucket"`
    Region        string `yaml:"region"`
    DynamoDBTable string `yaml:"dynamoDBTable"` // For state locking
}

// AzureProviderConfig holds Azure-specific environment configuration
type AzureProviderConfig struct {
    SubscriptionID    string `yaml:"subscriptionId"`
    TenantID          string `yaml:"tenantId"`
    ClientID          string `yaml:"clientId"`
    ResourceGroupName string `yaml:"resourceGroupName"`
    OIDCEnabled       bool   `yaml:"oidcEnabled"`
    // Terraform state backend
    StateBackend *AzureStateBackend `yaml:"stateBackend,omitempty"`
}

// AzureStateBackend holds Azure Storage state backend configuration
type AzureStateBackend struct {
    StorageAccountName string `yaml:"storageAccountName"`
    ContainerName      string `yaml:"containerName"`
}
```

**Validation Rules**:
- `Kind` must be `"aws"` or `"azure"`
- The provider config matching `Kind` must be populated
- State backend is required after `rad environment connect` completes

---

### 5. DeploymentPlan

**Purpose**: Ordered sequence of resource provisioning steps.

**Location**: `.radius/plan/<APP>/<ENV>/plan.yaml`

**Structure**:
```go
// DeploymentPlan defines the deployment plan for an application
type DeploymentPlan struct {
    Application          string            `yaml:"application"`
    ApplicationModelFile string            `yaml:"applicationModelFile"`
    Environment          string            `yaml:"environment"`
    Steps                []DeploymentStep  `yaml:"steps"`
    Summary              PlanSummary       `yaml:"summary"`
}

// DeploymentStep defines a single step in the deployment plan
type DeploymentStep struct {
    Sequence            int                `yaml:"sequence"`
    Resource            ResourceReference  `yaml:"resource"`
    Recipe              RecipeReference    `yaml:"recipe"`
    DeploymentArtifacts string             `yaml:"deploymentArtifacts"` // Directory path
    ExpectedChanges     ChangeCount        `yaml:"expectedChanges"`
    Status              string             `yaml:"status"` // "planned", "executing", "succeeded", "failed", "skipped"
}

// ResourceReference identifies the resource being deployed
type ResourceReference struct {
    Name       string         `yaml:"name"`
    Type       string         `yaml:"type"`
    Properties map[string]any `yaml:"properties"`
}

// RecipeReference identifies the recipe used for deployment
type RecipeReference struct {
    Name     string `yaml:"name"`
    Kind     string `yaml:"kind"`     // "terraform" or "bicep"
    Location string `yaml:"location"`
}

// ChangeCount summarizes expected resource changes
type ChangeCount struct {
    Add     int `yaml:"add"`
    Change  int `yaml:"change"`
    Destroy int `yaml:"destroy"`
}

// PlanSummary aggregates plan statistics
type PlanSummary struct {
    TotalSteps       int  `yaml:"totalSteps"`
    TerraformSteps   int  `yaml:"terraformSteps"`
    BicepSteps       int  `yaml:"bicepSteps"`
    TotalAdd         int  `yaml:"totalAdd"`
    TotalChange      int  `yaml:"totalChange"`
    TotalDestroy     int  `yaml:"totalDestroy"`
    AllVersionsPinned bool `yaml:"allVersionsPinned"`
}
```

**State Transitions**:
- `planned` → `executing` → `succeeded` | `failed`
- `planned` → `skipped` (if dependency failed)

---

### 6. DeploymentRecord

**Purpose**: Complete audit of a deployment execution.

**Location**: `.radius/deploy/<APP>/<ENV>/<COMMIT>/deploy-<COMMIT>.json`

**Structure**:
```go
// DeploymentRecord captures the full deployment execution audit
type DeploymentRecord struct {
    Application  string            `json:"application"`
    Environment  EnvironmentInfo   `json:"environment"`
    StartedAt    time.Time         `json:"startedAt"`
    CompletedAt  time.Time         `json:"completedAt"`
    Status       string            `json:"status"` // "succeeded", "failed", "partial"
    Git          GitContext        `json:"git"`
    Plan         PlanReference     `json:"plan"`
    Steps        []ExecutedStep    `json:"steps"`
    Resources    []ResourceInfo    `json:"resources"`
    Summary      ExecutionSummary  `json:"summary"`
}

// EnvironmentInfo provides environment context for the deployment
type EnvironmentInfo struct {
    Name                  string `json:"name"`
    EnvironmentFile       string `json:"environmentFile"`
    KubernetesContext     string `json:"kubernetesContext,omitempty"`
    KubernetesNamespace   string `json:"kubernetesNamespace,omitempty"`
}

// GitContext provides git information at deployment time
type GitContext struct {
    Commit      string `json:"commit"`
    CommitShort string `json:"commitShort"`
    Branch      string `json:"branch"`
    IsDirty     bool   `json:"isDirty"`
}

// PlanReference links back to the plan used
type PlanReference struct {
    PlanFile    string    `json:"planFile"`
    PlanCommit  string    `json:"planCommit"`
    GeneratedAt time.Time `json:"generatedAt"`
}

// ExecutedStep records execution details for a deployment step
type ExecutedStep struct {
    Sequence          int                    `json:"sequence"`
    Name              string                 `json:"name"`
    ResourceType      string                 `json:"resourceType"`
    Tool              string                 `json:"tool"` // "terraform" or "bicep"
    Status            string                 `json:"status"`
    StartedAt         time.Time              `json:"startedAt"`
    CompletedAt       time.Time              `json:"completedAt"`
    Duration          time.Duration          `json:"duration"`
    Changes           ChangeCount            `json:"changes"`
    Outputs           map[string]any         `json:"outputs,omitempty"`
    CapturedResources []CapturedResource     `json:"capturedResources,omitempty"`
    Error             *ErrorInfo             `json:"error,omitempty"`
}

// CapturedResource links to the captured resource definition
type CapturedResource struct {
    ResourceID             string `json:"resourceId"`
    ResourceDefinitionFile string `json:"resourceDefinitionFile"`
}

// ErrorInfo captures error details when a step fails
type ErrorInfo struct {
    Message    string `json:"message"`
    Details    string `json:"details,omitempty"`
    LogFile    string `json:"logFile,omitempty"`
}

// ExecutionSummary aggregates execution statistics
type ExecutionSummary struct {
    TotalSteps          int `json:"totalSteps"`
    SucceededSteps      int `json:"succeededSteps"`
    FailedSteps         int `json:"failedSteps"`
    SkippedSteps        int `json:"skippedSteps"`
    TotalResources      int `json:"totalResources"`
    ResourcesAdded      int `json:"resourcesAdded"`
    ResourcesChanged    int `json:"resourcesChanged"`
    ResourcesDestroyed  int `json:"resourcesDestroyed"`
}
```

---

### 7. DestructionRecord

**Purpose**: Audit record for resource teardown.

**Location**: `.radius/deploy/<APP>/<ENV>/<COMMIT>/destroy-<COMMIT>.json`

**Structure**: Same as `DeploymentRecord` with `status` reflecting destruction outcome.

---

## Relationships

```
┌─────────────────┐     ┌────────────────────┐
│    Workspace    │────▶│   Environment      │
│  (config.yaml)  │     │ (env.<name>.yaml)  │
└─────────────────┘     └────────────────────┘
                               │
                               ▼
┌─────────────────┐     ┌────────────────────┐
│  ResourceTypes  │◀────│   RecipesManifest  │
│  (types.yaml)   │     │  (recipes.yaml)    │
└─────────────────┘     └────────────────────┘
                               │
                               ▼
┌─────────────────┐     ┌────────────────────┐
│ DeploymentPlan  │────▶│ DeploymentRecord   │
│   (plan.yaml)   │     │ (deploy-*.json)    │
└─────────────────┘     └────────────────────┘
```

---

## File System Layout

```
repository/
├── .radius/
│   ├── types.yaml              # ResourceTypesManifest
│   ├── recipes.yaml            # RecipesManifest
│   ├── env.default.yaml        # Environment (default)
│   ├── env.production.yaml     # Environment (production)
│   ├── model/
│   │   └── <app>.bicep         # Application models
│   ├── plan/
│   │   └── <app>/
│   │       └── <env>/
│   │           ├── plan.yaml   # DeploymentPlan
│   │           └── 001-<res>-terraform/
│   │               ├── main.tf
│   │               ├── providers.tf
│   │               ├── variables.tf
│   │               ├── terraform.tfvars.json
│   │               ├── tfplan.txt
│   │               ├── terraform-context.txt
│   │               └── .terraform.lock.hcl
│   └── deploy/
│       └── <app>/
│           └── <env>/
│               └── <commit>/
│                   ├── deploy-<commit>.json  # DeploymentRecord
│                   ├── destroy-<commit>.json # DestructionRecord
│                   └── *.yaml                # Captured resources
└── ~/.rad/
    └── config.yaml              # WorkspaceSection with Workspaces
```
