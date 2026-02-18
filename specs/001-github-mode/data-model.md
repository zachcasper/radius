# Data Model: Radius on GitHub

**Phase**: 1 (Design & Contracts)  
**Date**: 2026-02-16 (updated from 2026-02-12)  
**Plan**: [plan.md](plan.md)

## Overview

This document defines the data model for "Radius on GitHub" feature, extracting entities from the feature specification and mapping to Go types following existing Radius patterns. Updated to reflect the two-phase deployment model, GitHub Environment variables (replacing local env files), and commit-hash scoped deployment artifacts.

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

### 2. GitHub Environment (External)

**Purpose**: Stores cloud provider configuration and recipes manifest reference as GitHub Environment-scoped variables. Replaces local `.radius/env.<name>.yaml` files.

**Location**: GitHub Environments API (not stored locally)

**Structure**:
```go
// GitHubEnvironment represents the environment configuration stored in GitHub.
// This is not persisted locally — it is read/written via the GitHub API.
type GitHubEnvironment struct {
    Name      string                     // GitHub Environment name (e.g., "dev", "prod")
    Provider  string                     // "aws" or "azure"
    Variables map[string]string           // Environment-scoped variables
}
```

**Azure Variables** (FR-025):
| Variable | Example |
|----------|--------|
| `AZURE_SUBSCRIPTION_ID` | `12345678-1234-...` |
| `AZURE_RESOURCE_GROUP_NAME` | `rg-radius-dev` |
| `AKS_CLUSTER_NAME` | `aks-dev-cluster` |
| `KUBERNETES_NAMESPACE` | `default` |
| `AZURE_TENANT_ID` | `87654321-4321-...` |
| `AZURE_CLIENT_ID` | `abcdefgh-abcd-...` |
| `RADIUS_RECIPES_MANIFEST` | `https://github.com/zachcasper/radius-config/recipes-azure-terraform.yaml` |

**AWS Variables** (FR-029):
| Variable | Example |
|----------|--------|
| `AWS_ACCOUNT_ID` | `123456789012` |
| `AWS_REGION` | `us-east-1` |
| `AWS_IAM_ROLE_NAME` | `radius-github-oidc` |
| `EKS_CLUSTER_NAME` | `eks-staging-cluster` |
| `KUBERNETES_NAMESPACE` | `default` |
| `RADIUS_RECIPES_MANIFEST` | `https://github.com/zachcasper/radius-config/recipes-aws-terraform.yaml` |

**Validation Rules**:
- Provider determined by variable presence (`AZURE_CLIENT_ID` → Azure, `AWS_IAM_ROLE_NAME` → AWS)
- All variables for the selected provider must be present
- `RADIUS_RECIPES_MANIFEST` must be a valid URL

---

### 3. ResourceTypesManifest (Repository Variable)

**Purpose**: References the external manifest of available resource types.

**Location**: GitHub repository variable `RADIUS_RESOURCE_TYPES_MANIFEST` (set during `rad init`)

**Value**: URL to types.yaml in config repository (e.g., `https://github.com/zachcasper/radius-config/types.yaml`)

**Note**: The types.yaml file itself is hosted externally, not stored in the repository. The repository variable is a URL reference only.

**Validation Rules**:
- Resource type names must follow `<Namespace>/<Type>` format (in the external manifest)
- The manifest URL must be accessible from GitHub Actions runners

---

### RecipesManifest (Environment Variable)

**Purpose**: References the external recipes manifest for a specific environment.

**Location**: GitHub Environment variable `RADIUS_RECIPES_MANIFEST` (per-environment)

**Value**: URL to recipes.yaml in config repository (e.g., `https://github.com/zachcasper/radius-config/recipes-aws-terraform.yaml`)

**Note**: The recipes manifest is referenced by URL in the GitHub Environment variable, not stored locally. The manifest is only read at workflow execution time by the Radius control plane.

---

### 4. DeploymentPlan

**Purpose**: Ordered sequence of resource provisioning steps generated by `rad deployment create`.

**Location**: `.radius/deploy/<APP>/<ENV>/<COMMIT_HASH>/deploy.yaml`

**Structure**:
```go
// DeploymentPlan defines the deployment plan for an application
type DeploymentPlan struct {
    Application              string            `yaml:"application"`
    ApplicationDefinitionFile string           `yaml:"applicationDefinitionFile"`
    Environment              string            `yaml:"environment"`
    Commit                   string            `yaml:"commit"`             // Short commit hash
    Steps                    []DeploymentStep  `yaml:"steps"`
    Summary                  PlanSummary       `yaml:"summary"`
}

// DeploymentStep defines a single step in the deployment plan
type DeploymentStep struct {
    Sequence            int                `yaml:"sequence"`
    Resource            ResourceReference  `yaml:"resource"`
    Recipe              RecipeReference    `yaml:"recipe"`
    DeploymentArtifacts string             `yaml:"deploymentArtifacts"` // Directory path
    ExpectedChanges     ChangeCount        `yaml:"expectedChanges"`
    Status              string             `yaml:"status"` // "planned", "deployed", "destroyed", "failed"
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
- `planned` → `deployed` → `destroyed`
- `planned` → `failed` (if deployment fails)

---

### 5. DeploymentRecord

**Purpose**: Complete audit of a deployment execution, stored as updated deploy.yaml.

**Location**: `.radius/deploy/<APP>/<ENV>/<COMMIT_HASH>/deploy.yaml` (same file as plan, updated after apply)

**Structure**:
```go
// DeploymentRecord captures the full deployment execution audit
type DeploymentRecord struct {
    Application  string            `yaml:"application"`
    Environment  EnvironmentInfo   `yaml:"environment"`
    StartedAt    time.Time         `yaml:"startedAt"`
    CompletedAt  time.Time         `yaml:"completedAt"`
    Status       string            `yaml:"status"` // "succeeded", "failed", "partial"
    Git          GitContext        `yaml:"git"`
    Plan         PlanReference     `yaml:"plan"`
    Steps        []ExecutedStep    `yaml:"steps"`
    Resources    []ResourceInfo    `yaml:"resources"`
    Summary      ExecutionSummary  `yaml:"summary"`
}

// EnvironmentInfo provides environment context for the deployment
type EnvironmentInfo struct {
    Name                  string `yaml:"name"`
    KubernetesContext     string `yaml:"kubernetesContext,omitempty"`
    KubernetesNamespace   string `yaml:"kubernetesNamespace,omitempty"`
}

// GitContext provides git information at deployment time
type GitContext struct {
    Commit      string `yaml:"commit"`
    CommitShort string `yaml:"commitShort"`
    Branch      string `yaml:"branch"`
    IsDirty     bool   `yaml:"isDirty"`
}

// PlanReference links back to the plan used
type PlanReference struct {
    PlanFile    string    `yaml:"planFile"`
    PlanCommit  string    `yaml:"planCommit"`
    GeneratedAt time.Time `yaml:"generatedAt"`
}

// ExecutedStep records execution details for a deployment step
type ExecutedStep struct {
    Sequence          int                    `yaml:"sequence"`
    Name              string                 `yaml:"name"`
    ResourceType      string                 `yaml:"resourceType"`
    Tool              string                 `yaml:"tool"` // "terraform" or "bicep"
    Status            string                 `yaml:"status"`
    StartedAt         time.Time              `yaml:"startedAt"`
    CompletedAt       time.Time              `yaml:"completedAt"`
    Duration          time.Duration          `yaml:"duration"`
    Changes           ChangeCount            `yaml:"changes"`
    Outputs           map[string]any         `yaml:"outputs,omitempty"`
    CapturedResources []CapturedResource     `yaml:"capturedResources,omitempty"`
    Error             *ErrorInfo             `yaml:"error,omitempty"`
}

// CapturedResource links to the captured resource definition
type CapturedResource struct {
    ResourceID             string `yaml:"resourceId"`
    ResourceDefinitionFile string `yaml:"resourceDefinitionFile"`
}

// ErrorInfo captures error details when a step fails
type ErrorInfo struct {
    Message    string `yaml:"message"`
    Details    string `yaml:"details,omitempty"`
    LogFile    string `yaml:"logFile,omitempty"`
}

// ExecutionSummary aggregates execution statistics
type ExecutionSummary struct {
    TotalSteps          int `yaml:"totalSteps"`
    SucceededSteps      int `yaml:"succeededSteps"`
    FailedSteps         int `yaml:"failedSteps"`
    SkippedSteps        int `yaml:"skippedSteps"`
    TotalResources      int `yaml:"totalResources"`
    ResourcesAdded      int `yaml:"resourcesAdded"`
    ResourcesChanged    int `yaml:"resourcesChanged"`
    ResourcesDestroyed  int `yaml:"resourcesDestroyed"`
}
```

---

### 6. DestructionRecord

**Purpose**: Audit record for resource teardown.

**Location**: `.radius/deploy/<APP>/<ENV>/<COMMIT_HASH>/deploy.yaml` (same file, steps status changed to `destroyed`)

**Structure**: Same as `DeploymentRecord` with step `status` fields updated to `destroyed`.

---

## Relationships

```
┌─────────────────┐     ┌─────────────────────────┐
│    Workspace    │────▶│  GitHub Environment     │
│  (config.yaml)  │     │  (GitHub API variables) │
└─────────────────┘     └─────────────────────────┘
        │                        │
        │                        ▼
        │               ┌─────────────────────────┐
        │               │  RADIUS_RECIPES_MANIFEST│
        │               │  (env-scoped variable)  │
        │               └─────────────────────────┘
        │
        ▼
┌─────────────────┐     ┌─────────────────────────┐
│  RADIUS_RESOURCE│     │  Application Definition │
│  _TYPES_MANIFEST│     │ (.radius/applications/) │
│  (repo variable)│     └─────────────────────────┘
└─────────────────┘              │
                                 ▼
                        ┌─────────────────────────┐
                        │   DeploymentPlan        │
                        │ (.radius/deploy/<app>/  │
                        │  <env>/<commit>/        │
                        │  deploy.yaml)           │
                        └─────────────────────────┘
                                 │
                          ┌──────┴──────┐
                          ▼             ▼
                   ┌──────────┐  ┌──────────┐
                   │ Artifacts│  │ Resources│
                   │ (*.tf)   │  │ (*.yaml) │
                   └──────────┘  └──────────┘
```

---

## File System Layout

```
repository/
├── .radius/
│   ├── applications/
│   │   └── <app>.bicep              # Application definitions
│   └── deploy/
│       └── <app>/
│           └── <env>/
│               └── <commit_hash>/
│                   ├── deploy.yaml           # DeploymentPlan / DeploymentRecord
│                   ├── 001-<res>-terraform/
│                   │   ├── main.tf
│                   │   ├── providers.tf
│                   │   ├── variables.tf
│                   │   ├── terraform.tfvars.json
│                   │   ├── tfplan.txt
│                   │   ├── terraform-context.txt
│                   │   ├── .terraform.lock.hcl
│                   │   └── resources/        # Captured resources (after apply)
│                   │       ├── deployment-db.yaml
│                   │       └── service-db.yaml
│                   └── 002-<res>-terraform/
│                       └── resources/
├── .github/
│   └── workflows/
│       ├── radius-deployment-create.yml
│       ├── radius-deployment-apply.yml
│       ├── radius-destroy.yml
│       └── radius-auth-test.yml
└── ~/.rad/
    └── config.yaml                   # WorkspaceSection with Workspaces
```

**External Storage** (not in repository):
- GitHub Environment variables (cloud config, recipes manifest URL)
- GitHub repository variable `RADIUS_RESOURCE_TYPES_MANIFEST`
- Terraform state (S3 for AWS, Azure Storage for Azure)
