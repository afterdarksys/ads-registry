# Build stage
FROM golang:latest AS builder

# Install build dependencies for CGO (sqlite3)
RUN apt-get update && apt-get install -y gcc g++ make libc6-dev

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the application
# CGO_ENABLED=1 is required for go-sqlite3
RUN CGO_ENABLED=1 GOOS=linux go build -o ads-registry ./cmd/ads-registry/

# Final stage
FROM debian:bookworm-slim

# Install runtime dependencies including PostgreSQL, jq, and su-exec
RUN apt-get update && \
    apt-get install -y \
    ca-certificates \
    libc6 \
    postgresql \
    postgresql-contrib \
    jq \
    gosu \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user for registry
RUN useradd -r -s /bin/false -U registry

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/ads-registry .

# Copy the configuration file and entrypoint script
COPY config.json ./
COPY entrypoint.sh ./

# Set permissions for the entrypoint script
RUN chmod +x entrypoint.sh

# Create the data directory for SQLite and blobs with proper ownership
RUN mkdir -p data/blobs certs && \
    chown -R registry:registry data certs && \
    chmod 750 data certs

# Expose the API port
EXPOSE 5005

# Volume for data persistence
VOLUME ["/app/data"]

# Entrypoint script manages starting Postgres (if enabled) and then the registry
ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["./ads-registry", "serve"]
