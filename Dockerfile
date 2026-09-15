# Build stage
FROM golang:1.27.1-bookworm AS builder

WORKDIR /app

# Copy source code
COPY . .

ARG APP_VERSION=dev

# Run go mod tidy to generate go.sum based on source
RUN go mod tidy

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.Version=${APP_VERSION}" -o /app/bin/goencode ./cmd/goencode

# Final stage
FROM debian:bookworm-slim

# Install ffmpeg and fetch the latest yt-dlp release from GitHub (Debian package is very outdated)
RUN apt-get update && \
    apt-get install -y --no-install-recommends ffmpeg ca-certificates tzdata curl python3 && \
    curl -fsSL https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /usr/local/bin/yt-dlp && \
    chmod a+rx /usr/local/bin/yt-dlp && \
    yt-dlp --version && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/bin/goencode /app/goencode

# Expose port
EXPOSE 8080

# Run the binary
ENTRYPOINT ["/app/goencode"]
CMD ["--config", "/etc/goencode/config.yaml"]
