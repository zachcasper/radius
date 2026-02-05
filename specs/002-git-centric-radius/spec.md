# Feature Specification: Git-Centric Radius

**Feature Branch**: `002-git-centric-radius`  
**Created**: 2026-02-04  
**Status**: Draft  
**Input**: User description: "Write a spec that creates a Git-centric version of Radius and does not have a control plane. The UX should be inspired by git with commands like rad init, rad add, rad commit, rad push, rad log, and rad status."

## Overview

Git-Centric Radius is a lightweight, control-plane-free version of Radius that uses Git as the source of truth for application deployments. Instead of requiring a running control plane service, this version stores all state in the Git repository itself, using familiar Git-inspired commands to manage the application lifecycle.

The core philosophy is that infrastructure deployment should follow the same mental model as code version control: initialize, stage changes, commit, and push to deploy.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Initialize a New Radius Workspace (Priority: P1)

A developer wants to start using Radius in an existing project. They run `rad init` in their repository root to set up the Radius workspace with all necessary configuration files, resource type definitions, and default environment settings.

**Why this priority**: This is the entry point for all users. Without initialization, no other commands can function. This establishes the `.radius/` directory structure that all other commands depend on.

**Independent Test**: Can be fully tested by running `rad init` in an empty Git repository and verifying the `.radius/` directory structure is created with proper configuration files.

**Acceptance Scenarios**:

1. **Given** a Git repository without Radius configuration, **When** the user runs `rad init`, **Then** the system downloads resource type YAML files from https://github.com/radius-project/resource-types-contrib/ into `.radius/config/types/`
2. **Given** a Git repository without Radius configuration, **When** the user runs `rad init`, **Then** the system creates a default `.env` file with placeholder environment configuration
3. **Given** a Git repository without Radius configuration, **When** the user runs `rad init`, **Then** the system creates a recipe configuration YAML in `.radius/config/` containing a mapping of [resourceType, recipeKind, recipeLocation] for each downloaded resource type
4. **Given** a repository that already has `.radius/` configured, **When** the user runs `rad init`, **Then** the system prompts for confirmation before overwriting existing configuration
5. **Given** a Git repository without Radius configuration, **When** the user runs `rad init`, **Then** all created files are ready to be committed to Git

---

### User Story 2 - Create a Deployment Plan from Application Model (Priority: P1)

A developer has defined their application infrastructure in a Bicep file (app.bicep). They run `rad add` to analyze the model and generate a deployment plan that can be reviewed, committed, and later deployed.

**Why this priority**: This is the core value proposition - translating application models into actionable deployment plans. Without this, users cannot progress to deployment.

**Independent Test**: Can be tested by running `rad add app.bicep` against a sample Bicep file and verifying a valid plan is generated in `.radius/plan/`.

**Acceptance Scenarios**:

1. **Given** an initialized Radius workspace and a valid app.bicep file, **When** the user runs `rad add app.bicep`, **Then** the system parses the Bicep model and identifies all resources
2. **Given** an initialized Radius workspace and a valid app.bicep file, **When** the user runs `rad add app.bicep`, **Then** the system looks up recipes for each resource type from the recipe configuration
3. **Given** an initialized Radius workspace and a valid app.bicep file, **When** the user runs `rad add app.bicep`, **Then** the system generates deployment artifacts in `.radius/plan/<app-name>/<environment>/`
4. **Given** an initialized Radius workspace and a valid app.bicep file, **When** the user runs `rad add app.bicep`, **Then** a `plan.yaml` file is created containing the ordered deployment steps
5. **Given** a Bicep file with unsupported resource types, **When** the user runs `rad add app.bicep`, **Then** the system displays clear error messages indicating which resource types are not supported

---

### User Story 3 - Commit a Deployment Plan (Priority: P1)

After generating a plan with `rad add`, the developer wants to record this plan in Git history. They run `rad commit` which creates a Git commit with special trailers that identify the application and environment.

**Why this priority**: Commits are the foundation of the Git-centric approach. They create the audit trail and enable the log/history functionality.

**Independent Test**: Can be tested by running `rad commit` after `rad add` and verifying a Git commit is created with proper trailers.

**Acceptance Scenarios**:

1. **Given** a staged plan in `.radius/plan/`, **When** the user runs `rad commit`, **Then** the system creates a Git commit containing the plan files
2. **Given** a staged plan in `.radius/plan/`, **When** the user runs `rad commit`, **Then** the commit includes Git trailers with `Application: <app-name>` (extracted from app.bicep)
3. **Given** a staged plan in `.radius/plan/`, **When** the user runs `rad commit`, **Then** the commit includes Git trailers with `Environment: <env-name>` (derived from .env file, defaulting to "default")
4. **Given** no staged plan exists, **When** the user runs `rad commit`, **Then** the system displays an error message instructing the user to run `rad add` first
5. **Given** a staged plan, **When** the user runs `rad commit -m "Custom message"`, **Then** the commit uses the custom message while still including the trailers

---

### User Story 4 - Deploy with Push (Priority: P1)

A developer has committed a plan and is ready to deploy. They run `rad push` which executes the deployment using the committed plan, deploying resources according to the recipes.

**Why this priority**: This is the culmination of the workflow - actually deploying infrastructure. Without deployment capability, the tool provides no practical value.

**Independent Test**: Can be tested by running `rad push` with a valid committed plan and verifying resources are deployed (requires infrastructure access).

**Acceptance Scenarios**:

1. **Given** a committed plan exists for the current application/environment, **When** the user runs `rad push`, **Then** the system reads the plan and executes deployment steps in order
2. **Given** a committed plan exists, **When** the user runs `rad push`, **Then** each deployment step executes the recipe (Terraform, Bicep, or other) for that resource
3. **Given** a committed plan exists, **When** the user runs `rad push`, **Then** the deployment progress is displayed with clear status for each step
4. **Given** a deployment step fails, **When** `rad push` encounters the failure, **Then** the system stops and displays the error with context about which step failed
5. **Given** no committed plan exists, **When** the user runs `rad push`, **Then** the system displays an error instructing the user to run `rad add` and `rad commit` first
6. **Given** a committed plan exists, **When** the user runs `rad push`, **Then** a deployment record is created in `.radius/deploy/<app>/<env>/<commit>/`

---

### User Story 5 - View Deployment History (Priority: P2)

A developer wants to see the history of deployments for an application. They run `rad log` which displays a chronological list of commits that represent deployment events, with visual diffs showing what changed between deployments.

**Why this priority**: History and auditability are key benefits of the Git-centric approach, but users need the ability to deploy first.

**Independent Test**: Can be tested by running `rad log` after several `rad commit` operations and verifying the history is displayed correctly.

**Acceptance Scenarios**:

1. **Given** multiple rad commits exist in history, **When** the user runs `rad log`, **Then** the system displays commits that have Radius trailers (Application/Environment)
2. **Given** multiple rad commits exist, **When** the user runs `rad log`, **Then** each entry shows the commit hash, date, message, and application/environment
3. **Given** two consecutive rad commits, **When** the user runs `rad log`, **Then** changes between commits are visualized using the dyff library (YAML diff visualization)
4. **Given** no rad commits exist, **When** the user runs `rad log`, **Then** the system displays a message indicating no deployment history exists
5. **Given** multiple rad commits exist, **When** the user runs `rad log -n 5`, **Then** only the last 5 deployment commits are shown

---

### User Story 6 - Check Workspace Status (Priority: P2)

A developer wants to understand the current state of their Radius workspace - whether there are uncommitted plans, what environment is active, and the state of deployed resources. They run `rad status` to get this overview.

**Why this priority**: Status awareness is important for workflow management, but is secondary to the core init/add/commit/push flow.

**Independent Test**: Can be tested by running `rad status` in various workspace states and verifying accurate status is reported.

**Acceptance Scenarios**:

1. **Given** an initialized Radius workspace, **When** the user runs `rad status`, **Then** the system displays the current environment (from .env)
2. **Given** an uncommitted plan exists, **When** the user runs `rad status`, **Then** the system shows "Changes staged for commit" with the affected resources
3. **Given** the latest plan has been committed but not pushed, **When** the user runs `rad status`, **Then** the system shows "Committed, ready to push"
4. **Given** the workspace is up-to-date (committed and deployed), **When** the user runs `rad status`, **Then** the system shows "Up to date" with the last deployment info
5. **Given** no initialization has occurred, **When** the user runs `rad status`, **Then** the system displays a message instructing the user to run `rad init`

---

### Edge Cases

- What happens when the user tries to `rad push` from a different branch than where the plan was committed?
  - The system should warn the user and require explicit confirmation
- What happens when recipe files referenced in the configuration are missing or invalid?
  - The system should display clear error messages during `rad add` identifying which recipes are problematic
- What happens when the remote resource-types-contrib repository is unavailable during `rad init`?
  - The system should display a connection error and suggest retrying or using cached types if available
- What happens when a plan references resources that no longer exist in the app.bicep?
  - During `rad add`, the system should detect removed resources and include destroy operations in the plan
- What happens when two developers create conflicting plans on different branches?
  - Git's native merge conflict resolution applies to plan files; `rad status` should detect conflict states

## Requirements *(mandatory)*

### Functional Requirements

#### rad init

- **FR-001**: System MUST download resource type YAML files from https://github.com/radius-project/resource-types-contrib/ during initialization
- **FR-002**: System MUST store downloaded resource types in `.radius/config/types/` directory
- **FR-003**: System MUST create a default `.env` file with standard environment configuration placeholders
- **FR-004**: System MUST generate a recipe configuration YAML file in `.radius/config/` containing mappings for each resource type to its recipe kind and location
- **FR-005**: System MUST prompt for confirmation if `.radius/` directory already exists
- **FR-006**: System MUST validate that the current directory is a Git repository before initializing

#### rad add

- **FR-007**: System MUST accept an application model file (Bicep) as input
- **FR-008**: System MUST parse the Bicep model to extract application name, resources, and their properties
- **FR-009**: System MUST look up recipes for each resource type from the recipe configuration
- **FR-010**: System MUST generate deployment artifacts in `.radius/plan/<app-name>/<environment>/`
- **FR-011**: System MUST create a `plan.yaml` file with ordered deployment steps
- **FR-012**: System MUST validate resource properties against resource type schemas
- **FR-013**: System MUST auto-detect the model file if only one exists in `.radius/model/`
- **FR-014**: System MUST support the `-e` or `--environment` flag to specify target environment

#### rad commit

- **FR-015**: System MUST create a Git commit containing all files in the plan directory
- **FR-016**: System MUST include an `Application:` Git trailer with the application name from the model
- **FR-017**: System MUST include an `Environment:` Git trailer with the environment name from .env (defaulting to "default")
- **FR-018**: System MUST reject commit if no plan exists
- **FR-019**: System MUST support a custom commit message via `-m` flag while preserving trailers
- **FR-020**: System MUST stage plan files automatically before committing

#### rad push

- **FR-021**: System MUST read the committed plan and execute deployment steps in sequence
- **FR-022**: System MUST execute recipes (Terraform apply, Bicep deploy, etc.) for each resource
- **FR-023**: System MUST display progress with clear status for each deployment step
- **FR-024**: System MUST halt on first failure and report the error with context
- **FR-025**: System MUST create deployment records in `.radius/deploy/<app>/<env>/<commit>/`
- **FR-026**: System MUST support `--dry-run` flag to preview deployment without executing
- **FR-027**: System MUST capture Kubernetes manifests and other deployed resources for audit

#### rad log

- **FR-028**: System MUST display Git commits that contain Radius trailers (Application/Environment)
- **FR-029**: System MUST show commit hash, date, message, application, and environment for each entry
- **FR-030**: System MUST visualize changes between consecutive deployments using dyff library
- **FR-031**: System MUST support `-n` flag to limit the number of entries displayed
- **FR-032**: System MUST support filtering by application name via `--app` flag

#### rad status

- **FR-033**: System MUST display the current active environment from .env
- **FR-034**: System MUST show whether uncommitted plan changes exist
- **FR-035**: System MUST show whether committed plans are ready to push
- **FR-036**: System MUST display last successful deployment information
- **FR-037**: System MUST indicate if the workspace is not initialized

### Key Entities

- **Workspace**: The Git repository containing `.radius/` configuration; represents the root context for all operations
- **Resource Type**: A YAML definition describing a deployable resource, its schema, and API versions; stored in `.radius/config/types/`
- **Recipe Configuration**: A YAML mapping connecting resource types to their recipe implementations (kind and location); stored in `.radius/config/`
- **Environment**: A named deployment target defined by an `.env` file; contains context-specific settings (Kubernetes context, namespace, cloud credentials)
- **Plan**: A structured YAML file describing ordered deployment steps, resource metadata, and recipe references; stored in `.radius/plan/<app>/<env>/`
- **Deployment Step**: An individual resource deployment within a plan; includes resource info, recipe reference, and deployment artifacts
- **Deployment Record**: A JSON file capturing the results of a `rad push` execution; stored in `.radius/deploy/<app>/<env>/<commit>/`
- **Application Model**: A Bicep file defining the application's resources and their relationships

## Assumptions

- Users have Git installed and the current directory is within a Git repository
- Network access to https://github.com/radius-project/resource-types-contrib/ is available during `rad init` (or cached types can be used)
- Recipe implementations (Terraform modules, Bicep templates) referenced in recipe configuration are accessible
- Kubernetes context and credentials are available when deploying to Kubernetes targets
- The dyff library is used for YAML diff visualization in `rad log`
- Environment files follow the `.env` naming convention (`.env`, `.env.local`, `.env.production`, etc.)

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can initialize a new Radius workspace in under 30 seconds (excluding network download time)
- **SC-002**: Users can complete the full workflow (init -> add -> commit -> push) for a simple 3-resource application in under 5 minutes
- **SC-003**: 90% of users can successfully deploy their first application without consulting documentation beyond initial setup
- **SC-004**: Deployment history for any application is viewable within 2 seconds regardless of history depth
- **SC-005**: All deployment operations are fully auditable through Git history with no external state dependencies
- **SC-006**: Users can understand the current workspace state (via `rad status`) within 1 second
- **SC-007**: The tool works offline for all commands except `rad init` (initial type download) and `rad push` (deployment execution)
