"""Executor gRPC server entry + lifecycle.

Run as `qcc-executor` (console script) or `python -m qcc_executor`.
"""

from __future__ import annotations

import logging
import os
import signal
from concurrent import futures
from importlib import metadata

import grpc

from qcc_executor.protostubs import executor_pb2_grpc
from qcc_executor.servicer import ExecutorServicer

LOG = logging.getLogger(__name__)


def _version() -> str:
    v = os.environ.get("QCC_EXECUTOR_VERSION")
    if v:
        return v
    try:
        return metadata.version("qcc-executor")
    except metadata.PackageNotFoundError:
        return "dev"


def serve(addr: str, workers: int) -> grpc.Server:
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=workers))
    executor_pb2_grpc.add_ExecutorServicer_to_server(ExecutorServicer(), server)
    server.add_insecure_port(addr)
    server.start()
    LOG.info("Executor listening on %s (workers=%d)", addr, workers)
    return server


def main() -> None:
    logging.basicConfig(
        level=os.environ.get("QCC_EXECUTOR_LOG_LEVEL", "INFO"),
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
    )
    LOG.info(
        "Starting qcc-executor %s (commit %s)",
        _version(),
        os.environ.get("QCC_EXECUTOR_REVISION", "unknown"),
    )
    addr = os.environ.get("QCC_EXECUTOR_ADDR", "0.0.0.0:9000")
    workers = int(os.environ.get("QCC_EXECUTOR_WORKERS", "8"))
    server = serve(addr=addr, workers=workers)

    def _handle(_signum, _frame):
        LOG.info("Shutdown signal received")
        server.stop(grace=5).wait()

    signal.signal(signal.SIGTERM, _handle)
    signal.signal(signal.SIGINT, _handle)

    server.wait_for_termination()


if __name__ == "__main__":
    main()
