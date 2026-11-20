# syntax=docker/dockerfile:1

# Keep in sync with go.mod and .github/workflows/ci.yml.
ARG GO_VERSION=1.27

# Build on the builder's own platform and cross-compile (no CPU emulation).
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-trixie AS build
ARG TARGETOS TARGETARCH
# Version shown by /healthz, e.g. 1.42 (set by CI).
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/family-api ./cmd/family-api

# Distroless: no shell, no package manager, CA certificates included (APNs
# needs them), and a non-root user.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/family-api /usr/local/bin/family-api
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/family-api"]
