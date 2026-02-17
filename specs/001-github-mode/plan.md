# Implementation Plan: Radius on GitHub

**Branch**: `001-github-mode` | **Date**: 2026-02-16 | **Spec**: [specs/001-github-mode/spec.md](spec.md)
**Input**: Feature specification from `/specs/001-github-mode/spec.md`

## Summary

Radius on GitHub adds a new operational mode where environment configuration is stored via the GitHub Environments API, application definitions and deployment artifacts live in the repository, and deployments execute in ephemeral k3d clusters within GitHub Action runners. The two-phase model (`rad deployment create` + `rad deployment apply`) separates plan generation from execution. Implementation extends the existing `rad` CLI with new commands and a `github` workspace kind, leveraging the existing Cobra/Runner/Factory patterns, `gh` CLI wrapper, Git helper, and workflow generation infrastructure already present in the codebase.

## Technical Context

**Language/Version**: Go 1.25.7
**Primary Dependencies**: Cobra (CLI framework), Viper (config), `gh` CLI (GitHub API), `go-git/v5` (Git operations), `gopkg.in/yaml.v3` (YAML marshaling), `go.uber.org/mock` (test mocks)
**Storage**: GitHub Environments API (env variables), GitHub repository variables, Git repository (`.radius/` directory tree), resource types repository (`resource-types-contrib`), cloud backends (S3/Azure Storage for Terraform state)
**Testing**: `go test` via `make test`; Cobra command tests using `test/radcli/shared.go` harness (`SharedCommandValidation`, `SharedValidateValidation`); `gomock` for interface mocking
**Target Platform**: macOS, Linux (CLI binary); GitHub Actions runners (workflow execution)
**Project Type**: Single Go module (monorepo with CLI + packages)
**Performance Goals**: `rad init` < 2 min; `rad environment create` < 10 min (with OIDC setup + auth test); k3d + Radius setup < 60 sec in GitHub Actions; deployment < 15 min for ≤5 resources
**Constraints**: No persistent Radius control plane in GitHub mode; all state in GitHub API + repository; `gh` CLI required on user workstations; GitHub Actions minutes quota required
**Scale/Scope**: ~15 new/modified Go source files; 4 new GitHub Actions workflow templates; 8 CLI commands (3 new, 5 modified)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| **I. API-First Design** | ✅ PASS | Uses existing Radius ARM-RPC APIs within ephemeral k3d cluster; new `Radius.Core/applications` resource type follows TypeSpec patterns; no new external APIs introduced — GitHub API is consumed, not created |
| **II. Idiomatic Code Standards** | ✅ PASS | Go: follows Cobra/Runner/Factory patterns established in codebase; godoc on all exported items; `gofmt` + `golangci-lint` enforced |
| **III. Multi-Cloud Neutrality** | ✅ PASS | Supports AWS and Azure with provider-specific abstractions behind `--provider` flag; cloud-specific logic isolated in provider step generators (`generateAWSAuthTestSteps`, `generateAzureAuthTestSteps`) |
| **IV. Testing Pyramid** | ✅ PASS | Unit tests for each command (Validate + Run); mock-based testing via existing `gomock` infrastructure; integration tests for workflow dispatch; functional tests via `test/radcli/shared.go` harness |
| **V. Collaboration-Centric** | ✅ PASS | Two-phase model explicitly separates developer experience (create plan) from platform engineer experience (review/approve/apply); GitHub PR review enables collaboration |
| **VI. Open Source / Community-First** | ✅ PASS | Design spec in public repo; feature branch workflow; DCO signed commits |
| **VII. Simplicity Over Cleverness** | ✅ PASS | Wraps existing `gh` CLI rather than implementing GitHub API client; reuses existing workflow generation infrastructure; no new abstraction layers |
| **VIII. Separation of Concerns** | ✅ PASS | CLI commands in `pkg/cli/cmd/`; GitHub client in `pkg/cli/github/`; workspace config in `pkg/cli/workspaces/`; workflow generation in `pkg/cli/github/workflows.go` — all existing separation maintained |
| **IX. Incremental Adoption** | ✅ PASS | GitHub mode is opt-in via `rad init --github`; Kubernetes mode unchanged; workspace switching supports both; `rad deploy` continues to work for Kubernetes users |
| **X-XI. TypeScript/React** | N/A | No dashboard changes in this feature |
| **XII-XIII. Resource Type/Recipe** | ✅ PASS | New `Radius.Core/applications` resource type follows schema quality standards; recipes referenced by manifest URL, not embedded |
| **XIV-XV. Documentation** | ⚠️ DEFERRED | CLI help text included in implementation; docs repo updates tracked separately |
| **XVI. Repository-Specific** | ✅ PASS | Follows radius repo conventions: `make build`, `make test`, `make lint` |
| **XVII. Polyglot Coherence** | ✅ PASS | Consistent terminology (workspace, environment, application); GitHub mode uses same resource type names as Kubernetes mode |

**Gate Result: PASS** — No blocking violations. Documentation updates deferred to separate docs repo PR.

**Post-Phase 1 Re-check (2026-02-16)**: Constitution compliance confirmed after design phase. Key design decisions:
- GitHub Environment variables (external API) replace local env files → aligns with Principle VII (simplicity) and VIII (separation)
- Commit-hash scoped deployment artifacts → aligns with Principle V (collaboration, auditable)
- Two-phase model (create/apply) → aligns with Principle V (collaboration) and IX (incremental adoption)
- All contracts updated to reflect current data model. No new violations introduced.

## Project Structure

### Documentation (this feature)

```text
specs/001-github-mode/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
├── tasks.md             # Phase 2 output (created by /speckit.tasks)
└── defects.md           # Implementation feedback
```

### Source Code (repository root)

```text
# Modified files (existing)
cmd/rad/cmd/root.go                          # Wire new commands: deploymentCmd group, rad app model
pkg/cli/workspaces/connection.go             # GitHub connection validation updates
pkg/cli/workspaces/github.go                 # GitHub workspace helpers (IsGitHubWorkspace, ParseOwnerRepo)
pkg/cli/github/client.go                     # Add DispatchAndWatch, SetEnvironmentVariable, GetEnvironmentVariables, DeleteEnvironment
pkg/cli/github/git.go                        # Add IsDirty (exists), HasUnpushedCommits, GetHeadCommitHash
pkg/cli/github/workflows.go                  # Update workflow generators for two-phase model (create, apply, destroy, auth-test)
pkg/cli/cmd/radinit/init.go                  # Remove --provider, --deployment-tool; simplify for GitHub mode
pkg/cli/cmd/radinit/github.go               # Update init flow: directory structure, workflow generation, set RADIUS_RESOURCE_TYPES_REPO, git commit+push
pkg/cli/cmd/env/create/create.go             # Branch on workspace kind: GitHub mode → GitHub Environments API + OIDC
pkg/cli/cmd/env/delete/delete.go             # Branch on workspace kind: GitHub mode → delete GitHub Environment + OIDC cleanup prompt
pkg/cli/cmd/deploy/deploy.go                 # Add GitHub mode guard (error if GitHub workspace)
pkg/cli/cmd/app/delete/delete.go             # Branch on workspace kind: GitHub mode → dispatch destroy workflow
pkg/cli/cmd/model/model.go                   # Rename to rad app model (move under applicationCmd)
pkg/cli/config.go                            # Rename workspace "default" to "current"

# New files
pkg/cli/cmd/deployment/create/create.go      # rad deployment create command
pkg/cli/cmd/deployment/create/create_test.go # Unit tests
pkg/cli/cmd/deployment/apply/apply.go        # rad deployment apply command
pkg/cli/cmd/deployment/apply/apply_test.go   # Unit tests
pkg/cli/github/environment.go               # GitHub Environments API operations via gh CLI
pkg/cli/github/environment_test.go           # Unit tests
pkg/cli/github/oidc.go                       # OIDC setup flows for AWS and Azure
pkg/cli/github/oidc_test.go                  # Unit tests
pkg/cli/github/progress.go                   # Animated progress indicator + L key log streaming
pkg/cli/github/progress_test.go              # Unit tests
```

**Structure Decision**: Follows established Radius CLI patterns — each command in its own package under `pkg/cli/cmd/`, shared infrastructure in `pkg/cli/github/`. The `deployment` command group is new, mirroring `env`, `app`, etc. No new top-level directories; all code fits within existing package organization.

## Complexity Tracking

> No constitution violations requiring justification. All implementation follows existing patterns.
