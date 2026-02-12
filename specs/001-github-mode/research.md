# Research: Radius on GitHub

**Phase**: 0 (Outline & Research)  
**Date**: 2026-02-12  
**Plan**: [plan.md](plan.md)

## Research Summary

This document consolidates technical research findings for implementing the "Radius on GitHub" feature. All items marked NEEDS CLARIFICATION in the Technical Context have been resolved.

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
