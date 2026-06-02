# Feature Specification: GitHub Radius

* **Author**: Zach Casper (@zachcasper)

## Vision

The GitHub Copilot app is the next generation of developer UX. The terminal, the IDE, and the cloud console were each designed for a world in which the human typed every instruction. The Copilot app is designed for a world in which the human expresses intent and an AI agent does the typing. That shift demands a new center of gravity for the developer experience: instead of files, commands, and consoles, the new center is the application—what it is, where it runs, and how it changes over time. The developer should be able to think and talk in terms of applications and environments, and the tooling should follow.

Radius is what makes that possible. Radius already models cloud-native applications as a graph of resources connected to environments, and it already abstracts the differences between clouds and runtimes. Until now, Radius has been delivered as a Kubernetes-hosted control plane with a CLI. This architecture was designed for enterprise platform engineering teams running infrastructure for many internal developers. GitHub Radius is a different delivery of the same core idea, optimized for AI coding agents used by individual developers, small teams, and open-source maintainers.

With GitHub Radius, the GitHub Copilot app understands what applications reside in which repositories, or what repositories compose which applications. 



is built toward is one in which the GitHub Copilot app understands, for any GitHub repository, what applications live in it, or, for any application, which repositories compose it, and can seamlessly deploy that application to Azure, AWS, Google Cloud, or Kubernetes based on the developer's preference. The developer describes intent; Copilot, powered by Radius, takes care of the rest. The Kubernetes-hosted version of Radius continues to exist for platform teams who need it, but for individual developers, small teams, and open-source maintainers, the experience is the conversation in the Copilot app.

This feature spec is the first concrete step toward that vision: the end-to-end application lifecycle, in the Copilot app, with Radius invisible.

Because GitHub Radius is consumed by an LLM through conversation rather than by humans through text editors, several long-standing assumptions no longer hold:

* **A long-running control plane is no longer the right shape.** Synchronous operations the user expects to feel instant, such as listing apps, viewing the graph, or modeling a new resource, should happen in the agent's process, not over a multi-minute control-plane round trip. Asynchronous operations the user already expects to take minutes, such as building, provisioning, and deploying, should happen in an ephemeral GitHub Actions runner that boots Radius, executes, and exits.
* **A separate CLI is no longer the right surface.** When the agent is the primary user of Radius capabilities, the right interface is a set of agent-callable tools and skills, not a human-typed CLI. The CLI was the means to an end; the end is "do the thing I asked for," which Copilot can now express directly.
* **Strongly-typed IaC files are no longer required.** Bicep (and similar IaC languages) earned its place because humans authoring infrastructure benefit from type checking, IDE autocomplete, schema-driven validation, and the ability to review a textual diff in a pull request. In an LLM-driven workflow, almost all of that value moves elsewhere. The LLM authors and reads JSON natively, so its "autocomplete" is the conversational round-trip. Validation moves into the tool schemas (JSON Schema on MCP tools and skill-invoked binaries), which the agent reads directly. Auditable history is preserved by storing the application graph as JSON on a `.radius` orphan branch in the repository (diffable, reviewable, and rollback-friendly) without forcing a human to author a second representation of the same information in a templating language. Forcing the LLM (or the developer) to round-trip through `.bicep` files would actually reduce safety, because we would have two competing sources of truth and a translation layer between them.



## Summary

This feature spec describes the end-to-end application lifecycle (define, plan, deploy, promote, update, and delete) experienced entirely through the GitHub Copilot app. The lifecycle is powered by GitHub Radius behind the scenes, but the user never sees Radius branding, never installs Radius, never learns Radius concepts, and never operates a Radius control plane. From the developer's point of view, Copilot simply understands how to take an application in a GitHub repository and run it in a cloud environment.

The technical foundation is the architecture proposed in [`architecture/2026-05-github-radius.md`](../architecture/2026-05-github-radius.md): a JSON application graph stored in a `.radius` orphan branch of the user's repository, mutated synchronously by Copilot through agent skills, and deployed asynchronously by a Radius control plane that runs ephemerally in a GitHub Actions runner.

The user surface is **only** the GitHub Copilot app. There is no `rad` CLI, no Dashboard, no Kubernetes prerequisite, and no separate web UI in scope.

### Top level goals

* Let a developer take an application in a GitHub repository from "I want to run this" to "it is running in my cloud" through natural-language conversation in the GitHub Copilot app.
* Cover the full lifecycle: define, plan, deploy, promote between environments, detect and reconcile drift, and delete.
* Require zero installation beyond the GitHub Copilot app itself. Enabling Radius is an opt-in, one-step action inside Copilot.
* Keep Radius invisible. The user thinks they are using Copilot, not Radius.
* Make the cloud the substrate. Kubernetes, container registries, and resource providers are implementation details the user does not need to know.

### Non-goals (out of scope)

* Enterprise scenarios: multi-tenant control planes, central platform teams, RBAC, policy authoring, audit, compliance reporting, fleet management.
* Self-hosted Radius on a long-running Kubernetes cluster.
* A `rad` CLI, Radius Dashboard, or Headlamp integration.
* IDE integrations (VS Code, JetBrains, etc.). The Copilot app is the only client.
* GCP and on-prem targets. AWS and Azure only for this scope.
* Manual authoring of `app.bicep` files. The graph is JSON-native; Bicep export is a future consideration.
* Migration from a hosted Radius instance to GitHub Radius.

## User profile and challenges

### User persona(s)

The primary user is an **individual developer working on a small application** in a GitHub repository. Concretely:

* A solo developer or a member of a 2–10 person team building a side project, prototype, internal tool, or commercial SaaS for a small business.
* A maintainer of an open-source project who wants contributors and evaluators to be able to deploy the project to their own cloud with minimal effort.
* A developer using a personal AWS or Azure account, or a small-business shared account. There is no central platform team.
* Comfortable with Git and GitHub, comfortable in a terminal, but **not** a Kubernetes operator and not a cloud infrastructure expert.
* Already uses GitHub Copilot for code generation and has installed (or is willing to install) the GitHub Copilot app.

Explicitly out of persona: enterprise platform engineers, SREs operating shared platforms, compliance officers, central infrastructure teams.

### Challenge(s) faced by the user

* **Getting an application running in the cloud is still hard.** Even for a simple web service plus database, the developer must learn at least one IaC tool, one container runtime, one cloud provider's identity model, and one deployment workflow.
* **The skills don't transfer between clouds.** Moving the same app from AWS to Azure means re-learning a different set of services and templates.
* **There is no single source of truth for "what my app is."** Application architecture is scattered across a Dockerfile, a `docker-compose.yml`, some Terraform, a CI workflow, and tribal knowledge.
* **Day-2 is harder than day-1.** Promoting from dev to test, detecting drift, and cleanly tearing down environments are afterthoughts in most tools.
* **Existing developer-friendly PaaS options force a tradeoff.** Heroku-style PaaS hides too much and locks the developer in. Raw Terraform/Pulumi exposes too much. There is little in the middle that respects developer agency while still being approachable.
* **AI coding assistants do not yet understand deployment.** Copilot can write the code, but it cannot reliably get the code running in a cloud account.

### Positive user outcome

The developer describes their intent in plain English to Copilot, such as "deploy this app to my AWS account," and Copilot does it. The developer keeps full ownership of their source code and their cloud account, never installs platform software, and gets a consistent experience whether they are deploying to AWS or Azure, to a dev environment or a production one. Going from "this repo exists" to "this app is running" takes minutes and one conversation.

## Key scenarios

### Scenario 1: First-run opt-in and application modeling

A developer in the Copilot app opens a repository that doesn't yet have a deployment story. They ask Copilot to deploy it. Copilot offers to enable the deployment capability for this repository, the developer accepts, and Copilot produces a model of the application by analyzing the source code. The model is shown back to the developer for confirmation and edits.

### Scenario 2: Connect a cloud environment

The developer asks Copilot to add a deployment environment for their AWS account or Azure subscription. Copilot walks them through credentialing using GitHub-native mechanisms (OIDC, environment secrets) so that no long-lived cloud credentials ever leave GitHub. The environment becomes a first-class concept in the conversation.

### Scenario 3: Plan and deploy to dev

The developer asks Copilot to deploy the application to their dev environment. Copilot shows a plan (for example, "here is what will be created in your AWS account"), the developer approves, Copilot dispatches a GitHub Actions workflow, and the chat surface shows live progress as resources come up.

### Scenario 4: Promote dev → test → prod

After validating dev, the developer asks Copilot to promote the application to the next environment. Copilot reuses the same application model, applies environment-specific configuration, shows the diff, and deploys on approval.

### Scenario 5: Detect and reconcile drift

The developer asks "is anything different in prod than what I expect?" or returns to a repo after time away. Copilot compares the application model in the repo to what is actually running in the cloud, surfaces drift, and offers to reconcile in either direction.

### Scenario 6: Modify and redeploy

The developer asks Copilot to "add a Redis cache" or "make this service public on a custom domain." Copilot edits the application model, shows the impact, and offers to redeploy affected environments.

### Scenario 7: Delete a deployment

The developer asks Copilot to tear down a specific environment, or all environments, for an application. Copilot shows what will be destroyed, gets explicit confirmation, and dispatches the teardown workflow.

## Key dependencies and risks

* **Dependency: GitHub Copilot app and Copilot CLI.** The entire user experience runs in this surface. Any feature, billing, or platform-policy change in the Copilot app affects users directly. *Mitigation*: align with the documented Copilot CLI extension points (skills, MCP servers, custom instructions) and avoid undocumented surfaces.
* **Dependency: GitHub Agent Skills standard.** The lifecycle is delivered as a set of agent skills installed via `gh skill install`. We depend on the skills spec remaining stable. *Mitigation*: the spec is an open standard; risk of breaking change is low, and skill descriptions are decoupled from the underlying `rad-graph` binary that does the real work.
* **Dependency: GitHub Actions runners as the deployment substrate.** All asynchronous deployment work runs in Actions. *Mitigation*: Actions is universally available to the persona; runner cost is borne by the user's repository, which is acceptable for small apps.
* **Dependency: The built-in GitHub MCP server.** Skills rely on it for workflow dispatch, run polling, and PR/issue operations. *Mitigation*: it is pre-configured in the Copilot app; no user action required.
* **Dependency: A graph-aware Radius runtime that can execute in an ephemeral runner.** The existing Radius control plane is designed for long-running Kubernetes. Significant work is required to make it boot-and-die inside an Actions job, write to the `.radius` orphan branch as the data store, and stream status.
* **Risk: Hiding Radius too well.** If something fails, the user must be able to understand and recover without knowing Radius internals. *Mitigation*: every skill produces user-readable explanations; failures map to actionable next steps in the conversation, not stack traces.
* **Risk: Orphan-branch concurrency.** Multiple skills or parallel Copilot sessions could race on the `.radius` branch. *Mitigation*: design the data store with optimistic concurrency (commit-then-rebase) and explicit lock files; treat the graph as an append-friendly structure where possible.
* **Risk: Credential model.** Small-business developers will not configure complex OIDC trust policies on their own. *Mitigation*: provide a guided, copy-paste-free credentialing skill that uses GitHub's native cloud federation flows end-to-end.
* **Risk: Cost surprises.** Provisioning real cloud resources from a conversational prompt can produce unexpected bills. *Mitigation*: every plan includes an estimated cost or a clear "this will create N resources in your account" summary; destructive operations require explicit confirmation.
* **Risk: User does not realize Radius is involved when they seek help.** Hiding the brand makes community support harder. *Mitigation*: surface a discreet "powered by Radius" link and a `/about` command in the skills for users who want to dig deeper.

## Key assumptions to test and questions to answer

* **Assumption: The GitHub Copilot app is a viable single surface for the full lifecycle.** Validate by walking each scenario end-to-end in a prototype before broader work.
* **Assumption: Skills + the built-in GitHub MCP server are sufficient for MVP; a custom MCP server is not required.** Validate by prototyping the deploy + status-streaming scenarios as skills only. If chat-native real-time streaming is too poor, add a small custom MCP server.
* **Assumption: Storing the graph in a `.radius` orphan branch is acceptable to developers.** Validate via user research: do developers find a hidden branch on their repo surprising, annoying, or fine?
* **Assumption: Developers will accept opt-in per-repository.** Validate that the friction of "enable for this repo" is low enough to not be a drop-off point.
* **Question: What level of cost-impact information does the user need before deploying?** Per-resource list, estimated monthly cost, or both?
* **Question: How is "promote" semantically different from "deploy to environment X"?** Should it be a distinct skill, or just a parameterized deploy?
* **Question: How does drift detection scale when the cloud has thousands of side-resources the user did not create?**
* **Question: Where does the user provide cloud credentials when they have never set up OIDC?** Is there a sufficiently-guided path that does not require leaving the Copilot app?
* **Question: When the user uninstalls the skills, what happens to deployed resources?** Is there a "graceful exit" obligation?

## Current state

* Radius today is a Kubernetes-hosted control plane with the `rad` CLI as its primary client. None of the user-facing surface required by this spec exists.
* An earlier feature spec, [`2026-03-github-radius-feature-spec.md`](2026-03-github-radius-feature-spec.md), explored a mock GitHub web GUI integration. This spec supersedes that direction by moving the entire surface into the GitHub Copilot app.
* The architecture proposal [`architecture/2026-05-github-radius.md`](../architecture/2026-05-github-radius.md) defines the data-store and execution-model changes that make this feature spec implementable: graph in `.radius` orphan branch, synchronous ops as skills, async deploy in an Actions-runner Radius.
* The agent skills standard ([agentskills](https://github.com/agentskills/agentskills)) is the chosen distribution mechanism. The GitHub Copilot app supports skills, MCP servers, and custom instructions out of the box.
* No existing component of the Radius repository runs in an ephemeral GitHub Actions runner today; the control plane assumes long-running operation against etcd/PostgreSQL.

## Details of user problem

I am a developer building a small application in a GitHub repository. I have written the code and it runs on my laptop, but getting it deployed to my AWS account or Azure subscription is consistently the hardest part of my project.

When I try to deploy my application, I have the following challenges:

* I have to choose and learn an IaC tool, then write hundreds of lines of templates that mostly express things my application code already implies.
* I have to wire up CI/CD myself: pick a workflow, plumb credentials, manage secrets, and keep it working over time.
* I have to learn a cloud provider's identity model just to grant my deployment workflow permission to create resources in my own account.
* When I want a second environment (test, staging, prod), I copy-paste my IaC and try to keep the copies in sync.
* When I come back to the project after a month, I cannot tell what is actually deployed without running `terraform plan` or clicking through the cloud console.
* When I am done with the project, tearing it down cleanly is its own multi-step ordeal.
* If I want to switch from AWS to Azure, almost none of the work I've done carries over.

These issues mean I spend a large fraction of my time on deployment plumbing rather than on the application itself. For side projects, this is the difference between shipping and abandoning. For small commercial projects, it is direct opportunity cost. And it means open-source projects I publish are harder for others to try in their own clouds, which limits adoption.

I already use GitHub Copilot and have installed the Copilot app. I'd like the Copilot app to be the single place where I think about my application, including where it runs.

## Desired user experience outcome

After this scenario is implemented, I can describe what I want in natural language to Copilot, such as "deploy this app to my AWS account," "promote to staging," "is anything different in prod?", or "tear down the dev environment," and Copilot does it. I never install platform software. I never learn a new templating language. I never operate a control plane. The same conversation works whether I'm targeting AWS or Azure, dev or prod. My GitHub repository stays the source of truth, my cloud account stays mine, and my application's deployment architecture is something I can actually see and reason about, because Copilot shows it to me as a graph.

As a result, I spend my time on the application instead of on the deployment system, I can hand my repo to a collaborator and they can stand up their own copy in their own cloud the same way, and I can move between clouds without throwing away my work.

### Detailed user experience

The following steps describe the end-to-end lifecycle. Each step happens entirely inside the GitHub Copilot app.

**Step 1: Opt in (one-time, per repository).**
The developer opens the Copilot app, selects their repository's workspace, and types something like *"Deploy this app to my AWS account."* Copilot replies that it can do this, but first needs to enable its deployment capability for this repository, which is a one-time action. The developer confirms. Copilot installs the necessary agent skills (via `gh skill install`, performed by Copilot on the developer's behalf, with consent) and confirms they're ready.

**Step 2: Define and develop (modeling the application).**
Copilot analyzes the repository (source layout, Dockerfiles, framework hints, declared dependencies) and produces a proposed application model: a small set of named application resources (e.g., *web*, *worker*, *postgres*) and the relationships between them. Copilot presents the model in chat as a readable summary and an inline Mermaid diagram. The developer can ask follow-up questions or request changes ("split the worker into two", "use Redis instead of in-memory cache") and Copilot updates the model. The model is persisted in the `.radius` orphan branch, but the developer doesn't need to know that; they just see that "the model is saved."

**Step 3: Connect an environment.**
The developer says *"Set up a dev environment in my AWS account."* Copilot asks for the AWS account ID and preferred region, then walks the developer through GitHub-native cloud federation: it generates the IAM trust policy, points the developer at the exact AWS console page to paste it into, validates that federation works, and stores the resulting GitHub environment + secrets. The environment becomes a first-class entity the developer can refer to by name ("dev", "test", "prod"). The same flow works for Azure.

**Step 4: Plan deployment.**
The developer says *"What would deploying to dev create?"* (or this happens implicitly before any deploy). Copilot computes a planned graph: for each application resource, it shows which concrete cloud resources will be created in the environment, with names and rough cost characterization. The plan is rendered as a Mermaid diagram and an enumerated list. The developer can ask questions, override choices, or accept.

**Step 5: Deploy.**
The developer says *"Deploy."* Copilot dispatches a GitHub Actions workflow that boots a Radius runtime inside the runner, reads the application model from the `.radius` branch, and provisions resources in the developer's cloud. Copilot polls and streams progress back into the chat: "container image built," "RDS instance creating (3/5)," "ingress ready at https://…". On completion, the deployed graph view is shown with real cloud resource IDs.

**Step 6: Promote to the next environment.**
The developer says *"Promote to test."* Copilot reuses the application model, layers in test-environment configuration, shows the diff against what will exist in the test environment, and (on approval) deploys. The developer sees a clear before/after.

**Step 7: Update deployment (drift detection and reconciliation).**
At any time, the developer can ask *"Is anything different in prod than what I expect?"* Copilot queries the live cloud state, compares to the deployed graph, and reports drift: resources missing, resources mutated outside the model, or resources present but unmodeled. Copilot offers two reconciliation paths: (a) update the model to match reality, or (b) re-deploy to make reality match the model.

**Step 8: Modify and redeploy.**
The developer asks for a change, such as *"Add a Redis cache to the web service"*, *"Use a custom domain"*, or *"Move postgres to a managed database in test and prod, but keep it as a container in dev"*. Copilot edits the application model, shows the impact across environments, and offers to redeploy each affected environment.

**Step 9: Delete a deployment.**
The developer says *"Tear down the dev environment."* Copilot lists exactly what will be destroyed, requires an explicit confirmation, dispatches a teardown workflow, and reports completion. The application model in the repo is retained unless the developer also asks to remove it.

**Step 10: Opt out.**
The developer can disable the capability for a repository at any time. Copilot offers to first tear down all environments cleanly, and explains exactly what will be left behind if they decline.

Throughout every step, the developer never sees the word "Radius," never opens a `.bicep` file, never installs a CLI, never runs `kubectl`, and never leaves the Copilot app.

## Key investments

### Feature 1: Opt-in capability bundle

A single, installable bundle of agent skills that the Copilot app can enable for a repository on user consent. The bundle is published in the GitHub Awesome Copilot collection (or equivalent) and installable via `gh skill install`. Includes a discoverable triggering description so that prompts like "deploy this app" reliably select the right skills without the user knowing they exist.

### Feature 2: Application analysis and modeling skill

Reads a repository, infers an application graph, writes it to the `.radius` orphan branch, and presents a developer-readable summary. Supports iterative edits via chat ("split this", "swap that").

### Feature 3: Environment provisioning skill

Guides the developer through cloud federation using GitHub OIDC, registers AWS accounts and Azure subscriptions as named environments, and stores the binding in the repository's GitHub environments. Avoids long-lived secrets.

### Feature 4: Graph visualization in chat

Renders the application model, planned graph, and deployed graph as Mermaid diagrams inline in the Copilot app conversation. Includes a textual rendering for accessibility and for when Mermaid is unavailable.

### Feature 5: Plan skill

Computes and presents what will be created, modified, or destroyed for a given (application, environment) pair, with cost characterization and a clear approval gate.

### Feature 6: Deploy and status-streaming skill

Dispatches the deployment workflow and reports live progress in the chat thread. Decides per scenario whether status streaming is acceptable from a polling skill or requires a small custom MCP server.

### Feature 7: Promote skill

Promotes an application from one environment to another, reusing the model and applying environment-specific config, with a clear diff and approval gate.

### Feature 8: Drift detection and reconciliation skill

Compares the deployed graph to live cloud state, surfaces drift in plain English, and offers reconciliation in either direction.

### Feature 9: Teardown skill

Destroys an environment or an entire application's footprint, with explicit confirmation and a clear summary of what will be removed.

### Feature 10: Ephemeral Radius runtime for GitHub Actions

The Radius control plane, packaged to run inside a GitHub Actions job: boots, reads the graph from the `.radius` branch, performs the requested operation, writes status back to the branch as it progresses, and exits. No etcd, no PostgreSQL, no long-running cluster.

### Feature 11: Graph data store on `.radius` orphan branch

The on-disk format, locking model, and commit conventions for the application graph stored as JSON on an orphan branch of the user's repository. Designed so that both skills (synchronous, conversational) and the ephemeral runtime (asynchronous, deployment) can safely co-author it.

### Feature 12: Guided credentialing experience

A skill that turns "set up AWS access" or "set up Azure access" into a finite, copy-once flow producing a working OIDC trust relationship and GitHub environment configuration, without the developer needing to understand OIDC, trust policies, or service principals.
