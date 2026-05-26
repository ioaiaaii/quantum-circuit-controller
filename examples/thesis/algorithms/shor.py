"""
Shor's algorithm — quantum period-finding for N=15, a=4.

This is the author's quantum-algorithms coursework circuit (Assignment 3.2),
the same one whose ibm_brisbane / ibm_sherbrooke runs open Chapter 1, kept
verbatim.

For N=15, a=4: 4^1 = 4, 4^2 ≡ 1 (mod 15) → r = 2, and gcd(4±1, 15) = {3, 5}.
"""

from qiskit import QuantumCircuit, QuantumRegister, ClassicalRegister
from qiskit.circuit.library import QFT


def oracle(index, target_reg):
    oracle_circuit = QuantumCircuit(target_reg)

    for i in range(2 ** index):
        oracle_circuit.swap(target_reg[0], target_reg[2])

    U = oracle_circuit.to_gate()
    U.name = f"CU^{2 ** index}"
    CU = U.control(1, label='mod_exp', annotated=True)

    return CU


control_register = QuantumRegister(4, name='control')
measure = ClassicalRegister(4, name='measure')

target_register = QuantumRegister(4, name='target')

qc = QuantumCircuit(control_register, target_register, measure)

# Prepare target register to |1>, with qubit_0 has the most significant bit
qc.x(target_register[0])

# Create superposition in control register
qc.h(control_register)

for index, qubit in enumerate(control_register):
    qc.compose(
        oracle(index, target_register),
        qubits=[qubit] + list(target_register),
        inplace=True,
    )

qc.compose(
    QFT(num_qubits=control_register.size, inverse=True),
    qubits=control_register,
    inplace=True
)

qc.measure(control_register, measure)
