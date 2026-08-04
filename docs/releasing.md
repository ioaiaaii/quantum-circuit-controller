# Releasing

How work moves from a branch to a published, verifiable release: the
branching model, the gate at each stage, what the supply-chain artifacts
are for, and what to do when a release goes wrong. Build and test
mechanics are in the [engineering guide](./engineering.md).

## Principles

Everything below follows from five properties the process holds.

Immutability comes first. A published tag or artifact is never moved,
edited, or deleted. Consumers pin to it, the Go module proxy caches it,
and a signature attests to it, so mutating any of those breaks a reference
others rely on. Fixes go forward as a new version.

An artifact is built once and promoted by digest, so the artifact that
passed the gate is the artifact that ships. Rebuilding for a later stage
produces a different artifact from the one that was tested, which voids
every test result upstream of it.

Releasing takes one command. The manual surface is a single signed tag.
Every additional manual step is toil that gets skipped under pressure and
becomes the reason a release is wrong.

Every artifact traces back to a commit. During an incident the question is
"where did this binary come from, and what was in it". Provenance and an
SBOM answer that without archaeology.

The failure path is defined in advance. A release process without a
documented recovery procedure still has one; it is improvised under
pressure instead. The procedure is in
[when a release goes wrong](#when-a-release-goes-wrong).

## Branching model

Trunk-based. Short-lived branches, one merge gate, tags cut from `main`.

```
feature branch (hours to days)
   └── PR ──[merge gate]──► main (always releasable)
                              └── tag vX.Y.Z ──► release
```

`main` is always releasable: if it is green, it can be tagged.

There are no long-lived release branches. Kubernetes and Prometheus
maintain `release-X.Y` branches with cherry-pick processes because they
must patch old minors for users who cannot upgrade; cert-manager runs
[a separate repository just for release tooling](https://cert-manager.io/docs/contributing/release-process/).
That machinery costs more than it returns at this size. Add a release
branch the first time a fix must ship to an older minor, and not before.

## Versioning and support

Semantic versioning, with one project-specific rule: the `v1.0.x` line is
frozen as the artifact the thesis cites. New work targets `v1.1.0`
and later.

Three interfaces are public surface and change additively within `v1.x`:

| Interface | Compatibility today |
|---|---|
| `qcc.io/v1alpha1` CRDs | additive fields only; an incompatible change starts `v1beta1` and both are served during migration |
| `qcc.executor.v1` gRPC contract | additive; `make proto-breaking` enforces it against `main` |
| the `qcc_*` metrics specification | additive; renaming or removing a metric or label is a breaking change because it silently breaks dashboards and alerts rather than failing loudly |

Only the latest release on the current minor receives fixes, matching
[SECURITY.md](../SECURITY.md). Deprecation, when it happens, is
announced in the release notes at least one minor before removal, and
deprecated fields keep working for that period.

Commits follow Conventional Commits (`feat:`, `fix:`, `docs:`, `test:`,
`chore:`), which is what the changelog is generated from.

## Gates

Each stage runs the cheapest checks that can catch what that stage is
responsible for.

| Stage | Catches | Runs | Blocking |
|---|---|---|---|
| Pre-commit (local) | hygiene, before it is public | format, trailing whitespace, secret scan, commit-message lint | yes, locally |
| Pull request | defects in the change itself | lint (Go, Python), unit tests with `-race`, integration (envtest), build, generated-code drift, proto breaking-change, docs links, dependency and SAST scan | **yes, merge gate** |
| Push to `main` | defects only visible in a real cluster | full e2e on kind that submits a real Circuit, image build tagged `main`, coverage trend | no, reports |
| Periodic | drift in the world, not the code | vulnerability rescan of published artifacts, external link check, Kubernetes version matrix | no, reports |
| Tag `v*` | anything the gate missed, plus release integrity | the full gate, then SBOM, signing, provenance, publish, changelog | yes |

Only deterministic checks block a pull request. A flaky check in the
merge gate trains everyone to press re-run, which devalues every other
check beside it. Anything timing-dependent or cluster-dependent runs
after merge.

The generated-code drift check earns its place here more than any other.
CRDs, RBAC, DeepCopy methods, and both sets of protobuf stubs are committed, so
they can diverge silently from their sources. The gate regenerates and
fails if the tree is dirty.

The tag re-runs everything. The pull-request run proved a merge
candidate; the tag is a different commit tree and gets its own proof.

## Periodic checks on GitHub

Nightly suits codebases with enough daily churn that a day of drift is
meaningful. This repository has research cadence, long quiet periods
punctuated by bursts, so a nightly cron would spend most of its runs
re-proving an unchanged tree and would teach you to ignore its
notifications.

What actually needs periodic execution is the small set of checks whose
input is *the outside world* rather than the code: a CVE published
against a dependency already shipped, a documentation URL that rotted, a
new Kubernetes minor.

GitHub offers four mechanisms, and picking the right one matters:

| Mechanism | Runs | Use it for | Caveat |
|---|---|---|---|
| Dependabot alerts and security updates | continuously, when an advisory publishes | dependency CVEs | native service, not Actions; no cron to maintain, no minutes consumed |
| Renovate or Dependabot version updates | its own schedule (weekly is right here) | dependency and pinned-tool bumps as PRs | PRs go through the normal gate, so bumps are proven before merge |
| CodeQL default setup | on push and on its own schedule | static security analysis | configured in repository settings, not a workflow file |
| Actions `schedule` (cron) | when you say | vulnerability rescan of *published images*, external link check, Kubernetes version matrix | see the trap below |

One property of `schedule` applies directly to this repository. GitHub
[automatically disables scheduled workflows after 60 days without commit
activity](https://docs.github.com/en/actions/how-tos/manage-workflow-runs/disable-and-enable-workflows),
and only commits reset the timer: releases, tags, issues, and merged pull
requests do not count. The disable also applies to the whole workflow
file, so a workflow carrying both `schedule` and `workflow_dispatch` loses
its manual trigger with it. A thesis artifact that goes quiet after
submission is exactly the profile this hits.

Applied here, that means:

1. Prefer the native services (Dependabot alerts, CodeQL) over cron.
   They do not rot when the repository goes quiet.
2. Keep genuinely scheduled jobs in their own workflow file, so the
   auto-disable cannot take a manual trigger down with it.
3. Use weekly rather than nightly, and pair every scheduled workflow with
   `workflow_dispatch` so it can be run on demand while investigating.

## Release runbook

Everything after step 1 is automated by the tag-triggered workflow.

1. **Cut the tag** from a green `main`:
   `git tag -s v1.1.0 && git push origin v1.1.0`. Signed, so the tag
   itself carries an identity.
2. **Re-verify on the tagged commit**: the full gate, from scratch.
3. **Build once**: multi-architecture images for the controller and the
   executor; `qcc` binaries for linux and darwin on amd64 and arm64.
   Version injected at build time through ldflags.
4. **Attest**: SBOM per artifact, keyless signatures, build provenance.
   See [what these are for](#supply-chain-artifacts-and-what-each-answers).
5. **Publish by digest**: images to GHCR, the Helm chart to the OCI
   registry, binaries and the generated `install.yaml` to the GitHub
   release, changelog generated from the commit range.
6. **Verify what shipped, not what was built**: from a clean machine,
   pull the image *by digest*, install the chart, register a QPU, run a
   Circuit, confirm it reaches `Succeeded`. A release that was never
   installed from its published form has not been tested, only built.
7. **Record**: confirm the archival DOI was minted and the release notes
   name the supported upgrade path.

Steps 3 through 5 must be **idempotent**. A registry timeout halfway
through publishing is a normal event; re-running the workflow on the same
tag has to converge rather than produce a second, differently-built
artifact.

## Supply-chain artifacts, and what each answers

Each of these artifacts exists to answer a specific question about a
released binary.

An _SBOM_, or software bill of materials, is a machine-readable inventory of
every component inside an artifact: dependency names, versions, licenses.
Generated with a scanner such as syft, published alongside the artifact.
It answers *"a CVE just dropped for library X; am I affected, and in
which released versions?"* Without one, the answer requires rebuilding
old releases and inspecting them by hand. This matters more here than for
a typical Go project, because the executor image carries the whole Qiskit
scientific-Python stack.

_Provenance_, also called a build attestation, is a signed statement describing how the
artifact was produced: which source commit, which build system, which
workflow, which inputs. It answers whether a binary really came from this
repository at that commit rather than from someone's laptop. This is the core
of [SLSA](https://slsa.dev), which grades it in levels: **L1** provenance
exists, **L2** a hosted build service signs it, **L3** the
provenance is non-forgeable because the build runs in an isolated,
auditable environment. GitHub's official generator reaches **L3** with no
self-hosted infrastructure, which is why the target is L3 rather than
something aspirational.

A _keyless signature_ is a cryptographic signature over the
artifact, so a consumer can verify it was published by this project and
not substituted in transit or in a compromised registry. *Keyless* means
there is no long-lived private key to store, rotate, or leak: the signing
identity is the CI workflow's own OIDC identity, certified for a few
minutes and recorded in a public transparency log. It answers *"is this
image the one the project published?"* Verification checks the identity
of the workflow that signed it, so a stolen registry credential is not
enough to forge a release.

A _digest_ is not a tag. A tag is a mutable pointer, while a digest
(`sha256:...`) is the content itself. Publishing and deploying by digest
is what makes "build once, promote" real. It answers *"is the image
running in production byte-identical to the one that passed the gate?"*

The changelog and the signed git tag are the human-readable and
cryptographically attributable record of what changed and who cut it.

Together these turn a published release into a provable one: what is
inside it, where it came from, and that nobody altered it in between.

## When a release goes wrong

The recovery rule is **roll forward, never unpublish**, and it is not a
preference: deletion is largely impossible and always harmful.

- **A pushed Git tag is effectively permanent.** Once anyone fetches the
  module, `proxy.golang.org` caches it immutably. Deleting the tag
  upstream does not remove it from the proxy, and it breaks every
  consumer who already resolved it.
- **A published image digest is permanent** for anyone who pulled it.
- **Deleting a release breaks reproducibility** for the thesis citation,
  which points at a specific version.

The procedure:

1. **Do not delete or move the tag.** Never force-push a tag.
2. **Publish a patch immediately.** `v1.1.1` with the fix is the
   remediation, and the affected version stays visible with its notes
   amended to point at it.
3. **For Go consumers, add a `retract` directive** to `go.mod` naming the
   bad version. This is Go's native mechanism: the version remains
   fetchable for reproducibility, but `go get` stops selecting it and
   reports the reason.
4. **Mark the GitHub release as deprecated** in its notes, linking to the
   fixed version, and if the failure is security-relevant, publish a
   security advisory so Dependabot notifies consumers automatically.
5. **Fix the gate, not just the code.** A bad release that passed every
   gate means a gate is missing. Add the check that would have caught it
   before shipping the next version, in the same patch series.

Partial publishes are handled by idempotency rather than cleanup:
re-running the tag workflow republishes the same digests, so a failure
between "images pushed" and "chart published" is resolved by re-running,
not by hand-repairing the registry.

## Deliberately not adopted

Practices that larger projects need and this one does not, listed so the
absence reads as a decision rather than an oversight.

| Practice | Why not here |
|---|---|
| Long-lived release branches with cherry-picks | no users pinned to an old minor yet |
| Release shepherd rotation and a release team | single maintainer |
| A separate release-tooling repository | one workflow file is enough |
| Release candidates and a beta channel | no external release cadence to protect yet |
| Nightly CI | research cadence; weekly plus native services covers the real drift, see [periodic checks](#periodic-checks-on-github) |
| A hard code-coverage threshold | thresholds get satisfied with vacuous tests; the trend is the useful signal |
| Versions derived automatically from commits | deliberate tagging is safer at this cadence, and the tag is the one manual step worth keeping |

## Current state

The gates above are the target, not the present. Today the repository has
six workflows covering lint, unit and integration tests, e2e, protobuf,
and documentation, all triggered on both push and pull request. There is
no image publishing, no signing, no provenance, and no release
automation: the project sits at SLSA L0. Progress toward the target is
tracked in
[issues](https://github.com/ioaiaaii/quantum-circuit-controller/issues),
and the [implementation status](./status.md) records the present state.
