# Build stage
FROM golang:1.26.5-bookworm AS builder

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

# Build
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o manager cmd/manager/main.go
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o storage-initializer cmd/storage-initializer/main.go

FROM gcr.io/projectsigstore/cosign:v3.1.1 AS cosign

# Hugging Face initializer stage. Dependencies are resolved at image-build time,
# so model pods do not need PyPI access during startup.
FROM python:3.12.12-slim-trixie@sha256:f3fa41d74a768c2fce8016b98c191ae8c1bacd8f1152870a3f9f87d350920b7c AS huggingface-initializer
ARG HUGGINGFACE_HUB_VERSION=1.24.0
ARG HF_XET_VERSION=1.5.2
ARG CLICK_VERSION=8.4.2
RUN apt-get update \
    && apt-get upgrade -y \
    && rm -rf /var/lib/apt/lists/* \
    && python -m pip install \
      --disable-pip-version-check \
      --no-cache-dir \
      --no-compile \
      --root-user-action=ignore \
      "huggingface_hub==${HUGGINGFACE_HUB_VERSION}" \
      "hf-xet==${HF_XET_VERSION}" \
      "click==${CLICK_VERSION}" \
    && python -m pip check \
    && hf --help >/dev/null \
    && python -c "import hf_xet"
ENV HOME=/tmp \
    HF_HOME=/tmp/huggingface \
    HF_HUB_DISABLE_TELEMETRY=1 \
    HF_HUB_DISABLE_UPDATE_CHECK=1 \
    PYTHONDONTWRITEBYTECODE=1
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/hf"]

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
