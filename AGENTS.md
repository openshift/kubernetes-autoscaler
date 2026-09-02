# AGENTS.md — openshift/kubernetes-autoscaler

This file provides AI-specific guidance for working in the OpenShift downstream fork of [kubernetes/autoscaler](https://github.com/kubernetes/autoscaler). For contribution guidelines, see [CONTRIBUTING_OPENSHIFT.md](CONTRIBUTING_OPENSHIFT.md).

## Project Overview

This repo contains two separate products for OpenShift, each with its own Go module:

- **Cluster Autoscaler (CA)** — Automatically adjusts the number of nodes in a cluster based on pending pod resource requests. Deployed and managed by [cluster-autoscaler-operator](https://github.com/openshift/cluster-autoscaler-operator).
- **Vertical Pod Autoscaler (VPA)** — Automatically adjusts container CPU and memory requests based on historical usage. Deployed and managed by [vertical-pod-autoscaler-operator](https://github.com/openshift/vertical-pod-autoscaler-operator).

Contributors typically work on one product or the other, not both simultaneously. Be aware which product you're modifying — they have separate build systems, test suites, and Go modules.

### Binaries

| Product | Binary | Source | Purpose |
|---------|--------|--------|---------|
| CA | `cluster-autoscaler` | `cluster-autoscaler/main.go` | Watches unschedulable pods, triggers node group scale-up/scale-down |
| VPA | `admission-controller` | `vertical-pod-autoscaler/pkg/admission-controller/` | Mutating webhook that applies VPA recommendations to new pods |
| VPA | `recommender` | `vertical-pod-autoscaler/pkg/recommender/` | Computes resource recommendations from metrics history |
| VPA | `updater` | `vertical-pod-autoscaler/pkg/updater/` | Applies resource recommendation updates to pods — either in-place or by evicting and recreating them |

## Repository Structure

```
cluster-autoscaler/              # CA Go module (k8s.io/autoscaler/cluster-autoscaler)
  main.go                        # CA binary entry point
  cloudprovider/
    openshift/                   # Downstream-only OpenShift cloud provider
    router/
      router_openshift.go        # Build-tag-gated import (//go:build openshift)
  apis/                          # CA API types (ProvisioningRequest, CapacityBuffer, etc.)
    */client/                    # Generated clientsets
  core/                          # Scaling logic, node group management
  estimator/                     # Resource estimation for scale-up decisions
  expander/                      # Strategy selection when multiple node groups can scale
  processors/                    # Processing pipeline hooks
  clusterstate/                  # Cluster state tracking and health checks
  config/                        # CA configuration types
  Makefile                       # CA-specific build/test targets
  hack/                          # CA-specific scripts (upstream)

vertical-pod-autoscaler/         # VPA Go module (k8s.io/autoscaler/vertical-pod-autoscaler)
  pkg/
    admission-controller/        # VPA admission webhook binary
    recommender/                 # VPA recommender binary
    updater/                     # VPA updater binary
    apis/                        # VPA CRD API types
    client/                      # Generated clientsets, informers, listers
  hack/                          # VPA-specific scripts (upstream)
  deploy/                        # Upstream VPA deployment manifests
  Dockerfile.rhel                # Downstream product image (builds all 3 VPA binaries)
  Dockerfile.openshift           # Alternative downstream image build

hack/                            # Top-level downstream build infrastructure
  verify_history.sh              # Validates UPSTREAM: commit prefix convention
  test-go.sh                     # CA unit test runner
  go-unit-tests-vpa.sh           # VPA unit test runner
  build-go.sh                    # Build script
  verify-gofmt.sh               # Format verification
  verify-govet.sh                # Vet verification
  verify-imports.sh              # Import ordering verification
  verify-upstream-commits.sh     # Commit prefix validation (alt)

images/cluster-autoscaler/
  Dockerfile.rhel                # Downstream CA product image (builds with --tags openshift)

Makefile                         # Top-level downstream Makefile
.ci-operator.yaml                # OpenShift CI build root config
```

## Upstream / Downstream Relationship

This repo tracks upstream `kubernetes/autoscaler`. The upstream tracking branch is defined in `hack/verify_history.sh`.

### Rebase strategy

When rebasing to upstream, the team usually picks a branch or tag corresponding to the CA release that goes with a given Kubernetes release. If the state of VPA at that upstream point is unstable or buggy, VPA-only fix commits are cherry-picked from upstream to bring both products to a good state for a given OpenShift release.

### What is downstream-only

- `OWNERS` — OpenShift approver list
- `.ci-operator.yaml` — OpenShift CI build root config
- `.coderabbit.yaml` — CodeRabbit configuration
- `.snyk` — Snyk configuration
- `Makefile` (top-level) — Downstream build/test/verify targets
- `cluster-autoscaler.spec` — RPM spec
- `cluster-autoscaler/cloudprovider/openshift/` — OpenShift cloud provider (downstream-only)
- `cluster-autoscaler/cloudprovider/router/router_openshift.go` — Build-tag-gated OpenShift import
- `hack/` (entire top-level directory) — Downstream build infrastructure
- `images/cluster-autoscaler/Dockerfile.rhel` — Downstream CA image
- `vertical-pod-autoscaler/Dockerfile.rhel` — Downstream VPA image
- `vertical-pod-autoscaler/Dockerfile.openshift` — Alternative downstream VPA image
- `tools/` — Downstream tooling
- `CONTRIBUTING_OPENSHIFT.md`, `AGENTS.md` — These files

### What is upstream

Everything else — the core CA and VPA source, upstream Makefiles in `cluster-autoscaler/Makefile` and `vertical-pod-autoscaler/` scripts, the `.github/` workflows (not used in our CI), upstream docs, and all other cloud providers. Avoid modifying upstream files; patches must be re-applied during every rebase cycle.

### Rebase Cycle

During a rebase:
- `UPSTREAM: <drop>:` commits are discarded
- `UPSTREAM: <carry>:` commits are re-applied
- `UPSTREAM: 1234:` commits are dropped if upstream PR 1234 is now included, otherwise re-applied

## Architecture: What Is Not Obvious

### Cloud Providers: Only clusterapi and openshift Matter

The upstream repo contains dozens of cloud providers in `cluster-autoscaler/cloudprovider/router/`, but OpenShift only supports two:

- **`clusterapi`** — The upstream Cluster API provider. OpenShift carries a patch (`allow clusterapi provider to skip paused resources`) that adds awareness of the CAPI pause annotation so paused MachineDeployments, MachineSets, and MachinePools are not reported to the autoscaler.
- **`openshift`** — A **downstream-only wrapper** around the clusterapi provider (`cluster-autoscaler/cloudprovider/openshift/`). Its purpose is to mediate the migration from Machine API to Cluster API: it detects whether the authoritative scaling resources on a given cluster are Machine API or Cluster API, and delegates accordingly. The Machine API logic lives here; Cluster API calls pass through to the clusterapi provider.

The openshift provider is compiled in via the `openshift` build tag — see `router_openshift.go`. The downstream product image is always built with `--tags openshift`. When building locally, pass `BUILD_TAGS=openshift` to include it. All other cloud providers (AWS, Azure, GCE, etc.) are upstream code and are not used in OpenShift.

### Two Go Modules, One Repo

CA and VPA are separate Go modules with independent `go.mod` and `vendor/` directories. Changes to one product do not affect the other's dependencies. Running `go mod tidy` or `go mod vendor` must be done from within the correct module directory.

### CA Build Tag System

The CA uses build tags to select which cloud provider(s) to include. The downstream build uses `--tags openshift` (sometimes `clusterapi,openshift`). The file `cluster-autoscaler/cloudprovider/router/` contains one `router_*.go` file per provider, each gated by a build tag.

### VPA Three-Binary Architecture

VPA produces three separate binaries from one module. They share API types and client code but have distinct runtime behavior. Changes to shared packages under `pkg/apis/` or `pkg/client/` affect all three.

## Common Pitfalls

1. **Add a carry prefix to every commit.** Every non-upstream commit must use `UPSTREAM: <carry>:`, `UPSTREAM: <drop>:`, or `UPSTREAM: 1234:`. CI enforces this via `hack/verify_history.sh`. Only commits merged from upstream don't require the prefix.

2. **Do not hand-edit generated files.** Files matching `zz_generated*` and clientsets in `*/client/` directories are generated. Use the appropriate `update-codegen.sh` script to regenerate.

3. **Do not use GitHub Actions.** The `.github/workflows/` directory is from upstream and is not used in our CI. Our CI runs through OpenShift CI (Prow). The CI config is in [openshift/release](https://github.com/openshift/release/tree/master/ci-operator/config/openshift/kubernetes-autoscaler).

4. **Do not modify vendor/ directly.** Run `go mod tidy && go mod vendor` from the appropriate module directory (`cluster-autoscaler/` or `vertical-pod-autoscaler/`) and commit vendor changes in a separate commit from logic changes.

5. **Build with the correct tags.** Forgetting `BUILD_TAGS=openshift` when building or testing the CA locally means the OpenShift cloud provider is excluded. The downstream product image always uses `--tags openshift`.

6. **Run tests from the right directory.** CA tests run from `cluster-autoscaler/` (e.g., `make test-unit`). VPA tests run from the repo root via `hack/go-unit-tests-vpa.sh`. Mixing these up gives confusing results.

7. **Only the `clusterapi` and `openshift` providers are relevant.** All other cloud providers in the repo are upstream code that OpenShift does not use. Do not modify them, and do not add new providers downstream — that means carrying them through every rebase. The `openshift` provider wraps `clusterapi` and exists only downstream; fixes to the clusterapi provider itself should go upstream when possible.

8. **Remember the two-module boundary.** Shared changes across CA and VPA are rare and require separate commits touching each module's dependencies independently.

9. **Do not modify upstream Makefiles.** The `cluster-autoscaler/Makefile` is upstream. The downstream build uses the top-level `Makefile` and scripts in `hack/`.

## Human-in-the-Loop Triggers

Stop and consult a human before:

- **Modifying API types** (CA `apis/` or VPA `pkg/apis/`) — API changes have compatibility implications and may require coordinated changes in the corresponding operator repo
- **Changing RBAC permissions** — Privilege changes require security review
- **Modifying the OpenShift cloud provider** (`cloudprovider/openshift/`) — This is the core downstream integration point with MachineSet-based autoscaling
- **Modifying the upstream/downstream boundary** — Any change to which files are carried vs. upstream
- **Rebase-related decisions** — Whether a carry patch is still needed, whether to drop or keep
- **Changing Dockerfiles** (`Dockerfile.rhel`, `Dockerfile.openshift`) — Build changes can affect the product payload
- **Cross-product changes** — Changes that affect both CA and VPA

## Paired Changes

These files must be updated together:

| If you change... | Also update... |
|-----------------|----------------|
| CA API types in `cluster-autoscaler/apis/` | Regenerate with `hack/update-codegen.sh` |
| VPA API types in `vertical-pod-autoscaler/pkg/apis/` | Regenerate with `vertical-pod-autoscaler/hack/update-codegen.sh` |
| `go.mod` in either module | Run `go mod vendor` in the same module directory and commit vendor separately |
| OpenShift cloud provider (`cloudprovider/openshift/`) | Update tests in the same directory |
| CA build tags or router files | Verify build with `make build BUILD_TAGS=openshift` |
| Dockerfile.rhel (CA or VPA) | Verify the binary names and paths match what the image expects |

## Further Reading

- [CONTRIBUTING_OPENSHIFT.md](CONTRIBUTING_OPENSHIFT.md) — Downstream contribution workflow, PR commands, test expectations
- [CONTRIBUTING.md](CONTRIBUTING.md) — Upstream Kubernetes autoscaler contribution guidelines
- [cluster-autoscaler/FAQ.md](cluster-autoscaler/FAQ.md) — Cluster Autoscaler FAQ (upstream)
- [vertical-pod-autoscaler/README.md](vertical-pod-autoscaler/README.md) — VPA overview and usage (upstream)
