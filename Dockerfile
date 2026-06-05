# Build stage
FROM golang:1.26.4-bookworm AS builder

WORKDIR /app

# Copy source code
COPY . .

# Run go mod tidy to generate go.sum based on source
RUN go mod tidy

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app/bin/goencode ./cmd/goencode

# Final stage
FROM debian:bookworm-slim

# Install ffmpeg, ca-certificates, and tzdata for timezone support
RUN apt-get update && \
    apt-get install -y --no-install-recommends ffmpeg ca-certificates tzdata && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/bin/goencode /app/goencode

# Expose port
EXPOSE 8080

# Run the binary
ENTRYPOINT ["/app/goencode"]
CMD ["--config", "/etc/goencode/config.yaml"]
