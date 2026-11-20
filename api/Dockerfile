# syntax=docker/dockerfile:1

# Keep in sync with go.mod, web/package.json and .github/workflows/ci.yml.
ARG GO_VERSION=1.27
ARG NODE_VERSION=24

# --- Build the website (static files, platform-independent) -------------------
FROM --platform=$BUILDPLATFORM node:${NODE_VERSION}-trixie-slim AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --include=dev --no-fund --no-audit
COPY web/ ./
# Outputs to /src/internal/webui/dist (see web/vite.config.ts), embedded below.
RUN npm run build

# Build on the builder's own platform and cross-compile (no CPU emulation).
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-trixie AS build
ARG TARGETOS TARGETARCH
# Version shown by /healthz, e.g. 1.42 (set by CI).
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/webui/dist ./internal/webui/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/family-api ./cmd/family-api

# Distroless: no shell, no package manager, CA certificates included (APNs
# needs them), and a non-root user.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/family-api /usr/local/bin/family-api
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/family-api"]
