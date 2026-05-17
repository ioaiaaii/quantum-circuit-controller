"""Shared pytest fixtures."""

from concurrent import futures

import grpc
import pytest

from qcc_executor.protostubs import executor_pb2_grpc
from qcc_executor.servicer import ExecutorServicer


@pytest.fixture()
def executor_channel():
    """A live gRPC channel to an in-process ExecutorServicer."""
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=2))
    executor_pb2_grpc.add_ExecutorServicer_to_server(ExecutorServicer(), server)
    port = server.add_insecure_port("127.0.0.1:0")
    server.start()
    channel = grpc.insecure_channel(f"127.0.0.1:{port}")
    yield channel
    channel.close()
    server.stop(0).wait()
