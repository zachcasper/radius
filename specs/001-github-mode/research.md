# Research: Radius on GitHub

**Phase**: 0 (Outline & Research)  
**Date**: 2026-02-16 (updated from 2026-02-12)  
**Plan**: [plan.md](plan.md)

## Research Summary

This document consolidates technical research findings for implementing the "Radius on GitHub" feature. All items marked NEEDS CLARIFICATION in the Technical Context have been resolved. Updated to reflect the two-phase deployment model (`rad deployment create` + `rad deployment apply`) and clarifications from Session 2026-02-15.

---

## 1. GitHub CLI (gh) Integration

**Decision**: Use `os/exec.Command` to shell out to `gh` CLI commands, following the existing pattern used for Azure CLI and Bicep CLI.

**Rationale**:
- No `github.com/cli/go-gh` library is currently in `go.mod`
- Existing patterns in Radius use `exec.Command` consistently for external CLIs (Azure CLI, Bicep, git)
- GitHub workflow instructions recommend using `gh` CLI because it provides automatic authentication context
- The `gh` CLI works identically locally (developer logged in) and in CI (via `GITHUB_TOKEN`)

**Alternatives considered**:
- `go-gh` library: Would add a new dependency and different pattern from existing CLI integrations
- Direct GitHub API calls: More complex, requires manual token handling

**Pattern reference**: [pkg/azure/azcli/azcli.go](../../pkg/azure/azcli/azcli.go), [pkg/cli/bicep/build.go](../../pkg/cli/bicep/build.go)

---

## 2. go-git Library Usage

**Decision**: Use existing `github.com/go-git/go-git/v5` (v5.16.4 in `go.mod`) for git operations.

**Rationale**:
- Already a direct dependency in Radius
- Existing usage for `git.PlainInit`, worktree operations, commits, and pushes
- Pure Go implementation, no external git binary required for basic operations
- Note: Radius also uses `exec.Command("git", ...)` for some config operations where go-git doesn't support the feature

**Key operations**:
- **Sparse checkout**: Not directly supported by go-git - use GitHub Actions `sparse-checkout` input in workflow template
- **Adding files**: `worktree.Add("filename.yaml")`
- **Committing with trailers**: Include trailer in commit message body (`"message\n\nRadius-Action: init"`)
- **Pushing**: `repo.Push(&git.PushOptions{...})`

**Alternatives considered**:
- `exec.Command` for all git operations: Works but less idiomatic, harder to test
- libgit2 bindings: CGO dependency, more complex build

**Pattern reference**: [pkg/recipes/driver/terraform/gitconfig.go](../../pkg/recipes/driver/terraform/gitconfig.go), [test/functional-portable/kubernetes/noncloud/flux_test.go](../../test/functional-portable/kubernetes/noncloud/flux_test.go)

---

## 3. GitHub Actions Workflow Generation

**Decision**: Use `github.com/goccy/go-yaml` for YAML marshaling (already in `go.mod`), not `text/template`.

**Rationale**:
- `goccy/go-yaml` is already a direct dependency used throughout Radius
- Provides proper YAML marshaling with struct tags, validation support, and strict mode
- `text/template` is error-prone for YAML (whitespace sensitivity) and harder to validate

**Alternatives considered**:
- `text/template`: Error-prone for complex YAML, whitespace issues
- `gopkg.in/yaml.v3`: Would add another YAML dependency when goccy/go-yaml is already used

**Pattern reference**: [pkg/cli/manifest/parser.go](../../pkg/cli/manifest/parser.go)

---

## 4. AWS OIDC Setup via CLI

**Decision**: Use AWS CLI (shelling out via `exec.Command`) for OIDC setup operations, with AWS SDK Go v2 for validation only.

**Rationale**:
- Spec explicitly states: "verifies AWS CLI authentication via `aws sts get-caller-identity`"
- AWS SDK Go v2 is already in `go.mod` for validation
- CLI approach provides transparency (user sees exact commands being executed)
- Allows user confirmation before executing infrastructure changes

**Commands needed**:
```bash
# Check authentication
aws sts get-caller-identity

# Get default region
aws configure get region

# Create OIDC provider for GitHub Actions
aws iam create-open-id-connect-provider \
    --url https://token.actions.githubusercontent.com \
    --client-id-list sts.amazonaws.com \
    --thumbprint-list 6938fd4d98bab03faadb97b34396831e3780aea1

# Create IAM role with trust policy
aws iam create-role \
    --role-name radius-deploy-role \
    --assume-role-policy-document file://trust-policy.json
```

**Trust policy template**:
```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": {"Federated": "arn:aws:iam::ACCOUNT:oidc-provider/token.actions.githubusercontent.com"},
    "Action": "sts:AssumeRoleWithWebIdentity",
    "Condition": {
      "StringEquals": {
        "token.actions.githubusercontent.com:aud": "sts.amazonaws.com"
      },
      "StringLike": {
        "token.actions.githubusercontent.com:sub": "repo:OWNER/REPO:*"
      }
    }
  }]
}
```

**Alternatives considered**:
- Pure SDK approach: Less transparent to user, harder to debug
- Terraform: Overkill for one-time setup

**Pattern reference**: [pkg/cli/aws/client.go](../../pkg/cli/aws/client.go)

---

## 5. Azure OIDC Setup via CLI

**Decision**: Use Azure CLI (shelling out) for OIDC setup, following the existing pattern.

**Rationale**:
- Existing `RunCLICommand` function handles cross-platform execution
- Spec explicitly references `az account show` and `az account list`
- Azure SDK for Go v2 is extensively used for resource operations
- Federated Identity Credential creation patterns exist in codebase

**Commands needed**:
```bash
# Check authentication
az account show

# List subscriptions
az account list --output json

# Create AD application
az ad app create --display-name "radius-github-deploy"

# Create federated credential for GitHub Actions
az ad app federated-credential create \
    --id <APP_ID> \
    --parameters '{
        "name": "github-radius-deploy",
        "issuer": "https://token.actions.githubusercontent.com",
        "subject": "repo:OWNER/REPO:ref:refs/heads/main",
        "audiences": ["api://AzureADTokenExchange"]
    }'

# Create service principal 
az ad sp create --id <APP_ID>

# Assign Contributor role
az role assignment create \
    --assignee <CLIENT_ID> \
    --role Contributor \
    --scope /subscriptions/<SUBSCRIPTION_ID>/resourceGroups/<RG_NAME>
```

**Alternatives considered**:
- Pure Azure SDK: More complex, existing patterns use CLI for setup operations
- Existing federated identity handler: Uses SDK but is for runtime container workload identity, not CLI setup

**Pattern reference**: [pkg/azure/azcli/azcli.go](../../pkg/azure/azcli/azcli.go), [pkg/corerp/handlers/azure_federatedidentity.go](../../pkg/corerp/handlers/azure_federatedidentity.go)

---

## 6. Terraform State Backend Configuration

**Decision**: Create new backend implementations for S3 and Azure Storage following the existing Backend interface pattern.

**Rationale**:
- Existing Kubernetes backend implementation provides clear pattern
- Backend interface is well-defined with `BuildBackend()` and `ValidateBackendExists()` methods
- Spec FR-093 requires cloud backends (S3 for AWS, Azure Storage for Azure)
- State backend location must be recorded in environment file (FR-097)

**S3 Backend configuration**:
```go
return map[string]any{
    "s3": map[string]any{
        "bucket":         bucket,
        "key":            generateStateKey(resourceRecipe),
        "region":         region,
        "dynamodb_table": lockTable, // For state locking (FR-096)
        "encrypt":        true,
    },
}
```

**Azure Storage Backend configuration**:
```go
return map[string]any{
    "azurerm": map[string]any{
        "storage_account_name": storageAccountName,
        "container_name":       containerName,
        "key":                  generateStateKey(resourceRecipe),
        "use_oidc":             true, // OIDC auth (FR-095)
    },
}
```

**Alternatives considered**:
- Local backend in repo: Violates FR-093, causes state conflicts
- Terraform Cloud: Additional dependency, more complex setup

**Pattern reference**: [pkg/recipes/terraform/config/backends/kubernetes.go](../../pkg/recipes/terraform/config/backends/kubernetes.go)

---

## 7. k3d in GitHub Actions

**Decision**: Use k3d for ephemeral Kubernetes clusters in GitHub Actions workflows.

**Rationale**:
- Spec explicitly requires k3d (FR-036, FR-037, SC-005)
- Existing Radius tooling uses k3d for local development
- k3d is lighter than kind for single-node ephemeral clusters
- Supports hostPath volume mapping required by FR-037

**Setup time and resources** (from spec):
- **Download size**: ~875 MiB
- **Startup time**: ~45 seconds
- **Total setup (k3d + Radius)**: Under 60 seconds (SC-005)
- **Runner requirements**: Standard GitHub-hosted runners (`ubuntu-latest`) are adequate

**Workflow steps**:
```yaml
# Install k3d
- name: Install k3d
  run: |
    curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash

# Create cluster with hostPath mapping
- name: Create k3d cluster
  run: |
    k3d cluster create radius \
      --volume "${GITHUB_WORKSPACE}:/github_workspace" \
      --k3s-arg "--disable=traefik@server:0"

# Install Radius
- name: Install Radius
  run: |
    rad install kubernetes
```

**Alternatives considered**:
- kind: Currently used in functional tests, heavier weight
- minikube: Too heavy for CI ephemeral clusters
- No cluster (direct API): Would require major architecture changes

**Pattern reference**: [build/scripts/start-radius.sh](../../build/scripts/start-radius.sh)

---

## 8. Resource Types Contrib Repository

**Repository**: `radius-project/resource-types-contrib`

**Structure** (as of 2026):
```
resource-types-contrib/
├── Compute/                           # Namespace directory
│   └── <resourceType>/
│       ├── <resourceType>.yaml        # Resource type definition
│       └── recipes/                   # Recipe implementations
│           ├── aws-*/                 # AWS-specific recipes
│           ├── azure-*/               # Azure-specific recipes
│           └── kubernetes/            # Kubernetes recipes
├── Data/                              # Namespace directory
│   └── redisCaches/
│       ├── redisCaches.yaml
│       └── recipes/
├── Security/secrets/                  # Nested namespace
│   └── <resourceType>/
├── docs/                              # Documentation
└── recipe-packs/                      # Bundled recipe collections
```

**Resource Type Definition Format** (YAML):
```yaml
# Example: Data/redisCaches/redisCaches.yaml
name: redisCaches
description: Redis cache resource type
provider: both  # aws, azure, or both
```

**Integration approach**:
- Use sparse checkout to fetch only namespace directories (Compute, Data, Security)
- Parse YAML files to extract resource type definitions
- Filter by provider (aws/azure/both) based on environment configuration
- Copy relevant recipe files to `.radius/recipes/` directory

**Pattern reference**: [pkg/cli/github/resourcetypes.go](../../pkg/cli/github/resourcetypes.go)

---

## Summary Table

| Topic | Decision | Pattern Reference |
|-------|----------|-------------------|
| GitHub CLI | `exec.Command` for `gh` CLI | pkg/azure/azcli/azcli.go |
| go-git | Use v5.16.4 (already in go.mod) | pkg/recipes/driver/terraform/gitconfig.go |
| Workflow YAML | `goccy/go-yaml` marshal | pkg/cli/manifest/parser.go |
| AWS OIDC | AWS CLI + SDK validation | pkg/cli/aws/client.go |
| Azure OIDC | Azure CLI via existing pattern | pkg/azure/azcli/azcli.go |
| Terraform Backend | New S3/AzureStorage implementations | pkg/recipes/terraform/config/backends/kubernetes.go |
| k3d | Install and create cluster in workflow | build/scripts/start-radius.sh |
| Resource Types | Sparse checkout from namespaces | pkg/cli/github/resourcetypes.go |

---

## 9. Two-Phase Deployment Model (rad deployment create / apply)

**Decision**: Implement `rad deployment create` and `rad deployment apply` as new commands under a `deployment` command group in `pkg/cli/cmd/deployment/`, following the existing Cobra/Runner/Factory pattern.

**Rationale**:
- No existing `deployment` command group exists — clean namespace
- `rad deployment create` dispatches `workflow_dispatch` on `radius-deployment-create.yml`
- `rad deployment apply` dispatches `workflow_dispatch` on `radius-deployment-apply.yml`
- Both commands use existing `github.Client.RunWorkflow()` for dispatch
- CLI monitoring uses existing `GetLatestWorkflowRun()` + `WatchWorkflowRun()`, enhanced with animated progress (see R10)

**Architecture**:
```
CLI (user machine)                      GitHub Actions Runner
┌────────────────────┐  workflow_       ┌─────────────────────────────┐
│ rad deployment     │──dispatch──────→ │ radius-deployment-create.yml│
│   create           │                  │  1. Checkout repo           │
│  - validate flags  │                  │  2. Start k3d cluster       │
│  - check worktree  │                  │  3. Install Radius          │
│  - dispatch wf     │                  │  4. Load types manifest     │
│  - show progress   │  poll/stream     │  5. Call plan API           │
│  - stream logs (L) │←─────────────────│  6. Commit artifacts        │
└────────────────────┘                  └─────────────────────────────┘

┌────────────────────┐  workflow_       ┌─────────────────────────────┐
│ rad deployment     │──dispatch──────→ │ radius-deployment-apply.yml │
│   apply            │                  │  1. Checkout repo           │
│  - validate flags  │                  │  2. Read deploy.yaml        │
│  - find plan       │                  │  3. OIDC auth to cloud      │
│  - dispatch wf     │                  │  4. For each step:          │
│  - show progress   │  poll/stream     │     terraform apply / bicep │
│  - stream logs (L) │←─────────────────│  5. Capture resources       │
└────────────────────┘                  └─────────────────────────────┘
```

**Workflow inputs** (via `workflow_dispatch`):
- `application`: Application name
- `environment`: Target environment name
- `commit`: Commit hash scoping the deployment

**Alternatives considered**:
- Single `rad deploy` with `--plan`/`--apply` flags: Muddies verb semantics; harder to compose in pipelines
- Local k3d execution: Requires Docker + k3d locally; inconsistent with cloud-only model

**Pattern reference**: [pkg/cli/cmd/pr/create/create.go](../../pkg/cli/cmd/pr/create/create.go) (existing workflow dispatch pattern)

---

## 10. Animated Progress Indicator with Log Streaming

**Decision**: Use [Charm Bubble Tea](https://github.com/charmbracelet/bubbletea) for animated progress indicator and log streaming toggle, consistent with existing `prompt.Interface` pattern.

**Rationale**:
- Codebase already depends on `bubbletea` via `pkg/cli/prompt/prompt.go` (`RunProgram` method)
- Bubble Tea provides terminal UI primitives (spinner, key handling) needed for progress indicator and L-key toggle
- `github.Client` already has `WatchWorkflowRun()` and `GetWorkflowRunLogs()` that feed data into the model

**UX Flow**:
1. CLI dispatches workflow → prints contextual message (e.g., "Creating deployment workflow...")
2. Switches to Bubble Tea program with spinner + status label ("Creating deployment...") + "[L] View logs" prompt
3. On `L` keypress: replace spinner with streaming log output from `gh run watch`
4. On `L` again: return to spinner view
5. On workflow completion: exit Bubble Tea, print result summary

**Implementation**: New file `pkg/cli/github/progress.go` with a Bubble Tea model that wraps workflow monitoring.

**Alternatives considered**:
- Simple `fmt.Print` spinner: No interactivity; can't toggle log streaming
- `gh run watch` directly: Exits `rad` CLI context; can't show custom status messages

**Pattern reference**: [pkg/cli/prompt/prompt.go](../../pkg/cli/prompt/prompt.go) (Bubble Tea integration)

---

## 11. GitHub Actions Concurrency Groups

**Decision**: Use GitHub Actions `concurrency:` key in workflow YAML to serialize deployments per app/environment combination.

**Rationale**:
- Native GitHub Actions feature; zero additional infrastructure
- Handles lock release automatically on success, failure, and cancellation
- `cancel-in-progress: false` ensures queued runs wait rather than being cancelled

**Configuration**:
```yaml
concurrency:
  group: radius-deploy-${{ inputs.application }}-${{ inputs.environment }}
  cancel-in-progress: false
```

**Alternatives considered**:
- GitHub Deployment API status checks: More complex; requires additional API calls
- Cloud-side locking (DynamoDB/Blob lease): Already covered by Terraform state locking; redundant at workflow level
- Repository file lock: Race conditions between check and commit

---

## 12. Commit Hash Scoping and Plan Resolution

**Decision**: Deployment artifacts at `.radius/deploy/<APP>/<ENV>/<COMMIT_HASH>/`. Short hash (7 chars) of HEAD at dispatch time. `rad deployment apply` defaults to most recent plan; `--git-commit` overrides.

**Rationale**:
- Short hash matches Git convention; keeps paths readable
- "Latest plan" resolved by most recent `Generated:` timestamp in `deploy.yaml`
- `--git-commit` flag provides explicit targeting for older plans

**Edge cases**:
- `rad deployment create` commits artifacts → new commit. Plan's `commit` field records the *input* commit (user's code), not the *output* commit (with artifacts)
- If `--git-commit` directory doesn't exist, error with available plan hashes
- Multiple plans: latest by `deploy.yaml` timestamp, not filesystem mtime

**Alternatives considered**:
- Full 40-char hash: Unwieldy paths
- Timestamp directories: Less traceable to source code
- Branch-based: Doesn't support multiple plans per branch

---

## 13. OIDC Verification via Auth Test Workflow

**Decision**: After `rad environment create` stores environment variables, dispatch `radius-auth-test.yml` workflow to verify OIDC federation works.

**Rationale**:
- Misconfigured federated credentials are the most common OIDC failure mode
- Catching at setup time saves users from discovering failures during first deployment (minutes later)
- Auth test workflow already exists in `workflows.go` (`GenerateAuthTestWorkflow`)
- CLI shows "Creating authentication test workflow..." → "Testing authentication to azure/aws..." with spinner

**Existing code**: `rad environment connect` already does this ([connect.go lines 184-218](../../pkg/cli/cmd/env/connect/connect.go#L184-L218)) — generates auth test workflow, commits, pushes, waits for result.

**Implementation**: Extract auth test dispatch logic from `connect.go` into reusable function in `pkg/cli/github/oidc.go`.

---

## 14. OIDC Cleanup on Environment Delete

**Decision**: Prompt during `rad environment delete` to optionally remove cloud OIDC resources. Read identifiers from GitHub Environment variables before deleting the environment.

**Rationale**:
- OIDC resources persist in cloud after GitHub Environment deletion
- Identifiers stored in env variables (`AZURE_CLIENT_ID`, `AWS_IAM_ROLE_NAME`) — must read before delete
- Opt-in because resources may be shared across repositories

**Implementation sequence**:
1. Read env variables via `gh api repos/{owner}/{repo}/environments/{name}/variables`
2. Determine provider from variables
3. Prompt: "Delete OIDC resources from [Azure/AWS]? (y/N)"
4. If yes (Azure): `az ad app delete --id <AZURE_CLIENT_ID>`
5. If yes (AWS): `aws iam delete-role` + `aws iam delete-open-id-connect-provider`
6. If no: display identifiers for manual cleanup
7. Delete GitHub Environment via `gh api ... -X DELETE`

---

## 15. GitHub Environment Variable Management

**Decision**: Use `gh variable set --env` for setting environment-scoped variables; `gh api` for environment lifecycle (create/delete) and querying variables.

**Rationale**:
- `gh variable set` supports `--env <name>` flag natively
- No `gh environment create` command exists — must use `gh api`
- Existing codebase uses `gh secret set` (no `--env` flag) for repository secrets; environment variables are the new pattern per spec
- `gh api` for listing variables provides structured JSON output

**Commands**:
```bash
# Create GitHub Environment
gh api repos/{owner}/{repo}/environments/{name} -X PUT

# Set environment variable
gh variable set VAR_NAME --env ENV_NAME --body "value"

# List environment variables
gh api repos/{owner}/{repo}/environments/{name}/variables

# Delete GitHub Environment
gh api repos/{owner}/{repo}/environments/{name} -X DELETE

# Set repository variable (for RADIUS_RESOURCE_TYPES_MANIFEST)
gh variable set VAR_NAME --body "value"
```

**Migration from secrets to variables**: Current `connect.go` uses `gh secret set` for `AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, etc. The new flow uses `gh variable set --env` instead — these are not secrets (they're identifiers, not credentials).

---

## 16. Workflow Generation Updates

**Decision**: Update `pkg/cli/github/workflows.go` to generate four workflow templates aligned with two-phase model:
1. `radius-deployment-create.yml` — replaces `radius-plan.yaml`
2. `radius-deployment-apply.yml` — replaces `radius-deploy.yaml`
3. `radius-destroy.yml` — updated command names
4. `radius-auth-test.yml` — unchanged structurally

**Key changes from existing generators**:
- Triggers change to `workflow_dispatch` with app/env/commit inputs
- `concurrency:` groups added (see R11)
- Step names updated for `rad deployment create`/`apply`
- All workflows use `environment:` key to scope GitHub Environment variables

---

## 17. `rad model` → `rad app model` Rename

**Decision**: Move `model` command from `RootCmd.AddCommand(modelCmd)` to `applicationCmd.AddCommand(modelCmd)` in `root.go`.

**Rationale**: One-line wiring change. Underlying `model.NewCommand()` function unchanged. Semantically correct — modeling is about applications.

**Pattern reference**: [cmd/rad/cmd/root.go line 472](../../cmd/rad/cmd/root.go#L472) (current wiring)

---

## Updated Summary Table

| Topic | Decision | Pattern Reference |
|-------|----------|-------------------|
| GitHub CLI | `exec.Command` for `gh` CLI | pkg/azure/azcli/azcli.go |
| go-git | Use v5 (already in go.mod) | pkg/recipes/driver/terraform/gitconfig.go |
| Workflow YAML | `goccy/go-yaml` marshal | pkg/cli/manifest/parser.go |
| AWS OIDC | AWS CLI + SDK validation | pkg/cli/aws/client.go |
| Azure OIDC | Azure CLI via existing pattern | pkg/azure/azcli/azcli.go |
| Terraform Backend | New S3/AzureStorage implementations | pkg/recipes/terraform/config/backends/kubernetes.go |
| k3d | Install and create cluster in workflow | build/scripts/start-radius.sh |
| Resource Types | Sparse checkout from namespaces | pkg/cli/github/resourcetypes.go |
| Two-Phase Deployment | New `deployment` command group | pkg/cli/cmd/pr/create/create.go |
| CLI Progress UX | Bubble Tea spinner + L-key toggle | pkg/cli/prompt/prompt.go |
| Concurrency | GitHub Actions `concurrency:` key | N/A (native GitHub feature) |
| Commit Scoping | Short hash dirs; latest by timestamp | N/A |
| OIDC Verification | Auth test workflow dispatch | pkg/cli/cmd/env/connect/connect.go |
| OIDC Cleanup | Prompt + cloud CLI delete | pkg/cli/cmd/env/connect/connect.go |
| Env Variables | `gh variable set --env` + `gh api` | pkg/cli/github/client.go |
| Workflow Generation | Four templates for two-phase model | pkg/cli/github/workflows.go |
| `rad app model` | Move under `applicationCmd` | cmd/rad/cmd/root.go |
