# syntax=docker/dockerfile:1
# Build context: repo root.
#   docker build -f build/package/qcc-controller/Dockerfile -t qcc-controller:latest .

# Runtime base, declared once so FROM and the base.* labels cannot disagree.
# Must precede the first FROM: an ARG after one belongs to that stage.
ARG BASE_IMAGE=gcr.io/distroless/static:nonroot
ARG BASE_DIGEST=sha256:f7f8f729987ad0fdf6b05eeeae94b26e6a0f613bdf46feea7fc40f7bd72953e6

# golang:1.25.7, matching go.mod
# BUILDPLATFORM: build natively, cross-compile via GOOS/GOARCH.
FROM --platform=$BUILDPLATFORM golang@sha256:2005724102f45917a63e9d092fc0e4ea56ea575048ce147caad5f5f61502c365 AS builder

ARG TARGETOS
ARG TARGETARCH
ARG LD_FLAGS

WORKDIR /workspace

# Cache mounts keep the caches out of the layers.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && go mod verify

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    go build -a -ldflags="${LD_FLAGS}" -o qcc-controller ./cmd/qcc-controller

FROM ${BASE_IMAGE}@${BASE_DIGEST}

ARG BASE_IMAGE
ARG BASE_DIGEST
ARG REVISION

# GHCR reads source, description and licenses. revision is the only route back
# to the commit; base.digest finds images carrying a vulnerable base.
LABEL org.opencontainers.image.title="qcc-controller" \
      org.opencontainers.image.url="https://github.com/ioaiaaii/quantum-circuit-controller" \
      org.opencontainers.image.documentation="https://github.com/ioaiaaii/quantum-circuit-controller/tree/main/docs" \
      org.opencontainers.image.source="https://github.com/ioaiaaii/quantum-circuit-controller" \
      org.opencontainers.image.description="Kubernetes controller for the Quantum Circuit Controller" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.revision="${REVISION}" \
      org.opencontainers.image.base.name="${BASE_IMAGE}" \
      org.opencontainers.image.base.digest="${BASE_DIGEST}"

WORKDIR /
COPY --from=builder /workspace/qcc-controller .
USER 65532:65532

# Probes only. Metrics default to off and push over OTLP.
EXPOSE 8081

ENTRYPOINT ["/qcc-controller"]
