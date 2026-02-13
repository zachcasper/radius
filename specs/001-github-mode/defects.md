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

---

## Recommendations for Future Specs

1. **Contract tests first**: Write contract tests in `contracts/` before implementation to catch API mismatches early

2. **Backward compatibility section**: Add explicit section in spec for migration/compatibility requirements

3. **Library limitations**: Document known limitations of chosen libraries in research.md

4. **Test impact analysis**: When tasks change behavior, explicitly list tests that need updating
