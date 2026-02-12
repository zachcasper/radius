# Tasks: Radius on GitHub

**Input**: Design documents from `/specs/001-github-mode/`  
**Prerequisites**: plan.md ✓, spec.md ✓, research.md ✓, data-model.md ✓, contracts/ ✓, quickstart.md ✓

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, etc.)
- File paths use repository root conventions (pkg/cli/...)

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and core infrastructure types

- [ ] T001 Add KindGitHub constant to pkg/cli/workspaces/connection.go
- [ ] T002 [P] Create GitHubConnection type in pkg/cli/workspaces/github.go
- [ ] T003 [P] Create unit tests for GitHubConnection in pkg/cli/workspaces/github_test.go
- [ ] T004 [P] Create ResourceTypesManifest type in pkg/cli/config/types.go
- [ ] T005 [P] Create RecipesManifest type in pkg/cli/config/recipes.go
- [ ] T006 [P] Create Environment type in pkg/cli/config/environment.go
- [ ] T007 [P] Create DeploymentPlan type in pkg/cli/config/plan.go
- [ ] T008 [P] Create DeploymentRecord type in pkg/cli/config/record.go
- [ ] T009 Update pkg/cli/workspaces/connection.go CreateConnection to handle GitHub kind
- [ ] T010 Create YAML parser for .radius/ config files in pkg/cli/config/parser.go

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [ ] T011 Create GitHub CLI wrapper package in pkg/cli/github/client.go with AuthStatus(), CreatePR(), MergePR(), RunWorkflow()
- [ ] T012 [P] Create unit tests for GitHub client in pkg/cli/github/client_test.go
- [ ] T013 [P] Create git operations helper in pkg/cli/github/git.go using go-git for init, add, commit with trailers
- [ ] T014 [P] Create unit tests for git helper in pkg/cli/github/git_test.go
- [ ] T015 Create S3 Terraform state backend in pkg/recipes/terraform/config/backends/s3.go implementing Backend interface
- [ ] T016 [P] Create unit tests for S3 backend in pkg/recipes/terraform/config/backends/s3_test.go
- [ ] T017 [P] Create Azure Storage Terraform state backend in pkg/recipes/terraform/config/backends/azurestorage.go
- [ ] T018 [P] Create unit tests for Azure Storage backend in pkg/recipes/terraform/config/backends/azurestorage_test.go
- [ ] T019 Create GitHub Actions workflow struct types in pkg/cli/github/workflows.go
- [ ] T020 [P] Create workflow YAML generation using goccy/go-yaml in pkg/cli/github/workflows.go

**Checkpoint**: Foundation ready - user story implementation can now begin in parallel

---

## Phase 3: User Story 1 - Initialize Repository for Radius (Priority: P1) 🎯 MVP

**Goal**: Users can run `rad init` to set up a GitHub repository for Radius deployments

**Independent Test**: Run `rad init --provider aws --deployment-tool terraform` on a fresh git clone, verify .radius/ files created and workspace registered

### Implementation for User Story 1

- [ ] T021 [US1] Add --provider flag (aws|azure) to rad init command in pkg/cli/cmd/radinit/init.go
- [ ] T022 [US1] Add --deployment-tool flag (terraform|bicep) to rad init command in pkg/cli/cmd/radinit/init.go
- [ ] T023 [US1] Add --environment flag with default "default" to rad init command in pkg/cli/cmd/radinit/init.go
- [ ] T024 [US1] Implement git repository validation (check .git directory) in pkg/cli/cmd/radinit/init.go
- [ ] T025 [US1] Implement GitHub remote validation (parse origin URL) in pkg/cli/cmd/radinit/init.go
- [ ] T026 [US1] Implement gh auth status check in pkg/cli/cmd/radinit/init.go
- [ ] T027 [US1] Implement .radius/ directory creation in pkg/cli/cmd/radinit/init.go
- [ ] T028 [US1] Implement resource types fetch from radius-project/resource-types-contrib using sparse checkout in pkg/cli/github/resourcetypes.go
- [ ] T029 [US1] Implement .radius/types.yaml generation from fetched resource types in pkg/cli/cmd/radinit/init.go
- [ ] T030 [US1] Implement .radius/recipes.yaml generation based on provider/tool selection in pkg/cli/cmd/radinit/init.go
- [ ] T031 [US1] Implement .radius/env.<name>.yaml generation with placeholder structure in pkg/cli/cmd/radinit/init.go
- [ ] T032 [US1] Implement ~/.rad/config.yaml update with github kind workspace in pkg/cli/cmd/radinit/init.go
- [ ] T033 [US1] Rename workspace config property "default" to "current" in pkg/cli/workspaces/types.go
- [ ] T034 [US1] Implement git add and commit with Radius-Action: init trailer in pkg/cli/cmd/radinit/init.go
- [ ] T035 [P] [US1] Add unit tests for new rad init GitHub mode in pkg/cli/cmd/radinit/init_test.go
- [ ] T036 [P] [US1] Add integration test for rad init end-to-end in test/functional-portable/github/init_test.go

**Checkpoint**: User Story 1 complete - rad init creates all .radius/ config files and registers workspace

---

## Phase 4: User Story 2 - Connect Cloud Provider Authentication (Priority: P1)

**Goal**: Users can run `rad environment connect` to configure OIDC authentication with AWS or Azure

**Independent Test**: Run `rad environment connect` after init, verify OIDC role/app created (or existing validated) and env file updated

### Implementation for User Story 2

- [ ] T037 [US2] Create rad environment connect command scaffold in pkg/cli/cmd/environment/connect/connect.go
- [ ] T038 [US2] Add --environment flag with default to current workspace environment in pkg/cli/cmd/environment/connect/connect.go
- [ ] T039 [US2] Implement GitHub workspace validation check in pkg/cli/cmd/environment/connect/connect.go
- [ ] T040 [US2] Create AWS OIDC setup helper in pkg/cli/github/oidc_aws.go
- [ ] T041 [US2] Implement AWS account ID prompt (default from aws sts get-caller-identity) in pkg/cli/github/oidc_aws.go
- [ ] T042 [US2] Implement AWS region prompt (default from aws configure get region) in pkg/cli/github/oidc_aws.go
- [ ] T043 [US2] Implement AWS IAM OIDC provider creation via aws CLI in pkg/cli/github/oidc_aws.go
- [ ] T044 [US2] Implement AWS IAM role creation with trust policy for GitHub Actions in pkg/cli/github/oidc_aws.go
- [ ] T045 [US2] Implement S3 state backend bucket creation in pkg/cli/github/oidc_aws.go
- [ ] T046 [US2] Implement DynamoDB table creation for state locking in pkg/cli/github/oidc_aws.go
- [ ] T047 [P] [US2] Create Azure OIDC setup helper in pkg/cli/github/oidc_azure.go
- [ ] T048 [P] [US2] Implement Azure subscription list prompt in pkg/cli/github/oidc_azure.go
- [ ] T049 [P] [US2] Implement Azure AD app creation via az CLI in pkg/cli/github/oidc_azure.go
- [ ] T050 [P] [US2] Implement Azure federated credential creation for GitHub Actions in pkg/cli/github/oidc_azure.go
- [ ] T051 [P] [US2] Implement Azure Storage account/container creation for state in pkg/cli/github/oidc_azure.go
- [ ] T052 [US2] Implement .radius/env.<name>.yaml update with provider config in pkg/cli/cmd/environment/connect/connect.go
- [ ] T053 [US2] Implement git commit with Radius-Action: environment-connect trailer in pkg/cli/cmd/environment/connect/connect.go
- [ ] T054 [P] [US2] Add unit tests for rad environment connect in pkg/cli/cmd/environment/connect/connect_test.go
- [ ] T055 [P] [US2] Add unit tests for AWS OIDC helper in pkg/cli/github/oidc_aws_test.go
- [ ] T056 [P] [US2] Add unit tests for Azure OIDC helper in pkg/cli/github/oidc_azure_test.go

**Checkpoint**: User Story 2 complete - OIDC authentication configured for chosen cloud provider

---

## Phase 5: User Story 3 - Create Deployment Plan via Pull Request (Priority: P2)

**Goal**: Users can run `rad pr create` to generate a deployment plan and create a PR for review

**Independent Test**: Have app model in .radius/model/, run `rad pr create --environment dev`, verify PR created with plan files

### Implementation for User Story 3

- [ ] T057 [US3] Create rad pr command group scaffold in pkg/cli/cmd/pr/pr.go
- [ ] T058 [US3] Create rad pr create command in pkg/cli/cmd/pr/create/create.go
- [ ] T059 [US3] Add --environment required flag to rad pr create in pkg/cli/cmd/pr/create/create.go
- [ ] T060 [US3] Add --application optional flag to rad pr create in pkg/cli/cmd/pr/create/create.go
- [ ] T061 [US3] Implement GitHub workflow trigger via gh workflow run in pkg/cli/cmd/pr/create/create.go
- [ ] T062 [US3] Create plan workflow template in pkg/cli/github/workflows/plan.yaml.tmpl
- [ ] T063 [US3] Implement k3d cluster setup step in workflow template in pkg/cli/github/workflows/plan.yaml.tmpl
- [ ] T064 [US3] Implement Radius install step in workflow template in pkg/cli/github/workflows/plan.yaml.tmpl
- [ ] T065 [US3] Create rad plan command group scaffold in pkg/cli/cmd/plan/plan.go
- [ ] T066 [US3] Create rad plan deploy command in pkg/cli/cmd/plan/deploy/deploy.go
- [ ] T067 [US3] Implement plan.yaml generation with ordered steps in pkg/cli/cmd/plan/deploy/deploy.go
- [ ] T068 [US3] Implement deployment artifact directory creation (main.tf, providers.tf, etc.) in pkg/cli/cmd/plan/deploy/deploy.go
- [ ] T069 [US3] Implement PR creation with plan files via gh pr create in workflow template
- [ ] T070 [P] [US3] Add unit tests for rad pr create in pkg/cli/cmd/pr/create/create_test.go
- [ ] T071 [P] [US3] Add unit tests for rad plan deploy in pkg/cli/cmd/plan/deploy/deploy_test.go

**Checkpoint**: User Story 3 complete - deployment plans can be created and reviewed via PRs

---

## Phase 6: User Story 4 - Deploy Application via PR Merge (Priority: P2)

**Goal**: Users can run `rad pr merge` to execute deployment from an approved PR

**Independent Test**: Have deployment PR, run `rad pr merge`, verify resources provisioned and deployment record created

### Implementation for User Story 4

- [ ] T072 [US4] Create rad pr merge command in pkg/cli/cmd/pr/merge/merge.go
- [ ] T073 [US4] Add --pr optional flag to specify PR number in pkg/cli/cmd/pr/merge/merge.go
- [ ] T074 [US4] Add --yes flag for automatic merge without confirmation in pkg/cli/cmd/pr/merge/merge.go
- [ ] T075 [US4] Implement latest PR detection when --pr not specified in pkg/cli/cmd/pr/merge/merge.go
- [ ] T076 [US4] Implement PR merge via gh pr merge in pkg/cli/cmd/pr/merge/merge.go
- [ ] T077 [US4] Create deploy workflow template in pkg/cli/github/workflows/deploy.yaml.tmpl
- [ ] T078 [US4] Create rad deploy command in pkg/cli/cmd/deploy/deploy.go
- [ ] T079 [US4] Implement plan.yaml parsing in rad deploy in pkg/cli/cmd/deploy/deploy.go
- [ ] T080 [US4] Implement sequential Terraform execution using hashicorp/terraform-exec in pkg/cli/cmd/deploy/deploy.go
- [ ] T081 [US4] Implement deployment record creation in .radius/deploy/<app>/<env>/<commit>/ in pkg/cli/cmd/deploy/deploy.go
- [ ] T082 [US4] Implement captured resource definition storage (YAML/JSON) in pkg/cli/cmd/deploy/deploy.go
- [ ] T083 [US4] Implement deployment queuing (FIFO) via GitHub Actions concurrency in pkg/cli/github/workflows/deploy.yaml.tmpl
- [ ] T084 [US4] Implement partial failure handling (leave resources in place) in pkg/cli/cmd/deploy/deploy.go
- [ ] T085 [P] [US4] Add unit tests for rad pr merge in pkg/cli/cmd/pr/merge/merge_test.go
- [ ] T086 [P] [US4] Add unit tests for rad deploy in pkg/cli/cmd/deploy/deploy_test.go

**Checkpoint**: User Story 4 complete - deployments execute on PR merge with audit records

---

## Phase 7: User Story 5 - Destroy Application Resources (Priority: P3)

**Goal**: Users can run `rad pr destroy` to tear down deployed resources

**Independent Test**: Have deployed resources, run `rad pr destroy --environment dev --application myapp`, verify destruction PR created and resources removed after merge

### Implementation for User Story 5

- [ ] T087 [US5] Create rad pr destroy command in pkg/cli/cmd/pr/destroy/destroy.go
- [ ] T088 [US5] Add --environment required flag in pkg/cli/cmd/pr/destroy/destroy.go
- [ ] T089 [US5] Add --application required flag (per clarification) in pkg/cli/cmd/pr/destroy/destroy.go
- [ ] T090 [US5] Add --commit optional flag to target specific deployment in pkg/cli/cmd/pr/destroy/destroy.go
- [ ] T091 [US5] Add --yes flag for automatic merge in pkg/cli/cmd/pr/destroy/destroy.go
- [ ] T092 [US5] Create rad plan destroy command in pkg/cli/cmd/plan/destroy/destroy.go
- [ ] T093 [US5] Implement destruction plan.yaml generation in pkg/cli/cmd/plan/destroy/destroy.go
- [ ] T094 [US5] Create destroy workflow template in pkg/cli/github/workflows/destroy.yaml.tmpl
- [ ] T095 [US5] Create rad destroy command in pkg/cli/cmd/destroy/destroy.go
- [ ] T096 [US5] Implement Terraform destroy execution in pkg/cli/cmd/destroy/destroy.go
- [ ] T097 [US5] Implement destruction record creation in .radius/deploy/<app>/<env>/<commit>/ in pkg/cli/cmd/destroy/destroy.go
- [ ] T098 [P] [US5] Add unit tests for rad pr destroy in pkg/cli/cmd/pr/destroy/destroy_test.go
- [ ] T099 [P] [US5] Add unit tests for rad destroy in pkg/cli/cmd/destroy/destroy_test.go

**Checkpoint**: User Story 5 complete - full lifecycle with destroy capability

---

## Phase 8: User Story 6 - Manage Workspaces Across Repositories (Priority: P3)

**Goal**: Users can switch between GitHub and Kubernetes workspaces seamlessly

**Independent Test**: Configure both workspace types in ~/.rad/config.yaml, switch between them, verify command behavior adapts

### Implementation for User Story 6

- [ ] T100 [US6] Update rad workspace commands to support github kind in pkg/cli/cmd/workspace/
- [ ] T101 [US6] Implement GitHub workspace warning for rad resource-type commands in pkg/cli/cmd/resourcetype/
- [ ] T102 [US6] Implement GitHub workspace warning for rad environment commands in pkg/cli/cmd/environment/
- [ ] T103 [US6] Implement GitHub workspace warning for rad recipe commands in pkg/cli/cmd/recipe/
- [ ] T104 [US6] Update rad workspace switch to handle github kind in pkg/cli/cmd/workspace/switch/switch.go
- [ ] T105 [P] [US6] Add unit tests for workspace switching in pkg/cli/cmd/workspace/switch/switch_test.go

**Checkpoint**: User Story 6 complete - multi-workspace support with GitHub and Kubernetes

---

## Phase 9: User Story 7 - View and Understand Deployment Plans (Priority: P3)

**Goal**: Deployment plans are clear and auditable in PR reviews

**Independent Test**: Create deployment PR, verify plan.yaml and artifacts are readable and document expected changes clearly

### Implementation for User Story 7

- [ ] T106 [US7] Enhance plan.yaml formatting for PR readability in pkg/cli/cmd/plan/deploy/deploy.go
- [ ] T107 [US7] Add summary section with total steps, add/change/destroy counts in pkg/cli/cmd/plan/deploy/deploy.go
- [ ] T108 [US7] Generate terraform-context.txt with version and environment info in pkg/cli/cmd/plan/deploy/deploy.go
- [ ] T109 [US7] Implement allVersionsPinned check in plan summary in pkg/cli/cmd/plan/deploy/deploy.go
- [ ] T110 [P] [US7] Add unit tests for plan formatting in pkg/cli/cmd/plan/deploy/deploy_test.go

**Checkpoint**: User Story 7 complete - deployment plans are clear and auditable

---

## Phase 10: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple user stories

- [ ] T111 [P] Add CLI help text for all new commands
- [ ] T112 [P] Add configuration file comment headers with GitHub Secrets reference syntax (FR-112)
- [ ] T113 Code cleanup and refactoring across pkg/cli/github/
- [ ] T114 [P] Update make lint and make test targets to include new packages
- [ ] T115 Run quickstart.md validation end-to-end
- [ ] T116 [P] Add functional test for complete deployment workflow in test/functional-portable/github/deploy_test.go

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies - can start immediately
- **Phase 2 (Foundational)**: Depends on Phase 1 - BLOCKS all user stories
- **Phases 3-4 (US1, US2)**: P1 priority - complete before P2/P3 user stories
- **Phases 5-6 (US3, US4)**: P2 priority - depend on Phase 2, can parallel with each other
- **Phases 7-9 (US5, US6, US7)**: P3 priority - depend on Phase 2, can parallel with each other
- **Phase 10 (Polish)**: Depends on all desired user stories being complete

### User Story Dependencies

| Story | Priority | Depends On | Can Parallel With |
|-------|----------|------------|-------------------|
| US1 - rad init | P1 | Phase 2 | US2 |
| US2 - rad environment connect | P1 | Phase 2 | US1 |
| US3 - rad pr create | P2 | US1, US2 | US4 |
| US4 - rad pr merge | P2 | US3 | US5-US7 |
| US5 - rad pr destroy | P3 | US4 | US6, US7 |
| US6 - Workspace management | P3 | US1 | US5, US7 |
| US7 - Plan readability | P3 | US3 | US5, US6 |

### MVP Scope (P1 Only)

For minimum viable product, complete:
1. Phase 1: Setup (T001-T010)
2. Phase 2: Foundational (T011-T020)
3. Phase 3: User Story 1 - rad init (T021-T036)
4. Phase 4: User Story 2 - rad environment connect (T037-T056)

**MVP delivers**: Repository initialization + OIDC authentication setup

### Parallel Opportunities

```bash
# Phase 1 parallel batch:
T002, T003, T004, T005, T006, T007, T008

# Phase 2 parallel batch:
T012, T013, T014, T016, T017, T018, T020

# US1 tests parallel:
T035, T036

# US2 AWS/Azure parallel:
T040-T046 (AWS) || T047-T051 (Azure)

# US3/US4 can run in parallel by different developers
# US5/US6/US7 can run in parallel by different developers
```

---

## Task Summary

| Phase | User Story | Task Count | Parallel | Sequential |
|-------|-----------|------------|----------|------------|
| 1 | Setup | 10 | 7 | 3 |
| 2 | Foundational | 10 | 6 | 4 |
| 3 | US1 - rad init | 16 | 2 | 14 |
| 4 | US2 - rad environment connect | 20 | 9 | 11 |
| 5 | US3 - rad pr create | 15 | 2 | 13 |
| 6 | US4 - rad pr merge | 15 | 2 | 13 |
| 7 | US5 - rad pr destroy | 13 | 2 | 11 |
| 8 | US6 - Workspace management | 6 | 1 | 5 |
| 9 | US7 - Plan readability | 5 | 1 | 4 |
| 10 | Polish | 6 | 4 | 2 |
| **Total** | | **116** | **36** | **80** |

---

## Notes

- All tasks follow existing Radius CLI patterns (Cobra commands, gomock testing)
- Tests use `radcli.SharedCommandValidation` and `radcli.SharedValidateValidation` helpers
- GitHub CLI integration via `exec.Command("gh", ...)` per research.md decision
- YAML generation via `goccy/go-yaml` per research.md decision
- Commit after each logical task group with relevant `Radius-Action:` trailer
