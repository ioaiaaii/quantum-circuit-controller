from .aer import AerAdapter
from .base import Adapter, AdapterUnavailable, JobHandle, JobStatus, TranspiledCircuit
from .ibm import IBMAdapter

# Provider → Adapter class.  Adding a new vendor is one entry here; the
# adapter file holds the implementation.  Alternative substrates (QRMI,
# CUDA-Q) are Ch9 future-work — see QCC-Design-State.md §7d (QEI direction).
_ADAPTERS: dict[str, type[Adapter]] = {
    "": AerAdapter,
    "local": AerAdapter,
    "ibm": IBMAdapter,
}


def get_adapter(provider: str, backend_name: str | None = None) -> Adapter:
    cls = _ADAPTERS.get(provider)
    if cls is None:
        raise AdapterUnavailable(f"unknown provider: {provider!r}")
    return cls(backend_name) if backend_name else cls()


__all__ = [
    "Adapter",
    "AdapterUnavailable",
    "AerAdapter",
    "IBMAdapter",
    "JobHandle",
    "JobStatus",
    "TranspiledCircuit",
    "get_adapter",
]
