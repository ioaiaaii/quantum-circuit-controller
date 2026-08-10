# syntax=docker/dockerfile:1
# Build context: ./qcc-executor
#   docker build -f build/package/qcc-executor/Dockerfile -t qcc-executor:latest qcc-executor
#
# Two stages, so uv and the wheel cache stay out of the runtime.
#
# Builder and runtime must match on Python minor (the compiled extensions are
# tagged cpython-311) and Debian release (glibc). Both are bookworm/3.11.

# Runtime base, declared once so FROM and the base.* labels cannot disagree.
# Must precede the first FROM: an ARG after one belongs to that stage.
ARG BASE_IMAGE=gcr.io/distroless/python3-debian12
ARG BASE_DIGEST=sha256:2fdb05402a2cf21cf78fdb3ba4c5db167241e9e498140f5bf689d7efb773731f

# python:3.11-slim-bookworm
FROM python@sha256:d29f48a31a8b408ed19272ca1e7b10ebae13b240a27e862d3d4217c528e2e0c3 AS builder

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    UV_PROJECT_ENVIRONMENT=/app/.venv \
    UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy

# uv 0.12.2, the version that resolved uv.lock. No pip involved.
COPY --from=ghcr.io/astral-sh/uv@sha256:069a51314a7bb6031777a9273205fe1b0b19e914ef418207d1338b268df641dd /uv /usr/local/bin/uv

WORKDIR /app

# Dependencies first, so they cache independently of source changes.
COPY pyproject.toml uv.lock ./
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --no-dev --no-install-project

COPY src ./src
RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --no-dev

FROM ${BASE_IMAGE}@${BASE_DIGEST}

ARG BASE_IMAGE
ARG BASE_DIGEST
ARG REVISION

# GHCR reads source, description and licenses. revision is the only route back
# to the commit; base.digest finds images carrying a vulnerable base.
LABEL org.opencontainers.image.source="https://github.com/ioaiaaii/quantum-circuit-controller" \
      org.opencontainers.image.description="Qiskit execution service for the Quantum Circuit Controller" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.revision="${REVISION}" \
      org.opencontainers.image.base.name="${BASE_IMAGE}" \
      org.opencontainers.image.base.digest="${BASE_DIGEST}"

# The runtime has its own interpreter, so only the packages cross over.
COPY --from=builder /app/.venv/lib/python3.11/site-packages /app/site-packages
COPY --from=builder /app/src /app/src

ENV PYTHONPATH="/app/site-packages:/app/src" \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

WORKDIR /app
USER 65532:65532
EXPOSE 9000

ENTRYPOINT ["/usr/bin/python3", "-m", "qcc_executor"]
