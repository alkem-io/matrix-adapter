# Build Stage
FROM golang:1.25-alpine AS builder

ARG TARGETARCH

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application for target architecture
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -o matrix-adapter ./cmd/adapter

# Final Stage
FROM alpine:3.19

ARG TARGETARCH

## Add the wait script to the image for production use
## Custom deployments override CMD to use /wait for startup sequencing
RUN apk add --no-cache wget && \
    if [ "$TARGETARCH" = "arm64" ]; then \
      wget -O /wait https://github.com/ufoscout/docker-compose-wait/releases/download/2.7.3/wait_arm64; \
    else \
      wget -O /wait https://github.com/ufoscout/docker-compose-wait/releases/download/2.7.3/wait; \
    fi && \
    chmod +x /wait && \
    apk del wget

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Copy binary from builder
COPY --from=builder /app/matrix-adapter .

# Create non-root user
RUN adduser -D -g '' appuser
USER appuser

# Expose AppService port (Matrix transactions, health checks, webhooks)
EXPOSE 8280

# Run the binary
CMD ["./matrix-adapter"]
