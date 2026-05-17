# Build the qcc-controller binary.
# Go version matches .mise.toml and go.mod.
#
# Build context: repo root.  Invoke via:
#   docker build -f build/package/qcc-controller/Dockerfile -t qcc-controller:latest .
FROM golang:1.25.7 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY . .

# GOARCH left empty → builds for the host architecture by default
# (so `make docker-build` on Apple Silicon produces an arm64 binary).
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    go build -a -o qcc-controller ./cmd/qcc-controller

# Distroless final stage — small, non-root, no shell.
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/qcc-controller .
USER 65532:65532

ENTRYPOINT ["/qcc-controller"]
