# QCC — Evidence Capture Run-book (2026-05-24)

> Commands executed during the 2026-05-24 evidence-capture session.
> Cluster: `kind-qcc-dev`. QCC at session HEAD (no git tag was cut for the pack).
> Capture tool: `charmbracelet/freeze` (terminal → PNG). The `freeze`
> invocations are intentionally **not** shown below — what's shown is
> the *actual command being captured*, ready to paste into the thesis
> above the figure that displays its output.

## How to use this in the thesis

For each figure in Ch7, the recommended LaTeX pattern is:

```latex
\begin{listing}[!htpb]
  \caption{Listing the registered QPUs.}
  \label{lst:qpu-get-all}
  \begin{minted}{bash}
kubectl get qpu
  \end{minted}
\end{listing}

\begin{figure}[!htpb]
  \centering
  \includegraphics[width=0.85\linewidth]{Figures/ch7/qpu_get_all.png}
  \caption[Registered QPUs in the demonstration cluster]{The kit's
  deployed QPU registry — three live IBM Heron r2 backends plus
  nine simulators including the user-added \texttt{fake-fez}.}
  \label{fig:qpu-get-all}
\end{figure}
```

The command goes in the listing, the screenshot in the figure. Both are referenced from the surrounding prose. The Phase-by-Phase sections below give the actual commands in execution order; the Command → Figure index at the end is the flat lookup for paste-and-cite work.

---

## Phase 0 — Cluster setup

Added the Heron r2 simulator to the QPU registry (the canonical manifest under `config/samples/qpu/`):

```bash
kubectl apply -f config/samples/qpu/fake-fez.yaml
```

No figure — setup-only step. The result of registration is visible in Phase 1.

## Phase 1 — QPU registry inspection

Listed all registered QPUs:

```bash
kubectl get qpu
```

→ Figure: `docs/qpu_get_all.png`

Inspected the freshly-registered Heron r2 simulator's status (probe-populated fields visible — basis gates, qubits, coherence, error medians, calibration timestamp):

```bash
kubectl get qpu fake-fez -o yaml
```

→ Figure: `docs/qpu_get_fale-fez.png`

Showed the minimal source manifest (four lines of spec — the controller's probe populates the rest):

```bash
cat config/samples/qpu/fake-fez.yaml
```

→ Figure: `docs/qpu_manifest_fake-fez.png`

## Phase 2 — Circuit demonstration

Rendered Shor's circuit through the controller's `mode=draw` path:

```bash
qcc draw examples/thesis/algorithms/shor.py
```

→ Figure: `docs/circuit_draw_shor.png`

## Phase 3 — Cross-substrate evaluation (the R5 evidence primitive)

Submitted the same Shor source across all candidate simulator QPUs under a shared experiment label; the platform produced one comparison table across substrates:

```bash
qcc run examples/thesis/algorithms/shor.py \
  --performance-test \
  --algorithm shor \
  --version v1 \
  --experiment thesis-perf-test
```

→ Figure: `docs/circuit_run_shor_perf-test.png`

Listed Circuits filtered by experiment label to confirm the campaign produced one Circuit per substrate:

```bash
kubectl get circuits -l qcc.io/experiment=thesis
```

→ Figure: `docs/circuit_get_filtered_1.png`

Inspected a single Circuit's full YAML to show the K8s-native resource shape:

```bash
kubectl get circuits shor-lpdb7 -o yaml
```

→ Figure: `docs/circuit_get_shor_lpdb7.png`

## Phase 4 — Real-hardware Shor submissions (vanilla opt-3, three Heron r2 backends)

Submitted Shor with `--detach` so the CLI exits as soon as the provider job is queued (the K8s-native "submit and walk away" pattern; the controller continues polling in the background):

```bash
qcc run examples/thesis/algorithms/shor.py \
  --algorithm shor --experiment thesis --version v1 \
  --backend ibm-fez --detach
```
→ produced Circuit `shor-wffxg` (job `d89dgpqs46sc73fafp3g`)

```bash
qcc run examples/thesis/algorithms/shor.py \
  --algorithm shor --experiment thesis --version v1 \
  --backend ibm-kingston --detach
```
→ produced Circuit `shor-2vv42` (job `d89dinp789is73935r0g` — the R4 anchor)

```bash
qcc run examples/thesis/algorithms/shor.py \
  --algorithm shor --experiment thesis --version v1 \
  --backend ibm-marrakesh --detach
```
→ produced Circuit `shor-wr9ds` (job `d89dir1789is73935r4g`)

The CLI output of one such detached submission was captured:

```bash
qcc run examples/thesis/algorithms/shor.py \
  --algorithm shor --experiment thesis --version v1 \
  --backend ibm-fez --detach
```

→ Figure: `docs/circuit_run_shor_fez_v1.png`

## Phase 5 — Per-Circuit inspection (real-hardware results)

Each backend's vanilla-opt-3 Shor result captured via the rich card:

```bash
qcc get circuit shor-wffxg
```
→ Figure: `docs/circuit_get_shor_fez_v1.png` (ibm-fez, depth 2062, ~10.8% correct mass)

```bash
qcc get circuits shor-2vv42
```
→ Figure: `docs/circuit_detached_v1.png` (ibm-kingston, depth 2048, captured `phase: Running`)

```bash
qcc get circuits shor-wr9ds
```
→ Figure: `docs/circuit_get_shor-wr9ds.png` (ibm-marrakesh, depth 2048, ~8.4% correct mass)

Listed all Circuits after the v1 sweep (pre-Tier-2):

```bash
qcc get circuits
```

→ Figure: `docs/circuit_all_after_run_v1.png`

## Phase 6 — Tier-2 evaluation on `ibm_kingston`

Authored two manifests differing only in their Tier-2 transpile block, applied each:

```bash
kubectl create -f examples/thesis/circuits/shor-v2.yaml
```
→ produced `shor-tuned-kingston-nb7q9` (vanilla opt-3, the Tier-2 baseline)

```bash
kubectl create -f examples/thesis/circuits/shor-v3.yaml
```
→ produced `shor-tuned-kingston-62qzb` (`scheduling_method: alap`)

Captured each result card:

```bash
qcc get circuits shor-tuned-kingston-2nxgj
```
→ Figure: `docs/circuit_get_shor-tuned.png` — an *earlier* Tier-2 attempt captured mid-run; transpile shape `depth 1387 / 1926 gates / 416 2Q` is the leanest of the three (the Sabre-variance illustration).

```bash
qcc get circuits shor-tuned-kingston-nb7q9
```
→ Figure: `docs/circuit_get_shor_tuned_v2.png` — v2 (vanilla opt-3) baseline: depth 1523, 2054 gates, 440 2Q, correct mass 26.1%.

```bash
qcc get circuits shor-tuned-kingston-62qzb
```
→ Figure: `docs/circuit_get_shor_tuned_v3.png` — v3 (`scheduling_method: alap`): depth 1524, **2548 gates** (+494 delay instructions), 442 2Q, correct mass 26.9%. alap is visibly observable in `totalGates`; outcome is within sampling noise.

## Phase 7 — Final state

Listed all Circuits at session end (19 total, all labels populated, all `providerJobId`s present):

```bash
qcc get circuits
```

→ Figure: `docs/final_get_all_circuits.png`

---

## Command → Figure index (flat lookup)

Quick paste-ready table. Use the LaTeX pattern at the top of this file.

| Figure file | Command shown |
|---|---|
| `qpu_get_all.png` | `kubectl get qpu` |
| `qpu_get_fale-fez.png` | `kubectl get qpu fake-fez -o yaml` |
| `qpu_manifest_fake-fez.png` | `cat config/samples/qpu/fake-fez.yaml` |
| `circuit_draw_shor.png` | `qcc draw examples/thesis/algorithms/shor.py` |
| `circuit_get_filtered_1.png` | `kubectl get circuits -l qcc.io/experiment=thesis` |
| `circuit_get_shor_lpdb7.png` | `kubectl get circuits shor-lpdb7 -o yaml` |
| `circuit_run_shor_perf-test.png` | `qcc run examples/thesis/algorithms/shor.py --performance-test --algorithm shor --version v1 --experiment thesis-perf-test` |
| `circuit_run_shor_fez_v1.png` | `qcc run examples/thesis/algorithms/shor.py --algorithm shor --experiment thesis --version v1 --backend ibm-fez --detach` |
| `circuit_get_shor_fez_v1.png` | `qcc get circuit shor-wffxg` |
| `circuit_detached_v1.png` | `qcc get circuits shor-2vv42` |
| `circuit_get_shor-wr9ds.png` | `qcc get circuits shor-wr9ds` |
| `circuit_all_after_run_v1.png` | `qcc get circuits` (pre-Tier-2 snapshot) |
| `circuit_get_shor-tuned.png` | `qcc get circuits shor-tuned-kingston-2nxgj` |
| `circuit_get_shor_tuned_v2.png` | `qcc get circuits shor-tuned-kingston-nb7q9` |
| `circuit_get_shor_tuned_v3.png` | `qcc get circuits shor-tuned-kingston-62qzb` |
| `final_get_all_circuits.png` | `qcc get circuits` (session end) |

### Grafana figures (no CLI command)

These are dashboard screenshots, not terminal captures. Cite them as figures only — no preceding listing block. For each, name the dashboard and the relevant template-variable filters in the caption.

| Figure file | Dashboard + filter |
|---|---|
| `grafana_qpu_availability.png` | `qcc-qpu-dashboard`, S+U sections, no QPU filter (all-fleet view) |
| `grafana_qpu_coherence.png` | `qcc-qpu-dashboard`, E section (T1/T2 + family comparison panel) |
| `grafana_qcc_qpu_metrics.png` | Grafana Explore, Prometheus, `qcc_qpu` metric-browser autocomplete |
| `grafana_qcc_circuit_metrics.png` | Grafana Explore, Prometheus, `qcc_circuit` metric-browser autocomplete |
| `grafana_circuit_get_shor_tuned_v2.png` | `qcc-circuit-dashboard`, `circuit=shor-tuned-kingston-nb7q9` |
| `grafana_circuit_get_shor_tuned_v3.png` | `qcc-circuit-dashboard`, `circuit=shor-tuned-kingston-62qzb` |
| `grafana_circut_1.png` | `qcc-circuit-dashboard`, `algorithm=shor`, `experiment=thesis`, `circuit=shor-lpdb7` |
| `grafana_shor_fake-fez.png` | `qcc-circuit-dashboard`, `experiment=thesis-perf-test`, `circuit=shor-m4n5v` |
| `grafana_shor-fake-marrakesh.png` | `qcc-circuit-dashboard`, `experiment=thesis-perf-test`, `circuit=shor-qpp8j` |
| `grafana_shor_fake-kyoto.png` | `qcc-circuit-dashboard`, `experiment=thesis-perf-test`, `circuit=shor-s8626` |

### External UI screenshots (no QCC command)

| Figure file | Source UI + context |
|---|---|
| `cli_detach_submission_kingston.png` | Terminal — QCC CLI output of the ibm-kingston `--detach` submission (the same command shown in Phase 4 above; this is the captured terminal scrollback rather than a `freeze`-rendered card) |
| `ibm_console_job_tag_kingston.png` | IBM Quantum Console — Details panel for job `d89dinp789is73935r0g` (the QCC-stamped tag `qcc.circuit.uid:32afb048-…` is the R4 forward-direction evidence) |
| `grafana_circuit_provider_job_link.png` | Grafana — `qcc-circuit-dashboard` Circuit identity panel for `shor-2vv42`, hovering on the `provider_job_id` cell to expose the "Open on IBM Quantum" data link (the R4 reverse-direction evidence) |
| `job_d89dinp789is73935r0g_results_measure_0.png` | IBM Quantum Console — Results tab for the same job, histogram view of measurement outcomes |

For the R4 hero figure in Ch7, the two IBM Console / Grafana screenshots (2:06 PM + 2:08 PM) pair as a single composite **without** preceding command listings — the prose introduces them as "the bidirectional UI linkage."

---

## Notes for the thesis prose

1. **Don't show `freeze` invocations.** They're the capture mechanism, not the system under demonstration. The base commands above are what the reader cares about.

2. **Match the prompt convention.** The cards in `docs/` were rendered with `freeze -t github`, which produces a GitHub-style terminal aesthetic. If you want a consistent prompt convention in the thesis (e.g., `❯` or `$`), regenerate the captures with a custom `-x` wrapper. The base commands stay the same.

3. **Detached submissions need a follow-up.** The `--detach` commands in Phase 4 exit when the job queues. The actual outcome arrives later; the cards in Phase 5 are what readers see for the *result*. Thesis prose should typically pair the `--detach` listing with a brief note that result inspection happens via the follow-up `qcc get circuit <name>` shown in the next section.

4. **The `qcc get circuit` vs `qcc get circuits` spelling.** Both work — the CLI accepts singular and plural. The captures in this pack used both. Pick one convention for thesis listings (the plural form is more conventional for kubectl-style commands; the singular reads more naturally in prose like "inspect a single circuit").

5. **For Grafana figures, screenshot the panel + state.** Always include the template-variable bar in the screenshot when relevant (algorithm/experiment/circuit selectors), so readers can see which Circuit the dashboard is filtered to.
