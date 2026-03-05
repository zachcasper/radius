# Tasks: Radius on GitHub

**Input**: Design documents from `/specs/001-github-mode/`
**Prerequisites**: plan.md, spec.md (7 user stories), research.md, data-model.md, contracts/

**Tests**: Test tasks included per Constitution Principle IV (Testing Pyramid, NON-NEGOTIABLE). Each new package includes corresponding unit test tasks.

**Organization**: Tasks grouped by user story to enable independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2)
- Exact file paths included per plan.md Project Structure

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization, workspace config updates, and shared infrastructure needed by all stories

- [X] T001 Rename workspace config property `default` to `current` in pkg/cli/config.go and update all references across pkg/cli/ (FR-012, FR-071)
- [X] T002 [P] Add GitHub connection validation to pkg/cli/workspaces/connection.go — validate `Kind == "github"` requires non-empty `URL` with GitHub hostname (FR-072)
- [X] T003 [P] Create helper functions `IsGitHubWorkspace()` and `ParseOwnerRepo()` in pkg/cli/workspaces/github.go (used by all GitHub-mode commands)
- [X] T004 [P] Leverage existing `IsDirty()` and add `GetHeadCommitHash()` method to pkg/cli/github/git.go (used by rad deploy, FR-041, FR-043)
- [X] T005 [P] Add `HasUnpushedCommits()` method to pkg/cli/github/git.go (used by rad deploy, FR-042)
- [X] T006 [P] Add default resource group fallback logic in pkg/cli/cmd/radinit/init.go or relevant resolver — when no `--group` flag and no workspace scope, fall back to `default` resource group (FR-115)
- [X] T007-A [P] Remove `rad pr` command group (cmd/rad/cmd/pr.go, pkg/cli/cmd/pr/) — `rad pr create`, `rad pr merge`, `rad pr destroy` are removed (FR-032, FR-033, FR-034)
- [X] T007-B [P] Rename `rad environment connect` to `rad environment create` — remove pkg/cli/cmd/env/connect/ and update command wiring in cmd/rad/cmd/root.go (FR-015)
- [X] T007-C [P] Add GitHub-mode routing in `rad resource-type` commands — when workspace kind is `github`, operate against `RADIUS_CONFIG_REPO` URL instead of UCP (FR-073)
- [X] T007-D [P] Add GitHub-mode routing in `rad recipe` commands — when workspace kind is `github`, operate against environment's `RADIUS_RECIPES_MANIFEST` variable instead of UCP (FR-075)

**Checkpoint**: Shared infrastructure ready — user story implementation can begin

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T008 Create pkg/cli/github/environment.go — implement GitHub Environments API operations via `gh` CLI: `CreateEnvironment()`, `DeleteEnvironment()`, `SetEnvironmentVariable()`, `GetEnvironmentVariables()`, `SetRepoVariable()` (used by US1, US2, US6)
- [X] T008-T [P] Create pkg/cli/github/environment_test.go — unit tests for environment API operations and parsing logic
- [X] T009 [P] Create pkg/cli/github/progress.go — implement animated progress indicator using Charm Bubble Tea with automatic workflow step status display showing each step's progress (FR-089-C through FR-089-G; used by US2, US4, US5, US6)
- [X] T009-T [P] Create pkg/cli/github/progress_test.go — unit tests for progress model state machine (state transitions, messages, view)
- [X] T010 [P] Update pkg/cli/github/workflows.go — add workflow generation functions: `GenerateDeployWorkflow()`, `GenerateAppDeleteWorkflow()`, `GenerateAuthTestWorkflow()` producing 3 YAML workflow files with concurrency groups (FR-098, FR-099, FR-112, FR-113, FR-113-A)
- [X] T011 [P] Add `DispatchAndWatch()` method to pkg/cli/github/client.go — dispatch a workflow, poll for run start, return run ID for progress tracking (combines `RunWorkflow` + `GetLatestWorkflowRun`; used by US2, US4, US5, US6)
- [X] T011-T [P] Add concurrency queue detection to `DispatchAndWatch()` — if workflow is queued behind another run, display message "Another deployment is in progress, this run is queued..." (FR-100)
- [X] T012 Wire `rad app model` subcommand in cmd/rad/cmd/root.go — move `rad model` under `applicationCmd` as `rad app model` (FR-068-A)

**Checkpoint**: Foundation ready — user story implementation can now begin in parallel

---

## Phase 3: User Story 1 — Initialize Repository for Radius (Priority: P1) 🎯 MVP

**Goal**: `rad init` sets up `.radius/` directory structure, generates 4 GitHub Actions workflows, sets `RADIUS_CONFIG_REPO` repo variable, creates GitHub workspace in `~/.rad/config.yaml`, commits and pushes.

**Independent Test**: Run `rad init` on a fresh GitHub repo clone → verify workspace registered, workflows created, repo variable set, changes committed.

### Implementation for User Story 1

- [X] T013 [US1] Update pkg/cli/cmd/radinit/init.go — remove `--provider` and `--deployment-tool` flags; add error if those flags are passed (FR-001, FR-007)
- [X] T014 [US1] Update pkg/cli/cmd/radinit/github.go — rewrite `Run()`:
  1. Call workflow generators from T010 to produce 3 workflow files under `.github/workflows/` (FR-112, FR-114)
  2. Call `SetRepoVariable("RADIUS_CONFIG_REPO", url)` with default or `--config-repo` value (FR-005, FR-006, FR-007)
  3. Create/update `~/.rad/config.yaml` with `github` workspace (FR-011)
  4. `git add`, `git commit` with `Radius-Action: init` trailer, `git push` (FR-013)
  5. Verify `.radius/types.yaml`, `.radius/recipes.yaml`, and `.radius/env.*.yaml` are NOT created (FR-002, FR-003, FR-004)
  6. Do NOT create `.radius/applications/` directory — this is created on demand by `rad app model` (FR-014-A)
- [X] T015 [US1] Update `Validate()` in pkg/cli/cmd/radinit/github.go — verify git repo (FR-008), GitHub remote (FR-009), `gh auth status` (FR-010); ensure non-interactive (FR-014)
- [X] T016 [US1] Handle re-initialization: if `.radius/` already exists, warn and offer to reinitialize (US1 scenario 6)
- [X] T016-T [US1] Add unit tests for pkg/cli/cmd/radinit/github.go — tests for workflow generation and git root finding

**Checkpoint**: `rad init` fully functional — repository can be initialized for Radius on GitHub

---

## Phase 4: User Story 2 — Create Environment with Cloud Provider (Priority: P1)

**Goal**: `rad environment create <name> --provider <aws|azure>` creates a GitHub Environment, sets up OIDC, stores env variables, dispatches auth test workflow, reports result.

**Independent Test**: Run `rad environment create dev --provider azure` → verify GitHub Environment created, env variables set, auth test passes.

### Implementation for User Story 2

- [X] T017 [US2] Create pkg/cli/github/oidc.go — extract OIDC setup flows from pkg/cli/cmd/env/connect/connect.go into reusable functions: `SetupAzureOIDC()` and `SetupAWSOIDC()` that handle prompting, credential creation, and federated identity setup (FR-024, FR-028)
- [X] T017-T [US2] Create pkg/cli/github/oidc_test.go — unit tests for OIDC structs and helper functions
- [X] T018 [US2] Update pkg/cli/cmd/env/create/create.go — branch on workspace kind:
  - GitHub mode: create GitHub Environment via API (FR-022), run OIDC setup (FR-024/FR-028), set all env variables (FR-025/FR-029), set `RADIUS_RECIPES_MANIFEST` with correct default based on provider + deployment tool (FR-026, FR-027, FR-030), accept `--recipes` override (FR-023)
  - Kubernetes mode: retain existing behavior (FR-016)
  - Error if `--provider aws --deployment-tool bicep` (FR-021)
- [X] T019 [US2] Add auth test dispatch to pkg/cli/cmd/env/create/create.go — after env variables are stored, dispatch `radius-auth-test.yml` workflow, show animated progress via T009, display success/failure with OIDC remediation hints (FR-030-E through FR-030-I)
- [X] T020 [US2] Add Terraform state backend provisioning to pkg/cli/cmd/env/create/create.go — create S3 bucket (FR-097-B) or Azure Storage account (FR-097-A) for Terraform state, store location as env variable (FR-093 through FR-097)
- [X] T021 [US2] Handle edge cases: environment already exists warning (US2 scenario 5), AWS CLI not installed error (US2 scenario 8), non-GitHub workspace error (US2 scenario 11)
- [X] T021-T [US2] Add unit tests for pkg/cli/cmd/env/create/create.go — test GitHub-mode branching, OIDC dispatch, TF state provisioning, and all edge cases

**Checkpoint**: `rad environment create` fully functional — environments can be provisioned with OIDC

---

## Phase 5: User Story 3 — Create Application Definition (Priority: P1)

**Goal**: `rad app model` generates a sample `.radius/applications/todolist.bicep` with application, container, and database resources.

**Independent Test**: Run `rad app model` in an initialized repo → verify Bicep file created with correct resources and syntax.

### Implementation for User Story 3

- [X] T022 [US3] Move pkg/cli/cmd/model/model.go under applicationCmd — rename from `rad model` to `rad app model`; update cmd/rad/cmd/root.go wiring from T012 (FR-068-A)
- [X] T023 [US3] Update pkg/cli/cmd/model/model.go `Run()` — create `.radius/applications/` directory if it doesn't exist (FR-068-B), then generate `.radius/applications/todolist.bicep` with `extension radius`, `Radius.Core/applications@2025-08-01-preview`, `Radius.Compute/containers@2025-08-01-preview`, `Radius.Data/postgreSqlDatabases@2025-08-01-preview` resources (FR-068-A, FR-090, FR-091, FR-092, spec appendix B.5)
- [X] T024 [US3] Add `Validate()` checks — require initialized repository (`.radius/` exists); if existing file at target path, prompt to overwrite or choose different name (US3 scenarios 4, 5)
- [X] T024-T [US3] Add unit tests for rad app model — test Validate() and Run() including overwrite prompt

**Checkpoint**: `rad app model` creates sample application definition

---

## Phase 6: User Story 4 — Deploy an Application (Priority: P1) 🎯 MVP

**Goal**: `rad deploy <bicep-file> --environment <env>` dispatches `rad-deploy.yaml` workflow that deploys the application to the specified environment.

**Independent Test**: Run `rad deploy .radius/applications/todolist.bicep --environment dev` → verify workflow dispatched, application deployed.

### Implementation for User Story 4

- [X] T025 [US4] Update pkg/cli/cmd/deploy/deploy.go — branch on workspace kind:
  - GitHub mode: dispatch `rad-deploy.yaml` workflow with bicep file path and environment inputs (FR-036, FR-037)
  - Kubernetes mode: retain existing deployment behavior (FR-037)
- [X] T026 [US4] Implement GitHub-mode `Validate()` in pkg/cli/cmd/deploy/deploy.go:
  - Require GitHub workspace for workflow dispatch (FR-037)
  - Require clean worktree via `IsWorktreeDirty()` (FR-041)
  - Require all commits pushed via `HasUnpushedCommits()` (FR-042)
  - Auto-select environment if exactly one GitHub Environment (FR-039); error if ambiguous (FR-040)
  - Resolve commit hash for traceability (FR-043)
- [X] T027 [US4] Implement GitHub-mode `Run()` in pkg/cli/cmd/deploy/deploy.go:
  - Dispatch `rad-deploy.yaml` workflow with bicep file, environment, commit inputs (FR-045)
  - Show animated progress indicator via progress model from T009 (FR-089-C through FR-089-G)
  - Display final result (success: deployed; failure: error details) (FR-089-G)
- [X] T027-T [US4] Add unit tests for GitHub-mode deploy in pkg/cli/cmd/deploy/deploy_test.go — runner structure and workspace detection tests

**Checkpoint**: `rad deploy` dispatches workflow and deploys application

---

## Phase 7: User Story 5 — Delete Deployed Application (Priority: P1)

**Goal**: `rad app delete --application <name> --environment <env>` dispatches `rad-app-delete.yaml` workflow in GitHub mode.

**Independent Test**: Deploy an app, run `rad app delete --application todolist --environment dev` → verify resources destroyed.

### Implementation for User Story 5

- [X] T028 [US5] Update pkg/cli/cmd/app/delete/delete.go — branch on workspace kind:
  - GitHub mode: dispatch `rad-app-delete.yaml` workflow with application and environment inputs (FR-106-A)
  - Kubernetes mode: retain existing destroy behavior (US5 scenario 5)
- [X] T029 [US5] Add progress and validation to pkg/cli/cmd/app/delete/delete.go:
  - Show animated progress indicator with automatic workflow step status display (FR-106-D)
  - Error if `--application` omitted (FR-106-B, FR-107)
  - Error if `--environment` omitted and multiple environments exist (FR-106-C)
  - Error if application not deployed to specified environment (US5 scenario 4)
- [X] T030 [US5] Verify destroy workflow executes `rad app delete` within k3d cluster to remove application resources (FR-106-A)
- [X] T030-T [US5] Add unit tests for GitHub-mode delete in pkg/cli/cmd/app/delete/delete_test.go — runner structure and workspace detection tests

**Checkpoint**: Full deployment lifecycle (init → env create → model → deploy → delete) works

---

## Phase 8: User Story 6 — Delete Environment (Priority: P2)

**Goal**: `rad environment delete <name>` deletes GitHub Environment, prompts for OIDC cleanup.

**Independent Test**: Run `rad environment delete dev` → verify GitHub Environment removed, OIDC resources cleaned up if confirmed.

### Implementation for User Story 6

- [X] T031 [US6] Update pkg/cli/cmd/env/delete/delete.go — branch on workspace kind:
  - GitHub mode: check for deployed applications, prompt for deletion strategy (FR-030-D, US6 scenario 4)
  - Kubernetes mode: retain existing behavior (FR-030-C, US6 scenario 3)
- [X] T032 [US6] Add OIDC cleanup prompt to pkg/cli/cmd/env/delete/delete.go:
  - Read `AZURE_CLIENT_ID` or `AWS_ROLE_ARN` from env variables before deletion (FR-030-N)
  - Prompt whether to delete cloud OIDC resources (FR-030-J)
  - Azure: `az ad app delete --id <CLIENT_ID>` (FR-030-K)
  - AWS: `aws iam delete-role` + `aws iam delete-open-id-connect-provider` (FR-030-L)
  - If declined: display resource identifiers for manual cleanup (FR-030-M)
- [X] T033 [US6] Delete GitHub Environment via `DeleteEnvironment()` after OIDC cleanup (FR-030-B); error if environment doesn't exist (US6 scenario 2)

**Checkpoint**: Environment lifecycle (create + delete) fully functional

---

## Phase 9: User Story 7 — Manage Workspaces Across Repositories (Priority: P3)

**Goal**: Seamless workspace switching between GitHub and Kubernetes modes with appropriate command behavior.

**Independent Test**: Configure multiple workspaces, switch between them, verify commands behave appropriately for each kind.

### Implementation for User Story 7

- [X] T034 [US7] Verify workspace switching logic in pkg/cli/workspaces/ — ensure `rad workspace switch` properly loads GitHub workspace URL and Kubernetes context; verify command routing uses workspace kind to select correct code paths (US7 scenarios 1-3)
- [X] T035 [US7] Verify `rad install kubernetes` path is unaffected — ensure `rad init` changes do not interfere with the Kubernetes control plane installation path (US7 scenario 4)
- [X] T036 [US7] Verify `rad deploy` behavior varies by workspace kind — GitHub mode dispatches workflow, Kubernetes mode executes directly (US7 scenario 2)

**Checkpoint**: Multi-workspace management works seamlessly

---

## Phase 10: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple user stories

- [X] T037 [P] Add `--help` text for all modified commands (rad deploy, rad app delete, rad app model) with examples and flag descriptions per Cobra conventions (FR-089-A naming convention in help text)
- [X] T038 [P] Add GitHub Secrets reference documentation in generated workflow file comments — document `${{ secrets.SECRET_NAME }}` syntax (FR-108 through FR-111)
- [X] T039 [P] Ensure all generated GitHub Actions workflows use only trusted external actions: `actions/*`, `radius-project/*`, `aws-actions/*`, `azure/*`, `google-github-actions/*`, `hashicorp/*` (FR-089-B)
- [X] T040 Verify workflow naming convention in all 3 generated workflows: `Radius deploy for <bicep_file> in <env> environment`, `Radius app delete for <app_name> in <env> environment`, `Radius authentication test for <env> environment` (FR-089-A)
- [X] T041 [P] Add `--skip-contour-install` and `--set dashboard.enabled=false` flags to Radius installation steps in generated deployment workflows (FR-088-A)
- [X] T042 Run quickstart.md validation — execute the end-to-end flow from specs/001-github-mode/quickstart.md and verify all steps pass
- [X] T042-A Verify partial failure handling in generated workflows — successfully deployed resources remain in place, workflow exits with failure status, re-run via `rad deploy` works (FR-102 through FR-105)
- [X] T042-B ⚠️ CROSS-REPO: Create `Radius.Core/applications` resource type in `radius-project/resource-types-contrib` repository with `environment` property (FR-090, FR-091) — tracked separately from this feature's implementation

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion — BLOCKS all user stories
- **User Stories (Phases 3-9)**: All depend on Foundational phase completion
  - P1 stories (Phases 3-7) form the MVP and should be completed first
  - P2 story (Phase 8) adds environment lifecycle cleanup
  - P3 story (Phase 9) adds workspace management polish
- **Polish (Phase 10)**: Depends on all desired user stories being complete

### User Story Dependencies

- **US1 (rad init)**: Can start after Foundational — No dependencies on other stories
- **US2 (rad env create)**: Can start after Foundational — Independent of US1 at code level (but logically runs after init)
- **US3 (rad app model)**: Can start after Foundational — Depends on T012 wiring only
- **US4 (rad deploy)**: Can start after Foundational — Uses shared infra from Phase 2
- **US5 (rad app delete)**: Can start after Foundational — Extends existing delete command
- **US6 (rad env delete)**: Can start after Foundational — Extends existing delete command
- **US7 (workspaces)**: Can start after T001-T003 — verification of existing switching logic

### Within Each User Story

- `Validate()` before `Run()` implementation
- Core flow before edge cases
- Command implementation before wiring

### Parallel Opportunities

- T002, T003, T004, T005, T006 can all run in parallel (Phase 1 — different files)
- T009, T010, T011 can all run in parallel (Phase 2 — different files)
- Once Foundational completes: US1 through US5 can all start in parallel (different command packages)
- US6 and US7 can run in parallel with each other

---

## Parallel Example: Phase 1 (Setup)

```
# Launch all parallelizable setup tasks together:
Task T002: "Add GitHub connection validation in pkg/cli/workspaces/connection.go"
Task T003: "Create IsGitHubWorkspace() helper in pkg/cli/workspaces/github.go"
Task T004: "Leverage existing IsDirty() and add GetHeadCommitHash() in pkg/cli/github/git.go"
Task T005: "Add HasUnpushedCommits() to pkg/cli/github/git.go"
Task T006: "Add default resource group fallback"
```

## Parallel Example: Phase 2 (Foundational)

```
# Launch all parallelizable foundational tasks together:
Task T009: "Create animated progress indicator in pkg/cli/github/progress.go"
Task T010: "Add workflow generation functions in pkg/cli/github/workflows.go"
Task T011: "Add DispatchAndWatch() to pkg/cli/github/client.go"
```

## Parallel Example: P1 User Stories (after Foundational)

```
# Different developers can work on different stories simultaneously:
Developer A: US1 (T013-T016) — rad init
Developer B: US2 (T017-T021) — rad environment create
Developer C: US4 (T025-T027) — rad deploy
Developer D: US5 (T028-T030) — rad app delete
```

---

## Implementation Strategy

### MVP First (P1 User Stories Only)

1. Complete Phase 1: Setup (T001-T007)
2. Complete Phase 2: Foundational (T008-T012)
3. Complete Phases 3-7: All P1 user stories (US1-US5)
4. **STOP and VALIDATE**: Test full lifecycle: init → env create → app model → deploy → app delete
5. Deploy/demo if ready

### Incremental Delivery

1. Setup + Foundational → Foundation ready
2. US1 (rad init) → Initialize a real GitHub repo (first visible output)
3. US2 (rad env create) → Set up cloud environment with OIDC
4. US3 (rad app model) → Create application definition
5. US4 (rad deploy) → Full deployment works (core MVP!)
6. US5 (rad app delete) → Cleanup works
7. US6 (rad env delete) → Full environment lifecycle management
8. US7 (workspaces) → Multi-workspace polish

### Parallel Team Strategy

With multiple developers:

1. Team completes Setup + Foundational together
2. Once Foundational is done:
   - Developer A: US1 (rad init) + US3 (rad app model) — setup commands
   - Developer B: US2 (rad env create) + US6 (rad env delete) — environment lifecycle
   - Developer C: US4 (rad deploy) — core deployment
   - Developer D: US5 (rad app delete) + US7 (workspaces) — cleanup and polish
3. Stories complete and integrate independently

---

## Notes

- [P] tasks = different files, no dependencies on incomplete tasks in this phase
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable after Foundational phase
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- All file paths are relative to repository root