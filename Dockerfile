# Build stage
FROM golang:1.25-bookworm AS builder

ARG TARGETOS=linux
ARG TARGETARCH

WORKDIR /workspace

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/

# Build
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -a -ldflags="-s -w" -o manager cmd/manager/main.go
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -a -ldflags="-s -w" -o storage-initializer cmd/storage-initializer/main.go

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
USER 65532:65532
ENTRYPOINT ["/storage-initializer"]
