# Releasing

The branching model, the versioning policy, and what CI enforces today.
Build and test mechanics are in the [engineering guide](./engineering.md).

## Branching model

Trunk-based. Short-lived branches, one merge gate, tags cut from `main`.

```
feature branch (hours to days)
   └── PR ──[merge gate]──► main (always releasable)
                              └── tag vX.Y.Z ──► release
```

There are no long-lived release branches. Kubernetes and Prometheus
maintain `release-X.Y` branches with cherry-pick processes because they
must patch old minors for users who cannot upgrade. That machinery costs
more than it returns at this size. Add a release branch the first time a
fix must ship to an older minor, and not before.

## Versioning and support

Semantic versioning, with one project-specific rule: the `v1.0.x` line is
frozen as the artifact the thesis cites. New work targets `v1.1.0` and
later.

Three interfaces are public surface and change additively within `v1.x`:

| Interface | Compatibility today |
|---|---|
| `qcc.io/v1alpha1` CRDs | additive fields only; an incompatible change starts `v1beta1` and both are served during migration |
| `qcc.executor.v1` gRPC contract | additive; `make proto-breaking` enforces it against `main` |
| the `qcc_*` metrics specification | additive; renaming or removing a metric or label is a breaking change because it silently breaks dashboards and alerts rather than failing loudly |

Only the latest release on the current minor receives fixes, matching
[SECURITY.md](../SECURITY.md). Deprecation, when it happens, is announced
in the release notes at least one minor before removal.

Commits follow Conventional Commits (`feat:`, `fix:`, `docs:`, `test:`,
`chore:`), which is what the changelog is generated from.

## What CI enforces

Six workflows run on both push and pull request, covering lint, unit and
integration tests, e2e, protobuf compatibility, executor tests, and
documentation links. Their definitions are in `.github/workflows/`.

Two properties hold regardless of what is added later. A published tag or
artifact is never moved, edited, or deleted: consumers pin to it and the
Go module proxy caches it immutably, so mutating one breaks a reference
others rely on, and fixes go forward as a new version. An artifact is
built once and promoted by digest, so the artifact that passed a gate is
the artifact that ships.

## Deliberately not adopted

Practices that larger projects need and this one does not, listed so the
absence reads as a decision rather than an oversight.

| Practice | Why not here |
|---|---|
| Long-lived release branches with cherry-picks | no users pinned to an old minor yet |
| Release shepherd rotation and a release team | single maintainer |
| A separate release-tooling repository | one workflow file is enough |
| Release candidates and a beta channel | no external release cadence to protect yet |
| Nightly CI | research cadence, so a nightly cron would re-prove an unchanged tree most nights |
| A hard code-coverage threshold | thresholds get satisfied with vacuous tests; the trend is the useful signal |
| Versions derived automatically from commits | deliberate tagging is safer at this cadence |

## Current state

There is no image publishing, no signing, no provenance, and no release
automation: the project sits at SLSA L0. The release pipeline, the gates
it will run, and the supply-chain artifacts it will produce are tracked in
[issues](https://github.com/ioaiaaii/quantum-circuit-controller/issues).
The [implementation status](./status.md) records what ships today.
