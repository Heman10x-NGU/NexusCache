# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Copy everything
COPY . .

# Update dependencies and build the binary
RUN go mod tidy && go build -o /app/cache-server .

# Runtime stage
FROM alpine:3.19

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/cache-server /app/cache-server

# Make executable
RUN chmod +x /app/cache-server

# Expose ports: 9999 for HTTP API, 8888 for gRPC, 9100 for metrics
EXPOSE 9999 8888 9100

# Default command
ENTRYPOINT ["/app/cache-server"]
