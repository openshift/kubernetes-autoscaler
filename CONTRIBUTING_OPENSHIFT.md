# Contributing to Cluster Autoscaler / Vertical Pod Autoscaler (OpenShift Downstream)

This document covers contribution guidelines specific to the OpenShift downstream fork of [kubernetes/autoscaler](https://github.com/kubernetes/autoscaler). For upstream contribution guidelines, see [CONTRIBUTING.md](CONTRIBUTING.md).

This repo contains two separate products: Cluster Autoscaler (CA) and Vertical Pod Autoscaler (VPA). Each has its own operator in a separate repo. You will typically be working on one or the other, not both at the same time.

## Related Resources

| Resource | Link |
|----------|------|
| Upstream repo | [kubernetes/autoscaler](https://github.com/kubernetes/autoscaler) |
| CA operator repo | [openshift/cluster-autoscaler-operator](https://github.com/openshift/cluster-autoscaler-operator) |
| VPA operator repo | [openshift/vertical-pod-autoscaler-operator](https://github.com/openshift/vertical-pod-autoscaler-operator) |
| CI configuration | [openshift/release/.../openshift-kubernetes-autoscaler/](https://github.com/openshift/release/tree/master/ci-operator/config/openshift/kubernetes-autoscaler) |
| AI guidance | [AGENTS.md](AGENTS.md) |
| OpenShift CA docs | [Cluster Autoscaler](https://docs.openshift.com/container-platform/latest/machine_management/applying-autoscaling.html) |
| OpenShift VPA docs | [Vertical Pod Autoscaler](https://docs.openshift.com/container-platform/latest/nodes/pods/nodes-pods-vertical-autoscaler.html) |

## Review and Approval Policy

Every change in every pull request must be understood and approved by two humans. This can be the PR author and a reviewer, or — if the author used an AI tool and does not fully understand the contents of the PR — two human reviewers.

**Exception:** PRs authored by deterministic automation tools that are part of our CI and related systems (whose code has been reviewed by the OpenShift engineering org) can be merged with a single human review.

Every change should be closely scrutinized for bugs. Our software is complex with many interdependencies. Review changes from multiple angles:

- **Product architecture**: Does this fit the intended design of OpenShift and Cluster Autoscaler or the VPA?
- **Security**: Are there new attack surfaces, credential handling issues, or privilege escalations?
- **Thread safety**: Both CA and VPA use concurrent goroutines — are shared resources properly synchronized?
- **Regressions**: Could this break existing scaling behavior?
- **Effects on other components**: How does this impact the CA operator, VPA operator, or the workloads they manage?

## Upstream Commit Convention

This is a downstream fork. All non-upstream commits must use one of the following prefixes to ensure changes are not lost during the next upstream rebase:

- `UPSTREAM: <carry>: ` -- A change that should be kept (carried) indefinitely, or as long as it makes sense to do so
- `UPSTREAM: <drop>: ` -- A change that should be discarded during the next rebase cycle
- `UPSTREAM: 1234: ` -- A change carried until the rebase includes upstream PR #1234

Examples:

```
UPSTREAM: <carry>: Add OpenShift cloud provider for Cluster Autoscaler
UPSTREAM: <drop>: Pin Go version to 1.25 until 1.26 builder is available
UPSTREAM: 5678: Backport fix for node group cache race condition
```

Commit prefixes are validated by CI via `hack/verify_history.sh`.

## Upstream-First Policy

New feature work should be directed to the [upstream autoscaler project](https://github.com/kubernetes/autoscaler). Downstream-only features are discouraged due to the ongoing cost of maintaining them through each rebase cycle. If a downstream-only change is necessary, use the `UPSTREAM: <carry>:` prefix and include a comment in the PR explaining why it cannot go upstream.

## CA Cloud Providers: Only clusterapi and openshift

The upstream repo contains dozens of Cluster Autoscaler cloud provider implementations, but OpenShift only uses two:

- **`clusterapi`** — The upstream Cluster API provider. OpenShift carries a patch that adds awareness of the CAPI pause annotation so paused scalable resources are not reported to the autoscaler. Fixes to this provider should go upstream when possible.
- **`openshift`** — A downstream-only wrapper around `clusterapi` in `cluster-autoscaler/cloudprovider/openshift/`. It mediates the migration from Machine API to Cluster API by detecting which resource type is authoritative on a given cluster and delegating accordingly.

The openshift provider is compiled in via the `openshift` build tag. The downstream product image is always built with `--tags openshift`. When building or testing locally, pass `BUILD_TAGS=openshift` to include it. All other cloud providers (AWS, Azure, GCE, etc.) are upstream code — do not modify them downstream.

## PR Title Convention

PR titles should be prefixed with a Jira ticket reference:

```
AUTOSCALE-123: Fix the whatsit in the thingamajig
OCPBUGS-456: Correct nil pointer in VPA recommender
NO-JIRA: Update Go module dependencies
```

The Jira prefix goes in the **PR title**. The upstream commit prefix goes in the **commit message**.

## PR Workflow

This repo uses [OpenShift CI (Prow)](https://docs.ci.openshift.org/) for continuous integration. GitHub Actions workflows in this repo are from upstream and are **not used** for our CI. PRs are automatically merged once all required tests pass and the correct labels are present.

### Required labels for merge

- `lgtm` — Added by a reviewer via the `/lgtm` command. Any developer from the OpenShift org can add this after reviewing the PR.
- `approved` — Added by an approver listed in the [OWNERS](OWNERS) file via the `/approve` command.
- `verified` — Added by anyone in the OpenShift org, but typically by the PR author.

### Useful commands

Comment these on the PR:

| Command | Effect |
|---------|--------|
| `/lgtm` | Add the `lgtm` label after reviewing. In repos using [LGTM mode](https://docs.ci.openshift.org/how-tos/creating-a-pipeline/#the-pipeline-required-command), this also triggers E2E and other second-stage tests. |
| `/lgtm cancel` | Remove the `lgtm` label |
| `/approve` | Add the `approved` label (OWNERS approvers only) |
| `/pipeline required` | Manually trigger all required second-stage tests (e.g., E2Es) without waiting for `/lgtm` |
| `/retest` | Re-run all failed required tests |
| `/retest-required` | Re-run only the failed required tests |
| `/test <test-name>` | Run a specific test, e.g. `/test e2e-aws-operator` |
| `/hold` | Prevent the PR from being merged |
| `/hold cancel` | Remove the hold and allow merging |
| `/verified` | Mark the PR as verified |
| `/cherry-pick release-4.18` | Create a cherry-pick PR to a release branch |

### LGTM mode and E2E tests

Repos enrolled in [LGTM mode](https://docs.ci.openshift.org/how-tos/creating-a-pipeline/#the-pipeline-required-command) defer second-stage tests (such as E2Es) until the `/lgtm` label is applied. This avoids wasting CI resources on PRs that haven't been reviewed yet. If you need to run E2Es before getting `/lgtm` (e.g., to validate before requesting review), use `/pipeline required`.

### Preventing premature merges

- Add the `WIP:` prefix to the PR title (e.g., `WIP: AUTOSCALE-123: Work in progress`). Prow adds the `do-not-merge/work-in-progress` label automatically.
- Use `/hold` to temporarily block merging while awaiting additional review or testing.

## Test Expectations

PRs should include tests to verify correctness and prevent future regressions.

### Cluster Autoscaler

- **Unit tests**: `make test-unit` (from `cluster-autoscaler/`)
- **Controller integration tests**: `make test-controllers` (uses envtest)
- **E2E tests**: Run in OpenShift CI against real clusters (e2e-aws, e2e-aws-operator, e2e-azure-operator, e2e-gcp-operator, e2e-hypershift)
- **Build verification**: `make build BUILD_TAGS=clusterapi,openshift`

### Vertical Pod Autoscaler

- **Unit tests**: `hack/go-unit-tests-vpa.sh` (from repo root)
- **E2E tests**: Run in OpenShift CI

## Verified Label

Use `/verified` to indicate changes have been verified. Examples:

```
/verified
/verified by e2e-aws-operator
/verified by unit tests
/verified by E2Es
/verified later @joelsmith
```

## Generated Code

The following files are generated and should never be hand-edited:

| File(s) | Location | Generator | Regenerate with |
|---------|----------|-----------|-----------------|
| `zz_generated.deepcopy.go` | CA `apis/` | controller-gen | `hack/update-codegen.sh` (CA) |
| `zz_generated.deepcopy.go` | VPA `pkg/apis/` | deepcopy-gen | `vertical-pod-autoscaler/hack/update-codegen.sh` |
| Clientsets, listers, informers | VPA `pkg/client/` | code-generator | `vertical-pod-autoscaler/hack/update-codegen.sh` |
| Clientsets | CA `apis/*/client/` | code-generator | `hack/update-codegen.sh` (CA) |

After modifying API types, regenerate and commit the results in the same PR.

## Development Quick Reference

### Cluster Autoscaler (from `cluster-autoscaler/`)

| Task | Command |
|------|---------|
| Build CA binary | `make build` |
| Build with OpenShift provider | `make build BUILD_TAGS=openshift` |
| Run unit tests | `make test-unit` |
| Run controller integration tests | `make test-controllers` |
| Run benchmarks | `make benchmark` |
| Format code | `make format` |

### Vertical Pod Autoscaler (from repo root)

| Task | Command |
|------|---------|
| Run VPA unit tests | `hack/go-unit-tests-vpa.sh` |
| Build VPA binaries | `cd vertical-pod-autoscaler && go build ./pkg/admission-controller && go build ./pkg/updater && go build ./pkg/recommender` |

### Top-Level (from repo root)

| Task | Command |
|------|---------|
| Build all | `make build` or `make all` |
| Verify formatting/vet/imports | `make verify` |
| Verify commit prefixes | `make verify-commits` |
| Run CA unit tests | `make test-unit` |

## Pre-Submit Checklist

Before requesting review:

1. `make build` — Verify the code compiles
2. `make test-unit` — Run unit tests (CA)
3. `hack/go-unit-tests-vpa.sh` — Run unit tests (if touching VPA)
4. `make verify` — Run formatting, vet, and import checks
5. Review your diff for secrets, credentials, or debug code
6. Address any [CodeRabbit](https://coderabbit.ai/) review feedback — as a courtesy to the human reviewer who follows. Responding with an explanation of why you're not acting on a suggestion is fine; the goal is to resolve straightforward issues so human reviewers can focus on the substantive aspects.

## Code Style

- Run `gofmt -s` before committing (or `make verify` which checks it)
- Follow Go conventions for error strings: lowercase, no trailing punctuation, wrap with `fmt.Errorf("context: %w", err)`
- Use structured logging with klog: constant messages, key-value pairs in lowerCamelCase
- Import ordering: stdlib, external packages, internal packages (separated by blank lines)
