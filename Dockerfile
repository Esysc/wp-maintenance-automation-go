# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build binaries
RUN CGO_ENABLED=0 GOOS=linux go build -o api-server ./cmd/api/
RUN CGO_ENABLED=0 GOOS=linux go build -o web-server ./cmd/web/
RUN CGO_ENABLED=0 GOOS=linux go build -o cli ./cmd/cli/

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates curl bash restic docker-cli docker-cli-compose

WORKDIR /app

# Copy binaries from builder
COPY --from=builder /app/api-server .
COPY --from=builder /app/web-server .
COPY --from=builder /app/cli .

# Create necessary directories
RUN mkdir -p /app/data /app/logs /app/backup_artifacts /app/restore /etc/caddy/certs

# Copy web files
RUN mkdir -p /app/web/static /app/web/templates
COPY web/static /app/web/static
COPY web/templates /app/web/templates

# Copy API docs used by /api/docs endpoint
RUN mkdir -p /app/api/docs
COPY api/docs /app/api/docs

# Copy staging support files
COPY staging/ /app/staging/

# Set permissions
RUN chmod +x ./api-server ./web-server ./cli

# Expose ports
EXPOSE 8080 8081

# Default command
CMD ["./api-server"]
