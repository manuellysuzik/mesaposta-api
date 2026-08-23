# Builder stage: compile Go binary
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache ca-certificates

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build binary
# CGO_ENABLED=0 ensures the binary is statically linked and portable across Linux distributions
RUN CGO_ENABLED=0 GOOS=linux go build \
	-ldflags="-s -w" \
	-o /server ./cmd/api

# Final stage: minimal runtime image
FROM alpine:3.19

WORKDIR /app

# Create non-root user for security
RUN addgroup -g 1001 appuser && \
	adduser -D -u 1001 -G appuser appuser

# Copy SSL certificates from builder for HTTPS support
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy binary from builder
COPY --from=builder /server /app/server

# Change ownership to appuser
RUN chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

EXPOSE 8080

# Health check: ping the liveness endpoint
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
	CMD wget --quiet --tries=1 --spider http://localhost:8080/healthz || exit 1

CMD ["/app/server"]
