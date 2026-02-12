# Quickstart: Radius on GitHub Implementation

**Phase**: 1 (Design & Contracts)  
**Date**: 2026-02-12

## Overview

This quickstart guide provides implementation patterns for the Radius on GitHub feature. It covers the essential code paths and integration points for developers working on this feature.

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
- Go 1.21+ (per go.mod)
- GitHub CLI (`gh`) installed and authenticated
- AWS CLI or Azure CLI for OIDC setup testing

---

## 2. Adding the GitHub Workspace Kind

**Location**: `pkg/cli/workspaces/connection.go`

```go
// Add new constant for GitHub workspace kind
const (
    KindKubernetes = "kubernetes"
    KindGitHub     = "github"  // NEW
)

// Update CreateConnection to handle GitHub
func CreateConnection(connectionMap map[string]any) (Connection, error) {
    kind, ok := connectionMap["kind"].(string)
    if !ok {
        return nil, errors.New("missing connection kind")
    }

    switch kind {
    case KindKubernetes:
        return createKubernetesConnection(connectionMap)
    case KindGitHub:
        return createGitHubConnection(connectionMap)  // NEW
    default:
        return nil, fmt.Errorf("unknown connection kind: %s", kind)
    }
}
```

**New file**: `pkg/cli/workspaces/github.go`

```go
package workspaces

// GitHubConnection represents a connection to a GitHub repository
type GitHubConnection struct {
    URL string
}

func (c *GitHubConnection) Kind() string {
    return KindGitHub
}

func (c *GitHubConnection) String() string {
    return c.URL
}

func createGitHubConnection(connectionMap map[string]any) (*GitHubConnection, error) {
    url, ok := connectionMap["url"].(string)
    if !ok {
        return nil, errors.New("GitHub connection requires 'url' field")
    }
    return &GitHubConnection{URL: url}, nil
}
```

---

## 3. Implementing rad init for GitHub Mode

**Location**: `pkg/cli/cmd/radinit/init.go` (modify existing)

Key changes:
1. Add `--provider` and `--deployment-tool` flags
2. Detect GitHub repository (check `.git` and remote)
3. Create `.radius/` directory structure
4. Fetch resource types via sparse checkout
5. Create workspace config

```go
// Runner for rad init (modified)
type Runner struct {
    factory      framework.Factory
    provider     string  // aws, azure
    deployTool   string  // terraform, bicep
    environment  string  // default: "default"
    // ... existing fields
}

func (r *Runner) Validate() error {
    // Validate directory is a git repo
    if !r.isGitRepository() {
        return errors.New("current directory is not a git repository")
    }
    
    // Validate GitHub remote exists
    remote, err := r.getGitHubRemote()
    if err != nil {
        return fmt.Errorf("no GitHub remote found: %w", err)
    }
    r.gitHubRemote = remote
    
    // Validate gh auth status
    if err := r.checkGHAuth(); err != nil {
        return fmt.Errorf("GitHub CLI not authenticated: %w", err)
    }
    
    return nil
}

func (r *Runner) Run(ctx context.Context) error {
    // 1. Create .radius directory
    if err := r.createRadiusDir(); err != nil {
        return err
    }
    
    // 2. Fetch and create types.yaml (sparse checkout)
    if err := r.fetchResourceTypes(ctx); err != nil {
        return err
    }
    
    // 3. Create recipes.yaml for provider
    if err := r.createRecipesManifest(); err != nil {
        return err
    }
    
    // 4. Create env.<name>.yaml
    if err := r.createEnvironmentFile(); err != nil {
        return err
    }
    
    // 5. Update ~/.rad/config.yaml with github workspace
    if err := r.createWorkspace(); err != nil {
        return err
    }
    
    // 6. Commit changes with trailer
    if err := r.commitChanges("Radius-Action: init"); err != nil {
        return err
    }
    
    return nil
}
```

---

## 4. GitHub CLI Wrapper

**Location**: `pkg/cli/github/ghcli.go` (new package)

```go
package github

import (
    "bytes"
    "encoding/json"
    "os/exec"
)

// Client wraps the gh CLI for GitHub operations
type Client struct{}

// AuthStatus checks if gh CLI is authenticated
func (c *Client) AuthStatus() error {
    return exec.Command("gh", "auth", "status").Run()
}

// CreatePR creates a pull request
func (c *Client) CreatePR(base, head, title, body string) (int, error) {
    cmd := exec.Command("gh", "pr", "create",
        "--base", base,
        "--head", head,
        "--title", title,
        "--body", body,
        "--json", "number",
    )
    
    output, err := cmd.Output()
    if err != nil {
        return 0, err
    }
    
    var result struct {
        Number int `json:"number"`
    }
    if err := json.Unmarshal(output, &result); err != nil {
        return 0, err
    }
    
    return result.Number, nil
}

// RunWorkflow triggers a workflow dispatch event
func (c *Client) RunWorkflow(workflow, ref string, inputs map[string]string) error {
    args := []string{"workflow", "run", workflow, "--ref", ref}
    for k, v := range inputs {
        args = append(args, "-f", k+"="+v)
    }
    return exec.Command("gh", args...).Run()
}

// MergePR merges a pull request
func (c *Client) MergePR(prNumber int, autoMerge bool) error {
    args := []string{"pr", "merge", fmt.Sprintf("%d", prNumber), "--squash"}
    if autoMerge {
        args = append(args, "--auto")
    }
    return exec.Command("gh", args...).Run()
}
```

---

## 5. Terraform State Backend Implementation

**Location**: `pkg/recipes/terraform/config/backends/s3.go` (new)

```go
package backends

import (
    "context"
    
    "github.com/radius-project/radius/pkg/recipes"
)

// S3Backend implements the Backend interface for AWS S3
type S3Backend struct {
    Bucket        string
    Region        string
    DynamoDBTable string
}

func (b *S3Backend) BuildBackend(resourceRecipe *recipes.ResourceMetadata) (map[string]any, error) {
    key := fmt.Sprintf("radius/%s/%s/%s.tfstate",
        resourceRecipe.Application,
        resourceRecipe.Environment,
        resourceRecipe.Name,
    )
    
    backend := map[string]any{
        "s3": map[string]any{
            "bucket":         b.Bucket,
            "key":            key,
            "region":         b.Region,
            "encrypt":        true,
        },
    }
    
    if b.DynamoDBTable != "" {
        backend["s3"].(map[string]any)["dynamodb_table"] = b.DynamoDBTable
    }
    
    return backend, nil
}

func (b *S3Backend) ValidateBackendExists(ctx context.Context, name string) (bool, error) {
    // Use AWS SDK to check if bucket exists
    // Implementation follows patterns in pkg/cli/aws/client.go
    return true, nil
}
```

---

## 6. Testing Patterns

**Unit tests** follow the existing pattern in `test/radcli/`:

```go
// pkg/cli/cmd/radinit/init_test.go (modify existing)

func Test_Run_GitHubMode(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()
    
    // Setup mocks
    mocks := &radcli.ValidateMocks{
        // Configure mocks for GitHub mode
    }
    
    runner := &Runner{
        provider:     "aws",
        deployTool:   "terraform",
        environment:  "default",
    }
    
    // Create temp git repo
    dir := t.TempDir()
    git.PlainInit(dir, false)
    
    // Run
    err := runner.Run(context.Background())
    require.NoError(t, err)
    
    // Verify files created
    assert.FileExists(t, filepath.Join(dir, ".radius", "types.yaml"))
    assert.FileExists(t, filepath.Join(dir, ".radius", "recipes.yaml"))
    assert.FileExists(t, filepath.Join(dir, ".radius", "env.default.yaml"))
}
```

---

## 7. Key Files to Implement

| Priority | File | Purpose |
|----------|------|---------|
| 1 | `pkg/cli/workspaces/github.go` | GitHub workspace connection type |
| 2 | `pkg/cli/github/ghcli.go` | GitHub CLI wrapper |
| 3 | `pkg/cli/cmd/radinit/init.go` | Enhanced rad init |
| 4 | `pkg/cli/cmd/environment/connect/connect.go` | rad environment connect |
| 5 | `pkg/cli/cmd/pr/create/create.go` | rad pr create |
| 6 | `pkg/cli/cmd/pr/merge/merge.go` | rad pr merge |
| 7 | `pkg/cli/cmd/pr/destroy/destroy.go` | rad pr destroy |
| 8 | `pkg/cli/cmd/plan/deploy/deploy.go` | rad plan deploy |
| 9 | `pkg/recipes/terraform/config/backends/s3.go` | S3 state backend |
| 10 | `pkg/recipes/terraform/config/backends/azurestorage.go` | Azure Storage backend |
| 11 | `pkg/cli/github/workflows/` | GitHub Actions templates |

---

## 8. Constitution Compliance Checklist

Before submitting PRs, verify:

- [ ] Unit tests cover new functionality (Principle IV)
- [ ] Go code passes `make lint` and `make format-check` (Principle II)
- [ ] No cloud-specific assumptions - both AWS and Azure work (Principle III)
- [ ] CLI help text is comprehensive (Principle XIV)
- [ ] Commits include `Signed-off-by` (Principle VI)
- [ ] Feature can be incrementally adopted (Principle IX)
