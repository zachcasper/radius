# Kubernetes Cluster Deployment

## Overview

This document describes the approach for deploying Radius application output resources
(Deployments, Services, ConfigMaps, etc.) to real Kubernetes clusters (AKS, EKS) while
keeping the Radius control plane on an ephemeral k3d cluster within GitHub Actions.

## Problem Statement

In the GitHub mode architecture, the Radius control plane runs on an ephemeral k3d cluster
inside a GitHub Actions runner. By default, when Radius deploys application output resources,
it deploys them to the same cluster where the control plane is running — the k3d cluster.

For production use, application resources need to deploy to real managed Kubernetes clusters
(AKS, EKS, GKE, etc.) that persist beyond the lifetime of the GitHub Actions workflow.

### Key Constraint

The Radius control plane **must** remain ephemeral. It is spun up at the beginning of a
workflow run and torn down at the end. This is a fundamental design requirement of GitHub mode
that keeps Radius out of the critical path for running applications.

## Architecture

```
┌─────────────────────────────────────────────────────┐
│  GitHub Actions Runner                              │
│                                                     │
│  ┌───────────────────────────────────────────┐      │
│  │  k3d Cluster (ephemeral)                  │      │
│  │                                           │      │
│  │  ┌─────────────────┐  ┌──────────────┐   │      │
│  │  │ applications-rp  │  │   bicep-de   │   │      │
│  │  │                  │  └──────────────┘   │      │
│  │  │  RADIUS_TARGET_  │  ┌──────────────┐   │      │
│  │  │  KUBECONFIG ─────┼──│ Secret:      │   │      │
│  │  │  (env var)       │  │ target-      │   │      │
│  │  │                  │  │ kubeconfig   │   │      │
│  │  └──────┬───────────┘  └──────────────┘   │      │
│  │         │                                  │      │
│  └─────────┼──────────────────────────────────┘      │
│            │                                         │
│            │ Deploys output resources via             │
│            │ external kubeconfig                     │
└────────────┼─────────────────────────────────────────┘
             │
             ▼
┌────────────────────────────┐
│  Target Cluster (AKS/EKS)  │
│                             │
│  ┌───────────┐ ┌─────────┐ │
│  │ Deployment│ │ Service │ │
│  │ (app)     │ │ (app)   │ │
│  └───────────┘ └─────────┘ │
│                             │
│  Resources persist after    │
│  k3d cluster is deleted     │
└─────────────────────────────┘
```

## Approach: Dual-Cluster with Kubeconfig Injection

### Summary

1. The Radius control plane runs on an ephemeral k3d cluster (unchanged).
2. The target cluster kubeconfig is fetched via cloud CLI (az/aws) during the workflow.
3. The kubeconfig is injected as a Kubernetes Secret into k3d's `radius-system` namespace.
4. The `applications-rp` deployment is patched to mount the secret and set the
   `RADIUS_TARGET_KUBECONFIG` environment variable.
5. The async worker in `applications-rp` detects the env var and creates a separate set of
   Kubernetes clients pointing at the target cluster for deploying output resources.

### Why Not Install Radius Directly on the Target Cluster?

Installing the Radius control plane directly on AKS/EKS would be simpler but violates the
ephemeral requirement. The control plane must be spun down after taking action — leaving
Radius components running on the production cluster would defeat the purpose of GitHub mode.

## Code Changes

### 1. `pkg/kubeutil/config.go` — New `NewClientConfigFromFile` Function

A new exported function that loads a Kubernetes client configuration from an arbitrary
kubeconfig file path. This is used by the async worker to create clients targeting the
external cluster.

Also adds the `TargetKubeconfigEnvVar` constant (`RADIUS_TARGET_KUBECONFIG`) as the
canonical reference for the environment variable name.

### 2. `pkg/server/asyncworker.go` — Target Kubeconfig Support in `Run()`

The `AsyncWorker.Run()` method is modified to check for `RADIUS_TARGET_KUBECONFIG`:

- If **not set**: behavior is unchanged; output resources deploy to the local cluster.
- If **set**: a second set of Kubernetes clients is created from the file at that path.
  These "target" clients are passed to `NewApplicationModel()` and `NewDeploymentProcessor()`
  so that output resources (Deployments, Services, etc.) are deployed to the external cluster.
  The original `k8s` clients are still used for `KubeClient` (control plane operations).

### 3. `pkg/cli/github/workflows.go` — Workflow Step Generation

Both `generateRadDeploySteps()` and `generateRadAppDeleteSteps()` are updated with new steps:

#### New Steps (after "Export kubeconfig")

| Step | Description |
|------|-------------|
| **Azure Login (target cluster)** | Conditional (`vars.RADIUS_TARGET_CLUSTER_PROVIDER == 'azure'`). Uses `azure/login@v2` with OIDC. |
| **Configure AWS credentials (target cluster)** | Conditional (`vars.RADIUS_TARGET_CLUSTER_PROVIDER == 'aws'`). Uses `aws-actions/configure-aws-credentials@v4`. |
| **Fetch target cluster credentials** | Uses `az aks get-credentials` or `aws eks update-kubeconfig` based on provider. Saves to `/tmp/target-kubeconfig.yaml`. Skips if provider not configured. |

#### New Step (after "Wait for deployment engine")

| Step | Description |
|------|-------------|
| **Configure external deployment target** | Creates a K8s Secret from the target kubeconfig, patches `applications-rp` to mount it and set `RADIUS_TARGET_KUBECONFIG`, waits for rollout. Skips if no target kubeconfig file exists. |

## Configuration

Users configure the target cluster via **GitHub repository variables**:

| Variable | Required | Description |
|----------|----------|-------------|
| `RADIUS_TARGET_CLUSTER_PROVIDER` | Yes | Cloud provider: `azure` or `aws` |
| `RADIUS_TARGET_CLUSTER_NAME` | Yes | Cluster name (AKS or EKS) |
| `RADIUS_TARGET_CLUSTER_RG` | Azure only | Azure resource group containing the AKS cluster |
| `RADIUS_TARGET_CLUSTER_REGION` | AWS only | AWS region for the EKS cluster |

Cloud authentication secrets must also be configured:

| Secret | Provider | Description |
|--------|----------|-------------|
| `AZURE_CLIENT_ID` | Azure | Service principal / managed identity client ID |
| `AZURE_TENANT_ID` | Azure | Azure AD tenant ID |
| `AZURE_SUBSCRIPTION_ID` | Azure | Azure subscription ID |
| `AWS_OIDC_ROLE_ARN` | AWS | IAM role ARN for OIDC federation |
| `AWS_REGION` | AWS | AWS region |

## Backward Compatibility

When `RADIUS_TARGET_CLUSTER_PROVIDER` is **not set**:

- No cloud authentication steps run (conditional `if` in workflow YAML).
- The "Fetch target cluster credentials" step exits early.
- The "Configure external deployment target" step exits early.
- `RADIUS_TARGET_KUBECONFIG` is never set on `applications-rp`.
- All output resources deploy to the k3d cluster as before.

This is fully backward-compatible with existing GitHub mode workflows.

## Limitations (POC)

- The `applications-rp` deployment is patched via `kubectl patch` post-install rather than
  through Helm values. A production implementation should add `extraEnv`/`extraVolumeMounts`
  support to the Helm chart.
- Only AKS and EKS are supported. GKE and other providers can be added later.
- The target kubeconfig credential may expire for long-running workflows (typically 1 hour
  for cloud-issued tokens). This is acceptable for the POC scope.
- Namespace creation on the target cluster is not handled — Radius expects the target
  namespace to exist or to be created by the Kubernetes resource manager.
