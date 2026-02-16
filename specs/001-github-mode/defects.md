# Defects and Lessons Learned

This file tracks defects discovered during implementation that should inform future spec revisions or implementations.

## Format

Each defect should include:
- **ID**: Unique identifier (D001, D002, etc.)
- **Phase/Task**: Which task revealed the issue
- **Category**: Spec gap | Implementation bug | Design flaw | Assumption error
- **Description**: What went wrong
- **Resolution**: How it was fixed
- **Spec Impact**: What should be added/changed in the spec to prevent recurrence

---

## Defects

### D001: CreateBranch did not checkout new branch ✅ FIXED
- **Phase/Task**: T037 (Unit tests)
- **Category**: Implementation bug
- **Description**: `GitHelper.CreateBranch()` created the branch reference but didn't checkout the new branch, causing tests to fail because `GetCurrentBranch()` still returned "main"
- **Resolution**: Modified `CreateBranch()` to call `CheckoutBranch()` after creating the reference, and added validation for empty branch names and existing branches
- **Spec Impact**: Add to contracts/git.go that `CreateBranch` should switch to the new branch after creation

### D002: WorkspaceSection.Default field backward compatibility ✅ FIXED
- **Phase/Task**: T033 (Rename default to current)
- **Category**: Design flaw
- **Description**: Renaming `Default` to `Current` in WorkspaceSection would break existing config files
- **Resolution**: Added `Current` as new field while keeping `Default` for backward compatibility. Added `GetCurrentWorkspaceName()` method that prefers `Current` over `Default`
- **Spec Impact**: Document migration strategy for config property changes - always maintain backward compatibility

### D003: go-git sparse checkout support ✅ FIXED
- **Phase/Task**: T028 (Resource types fetch)
- **Category**: Assumption error
- **Description**: Spec assumed go-git supports sparse checkout natively. It has limited support - requires manual worktree configuration
- **Resolution**: Implemented custom sparse checkout logic using go-git's low-level APIs
- **Spec Impact**: Research.md should note go-git limitations with sparse checkout

### D004: Test expectations for output messages ✅ FIXED
- **Phase/Task**: T033 (Workspace switch tests)
- **Category**: Implementation bug
- **Description**: Changing "default" to "current" in user-facing messages broke existing test expectations
- **Resolution**: Updated test expectations to match new wording
- **Spec Impact**: When changing user-facing terminology, list all affected tests in the task

### D005: types.yaml non-conformance with Appendix C.1 ✅ FIXED
- **Phase/Task**: T029 (types.yaml generation)
- **Category**: Spec gap
- **Description**: `rad init` generated types.yaml contains incorrect information and is not in conformance with Appendix C.1 of the spec. Key discrepancies:
  - **Type names**: Implementation used `Applications.Core/containers`, `Applications.Datastores/mongoDatabases` but spec defines `Radius.Core/applications`, `Radius.Compute/containers`, `Radius.Data/postgreSqlDatabases`
  - **Path format**: Implementation used `//types/core/containers?ref=v1.0` but spec defines `//Core/applications/applications.yaml?ref=v0.54.0`
  - **Missing types**: Spec lists `Radius.Compute/persistentVolumes`, `Radius.Compute/routes`, `Radius.Security/secrets` which were not in implementation
  - **Extra types**: Implementation included types not in spec like `gateways`, `mongoDatabases`, `redisCaches`, `sqlDatabases`
  - **FR-008 compliance**: Types should be fetched from resource-types-contrib, not hardcoded
- **Resolution**: FIXED - Updated `DefaultTypesManifest()` to match spec exactly with correct type names, paths, and version. Added `TypesManifestFromFetched()` to convert fetched types to manifest format. Updated `runGitHubInit()` to fetch types per FR-008, with fallback to default manifest if fetch fails.
- **Spec Impact**: Consider adding contract validation test that verifies generated YAML matches spec appendix format

### D006: recipes.yaml does not match types.yaml per FR-009-A ✅ FIXED
- **Phase/Task**: T030 (recipes.yaml generation)
- **Category**: Spec gap
- **Description**: `DefaultRecipesManifest()` generates recipes for hardcoded types (`Applications.Datastores/mongoDatabases`, `Applications.Datastores/redisCaches`, etc.) rather than generating recipes for all types in types.yaml. Per new FR-009-A, recipes.yaml MUST include a recipe entry for every resource type defined in types.yaml, with recipe locations pointing to the same `radius-project/resource-types-contrib` repository. Recipe paths should follow the pattern: `<Namespace>/<TypeName>/recipes/<target>/<tool>/`
- **Resolution**: FIXED - Implemented `RecipesManifestFromTypes()` function that:
  1. Accepts the types manifest as input
  2. Generates a recipe entry for each type (excluding metadata-only types like `Radius.Core/applications`)
  3. Uses consistent path format: `git::https://github.com/radius-project/resource-types-contrib.git//Compute/containers/recipes/kubernetes/terraform?ref=v0.54.0`
  4. Supports OCI format for Bicep recipes: `https://ghcr.io/radius-project/recipes/<provider>/<shortname>:v0.54.0`
  5. Updated `DefaultRecipesManifest()` to delegate to `RecipesManifestFromTypes(DefaultTypesManifest(), ...)`
  6. Updated `runGitHubInit()` to use `RecipesManifestFromTypes(typesManifest, provider, tool)`
  7. Added helper functions `extractTypeBasePath()` and `extractTypeShortName()` with unit tests
- **Spec Impact**: FR-009-A added to spec.md with detailed recipe path format requirements

### D007: Incorrect next steps message after rad init ✅ FIXED
- **Phase/Task**: T036 (Git commit with init trailer)
- **Category**: Implementation bug
- **Description**: The user-facing "Next steps" message displayed after `rad init --github` completion contains incorrect instructions:
  ```
  Next steps:
    1. Run 'git push' to push the changes to GitHub
    2. Run 'rad environment connect' to configure OIDC authentication
    3. Run 'rad deploy' to create a deployment plan
  ```
  Step 3 should reference `rad model` to create an application model, not `rad deploy` to create a deployment plan.
- **Resolution**: FIXED - Updated pkg/cli/cmd/radinit/github.go to display correct message:
  ```
  Next steps:
    1. Run 'git push' to push the changes to GitHub
    2. Run 'rad environment connect' to configure OIDC authentication
    3. Run 'rad model' to create an application model
  ```
- **Spec Impact**: Verify next steps messages match spec's user story workflows

### D008: Recipe locations should not include version tags ✅ FIXED
- **Phase/Task**: T030-A (recipes.yaml generation)
- **Category**: Design flaw
- **Description**: Recipe locations were initially generated with version tags (e.g., `?ref=v0.54.0` for Terraform, `:v0.54.0` for Bicep OCI). To maintain consistency with resource type definitions (which also don't have version tags), recipe locations should not include version tags either. Version pinning will be addressed as a future enhancement.
- **Resolution**: FIXED - Removed version tags from recipe location generation in `RecipesManifestFromTypes()`. Updated:
  - Terraform format: `git::https://github.com/radius-project/resource-types-contrib.git//Compute/containers/recipes/aws/terraform` (no `?ref=`)
  - Bicep format: `https://ghcr.io/radius-project/recipes/azure/containers` (no `:version`)
  - Updated spec.md FR-009-A to remove version tag from example
  - Added recipe versioning to Future Enhancements section
- **Spec Impact**: Future Enhancements section updated to include recipe versioning alongside type definition versioning

### D009: GitHub workspace includes environment and scope properties ✅ FIXED
- **Phase/Task**: T032 (Workspace config update)
- **Category**: Implementation bug
- **Description**: The `updateGitHubWorkspace()` function in `pkg/cli/cmd/radinit/github.go` generates a workspace entry with `Environment` and `Scope` properties. Per Appendix C.4, a `github` kind workspace should only have a `connection` with `url` and `kind`. The `environment` and `scope` properties are specific to `kubernetes` kind workspaces. Current implementation:
  ```go
  Environment: fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/Applications.Core/environments/%s",
      opts.Repo, opts.EnvironmentName),
  Scope: fmt.Sprintf("/planes/radius/local/resourceGroups/%s", opts.Repo),
  ```
  Per C.4, the github workspace should be:
  ```yaml
  my-app-repo:
    connection:
      url: https://github.com/myorg/my-app-repo
      kind: github
  ```
  Not:
  ```yaml
  my-app-repo:
    connection:
      url: https://github.com/myorg/my-app-repo
      kind: github
    environment: /planes/radius/local/...
    scope: /planes/radius/local/...
  ```
- **Resolution**: FIXED - Removed `Environment` and `Scope` from the workspace entry in `updateGitHubWorkspace()`. Also changed connection key from `source` to `url` per spec C.4. GitHub workspaces now only contain:
  ```go
  ws := &workspaces.Workspace{
      Name: opts.Repo,
      Connection: map[string]any{
          "kind": workspaces.KindGitHub,
          "url":  repoURL,
      },
  }
  ```
  Environment info is stored in `.radius/env.<name>.yaml` files, not in workspace config.
- **Spec Impact**: None - implementation now matches existing spec

### D010: Azure subscription prompt does not show list of subscriptions (FR-024) ✅ FIXED
- **Phase/Task**: T050 (Azure subscription prompt)
- **Category**: Implementation bug
- **Description**: FR-024 states "For Azure environments, system MUST prompt for subscription (from `az account list`)." The current implementation uses `GetTextInput` with a default value from `az account show`, but does not display a list of available subscriptions for the user to choose from. Users must know their subscription ID or accept the default.
- **Resolution**: FIXED - Implemented `promptForAzureSubscription()` function in connect.go that:
  1. Runs `az account list --output json` to get all available subscriptions
  2. Parses the response with `parseAzureSubscriptions()` to extract names, IDs, and tenant IDs
  3. Filters to only show enabled subscriptions
  4. Uses `GetListInput` to display a selectable list formatted as "Subscription Name (subscription-id)"
  5. Returns both subscription ID and tenant ID from the selected subscription
- **Spec Impact**: None - implementation now matches existing spec

### D011: Azure OIDC auth test workflow fails with missing client-id/tenant-id ✅ FIXED
- **Phase/Task**: T058-C (Azure auth test job in workflow)
- **Category**: Implementation bug
- **Description**: When the `radius-auth-test.yaml` workflow runs for Azure environments, the `azure/login@v2` step fails with authentication errors. Initial error was:
  ```
  Login failed with Error: Using auth-type: SERVICE_PRINCIPAL. Not all values are present. 
  Ensure 'client-id' and 'tenant-id' are supplied.
  ```
  After first fix, workflow still failed with:
  ```
  Error: AADSTS700016: Application with identifier '***' was not found in the directory 'Default Directory'.
  You may have sent your authentication request to the wrong tenant.
  ```
- **Root Causes**:
  1. Workflow used secrets references (`${{ secrets.AZURE_CLIENT_ID }}`) but values are in env file
  2. GitHub Actions shallow clone (depth=1) caused `HEAD~1` to fail, so env file wasn't found correctly
  3. Grep pattern `.radius/env.*.yaml` was interpreted as regex, not literal path
  4. yq returned `null` for missing fields, which got passed to azure/login as literal "null" string
- **Resolution**: FIXED - Updated `generateAWSAuthTestSteps()` and `generateAzureAuthTestSteps()` in `pkg/cli/github/workflows.go`:
  1. Added `fetch-depth: 2` to checkout step to enable `git diff HEAD~1`
  2. Added fallback check `git rev-parse HEAD~1` before using git diff
  3. Used `grep -F` for literal string matching instead of regex
  4. Added validation to fail early if values are empty or "null"
  5. Added debug output showing parsed values (masked) for troubleshooting
  6. Use `yq -e` with `// ""` fallback to handle missing fields gracefully
- **Spec Impact**: None - values were never meant to be secrets. The env file is committed to the repository.

### D012: Resource types fetch fails with "types directory not found" ✅ FIXED
- **Phase/Task**: T028 (Resource types fetch)
- **Category**: Implementation bug
- **Description**: When running `rad init --github --provider azure`, the following warning is displayed:
  ```
  Warning: Failed to fetch resource types, using defaults: failed to fetch resource types: failed to parse resource types: types directory not found in repository
  ```
  The implementation attempted to fetch resource types from `radius-project/resource-types-contrib` repository using sparse checkout, but the expected directory structure did not match the actual repository structure. The code was looking for a `types` directory that doesn't exist.
- **Root Cause (Part 1)**: The actual repository structure uses namespace directories (`Compute`, `Data`, `Security`) at the root level, not a `types` directory.
- **Root Cause (Part 2)**: Even after fixing the directory structure, parsing failed with "no resource types found" because the YAML format was different than expected. The actual format is:
  ```yaml
  namespace: Radius.Data
  types:
    mySqlDatabases:
      description: |
        ...
  ```
  But the code expected flat fields `type:` and `name:`.
- **Resolution**: FIXED - Updated `pkg/cli/github/resourcetypes.go`:
  1. Added `ResourceTypeNamespaces` variable with actual namespace directories
  2. Updated `sparseClone()` to fetch `Compute`, `Data`, `Security` directories
  3. Added `resourceTypeFile` and `resourceTypeEntry` structs to match actual YAML format
  4. Updated `parseResourceTypes()` to:
     - Walk namespace directories instead of `types`
     - Parse YAML as `namespace` + `types` map structure
     - Extract type name from `namespace/typeName` (e.g., `Radius.Data/mySqlDatabases`)
     - Fallback to legacy format for backward compatibility
- **Spec Impact**: None - the implementation was based on incorrect assumptions about the repo structure

### D013: Azure AD app created in wrong tenant causing OIDC failure ✅ FIXED
- **Phase/Task**: T051 (Azure OIDC setup)
- **Category**: Implementation bug
- **Description**: When running the auth test workflow, Azure OIDC login fails with:
  ```
  Error: AADSTS700016: Application with identifier '***' was not found in the directory 'Default Directory'.
  This can happen if the application has not been installed by the administrator of the tenant or
  consented to by any user in the tenant. You may have sent your authentication request to the wrong tenant.
  ```
  The `client-id` and `tenant-id` are correctly being passed from the env file (D011 fix is working), but the Azure AD application doesn't exist in that tenant.
- **Root Cause**: The `createAzureApp()` function in `connect.go` creates the Azure AD app using the current Azure CLI tenant context, but this may differ from the tenant associated with the selected subscription. If a user has multiple tenants/subscriptions and their default Azure CLI context is different from the subscription they select, the app gets created in the wrong tenant.
- **Resolution**: FIXED - Updated `configureAzureOIDC()` to:
  1. Call `az account set --subscription` before creating the Azure AD app to ensure the correct tenant context
  2. Added debug logging showing which tenant the app was created in
  3. Added explicit Client ID logging for user verification
- **Steps to Reproduce Issue**:
  1. User has multiple Azure tenants (e.g., personal and work)
  2. Azure CLI is logged into Tenant A
  3. User selects a subscription in Tenant B during `rad environment connect`
  4. Azure AD app is created in Tenant A (wrong tenant)
  5. Env file saves Tenant B's ID
  6. Workflow tries to authenticate to Tenant B but app is in Tenant A → fails
- **Spec Impact**: None - this is a multi-tenant edge case that wasn't anticipated

### D014: Duplicate Azure AD apps created on repeated runs ✅ FIXED
- **Phase/Task**: T051 (Azure OIDC setup)
- **Category**: Implementation bug
- **Description**: Running `rad environment connect` multiple times creates duplicate Azure AD applications with the same display name. Azure AD allows multiple apps with identical display names, so each run creates a new app. If a previous run saved an old client ID to the env file, subsequent runs don't update it, causing OIDC auth to fail with the wrong client ID.
- **Symptoms**: 
  - Client ID in env file doesn't match the app visible in Azure Portal
  - Multiple apps with same name in Azure AD (e.g., `radius-owner-repo`)
  - OIDC fails with "Application not found" even though app exists
- **Resolution**: FIXED - Updated `createAzureApp()` to:
  1. Check if an app with the same name already exists using `az ad app list --display-name`
  2. Reuse existing app instead of creating a duplicate
  3. Log "Using existing Azure AD application" when reusing
- **Spec Impact**: None - idempotent operations are a best practice

### D015: Service principal lacks subscription access for azure/login ✅ FIXED
- **Phase/Task**: T051 (Azure OIDC setup)
- **Category**: Implementation bug
- **Description**: After successful OIDC token exchange, `azure/login@v2` fails with:
  ```
  Error: No subscriptions found for ***.
  ```
  This occurs because the service principal only had Contributor role at resource group level, but `az login` requires at least Reader access at subscription level to list available subscriptions.
- **Root Cause**: The role assignment was scoped too narrowly - only to the resource group, not the subscription. Azure CLI's login process needs to enumerate subscriptions to complete the authentication flow.
- **Resolution**: FIXED - Updated `createAzureApp()` to add two role assignments:
  1. **Contributor** at resource group level (for managing Radius resources)
  2. **Reader** at subscription level (required for azure/login to work)
- **Spec Impact**: Should clarify in FR-052 that subscription-level Reader role is required for OIDC login

### D016: Azure AD application list returns no apps despite apps existing ✅ FIXED
- **Phase/Task**: T050-B (Azure application list prompt)
- **Category**: Implementation bug
- **Description**: When running `rad environment connect` and choosing "Use existing Azure AD application", the system reports "No applications found, prompting for ID" even though `az ad app list` returns applications. The user is forced to manually enter the application ID.
- **Symptoms**:
  ```
  Fetching your Azure AD applications...
  No applications found, prompting for ID
  ```
- **Root Cause**: The implementation uses `az ad app list --show-mine` which only returns applications where the current user is the **owner**. Many users have access to applications they don't own, especially in enterprise environments where apps are created by administrators.
- **Resolution**: FIXED - Updated `promptForExistingAzureApp()` to:
  1. Use `az ad app list --all` to list all accessible applications  
  2. Limit results with `--top 50` to avoid loading thousands of apps
  3. Filter by display names starting with "radius-" to show relevant apps first
  4. Still provide manual entry option for apps not in the list
- **Spec Impact**: Update FR-026 to clarify which apps are listed

### D017: Git hints displayed during resource types fetch ✅ FIXED
- **Phase/Task**: T028 (Resource types fetch)
- **Category**: Implementation bug
- **Description**: When running `rad init --github`, git hints and progress messages are displayed during the sparse checkout of the resource-types-contrib repository:
  ```
  hint: Using 'master' as the name for the initial branch. This default branch name
  hint: is subject to change. To configure the initial branch name to use in all
  hint: of your new repositories, which will suppress this warning, call:
  hint:
  hint:   git config --global init.defaultBranch <name>
  ...
  remote: Enumerating objects: 114, done.
  remote: Counting objects: 100% (114/114), done.
  ```
  These messages are noise to the user and should be suppressed.
- **Root Cause**: The `sparseClone()` function in resourcetypes.go was piping stderr to `os.Stderr` and not using quiet flags for git commands.
- **Resolution**: FIXED - Updated `sparseClone()` to:
  1. Add `-q` flag to `git init` to suppress branch name hints
  2. Add `-q` flag to `git fetch` to suppress remote progress output
  3. Add `-q` flag to `git checkout` to suppress checkout messages
  4. Remove `cmd.Stderr = os.Stderr` to not pipe stderr to console
- **Spec Impact**: None - this is a UX polish issue

### D018: Spec incorrectly defined `rad plan` as a CLI command ✅ FIXED
- **Phase/Task**: T067-T070 (rad plan command implementation)
- **Category**: Spec gap
- **Description**: The spec defined `rad plan deploy` and `rad plan destroy` as CLI commands, but this was incorrect. The plan generation functionality is a Radius control plane operation invoked via API, similar to how `rad deploy` works. The workflow running in k3d calls the control plane's plan API, not a separate CLI command.
- **What was implemented incorrectly**: Created `rad plan` and `rad plan deploy` CLI commands in `pkg/cli/cmd/plan/`
- **Resolution**: FIXED - 
  1. Removed the `rad plan` CLI commands from the implementation
  2. Updated FR-039, FR-040, FR-059 to clarify plan is a control plane API
  3. Updated acceptance scenarios to use `rad deploy --plan` instead of `rad plan deploy`
  4. Updated workflow template to call `rad deploy --plan` instead of `rad plan deploy`
  5. Marked T067-T070 as REMOVED with clarification note
- **Spec Impact**: Clarified that plan generation is a control plane API operation. The CLI triggers the workflow which runs `rad deploy --plan` inside k3d to invoke the control plane's plan API.

### D019: Deploy and destroy workflows missing control plane installation ✅ FIXED
- **Phase/Task**: T079 (Deploy workflow), T096 (Destroy workflow)
- **Category**: Implementation bug
- **Description**: The `generateDeploySteps()` and `generateDestroySteps()` functions in `pkg/cli/github/workflows.go` were missing the critical "Install Radius control plane" step that exists in `generatePlanSteps()`. Without installing the Radius control plane on the k3d cluster, `rad deploy` and `rad destroy` commands would fail with connection errors.
- **What was missing**:
  ```yaml
  - name: Install Radius control plane
    run: rad install kubernetes --set global.repositoryPath=.radius
    env:
      KUBECONFIG: /etc/rancher/k3d/kubeconfig-radius-ephemeral.yaml
  ```
- **Resolution**: FIXED - Added the "Install Radius control plane" step to both `generateDeploySteps()` and `generateDestroySteps()`, consistent with `generatePlanSteps()`
- **Spec Impact**: Workflow generation requirements should specify that all workflows using `rad deploy` or `rad destroy` must install the control plane first

### D020: rad pr create/merge missing workspace flag ✅ FIXED
- **Phase/Task**: T060 (rad pr create), T074 (rad pr merge)
- **Category**: Implementation bug
- **Description**: Both `rad pr create` and `rad pr merge` commands failed with error "flag accessed but not defined: workspace" because `cli.RequireWorkspace()` expects a `--workspace` flag to be registered on the command.
- **Root Cause**: The commands used `cli.RequireWorkspace()` to load workspace configuration, but did not call `commonflags.AddWorkspaceFlag(cmd)` to register the flag.
- **Resolution**: FIXED - Added `commonflags.AddWorkspaceFlag(cmd)` to both commands, along with import for `commonflags` package
- **Spec Impact**: Task templates for new commands should include a checklist item: "Register workspace flag if using RequireWorkspace()"

### D021: Non-existent k3d GitHub Action ✅ FIXED
- **Phase/Task**: Runtime (GitHub Actions execution)
- **Category**: Implementation bug
- **Description**: The generated workflow files referenced `abhinavsingh/setup-k3d@v1` GitHub Action which does not exist, causing workflow failures with error "Unable to resolve action abhinavsingh/setup-k3d, repository not found".
- **Root Cause**: The action name was fabricated during implementation without verifying it exists in the GitHub Actions marketplace.
- **Resolution**: FIXED - Replaced the non-existent action with a direct shell installation of k3d using the official installer script:
  ```yaml
  - name: Install k3d
    run: curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash
  ```
- **Files Changed**: `pkg/cli/github/workflows.go` - Updated `generateDeploySteps()`, `generateDestroySteps()`, and `generatePlanSteps()`
- **Spec Impact**: Implementation should verify third-party actions exist before referencing them

### D022: Non-existent Radius CLI installation URL ✅ FIXED
- **Phase/Task**: Runtime (GitHub Actions execution)
- **Category**: Implementation bug
- **Description**: The generated workflow files used `curl -fsSL https://get.radapp.io/tools/rad/install.sh | bash` to install the Radius CLI, but `get.radapp.io` does not exist. The workflow failed with "Could not resolve host: get.radapp.io". Additionally, because the curl output was piped directly to bash, the failure was silent in the GitHub UI - the next step "Install Radius control plane" failed with "rad: command not found".
- **Root Cause**: The URL `get.radapp.io` was fabricated during implementation without verifying it exists. The actual Radius install script is at GitHub.
- **Resolution**: FIXED - Replaced with the actual Radius install script URL from GitHub:
  ```yaml
  - name: Install Radius CLI
    run: curl -fsSL https://raw.githubusercontent.com/radius-project/radius/main/deploy/install.sh | /bin/bash
  ```
- **Files Changed**: `pkg/cli/github/workflows.go` - Updated all three workflow generators
- **Spec Impact**: Implementation should verify URLs exist before hardcoding them; consider using explicit `/bin/bash` for clarity

### D023: Unnecessary empty .radius/schemas directory created ✅ FIXED
- **Phase/Task**: T028 (Resource types fetch)
- **Category**: Implementation bug
- **Description**: `rad init --github` creates an empty `.radius/schemas/` directory that is not used anywhere. This directory was part of an earlier design to copy JSON schema files from resource-types-contrib, but the implementation uses `types.yaml` instead. The schemas directory adds clutter and may confuse users.
- **Root Cause**: `fetchTypesManifest()` in github.go passes the `.radius` directory to `FetchResourceTypes()`, which then creates a `schemas` subdirectory and attempts to copy schema files (which aren't present in the source repo structure).
- **Resolution**: FIXED - Changed the call to `FetchResourceTypes()` to pass an empty string for `targetDir` instead of `radiusDir`. This skips the schemas directory creation and file copying logic entirely since we don't need it.
- **Files Changed**: `pkg/cli/cmd/radinit/github.go` - Changed `FetchResourceTypes(ctx, radiusDir)` to `FetchResourceTypes(ctx, "")`
- **Spec Impact**: None - the schemas directory was never part of the spec

### D024: k3d kubeconfig path does not exist ✅ FIXED
- **Phase/Task**: Runtime (GitHub Actions execution)
- **Category**: Implementation bug
- **Description**: The generated workflow files set `KUBECONFIG` to `/etc/rancher/k3d/kubeconfig-radius-ephemeral.yaml`, but k3d does not create kubeconfigs at that path. This caused `rad install kubernetes` to fail with:
  ```
  Error: Kubernetes cluster unreachable: Get "http://localhost:8080/version": dial tcp [::1]:8080: connect: connection refused
  ```
- **Root Cause**: The kubeconfig path was fabricated during implementation. k3d by default merges kubeconfig to `~/.kube/config` or requires explicit export using `k3d kubeconfig get`.
- **Resolution**: FIXED - Added an "Export kubeconfig" step after cluster creation:
  ```yaml
  - name: Export kubeconfig
    run: k3d kubeconfig get radius-ephemeral > /tmp/kubeconfig.yaml
  ```
  And updated all `KUBECONFIG` references to use `/tmp/kubeconfig.yaml`.
- **Files Changed**: `pkg/cli/github/workflows.go` - Updated `generateDeploySteps()`, `generateDestroySteps()`, and `generatePlanSteps()`
- **Spec Impact**: Implementation should verify k3d kubeconfig handling; consider documenting k3d usage patterns

---

### D025: rad environment connect exits before workflow completes ✅ FIXED
- **Phase/Task**: Runtime (rad environment connect)
- **Category**: Implementation bug
- **Description**: `rad environment connect` exits immediately after pushing changes instead of waiting for the auth test workflow to complete. The command shows "Authentication verified successfully!" even when the workflow hasn't finished or failed.
- **Root Cause**: `GetLatestWorkflowRun()` returns the most recent workflow run for the given workflow file, which might be a **previously completed** run. Since its status is already "completed", `WatchWorkflowRun()` returns immediately. The code doesn't ensure it's watching the **newly triggered** run.
- **Resolution**: FIXED - Modified `waitForAuthWorkflow()` to:
  1. Filter for workflow runs with status "queued" or "in_progress"
  2. If a completed run is found, continue polling until a new in-progress run appears
  3. Add a timeout to prevent infinite waiting (2 minutes max)
- **Files Changed**: `pkg/cli/cmd/env/connect/connect.go`
- **Spec Impact**: None - workflow watching behavior was underspecified

---

### D026: rad model fails to stage file with "entry not found" ✅ FIXED
- **Phase/Task**: Phase 5 (US3) - rad model
- **Category**: Implementation bug
- **Description**: `rad model` creates the model file successfully but fails when staging it for commit with: `Failed to stage model file: failed to add /path/to/.radius/model/todolist.bicep: entry not found`
- **Root Cause**: The `gitHelper.Add()` function (using go-git library) expects a **relative path** from the repository root, but `model.go` passes the **absolute path** to the model file.
- **Resolution**: FIXED - Changed `gitHelper.Add(modelFile)` to use a relative path: `filepath.Join(".radius", "model", DefaultModelName+ModelFileExtension)`
- **Files Changed**: `pkg/cli/cmd/model/model.go`
- **Spec Impact**: None

---

### D027: rad pr create fails to detect applications in .radius/model/ ✅ FIXED
- **Phase/Task**: Phase 6 (US4) - rad pr create
- **Category**: Implementation bug
- **Description**: `rad pr create` reports "No applications found" even when `.radius/model/todolist.bicep` exists. The file is clearly present but the command doesn't detect it.
- **Root Cause**: The `detectApplications()` function in `create.go` looks for **directories** inside `.radius/model/` (`entry.IsDir()`), but `rad model` creates a `.bicep` **file** not a directory. The detection logic doesn't match the model file structure.
- **Resolution**: FIXED - Changed `detectApplications()` to look for `.bicep` files instead of directories. Extract application name from filename (e.g., `todolist.bicep` → `todolist`).
- **Files Changed**: `pkg/cli/cmd/pr/create/create.go`
- **Spec Impact**: None

---

### D028: Azure login fails - GitHub secrets not created by rad environment connect ✅ FIXED
- **Phase/Task**: Phase 4 (US2) / Phase 6 (US4) - Azure workflow authentication
- **Category**: Specification gap / Implementation bug
- **Description**: The plan/deploy workflows fail with `azure/login@v2` error: "Using auth-type: SERVICE_PRINCIPAL. Not all values are present. Ensure 'client-id' and 'tenant-id' are supplied." The workflows reference `${{ secrets.AZURE_CLIENT_ID }}`, `${{ secrets.AZURE_TENANT_ID }}`, and `${{ secrets.AZURE_SUBSCRIPTION_ID }}` but these secrets are never created.
- **Root Cause**: The spec does not require `rad environment connect` to create GitHub repository secrets. The workflows expect secrets to exist, but there's no implementation to set them. This is a missing feature.
- **Resolution**: FIXED - Added FR-030-A/B/C to spec and implemented GitHub secret setting via `gh secret set` in both `connectAWS()` and `connectAzure()` functions.
- **Files Changed**: `pkg/cli/cmd/env/connect/connect.go`, `specs/001-github-mode/spec.md`
- **Spec Impact**: Added FR-030-A, FR-030-B, FR-030-C

---

### D029: rad deploy --plan flag does not exist ✅ FIXED
- **Phase/Task**: Phase 6 (US4) - Plan workflow
- **Category**: Missing implementation
- **Description**: The plan workflow generated by `rad pr create` uses `rad deploy --plan --environment ... --application ... --output .radius/plan/` but the existing `rad deploy` command does not have a `--plan` flag. The workflow fails with "Error: unknown flag: --plan".
- **Root Cause**: The spec references `rad deploy --plan` for plan generation (FR-039), but there's no explicit FR requiring the `--plan` flag to be added to the existing deploy command. The tasks marked this as "control plane API" but didn't implement the CLI wrapper.
- **Resolution**: FIXED - Added `--plan` and `--output` flags to `rad deploy` command. When `--plan` is specified, the command generates a deployment plan (plan.yaml) and Terraform artifacts without executing the deployment.
- **Files Changed**: `pkg/cli/cmd/deploy/deploy.go`
- **Spec Impact**: Added FR-039-A

---

## Recommendations for Future Specs

1. **Contract tests first**: Write contract tests in `contracts/` before implementation to catch API mismatches early

2. **Backward compatibility section**: Add explicit section in spec for migration/compatibility requirements

3. **Library limitations**: Document known limitations of chosen libraries in research.md

4. **Test impact analysis**: When tasks change behavior, explicitly list tests that need updating

---

### D030: Workflow downloads upstream rad CLI which doesn't have --plan flag ✅ FIXED
- **Phase/Task**: Phase 6 (US4) - Plan workflow
- **Category**: Architecture issue
- **Description**: The generated workflows download the rad CLI from `radius-project/radius` main branch using `curl ... | /bin/bash`. This public CLI doesn't have the `--plan` flag we added. The workflow fails with "Error: unknown flag: --plan".
- **Root Cause**: The workflow assumes the upstream rad CLI has all required features, but `--plan` is a custom feature in this branch that hasn't been merged upstream.
- **Resolution**: FIXED - Updated all workflow generators (deploy, destroy, plan) to build rad CLI from source:
  1. Added `actions/setup-go@v5` step with Go 1.23
  2. Added `go build -o /usr/local/bin/rad ./cmd/rad` step
  3. Removed `curl ... | /bin/bash` install script
  This ensures workflows use the rad CLI with all custom features from the current repository.
- **Files Changed**: `pkg/cli/github/workflows.go`
- **Spec Impact**: None - implementation detail

---

### D031: Image registry/tag need explicit workflow configuration ✅ DOCUMENTED
- **Phase/Task**: Phase 6 (US4) - Workflow generation
- **Category**: Configuration gap
- **Description**: User needs custom Radius images to be used in workflows, but the current implementation relies on optional repository variables which may not be set. Need explicit configuration during `rad init`.
- **Root Cause**: Image registry configuration added via repository variables, but no way to configure during `rad init`.
- **Resolution**: DOCUMENTED - Current solution requires manual setup:
  1. Run `make github-mode-publish` to build and push images to your GHCR
  2. In GitHub repository: Settings → Secrets and variables → Actions → Variables
  3. Add `RADIUS_IMAGE_REGISTRY` = `ghcr.io/<your-username>`
  4. Add `RADIUS_IMAGE_TAG` = `github-mode`
  
  Workflows automatically use these variables when set. See plan.md "Publishing Radius Control Plane Images" section.
- **Files Changed**: Documentation in `specs/001-github-mode/plan.md`, `build/github-mode.mk`
- **Spec Impact**: None - documented workaround

---

### D032: Workflow builds rad CLI from app repo instead of Radius repo ✅ FIXED
- **Phase/Task**: Phase 6 (US4) - Workflow runtime
- **Category**: Implementation bug
- **Description**: After D030 fix to build rad CLI from source, workflows fail with "go: cannot find main module" error. This is because the workflow checks out the user's application repository (not the Radius source repository), then attempts to run `go build ./cmd/rad` which doesn't exist in the app repo.
- **Root Cause**: Workflow used a single checkout step that checked out the app repository where the workflow runs. The rad CLI source code is in the Radius repository, not the user's application repository.
- **Resolution**: FIXED - Updated all workflow generators (`generateDeploySteps`, `generateDestroySteps`, `generatePlanSteps`) to use a dual-checkout pattern:
  1. First checkout the Radius source repository to `radius-src` path using configurable `RADIUS_REPO` and `RADIUS_REF` variables
  2. Build rad CLI: `cd radius-src && go build -o /usr/local/bin/rad ./cmd/rad`
  3. Then checkout the application repository to `app` path
  4. Update all subsequent commands to run from `app` directory (e.g., `cd app && rad deploy ...`)
  5. Mount `$GITHUB_WORKSPACE/app` instead of `$GITHUB_WORKSPACE` for k3d cluster volumes
  
  Users can configure via repository variables:
  - `RADIUS_REPO`: Source repo (default: `radius-project/radius`)
  - `RADIUS_REF`: Branch/tag/commit (default: `main`)
- **Files Changed**: `pkg/cli/github/workflows.go`
- **Spec Impact**: None - implementation detail for source-building approach

---

### D033: rad deploy requires --group flag in GitHub mode ✅ FIXED
- **Phase/Task**: Phase 6 (US4) - Workflow runtime
- **Category**: Spec gap
- **Description**: FR-077 specifies that generated workflows call `rad deploy` to execute deployments within ephemeral k3d clusters. However, `rad deploy` requires a resource group scope via either `--group` flag or a workspace `Scope` property. The GitHub-mode workspace intentionally does not set `Scope` (per D009 fix), causing `rad deploy` to fail with "No resource group set, use --group to pass in a resource group name."
- **Root Cause**: `RequireScope()` in `pkg/cli/clivalidation.go` only checked for `--group` flag or workspace `Scope` property, with no fallback for GitHub-mode workspaces. The spec did not account for `rad deploy`'s requirement for a resource group scope.
- **Resolution**: FIXED - Two changes:
  1. Updated `RequireScope()` in `pkg/cli/clivalidation.go` to detect GitHub-kind workspaces (via `workspace.Connection["kind"]`) and automatically default to `/planes/radius/local/resourceGroups/default` — no `--group` flag needed.
  2. Fixed all three workflow generators to pass the Bicep model file as the required positional argument (was missing entirely, causing "accepts 1 arg(s), received 0").
  
  The `--group default` workaround added earlier was removed since `RequireScope` now handles it natively.
- **Files Changed**: `pkg/cli/clivalidation.go`, `pkg/cli/github/workflows.go`
- **Spec Impact**: FR-077 should document that GitHub-mode workspaces automatically use `default` resource group. The Bicep file path should also be documented as a required positional argument for `rad deploy`.

---

### D034: Unused clierrors import after RequireScope refactor ✅ FIXED
- **Phase/Task**: D033 fix (RequireScope refactor)
- **Category**: Implementation bug
- **Description**: After refactoring `RequireScope()` in `pkg/cli/clivalidation.go` to fall back to the `default` resource group instead of returning a `clierrors.Message(...)` error, the `clierrors` package import became unused, causing a compile error: `"github.com/radius-project/radius/pkg/cli/clierrors" imported and not used`.
- **Root Cause**: The only usage of `clierrors` in `clivalidation.go` was the error message in `RequireScope`. When the error path was removed in favor of a default fallback, the import was left behind.
- **Resolution**: FIXED - Removed the unused `clierrors` import from `pkg/cli/clivalidation.go`.
- **Files Changed**: `pkg/cli/clivalidation.go`
- **Spec Impact**: None - build hygiene fix.

---

### D035: Duplicate OIDC setup log message in `rad env create` ✅ FIXED
- **Phase/Task**: Phase 4 (US2 — rad env create)
- **Category**: Implementation bug
- **Description**: Running `rad env create dev --provider azure` printed two OIDC setup messages: `"Setting up azure OIDC authentication..."` (lowercase, from `create.go`) followed by `"Setting up Azure OIDC authentication..."` (capitalized, from `oidc.go`). The first message was redundant since `oidc.go` already prints the properly capitalized message at the start of each OIDC flow.
- **Root Cause**: `pkg/cli/cmd/env/create/create.go` added its own `LogInfo("Setting up %s OIDC authentication...", r.Provider)` before calling `oidcSetup.SetupAzureOIDC()` or `oidcSetup.SetupAWSOIDC()`, but those functions already print the same message with correct capitalization.
- **Resolution**: FIXED - Removed the duplicate `LogInfo` call from `create.go`. The OIDC setup functions in `oidc.go` remain the single source of the log message.
- **Files Changed**: `pkg/cli/cmd/env/create/create.go`
- **Spec Impact**: None - log output cleanup.

---

### D036: Environment variables API uses environment ID instead of repository ID (HTTP 404) ✅ FIXED
- **Phase/Task**: Phase 4 (US2 — rad env create)
- **Category**: Implementation bug
- **Description**: Running `rad env create dev --provider azure` failed with `exit status 1: gh: Not Found (HTTP 404)` when setting environment variables (e.g., `AZURE_TENANT_ID`). The GitHub REST API path `repositories/{id}/environments/{env_name}/variables` requires the **repository** ID, but the code was passing the **environment** ID extracted from `GET /repos/{owner}/{repo}/environments/{env_name}`.
- **Root Cause**: `SetEnvironmentVariable()` and `GetEnvironmentVariables()` in `pkg/cli/github/environment.go` fetched the environment object and used its `id` field as the `{repository_id}` in the variables API path. The environment `id` is different from the repository `id`, causing a 404.
- **Resolution**: FIXED - Added a `getRepoID()` helper that fetches the repository ID from `GET /repos/{owner}/{repo}` and updated `SetEnvironmentVariable()`, `GetEnvironmentVariables()`, and related helper functions to use the repository ID instead of the environment ID.
- **Files Changed**: `pkg/cli/github/environment.go`
- **Spec Impact**: None - API implementation bug.

---

### D037: Vague "Setting environment variables..." log message ✅ FIXED
- **Phase/Task**: Phase 4 (US2 — rad env create)
- **Category**: UX / Log clarity
- **Description**: `rad env create dev --provider azure` printed `"Setting environment variables..."` without specifying which GitHub Environment the variables were being set in. Users need to know exactly where variables are being stored.
- **Root Cause**: The log message in `pkg/cli/cmd/env/create/create.go` used a generic string instead of including the environment name.
- **Resolution**: FIXED - Changed both AWS and Azure log messages to `"Setting GitHub Environment variables for '<name>'..."` to clearly identify the target GitHub Environment.
- **Files Changed**: `pkg/cli/cmd/env/create/create.go`
- **Spec Impact**: None - log output clarity improvement.

---

### D038: `rad init` should not create `.radius/` directory or use `.gitkeep` ✅ FIXED
- **Phase/Task**: Phase 3 (US1 — rad init)
- **Category**: Spec/implementation change
- **Description**: `rad init --github` was creating a `.radius/` directory with a `.gitkeep` file. Directories should only be created when necessary by the commands that actually use them (e.g., `rad app model` creates `.radius/applications/`, `rad deployment create` creates `.radius/deploy/`). `.gitkeep` files should never be used.
- **Root Cause**: Original FR-014-A specified that `rad init` should create the `.radius/` directory structure upfront. This was changed to defer all directory creation to the commands that need them.
- **Resolution**: FIXED — Removed `initializeRadiusDirectory()` entirely. `rad init` no longer creates `.radius/` or any `.gitkeep` files. Updated re-init detection to check for workflow files instead. Updated commit logic to only stage `.github/workflows/`.
- **Files Changed**: `specs/001-github-mode/spec.md`, `specs/001-github-mode/tasks.md`, `pkg/cli/cmd/radinit/github.go`
- **Spec Impact**: FR-014-A rewritten; FR-068-B and FR-047-A added.

### D039: `rad deployment create` treats untracked files as uncommitted changes ✅ FIXED
- **Phase/Task**: Phase 6 (US4 — rad deployment create)
- **Category**: Bug
- **Description**: `rad deployment create` uses `IsDirty()` to check for uncommitted changes before proceeding. The go-git `worktree.Status().IsClean()` method considers untracked files as dirty, causing a false positive "Working directory has uncommitted changes" error when the only difference is an untracked file (e.g., the `rad` binary itself).
- **Root Cause**: `go-git`'s `IsClean()` returns `false` for untracked files. The dirty check should only consider modifications to tracked files.
- **Resolution**: FIXED — Replaced `go-git` worktree status check with `git status --porcelain` and filtering out lines starting with `??` (untracked files).
- **Files Changed**: `pkg/cli/github/git.go`
- **Spec Impact**: None.

### D040: Remove redundant `RADIUS_CLOUD_PROVIDER` environment variable ✅ FIXED
- **Phase/Task**: Phase 4 (US2 — rad env create OIDC setup)
- **Category**: Cleanup
- **Description**: `RADIUS_CLOUD_PROVIDER` was set as a GitHub Environment variable during OIDC setup (value `"aws"` or `"azure"`) and consumed only by `rad env delete` to route OIDC cleanup. The variable is redundant because the provider can be inferred from the presence of provider-specific variables (`AZURE_CLIENT_ID` → Azure, `AWS_IAM_ROLE_NAME` → AWS).
- **Root Cause**: Original implementation stored the provider explicitly rather than inferring it from existing environment variables.
- **Resolution**: FIXED — Removed `RADIUS_CLOUD_PROVIDER` from `SetAWSEnvironmentVariables()` and `SetAzureEnvironmentVariables()` in `oidc.go`. Updated `promptOIDCCleanup()` in `delete.go` to infer the provider from `AZURE_CLIENT_ID` or `AWS_IAM_ROLE_NAME` presence.
- **Files Changed**: `pkg/cli/github/oidc.go`, `pkg/cli/cmd/env/delete/delete.go`
- **Spec Impact**: None — `RADIUS_CLOUD_PROVIDER` was not referenced in any workflow files or spec FRs.

### D041: Rename `RADIUS_K8S_CLUSTER` and `RADIUS_K8S_NAMESPACE` environment variables ✅ FIXED
- **Phase/Task**: Phase 4 (US2 — rad env create OIDC setup)
- **Category**: Naming convention
- **Description**: The GitHub Environment variables `RADIUS_K8S_CLUSTER` and `RADIUS_K8S_NAMESPACE` used a `RADIUS_K8S_` prefix. These should be named `KUBERNETES_CLUSTER` and `KUBERNETES_NAMESPACE` for clarity and consistency with other environment variable naming.
- **Root Cause**: Original naming used a Radius-specific prefix for Kubernetes configuration.
- **Resolution**: FIXED — Renamed to `KUBERNETES_CLUSTER` and `KUBERNETES_NAMESPACE` in both `SetAWSEnvironmentVariables()` and `SetAzureEnvironmentVariables()` in `oidc.go`.
- **Files Changed**: `pkg/cli/github/oidc.go`
- **Spec Impact**: None — these variable names were not referenced in spec, plan, or tasks.

### D042: `rad init` fails with "cannot create empty commit" on re-initialization ✅ FIXED
- **Phase/Task**: Phase 3 (US1 — rad init)
- **Category**: Bug
- **Description**: Running `rad init --github` a second time (or after `.radius/` creation was removed) fails with `failed to create commit: cannot create empty commit: clean working tree` because the workflow files already exist unchanged and there's nothing to stage.
- **Root Cause**: `go-git`'s `worktree.Commit()` errors on an empty commit. After removing `.radius/` directory creation (D038), the only files staged are workflow files — if they haven't changed, the staging area is empty.
- **Resolution**: FIXED — `CommitWithTrailer()` now checks for staged changes via `git diff --cached --quiet` before committing; returns empty hash if nothing to commit. `commitRadiusInit()` returns a bool indicating whether a commit was created. `runGitHubInit()` skips push and prints "No changes to commit." when no commit was made.
- **Files Changed**: `pkg/cli/github/git.go`, `pkg/cli/cmd/radinit/github.go`
- **Spec Impact**: None.

### D043: Implementation does not match FR-030-G ✅ FIXED
- **Phase/Task**: Phase 4 (US2 — rad env create)
- **Category**: Implementation gap
- **Description**: FR-030-G specifies "Creating authentication test workflow..." with animated progress indicator showing "Testing authentication to <provider>..." and `L` key support for real-time log streaming. The original implementation displayed "Dispatching authentication test workflow..." and used a simple `DispatchAndWatch` pattern without animated progress or `L` key log streaming.
- **Root Cause**: Implementation used a simpler dispatch-and-wait pattern instead of the spec's animated UX.
- **Resolution**: FIXED — Updated `dispatchAuthTest()` to: display "Creating authentication test workflow...", use `ProgressModel` with "Testing authentication to <provider>..." label and `L` key support, report success per FR-030-I ("Authentication test passed! Environment is ready for deployments."), report failure per FR-030-H with common OIDC misconfiguration hints and re-test command.
- **Files Changed**: `pkg/cli/cmd/env/create/create.go`
- **Spec Impact**: None — spec is correct as-is.

### D044: Workflow detect-provider steps fail because GitHub Environment variables aren't exposed as shell env vars ✅ FIXED
- **Phase/Task**: Phase 11 (Workflow generation)
- **Category**: Bug
- **Description**: All 4 "detect provider" steps in generated workflows (`radius-deployment-create.yaml`, `radius-deployment-apply.yaml`, `radius-destroy.yaml`, `radius-auth-test.yaml`) use `$AZURE_CLIENT_ID` and `$AWS_IAM_ROLE_NAME` as shell environment variables to detect the cloud provider. However, GitHub Environment **variables** (set via `vars.*`) are not automatically available as shell env vars — they require an explicit `env:` mapping in the step.
- **Root Cause**: GitHub Environment variables are accessed via the `${{ vars.* }}` context, not as shell env vars. The detect-provider shell scripts referenced bare `$AZURE_CLIENT_ID` / `$AWS_IAM_ROLE_NAME` without an `env:` mapping.
- **Resolution**: FIXED — Added `env:` blocks to all 4 detect-provider steps mapping `${{ vars.AZURE_CLIENT_ID }}` → `AZURE_CLIENT_ID` and `${{ vars.AWS_IAM_ROLE_NAME }}` → `AWS_IAM_ROLE_NAME`.
- **Files Changed**: `pkg/cli/github/workflows.go`
- **Spec Impact**: None.

### D045: Azure federated credential subject uses `ref:refs/heads/main` instead of `environment:<name>` ✅ FIXED
- **Phase/Task**: Phase 4 (US2 — Azure OIDC setup)
- **Category**: Bug
- **Description**: The Azure AD federated credential was created with subject `repo:OWNER/REPO:ref:refs/heads/main`, but all generated workflows run with a GitHub Environment (`environment: ${{ inputs.environment }}`). When a job uses a GitHub Environment, GitHub presents the subject as `repo:OWNER/REPO:environment:ENV_NAME`, which doesn't match the `ref:refs/heads/main` credential, causing OIDC login to fail with `AADSTS700213: No matching federated identity record found`.
- **Root Cause**: Federated credential subject was hardcoded to `ref:refs/heads/main` instead of using the environment-scoped subject.
- **Resolution**: FIXED — Changed federated credential subject to `repo:OWNER/REPO:environment:ENV_NAME`. Threaded `envName` parameter through `SetupAzureOIDC()` → `createAzureApp()`. Credential name now includes environment name (`github-actions-ENV_NAME`) to support multiple environments.
- **Files Changed**: `pkg/cli/github/oidc.go`, `pkg/cli/cmd/env/create/create.go`
- **Spec Impact**: None.

### D046: L key does not stream logs and q key allows premature quit ✅ FIXED
- **Phase/Task**: Phase 4 (FR-089-E, FR-089-F — CLI Workflow Dispatch UX)
- **Category**: Bug
- **Description**: Two issues: (1) Pressing `L` during workflow progress toggled the ShowLogs flag but no log data was ever fetched — `CreatePollFunc` only polls status, never sends `WorkflowLogMsg`. (2) The `q` key allowed the user to quit prematurely during workflow execution, which should not be permitted — the user must wait for success or failure.
- **Root Cause**: (1) No log polling mechanism existed. `WorkflowLogMsg` type was defined but never sent. (2) `q` key handler was present in `ProgressModel.Update` alongside `ctrl+c`.
- **Resolution**: FIXED — (1) Added `CreateLogPollFunc(runID)` to `Client` that fetches logs via `gh run view <id> --log`, tracks previously seen lines, and returns only new lines as `WorkflowLogMsg`. Added `LogPollFunc` field to `ProgressModel`. When `L` is pressed, starts a 2-second log polling loop. `WorkflowLogMsg` handler appends lines and continues polling while ShowLogs is active. (2) Removed `q` key from handler (kept `ctrl+c` for emergency). Updated help text to show only "Press L to show/hide logs".
- **Files Changed**: `pkg/cli/github/progress.go`, `pkg/cli/github/client.go`, `pkg/cli/cmd/env/create/create.go`, `pkg/cli/cmd/deployment/create/create.go`, `pkg/cli/cmd/deployment/apply/apply.go`, `pkg/cli/cmd/app/delete/delete.go`
- **Spec Impact**: Updated FR-089-E (no quit option, user must wait) and FR-089-F (logs via `gh run view --log` polling).

### D047: L key shows no output during in-progress workflow runs ✅ FIXED
- **Phase/Task**: Phase 4 (FR-089-E, FR-089-F — CLI Workflow Dispatch UX)
- **Category**: Bug
- **Description**: Pressing `L` during a running workflow appeared to do nothing. Logs only appeared after the workflow completed. Root cause: `CreateLogPollFunc` used `gh run view <id> --log`, which only returns output for completed runs. For in-progress runs, `--log` returns an error/empty output, so no `WorkflowLogMsg` lines were ever sent while the workflow was running.
- **Root Cause**: `gh run view --log` only works for completed GitHub Actions runs. Pressing `L` toggled `ShowLogs` but the log poll returned empty results for in-progress runs. Additionally, `WorkflowLogMsg` appended lines instead of replacing them, which caused duplicate entries if the same data was fetched multiple times.
- **Resolution**: FIXED — Changed `CreateLogPollFunc` to use `gh run view <id> --json jobs` which returns real-time job/step status for both in-progress and completed runs. Step status is formatted with icons: `✓` (success), `✗` (failure), `●` (in progress), `○` (pending). Changed `WorkflowLogMsg` handling from append to replace (each poll returns a full snapshot of step statuses). Updated View header from "Workflow Logs" to "Workflow Steps".
- **Files Changed**: `pkg/cli/github/client.go`, `pkg/cli/github/progress.go`
- **Spec Impact**: None (FR-089-F already updated in D046 to specify polling mechanism).

### D048: L key shows step status icons instead of actual logs on completed runs ✅ FIXED
- **Phase/Task**: Phase 4 (FR-089-F — CLI Workflow Dispatch UX)
- **Category**: Bug
- **Description**: When pressing `L` during or after a workflow run, the display showed step status icons (✓/✗/●/○) instead of actual log content. The `CreateLogPollFunc` only used `gh run view --json jobs` which returns step names and status but not log output. The user expected to see actual step logs (command output, authentication details, etc.).
- **Root Cause**: `CreateLogPollFunc` only used `--json jobs` for step status. It never tried `gh run view --log` which returns actual log output for completed runs.
- **Resolution**: FIXED — Implemented hybrid approach in `CreateLogPollFunc`: tries `gh run view --log` first (returns actual logs for completed runs), parses tab-separated output into step-grouped formatted lines with timestamps stripped. If `--log` is not available (in-progress runs), falls back to `--json jobs` step status with icons. Added `parseWorkflowLogs()` helper that groups log lines by step, filters internal steps (Set up job, Complete job, Post *), and caps output at 15 lines per step.
- **Files Changed**: `pkg/cli/github/client.go`
- **Spec Impact**: Updated FR-089-F to describe hybrid log/step-status behavior.

### D049: Replace L key toggle with automatic step status display ✅ FIXED
- **Phase/Task**: Phase 4 (FR-089-E, FR-089-F — CLI Workflow Dispatch UX)
- **Category**: UX improvement
- **Description**: The L key to toggle log streaming was unreliable — `gh run view --log` only works for completed runs, making it appear broken during in-progress workflows. The step status display (via `--json jobs`) works reliably in all states and provides useful real-time feedback. Simplification: show steps automatically, remove L key toggle entirely, and direct users to the workflow run URL for detailed logs.
- **Root Cause**: `gh run view --log` is inherently unavailable for in-progress runs. The L key toggle added complexity without reliable value.
- **Resolution**: FIXED — Removed `ShowLogs` field, `LogPollFunc` field, L key handler, and help text from `ProgressModel`. Added `StepPollFunc` field that is polled automatically from `Init()`. Steps are always displayed below the spinner. Renamed `CreateLogPollFunc` to `CreateStepPollFunc`, simplified to only use `--json jobs`. Removed `parseWorkflowLogs()` and `logTimestampRegex`. Added `⊘` icon for skipped steps. Updated all 4 callers (env/create, deployment/create, deployment/apply, app/delete) to use `StepPollFunc`.
- **Files Changed**: `pkg/cli/github/progress.go`, `pkg/cli/github/client.go`, `pkg/cli/cmd/env/create/create.go`, `pkg/cli/cmd/deployment/create/create.go`, `pkg/cli/cmd/deployment/apply/apply.go`, `pkg/cli/cmd/app/delete/delete.go`
- **Spec Impact**: Updated FR-030-G, FR-089-E, FR-089-F, FR-106-D, and Q&A to replace L key toggle with automatic step status display.

### D050: Azure AD Application prompt should be a single merged list ✅ FIXED
- **Phase/Task**: Phase 4 (FR-026 — Azure OIDC setup)
- **Category**: UX improvement
- **Description**: The Azure AD Application prompt used a two-step flow: first asking "Create new" vs "Use existing", then listing apps or prompting for ID. This was inconsistent with the resource group prompt which shows "Create new" as the first option in a single list of existing items.
- **Root Cause**: Original implementation used separate choice prompt and app list prompt instead of a merged single-list approach.
- **Resolution**: FIXED — Merged into a single `GetListInput` prompt: "Create new Azure AD application" as the first option, followed by existing apps from `az ad app list`. Applied to both `pkg/cli/github/oidc.go` (`promptForAzureApp`) and `pkg/cli/cmd/env/connect/connect.go` (`promptForAzureApp`, replacing old `promptForExistingAzureApp`). Updated test expectations in `connect_test.go`.
- **Files Changed**: `pkg/cli/github/oidc.go`, `pkg/cli/cmd/env/connect/connect.go`, `pkg/cli/cmd/env/connect/connect_test.go`
- **Spec Impact**: None.

### D051: Duplicate workflow URL displayed before progress model ✅ FIXED
- **Phase/Task**: Phase 4 (FR-089-D — CLI Workflow Dispatch UX)
- **Category**: Bug
- **Description**: After dispatching a workflow, `LogInfo("Workflow started: %s", runURL)` was printed before the progress model started. The progress model also displays "View run: URL". This resulted in duplicate URL display.
- **Root Cause**: Redundant `LogInfo` calls added before progress model, which already renders the run URL in its view.
- **Resolution**: FIXED — Removed `r.Output.LogInfo("Workflow started: %s", runURL)` and any following blank `LogInfo("")` from all 4 workflow-dispatching commands.
- **Files Changed**: `pkg/cli/cmd/env/create/create.go`, `pkg/cli/cmd/deployment/create/create.go`, `pkg/cli/cmd/deployment/apply/apply.go`, `pkg/cli/cmd/app/delete/delete.go`
- **Spec Impact**: None.

### D052: `rad app model` fails with "Radius not initialized" after `rad env create` ✅ FIXED
- **Phase/Task**: Phase 4 (E2E testing)
- **Category**: Bug
- **Description**: After successfully running `rad env create`, running `rad app model` failed with "Radius not initialized. Run 'rad init' first to initialize the repository." The `Validate` method checked for a `.radius/` directory, but `rad init --github` no longer creates it (removed in D038). The workspace-kind check (GitHub vs Kubernetes) was the proper init guard.
- **Root Cause**: Stale `.radius/` directory check in `model.go` `Validate()` was not updated when D038 removed `.radius/` creation from `rad init --github`. `rad app model` itself creates `.radius/applications/` via `os.MkdirAll` in `Run()`, making the pre-check unnecessary.
- **Resolution**: FIXED — Removed the `.radius/` directory existence check from `Validate()`. The workspace-kind validation (lines 144-148) is sufficient. Updated model tests to reflect that validation passes without `.radius/` directory.
- **Files Changed**: `pkg/cli/cmd/model/model.go`, `pkg/cli/cmd/model/model_test.go`
- **Spec Impact**: None.
