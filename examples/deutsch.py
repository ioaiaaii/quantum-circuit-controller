from qiskit import QuantumCircuit, QuantumRegister, ClassicalRegister, transpile

# # Construct the circuit
# qr = QuantumRegister(2, 'qr')
# cr = ClassicalRegister(1, 'cr')
# qc = QuantumCircuit(qr, cr)
# qc.x(qr[0])
# qc.barrier()
# qc.h(qr)
# qc.barrier()
# qc.cx(qr[1], qr[0])
# qc.barrier()
# qc.h(qr[1])

# # Instruct circuit simulator to save the final statevector, before measurement
# qc.save_statevector()  # pylint: disable=no-member

# # Measure only q1 and save to cbit0
# qc.measure(qr[1], cr[0])

# Deutsch's algorithm:

# Step 1: Map the problem
# Step 1: Map

# from qiskit import QuantumCircuit

qc = QuantumCircuit(2)


def twobit_function(case: int):
    """
    Generate a valid two-bit function as a `QuantumCircuit`.
    """
    if case not in [1, 2, 3, 4]:
        raise ValueError("`case` must be 1, 2, 3, or 4.")

    f = QuantumCircuit(2)
    if case in [2, 3]:
        f.cx(0, 1)
    if case in [3, 4]:
        f.x(1)
    return f


# first, convert oracle circuit (above) to a single gate for drawing purposes. otherwise, the circuit is too large to display
# blackbox = twobit_function(2).to_gate()  # you may edit the number inside "twobit_function()" to select among the four valid functions
# blackbox.label = "$U_f$"

qc.h(0)
qc.barrier()
qc.compose(twobit_function(2), inplace=True)
qc.measure_all()


# first, convert oracle circuit (above) to a single gate for drawing purposes. otherwise, the circuit is too large to display
blackbox = twobit_function(
    3
    # you may edit the number (1-4) inside "twobit_function()" to select among the four valid functions
).to_gate()
blackbox.label = "$U_f$"


qc_deutsch = QuantumCircuit(2, 1)

qc_deutsch.x(1)
qc_deutsch.h(range(2))

qc_deutsch.barrier()
qc_deutsch.compose(twobit_function(2), inplace=True)
qc_deutsch.barrier()

qc_deutsch.h(0)
qc_deutsch.measure(0, 0)
