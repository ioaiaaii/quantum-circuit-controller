# Development

Building, running, and testing QCC from a local checkout. For the system
design, see [architecture.md](./architecture.md), and for code conventions
and internals, see [engineering.md](./engineering.md).

## Required tools

See the [requirements](./getting-started.md#requirements), then:

```bash
make tools-install
make tools-check
```

If you are on OSX with colima container runtime, it is suggested to start with rosetta, due to mutli-arch image builds.

```shell
colima stop && colima start --vm-type vz --vz-rosetta 
```

## Building

```bash
make build          # controller into dist/qcc-controller
make qcc-build      # CLI into dist/qcc, version from git describe
make images-build   # qcc-controller and qcc-executor container images
```

The workflows below build what they need. These targets exist for
building one artifact on its own. `make help` lists every target grouped
by section.

## Local development

Each branch gets its own environment: a kind cluster named
`qcc-dev-<branch>`, matching the image tags, hosting the observability
stack (kube-prometheus-stack, OpenTelemetry Collector).

```bash
make dev-up   # cluster + observability + CRDs
```

Then run QCC in one of two ways. Use one at a time: two controllers
reconciling the same resources fight each other.

**Out-of-cluster**, the fastest cycle: no image builds, and you can
attach a debugger to either process. Two terminals. The controller uses
your kubeconfig and dials the executor on `127.0.0.1:9000`. Re-run
after a change:

```bash
uv run --project qcc-executor python -m qcc_executor   # terminal 1
make run                                               # terminal 2
```

**In-cluster**, the real images. Re-run after a change. The target
rebuilds, reloads, and rolls the pods:

```bash
make dist-up
```

With QCC running, register the simulator QPUs (once per cluster) and
run a circuit:

```bash
kubectl apply -k config/samples/qpu/local/
make qcc-build
./dist/qcc get qpu
./dist/qcc run examples/bell-state.qasm
```

`make platform-status` shows what is running, and `make dev-down` deletes
the environment. Grafana:
`kubectl port-forward -n monitoring svc/kps-grafana 3000:80`
(admin/admin).

## Testing

| Suite | Command | Covers |
|---|---|---|
| Go unit and envtest | `make test` | reconcilers against a real kube-apiserver with a fake executor |
| Python unit | `make executor-test` | adapters, qiskit_io, in-process gRPC round-trips with a real AerAdapter |
| End to end | `make test-e2e` | both images on a throwaway kind cluster, independent of the dev cluster |

Before a PR, run the same two stages the workflows run. Stage one is
cluster-free and takes a couple of minutes. Stage two is the system
suite on a throwaway kind cluster and takes several more:

```bash
make ci         # stage 1: toolchain check, every lint, every unit and integration test
make test-e2e   # stage 2: the system suite
```

## Before opening a PR

Run both stages above. If you changed the API or the gRPC contract, regenerate and commit the
output. Never edit generated files by hand:

```bash
make manifests generate   # api/v1alpha1/ -> CRDs, RBAC, DeepCopy
make generate-all         # proto/ -> Go and Python stubs
make proto-breaking       # contract changes must stay backward compatible
```

## Updating the kubebuilder scaffold

The root Makefile, `config/`, and the project layout are kubebuilder
output. When a new kubebuilder release lands, `alpha update` replays our
changes on top of the new scaffold with a three way merge and leaves the
result on a review branch:

```bash
GOTOOLCHAIN=auto kubebuilder alpha update \
  --from-branch main \
  --restore-path .github/workflows \
  --restore-path README.md \
  --restore-path hack/boilerplate.go.txt
```

`GOTOOLCHAIN=auto` lets the new scaffold pull a newer Go than the one we
pin. The restore paths are files the scaffold also generates but that
this project owns, so the merge keeps our version.

Afterwards resolve any conflict markers, run `make ci` and
`make test-e2e` on the review branch, and merge it through a normal PR.
