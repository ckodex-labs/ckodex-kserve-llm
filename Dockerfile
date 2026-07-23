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
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o huggingface-initializer cmd/huggingface-initializer/main.go

FROM gcr.io/projectsigstore/cosign:v3.1.1 AS cosign

# Hugging Face initializer stage. Dependencies are resolved at image-build time,
# so model pods do not need PyPI access during startup.
FROM python:3.12.13-slim-trixie@sha256:57cd7c3a7a273101a6485ba99423ee568157882804b1124b4dd04266317710de AS huggingface-initializer
COPY build/huggingface-initializer-requirements.txt /tmp/requirements.txt
COPY --from=builder /workspace/huggingface-initializer /huggingface-initializer
RUN python -m pip install \
      --disable-pip-version-check \
      --no-cache-dir \
      --no-compile \
      --root-user-action=ignore \
      --require-hashes \
      --requirement /tmp/requirements.txt \
    && rm /tmp/requirements.txt \
    && python -m pip check \
    && hf --help >/dev/null \
    && python -c "import hf_xet"
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
