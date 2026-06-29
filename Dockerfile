# Multi-stage build for production
FROM golang:1.26-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o thread-route-updater .

# Final stage - minimal image
FROM alpine:3.24

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata libcap

# Create non-root user
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/thread-route-updater .

# Change ownership to non-root user, grant NET_RAW for ICMPv6 raw socket
RUN chown -R appuser:appgroup /app && \
    setcap cap_net_raw+ep /app/thread-route-updater

# Switch to non-root user
USER appuser

# Expose port (if needed for health checks)
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD pgrep thread-route-updater || exit 1

# Run the application
CMD ["./thread-route-updater"]
