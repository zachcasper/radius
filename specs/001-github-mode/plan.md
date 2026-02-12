# Implementation Plan: Radius on GitHub

**Branch**: `001-github-mode` | **Date**: 2026-02-12 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/001-github-mode/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Implement "Radius on GitHub" mode enabling users to deploy cloud applications using Radius without a centralized Kubernetes control plane. All configuration, plans, and deployment records are stored in the GitHub repository, leveraging GitHub Actions for execution and Pull Requests for review workflows. Requires new CLI commands (`rad init` enhanced, `rad environment connect`, `rad pr create/merge/destroy`) with GitHub workspace support and ephemeral k3d-based control plane execution.

## Technical Context

**Language/Version**: Go 1.21+ (per go.mod)  
**Primary Dependencies**: spf13/cobra (CLI), spf13/viper (config), go-git/go-git (git operations), hashicorp/terraform-exec (Terraform execution), aws-sdk-go-v2 (AWS OIDC), azure-sdk-for-go (Azure OIDC)  
**Storage**: File-based (`.radius/*.yaml` in repository, `~/.rad/config.yaml` locally)  
**Testing**: Go testing with gomock, radcli test helpers, functional tests in test/  
**Target Platform**: Cross-platform CLI (Linux, macOS, Windows), GitHub Actions runners (Linux)
**Project Type**: Single monorepo with multiple command targets  
**Performance Goals**: `rad init` < 5 minutes, k3d + Radius setup < 60 seconds, deployment of 5 resources < 15 minutes  
**Constraints**: Terraform-only initially (Bicep future), AKS/EKS targets only, single active deployment per app/env  
**Scale/Scope**: Individual repositories with multiple environments, concurrent users via Git branching

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. API-First Design | ✅ PASS | No new external APIs exposed; internal CLI command structure follows existing patterns |
| II. Idiomatic Code Standards | ✅ PASS | Go code follows existing Radius CLI patterns (Cobra, Viper, go-git) |
| III. Multi-Cloud Neutrality | ✅ PASS | Supports AWS and Azure equally via provider abstraction in environment config |
| IV. Testing Pyramid Discipline | ✅ PASS | Unit tests for commands, integration tests for GitHub/cloud operations, functional tests for workflows |
| V. Collaboration-Centric Design | ✅ PASS | PR-based workflow enables developer/platform-engineer collaboration via code review |
| VI. Open Source and Community-First | ✅ PASS | Spec in design-notes, external resource-types-contrib dependency |
| VII. Simplicity Over Cleverness | ✅ PASS | File-based storage (YAML), existing CLI patterns, no new abstraction layers |
| VIII. Separation of Concerns | ✅ PASS | Clear separation: CLI commands → GitHub Actions → k3d control plane → cloud providers |
| IX. Incremental Adoption | ✅ PASS | New mode does not break existing Kubernetes mode; users choose via `rad init` vs `rad install kubernetes` |
| X-XI. TypeScript/React Standards | N/A | No dashboard changes in this feature |
| XII-XIII. Resource Types/Recipes | ✅ PASS | Leverages existing resource-types-contrib repo; adds new `Radius.Core/applications` type |
| XIV-XV. Documentation | ✅ PASS | CLI help text, configuration file comments, generated workflow files are self-documenting |
| XVI. Repository-Specific Standards | ✅ PASS | Follows radius repo CONTRIBUTING.md patterns |
| XVII. Polyglot Project Coherence | ✅ PASS | Uses existing terminology (workspace, environment, resource type, recipe); no new cross-repo concepts |

**Gate Result**: ✅ All applicable principles pass. Proceed to Phase 0.

## Project Structure

### Documentation (this feature)

```text
specs/001-github-mode/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
│   └── github-actions-schema.json  # GitHub Actions workflow schema
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
# Existing structure with additions marked [NEW]

pkg/cli/
├── cmd/
│   ├── radinit/
│   │   ├── init.go               # [MODIFY] Enhanced for GitHub mode
│   │   └── init_test.go          # [MODIFY] Tests for new flags/behavior
│   ├── environment/
│   │   └── connect/              # [NEW] rad environment connect
│   │       ├── connect.go
│   │       └── connect_test.go
│   ├── pr/                       # [NEW] rad pr commands
│   │   ├── create/
│   │   │   ├── create.go
│   │   │   └── create_test.go
│   │   ├── merge/
│   │   │   ├── merge.go
│   │   │   └── merge_test.go
│   │   └── destroy/
│   │       ├── destroy.go
│   │       └── destroy_test.go
│   └── plan/                     # [NEW] rad plan commands
│       ├── deploy/
│       │   ├── deploy.go
│       │   └── deploy_test.go
│       └── destroy/
│           ├── destroy.go
│           └── destroy_test.go
├── workspaces/
│   ├── connection.go             # [MODIFY] Add KindGitHub
│   ├── github.go                 # [NEW] GitHub workspace implementation
│   └── github_test.go            # [NEW]
├── github/                       # [NEW] GitHub integration
│   ├── client.go                 # gh CLI wrapper
│   ├── actions.go                # Workflow generation
│   ├── oidc_aws.go               # AWS OIDC setup
│   ├── oidc_azure.go             # Azure OIDC setup
│   └── *_test.go
└── config/
    ├── radyaml.go                # [MODIFY] Support .radius/ config files
    └── types.go                  # [NEW] types.yaml, recipes.yaml, env.yaml parsing

# GitHub Actions workflow templates
pkg/cli/github/workflows/
├── plan.yaml.tmpl                # [NEW] Template for deployment planning workflow
├── deploy.yaml.tmpl              # [NEW] Template for deployment execution workflow
└── destroy.yaml.tmpl             # [NEW] Template for destruction workflow

# Tests
test/
├── functional-portable/
│   └── github/                   # [NEW] Functional tests for GitHub mode
│       ├── init_test.go
│       ├── deploy_test.go
│       └── fixtures/
└── radcli/                       # Use existing test helpers
```

**Structure Decision**: Extends existing CLI structure in `pkg/cli/cmd/` with new command groups (`pr/`, `plan/`) and adds GitHub integration package (`pkg/cli/github/`). No new top-level directories; maintains existing patterns.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No violations identified. All constitution principles pass without exceptions.

## Constitution Re-evaluation (Post-Design)

*Performed after Phase 1 design artifacts were created.*

| Principle | Status | Post-Design Notes |
|-----------|--------|-------------------|
| I. API-First Design | ✅ PASS | Configuration schemas defined in contracts/ (JSON Schema). No external REST APIs. |
| II. Idiomatic Code Standards | ✅ PASS | Quickstart shows idiomatic Go patterns following existing codebase conventions. |
| III. Multi-Cloud Neutrality | ✅ PASS | Data model defines symmetric AWS/Azure provider configs. Both providers supported equally. |
| IV. Testing Pyramid Discipline | ✅ PASS | Quickstart documents testing patterns. Unit tests use gomock, integration tests for cloud ops. |
| VII. Simplicity Over Cleverness | ✅ PASS | Data model uses simple YAML/JSON structures. No abstract factory or DAO patterns introduced. |
| VIII. Separation of Concerns | ✅ PASS | Clear packages: `workspaces/`, `github/`, `cmd/`. No circular dependencies. |
| XVII. Polyglot Project Coherence | ✅ PASS | Reuses existing types (Backend interface, Workspace). Consistent terminology across artifacts. |

**Post-Design Gate Result**: ✅ All principles continue to pass. Design is constitution-compliant.

