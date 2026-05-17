"""
Shor's algorithm — quantum period-finding for N=15, a=7.

Shor's algorithm factors a composite integer N by reducing the problem to
finding the period r of the modular-exponentiation function

    f(x) = a^x mod N    for some a coprime to N

If r is even and a^(r/2) is not ≡ -1 (mod N), then

    gcd(a^(r/2) ± 1, N)

reveals two non-trivial factors of N.  The quantum subroutine that QCC
runs is the period-finding circuit; the classical post-processing
(GCD, continued-fraction expansion) happens outside the circuit and is
omitted here.

For N=15, a=7:

    7^1 = 7,  7^2 = 49 ≡ 4,  7^3 ≡ 13,  7^4 ≡ 1   (mod 15)

so r = 4.  The post-processing then yields:

    gcd(7^2 − 1, 15) = gcd(48, 15) = 3
    gcd(7^2 + 1, 15) = gcd(50, 15) = 5

Layout
------
  - 4 counting qubits (top register, measured)         → r read out as a peak
  - 4 work qubits initialised to |1⟩                    → modular-multiplication
  - 4 controlled modular multiplications by 7^(2^k)     → period encoding
  - inverse QFT on the counting register                 → period extraction
  - measure counting register

The controlled modular multiplication for N=15 uses the textbook
swap-network decomposition that is valid only for this specific (a, N)
pair — sufficient for a thesis-scale demonstration on an Aer simulator,
not a general modular-arithmetic block.

Refs
----
  Shor 1994, "Algorithms for quantum computation"
  Nielsen & Chuang §5.3
  Qiskit Textbook, "Shor's Algorithm"
"""

from qiskit import QuantumCircuit
from qiskit.circuit.library import QFT

# Parameters.  The (a, N) pair determines the modular-multiplication unitary
# below; only a ∈ {2, 4, 7, 8, 11, 13} (the units mod 15) are supported here.
N = 15
A = 7
N_COUNT = 4  # counting-register width — period r < 2^N_COUNT


def controlled_mult_mod15(a: int, power: int) -> "Gate":
    """Return a controlled-U^(power) gate where U|y⟩ = |a·y mod 15⟩.

    Built from the swap-network + bit-flip decomposition that exploits
    the order-4 cycles in the multiplicative group (Z/15Z)*.  Valid for
    N=15 only.
    """
    if a not in {2, 4, 7, 8, 11, 13}:
        raise ValueError(f"a={a} is not coprime to N=15")
    work = QuantumCircuit(4)
    for _ in range(power):
        if a in {2, 13}:
            work.swap(0, 1)
            work.swap(1, 2)
            work.swap(2, 3)
        if a in {7, 8}:
            work.swap(2, 3)
            work.swap(1, 2)
            work.swap(0, 1)
        if a in {4, 11}:
            work.swap(1, 3)
            work.swap(0, 2)
        if a in {7, 11, 13}:
            for q in range(4):
                work.x(q)
    gate = work.to_gate()
    gate.name = f"{a}^{power} mod 15"
    return gate.control()


circuit = QuantumCircuit(N_COUNT + 4, N_COUNT, name=f"shor_N{N}_a{A}")

# 1. Counting register in superposition.
for q in range(N_COUNT):
    circuit.h(q)

# 2. Work register initialised to |1⟩.
circuit.x(N_COUNT)

# 3. Controlled modular multiplications by a^(2^k).
for k in range(N_COUNT):
    circuit.append(
        controlled_mult_mod15(A, 2**k),
        [k] + [i + N_COUNT for i in range(4)],
    )

# 4. Inverse QFT on the counting register reveals the period.
circuit.append(QFT(N_COUNT, inverse=True, do_swaps=True), range(N_COUNT))

# 5. Measure the counting register.  Expected peaks at 0, 4, 8, 12 → r = 4.
circuit.measure(range(N_COUNT), range(N_COUNT))
