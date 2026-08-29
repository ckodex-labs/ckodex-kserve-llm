# Build stage. Compile on the native builder platform and cross-compile for the
# requested target so multi-architecture builds do not run the Go compiler
# through CPU emulation.
FROM --platform=$BUILDPLATFORM golang:1.27.0-bookworm@sha256:ded31c68586d2e49e760acc2e65a884b23d032e9bbbed0ae0c55abd3fcaf4452 AS builder-base

ARG TARGETOS=linux
ARG TARGETARCH

WORKDIR /workspace
ENV GOCACHE=/workspace/.cache/go-build
ENV GOTMPDIR=/workspace/.tmp

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/
RUN mkdir -p /workspace/.cache/go-build /workspace/.tmp

FROM builder-base AS builder

# Build serially. The hosted arm64 builder has intermittently crashed in the
# native Go compiler while cross-compiling this image; limiting package
# parallelism keeps the build within the runner's memory/CPU envelope.
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -p=1 -trimpath -ldflags="-s -w" -o manager ./cmd/manager
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -p=1 -trimpath -ldflags="-s -w" -o storage-initializer ./cmd/storage-initializer

FROM builder-base AS huggingface-builder
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -p=1 -trimpath -ldflags="-s -w" -o huggingface-initializer ./cmd/huggingface-initializer

FROM gcr.io/projectsigstore/cosign:v3.1.3 AS cosign

# Resolve and unpack target-architecture wheels on the native build platform.
# The final image still executes a short target-platform import check, but does
# not run pip's resolver and installer through CPU emulation.
FROM --platform=$BUILDPLATFORM python:3.12.14-slim-trixie@sha256:7a8b475003c4fe15a2cd4e55e5cfc2f3560bdc9333d624f24cdd6d4340fd7a17 AS huggingface-python-deps
ARG TARGETARCH
COPY build/huggingface-initializer-requirements.txt /tmp/requirements.txt
RUN set -eu; \
    case "${TARGETARCH}" in \
      amd64) pip_platform=manylinux2014_x86_64 ;; \
      arm64) pip_platform=manylinux_2_28_aarch64 ;; \
      *) echo "unsupported target architecture: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    python -m pip install \
      --disable-pip-version-check \
      --no-cache-dir \
      --no-compile \
      --root-user-action=ignore \
      --require-hashes \
      --only-binary=:all: \
      --platform="${pip_platform}" \
      --target=/opt/huggingface-python \
      --requirement /tmp/requirements.txt; \
    PYTHONPATH=/opt/huggingface-python python -m pip check; \
    test -x /opt/huggingface-python/bin/hf

# Hugging Face initializer stage. Dependencies are resolved at image-build time,
# so model pods do not need PyPI access during startup.
FROM python:3.12.14-slim-trixie@sha256:7a8b475003c4fe15a2cd4e55e5cfc2f3560bdc9333d624f24cdd6d4340fd7a17 AS huggingface-initializer
# The official Python image is rebuilt on a release cadence, while Debian
# security updates can land between image rebuilds. Apply the current security
# repository updates for the runtime libraries Trivy gates before copying the
# application payload, then remove package metadata from the final image.
RUN apt-get update \
    && apt-get install -y --no-install-recommends openssl util-linux \
    && rm -rf /var/lib/apt/lists/*
COPY --from=huggingface-builder /workspace/huggingface-initializer /huggingface-initializer
COPY --from=huggingface-python-deps /opt/huggingface-python /usr/local/lib/python3.12/site-packages
COPY --from=huggingface-python-deps /opt/huggingface-python/bin/hf /usr/local/bin/hf
ENV HOME=/tmp \
    HF_HOME=/tmp/huggingface \
    HF_HUB_DISABLE_TELEMETRY=1 \
    HF_HUB_DISABLE_UPDATE_CHECK=1 \
    PYTHONDONTWRITEBYTECODE=1
USER 65532:65532
ENTRYPOINT ["/huggingface-initializer"]

# Runtime stage — distroless for minimal attack surface
FROM gcr.io/distroless/static:nonroot AS manager
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532
ENTRYPOINT ["/manager"]

# Storage Initializer stage
FROM gcr.io/distroless/static:nonroot AS storage-initializer
WORKDIR /
COPY --from=builder /workspace/storage-initializer .
COPY --from=cosign /ko-app/cosign /cosign
USER 65532:65532
ENTRYPOINT ["/storage-initializer"]
