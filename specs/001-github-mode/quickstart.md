# Quickstart: Radius on GitHub Implementation

**Phase**: 1 (Design & Contracts)  
**Date**: 2026-02-16 (updated from 2026-02-12)

## Overview

This quickstart guide provides implementation patterns for the Radius on GitHub feature using the **two-phase deployment model**:
1. **`rad deployment create`** — generates a deployment plan locally, commits it to a PR branch
2. **`rad deployment apply`** — dispatches a GitHub Actions workflow to execute the plan

Configuration is stored in **GitHub Environment variables** (not local files), and deployment artifacts are scoped by **commit hash**.

---

## 1. Setting Up the Development Environment

```bash
# Clone and setup
cd ~/radius/dev/github-radius

# Ensure dev container is running or local prerequisites met
make build

# Run unit tests to verify setup
make test
```

**Prerequisites** (from constitution):
- Go 1.25.7 (per go.mod)
- GitHub CLI (`gh`) installed and authenticated
- AWS CLI or Azure CLI for OIDC setup testing

---

## 2. GitHub Workspace Connection (Already Partially Exists)

**Location**: `pkg/cli/workspaces/connection.go` — `KindGitHub = "github"` already defined

The new `GitHubConnectionConfig` struct in `pkg/cli/workspaces/github.go` stores the GitHub repository URL:

```go
package workspaces

// GitHubConnectionConfig stores GitHub workspace connection details
type GitHubConnectionConfig struct {
    URL string `yaml:"url"` // GitHub repository URL
}
```

Workspace config (`~/.rad/config.yaml`):
```yaml
current: my-app
items:
  my-app:
    connection:
      kind: github
      url: https://github.com/org/my-app
    environment: dev
```

---

## 3. Two-Phase Deployment: `rad deployment create`

**New package**: `pkg/cli/cmd/deployment/create/`

This command runs **locally** to generate the deployment plan:

```go
// Runner for rad deployment create
type Runner struct {
    factory     framework.Factory
    application string
    environment string
    gitCommit   string // from --git-commit flag or auto-detected HEAD
    Output      output.Interface
    Prompter    prompt.Interface
    GitHub      github.Interface
    Git         github.GitInterface
}

func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
    // 1. Verify GitHub workspace mode
    workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder)
    if err != nil || workspace.Connection.Kind != workspaces.KindGitHub {
        return clierrors.Message("This command requires a GitHub workspace.")
    }

    // 2. Require clean worktree (no uncommitted changes)
    if dirty, err := r.Git.IsWorktreeDirty(); err != nil || dirty {
        return clierrors.Message("Working tree must be clean. Commit or stash changes first.")
    }

    // 3. Resolve commit hash (default: HEAD)
    if r.gitCommit == "" {
        r.gitCommit, err = r.Git.GetHeadCommitShort()
    }

    return nil
}

func (r *Runner) Run(ctx context.Context) error {
    // 1. Read application definition from .radius/applications/<app>.bicep
    // 2. Fetch resource types manifest (from RADIUS_RESOURCE_TYPES_MANIFEST repo variable)
    // 3. Fetch recipes manifest (from RADIUS_RECIPES_MANIFEST env variable)
    // 4. Generate deployment plan with ordered steps
    // 5. Generate Terraform/Bicep artifacts for each step
    // 6. Write deploy.yaml to .radius/deploy/<app>/<env>/<commit>/deploy.yaml
    // 7. Create PR branch, commit, push
    // 8. Create PR with plan summary in body
    return nil
}
```

Output: `.radius/deploy/<app>/<env>/<commit>/deploy.yaml`

---

## 4. Two-Phase Deployment: `rad deployment apply`

**New package**: `pkg/cli/cmd/deployment/apply/`

This command **dispatches a GitHub Actions workflow** and shows animated progress:

```go
// Runner for rad deployment apply
type Runner struct {
    factory     framework.Factory
    application string
    environment string
    gitCommit   string // from --git-commit flag or auto-detected from PR
    Output      output.Interface
    Prompter    prompt.Interface
    GitHub      github.Interface
}

func (r *Runner) Run(ctx context.Context) error {
    // 1. Dispatch radius-deployment-apply.yml workflow
    err := r.GitHub.RunWorkflow("radius-deployment-apply.yml", r.branch, map[string]string{
        "application": r.application,
        "environment": r.environment,
        "commit":      r.gitCommit,
    })

    // 2. Wait for workflow to start
    run, err := r.GitHub.GetLatestWorkflowRun("radius-deployment-apply.yml", r.branch)

    // 3. Show animated progress indicator (Bubble Tea)
    // User can press 'L' to stream logs
    model := progress.NewModel(r.GitHub, run.ID)
    if _, err := r.Prompter.RunProgram(model); err != nil {
        return err
    }

    // 4. Report final status
    return nil
}
```

---

## 5. GitHub Client Extensions

**Location**: `pkg/cli/github/client.go` (existing — extend)

New methods needed:

```go
// SetEnvironmentVariable sets a GitHub Environment-scoped variable
func (c *Impl) SetEnvironmentVariable(envName, name, value string) error {
    return c.runGH("variable", "set", name, "--env", envName, "--body", value)
}

// GetEnvironmentVariables lists variables for a GitHub Environment
func (c *Impl) GetEnvironmentVariables(envName string) (map[string]string, error) {
    output, err := c.runGHOutput("variable", "list", "--env", envName, "--json", "name,value")
    // parse JSON output
    return vars, err
}

// SetRepoVariable sets a GitHub repository variable
func (c *Impl) SetRepoVariable(name, value string) error {
    return c.runGH("variable", "set", name, "--body", value)
}

// CreateEnvironment creates a GitHub Environment via API
func (c *Impl) CreateEnvironment(envName string) error {
    return c.runGH("api", "--method", "PUT",
        fmt.Sprintf("repos/{owner}/{repo}/environments/%s", envName))
}

// DeleteEnvironment deletes a GitHub Environment via API
func (c *Impl) DeleteEnvironment(envName string) error {
    return c.runGH("api", "--method", "DELETE",
        fmt.Sprintf("repos/{owner}/{repo}/environments/%s", envName))
}
```

---

## 6. Workflow Generation

**Location**: `pkg/cli/github/workflows.go` (existing — extend)

Four workflows are generated by `rad init` (FR-112):

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| `radius-deployment-create.yml` | `workflow_dispatch` | Generates deployment plan |
| `radius-deployment-apply.yml` | `workflow_dispatch` | Executes deployment plan |
| `radius-destroy.yml` | `workflow_dispatch` | Destroys deployed resources |
| `radius-auth-test.yml` | `workflow_dispatch` | Verifies OIDC authentication |

Each workflow uses GitHub Actions `concurrency` groups to prevent parallel deployments:

```yaml
concurrency:
  group: radius-deploy-${{ inputs.application }}-${{ inputs.environment }}
  cancel-in-progress: false
```

---

## 7. OIDC Setup and Verification

**Location**: `pkg/cli/cmd/env/connect/connect.go` (existing — 1300+ lines)

After OIDC setup, auto-verify by dispatching `radius-auth-test.yml`:

```go
// After OIDC configuration completes:
err := r.GitHub.RunWorkflow("radius-auth-test.yml", "main", map[string]string{
    "environment": r.environment,
})
// Monitor workflow result for pass/fail
run, err := r.GitHub.WatchWorkflowRun(ctx, runID)
```

---

## 8. Environment Delete with OIDC Cleanup

**Location**: `pkg/cli/cmd/env/delete/` (new or modified)

When deleting a GitHub-mode environment:

```go
func (r *Runner) Run(ctx context.Context) error {
    // 1. Prompt: "Do you want to clean up OIDC configuration from <provider>?"
    if cleanup {
        // 2. Azure: delete app registration + federated credential
        //    AWS: delete IAM role + OIDC identity provider
    }

    // 3. Delete GitHub Environment (removes all env-scoped variables)
    err := r.GitHub.DeleteEnvironment(r.environment)

    return nil
}
```

---

## 9. Testing Patterns

**Unit tests** follow the existing pattern in `test/radcli/`:

```go
func Test_Validate_GitHubWorkspaceRequired(t *testing.T) {
    ctrl := gomock.NewController(t)

    runner := &Runner{
        // ... setup with Kubernetes workspace (should fail)
    }

    err := runner.Validate(cmd, args)
    require.Error(t, err)
    assert.Contains(t, err.Error(), "GitHub workspace")
}

func Test_Run_DeploymentCreate(t *testing.T) {
    ctrl := gomock.NewController(t)

    mockGit := github.NewMockGitInterface(ctrl)
    mockGit.EXPECT().IsWorktreeDirty().Return(false, nil)
    mockGit.EXPECT().GetHeadCommitShort().Return("abc1234", nil)

    mockGH := github.NewMockInterface(ctrl)
    // ... expect workflow dispatch, PR creation, etc.

    runner := &Runner{
        application: "myapp",
        environment: "dev",
        Git:         mockGit,
        GitHub:      mockGH,
    }

    err := runner.Run(context.Background())
    require.NoError(t, err)
}
```

---

## 10. Key Files to Implement

| Priority | File | Purpose |
|----------|------|---------|
| 1 | `pkg/cli/cmd/deployment/create/create.go` | `rad deployment create` command |
| 2 | `pkg/cli/cmd/deployment/apply/apply.go` | `rad deployment apply` command |
| 3 | `pkg/cli/cmd/radinit/github.go` | Enhanced `rad init` for GitHub mode |
| 4 | `pkg/cli/github/client.go` | Extend with env variable methods |
| 5 | `pkg/cli/github/workflows.go` | Add deployment-create/apply workflow generation |
| 6 | `pkg/cli/cmd/env/connect/connect.go` | Add OIDC auto-verify after setup |
| 7 | `pkg/cli/cmd/env/delete/delete.go` | OIDC cleanup on env delete |
| 8 | `pkg/cli/cmd/app/delete/delete.go` | `rad app delete` via workflow dispatch |
| 9 | `pkg/cli/cmd/app/model/model.go` | Rename from `rad model` → `rad app model` |
| 10 | `cmd/rad/cmd/root.go` | Wire new deployment subcommands |

---

## 11. Constitution Compliance Checklist

Before submitting PRs, verify:

- [ ] Unit tests cover new functionality (Principle IV)
- [ ] Go code passes `make lint` and `make format-check` (Principle II)
- [ ] No cloud-specific assumptions — both AWS and Azure work (Principle III)
- [ ] CLI help text is comprehensive (Principle XIV)
- [ ] Commits include `Signed-off-by` (Principle VI)
- [ ] Feature can be incrementally adopted (Principle IX)
- [ ] GitHub workspace commands error clearly when used in Kubernetes mode (Principle XI)
