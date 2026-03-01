# Go build stage
# Use BUILDPLATFORM to build on native architecture (fast)
FROM --platform=$BUILDPLATFORM golang:1.25-alpine3.22 AS go-builder

# Install build dependencies
RUN apk add --no-cache git

# Cross-compilation arguments from Docker BuildKit
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build arguments
ARG VERSION=dev

# Set cross-compilation environment variables
ENV GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH}

# Handle ARM variants
RUN case "${TARGETARCH}" in \
      "arm") \
        case "${TARGETVARIANT}" in \
          "v6") export GOARM=6 ;; \
          "v7") export GOARM=7 ;; \
        esac \
      ;; \
    esac

# Build the application
RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w -X github.com/autogrr/rui/internal/buildinfo.Version=${VERSION}" \
    -o rui ./cmd/rui

# Final stage
FROM alpine:3.22

LABEL org.opencontainers.image.source="https://github.com/autogrr/rui"

# Set environment variables for config paths
ENV HOME="/config" \
    XDG_CONFIG_HOME="/config" \
    XDG_DATA_HOME="/config"

# Install runtime dependencies
RUN apk --no-cache add ca-certificates curl tzdata bash

WORKDIR /config

# Declare volume for persistent data
VOLUME /config

# Copy binary
COPY --from=go-builder /app/rui /usr/local/bin/

EXPOSE 7476

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:7476/health || exit 1

ENTRYPOINT ["/usr/local/bin/rui"]
CMD ["serve"]
