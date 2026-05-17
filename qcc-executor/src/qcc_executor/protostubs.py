"""sys.path shim for generated gRPC stubs.

The grpcio-tools plugin emits absolute imports rooted at the proto's package
path (`from qcc.executor.v1 import executor_pb2`).  This module prepends the
proto output directory to sys.path so those imports resolve, then re-exports
the stubs.
"""

import os
import sys

_PROTO_ROOT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "proto")
if _PROTO_ROOT not in sys.path:
    sys.path.insert(0, _PROTO_ROOT)

from qcc.executor.v1 import executor_pb2, executor_pb2_grpc  # noqa: E402

__all__ = ["executor_pb2", "executor_pb2_grpc"]
