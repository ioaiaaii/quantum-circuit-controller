# qcc CLI reference

The `qcc` CLI is a Kubernetes client. Every command reads or writes
Circuit and QPU resources through the API server, so anything it does you
can also do with `kubectl`. It links no quantum SDK, and its only trust
boundary is the API server.

Every command that talks to the cluster takes two common flags.
`--kubeconfig` defaults to `KUBECONFIG` and then `~/.kube/config`.
`-n/--namespace` defaults to `default` and applies to Circuits only, since
QPUs are cluster-scoped.

## qcc run

Submits a circuit and streams progress to a result card. The file
extension chooses the format: `.qasm` for OpenQASM 3, `.py` for Qiskit
Python, which the executor converts server-side.

```bash
qcc run <file> [flags]
```

| Flag | Default | Description |
|---|---|---|
| `--backend` | none | Target one QPU by Kubernetes name (`fake-brisbane`) or provider-native name (`fake_brisbane`) |
| `--provider` | none | Restrict selection to `local` or `ibm` |
| `--shots` | `1024` | Number of executions |
| `--detach` | off | Exit once the provider job is queued and let the controller keep polling |
| `--select-only` | off | Choose a backend and stop without submitting, so no QPU time is used |
| `--performance-test` | off | Submit to every available simulator under one experiment label and print a comparison table |
| `--include-hardware` | off | Include hardware QPUs in a performance test, which spends real credits |
| `--algorithm`, `--version`, `--experiment` | none | Set the `qcc.io/*` grouping labels; version and experiment require an algorithm |
| `-l`, `--label` | none | Add a label in `key=value` form, repeatable |
| `--timeout` | `30m` | Wall-clock ceiling |
| `--poll` | `500ms` | Status poll interval |

## qcc draw

Renders the circuit as ASCII through the executor, without selecting a
backend or using QPU time. The Circuit it creates is deleted afterwards
unless you pass `--keep`.

```bash
qcc draw <file> [--keep] [--timeout 60s] [--poll 250ms]
```

## qcc schedule

Transpiles and schedules the circuit against a backend's Target, then
prints a per-qubit timeline in wall-clock units showing gate starts,
durations, and the critical path. The backend must report instruction
durations, so use a `fake_*` snapshot or real hardware; generic Aer fails
with `SchedulingUnsupported`.

```bash
qcc schedule <file> --backend fake-brisbane [--provider local] [--keep] [--timeout 120s]
```

## qcc get

Inspects Circuits and QPUs. Singular and plural forms both work.

```bash
qcc get circuits [--algorithm shor] [--version v2] [--experiment thesis]
qcc get circuit <circuit-name>
qcc get circuit <circuit-name> --qasm
qcc get circuit <circuit-name> --draw
qcc get circuit <circuit-name> --schedule
qcc get qpus
qcc get qpu <qpu-name>
```

`--qasm` prints the converted OpenQASM 3, `--draw` the ASCII drawing, and
`--schedule` the rendered timeline. The three are mutually exclusive, and
the label filters apply to Circuit lists only.

## qcc version

Prints the version, which is injected at build time and falls back to the
version control revision.

## Choosing a command

| To do this | Use |
|---|---|
| Check a circuit's structure | `qcc draw` |
| Find out whether any backend accepts it, without running | `qcc run --select-only` |
| See the microsecond-scale timeline on a backend | `qcc schedule --backend ...` |
| Run on the ideal simulator | `qcc run --backend aer-statevector` |
| Compare across every simulator | `qcc run --performance-test` |
| Run on IBM hardware | `qcc run --backend ibm-... --detach`, then `qcc get circuit` |
| Inspect a past run or a backend | `qcc get` |

## What's next

The [tutorial](./getting-started.md) walks these commands in order against
a local cluster. The [API reference](./api.md) documents the Circuit and
QPU fields the CLI writes, and the [demonstration](./demonstration.md)
shows the output of each command on a real workload.
