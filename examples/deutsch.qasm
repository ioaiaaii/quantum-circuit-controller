OPENQASM 3.0;
include "stdgates.inc";

// Deutsch's algorithm — decide whether a one-bit function f is constant or
// balanced with a single query of the oracle U_f.
//
//   q[0] : query qubit  (measured)
//   q[1] : ancilla, prepared in |1> so the oracle kicks its phase back
//
// Measuring q[0] resolves the question directly:
//   0  ->  f is constant
//   1  ->  f is balanced
//
// The oracle below implements the balanced f(x) = x as a single CX, so a
// faithful backend reads q[0] = 1 on every shot. On a noisy or stale-
// calibration backend the peak erodes toward 50/50 — the smallest circuit
// that still exposes calibration quality.

qubit[2] q;
bit c;

x q[1];          // ancilla |1>
h q[0];
h q[1];

cx q[0], q[1];   // oracle U_f for balanced f(x) = x

h q[0];

c = measure q[0];
