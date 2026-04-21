# Production Dockerfile for otel-hive
# Multi-stage build: Go backend + React frontend → single container

# =============================================================================
# Stage 1: Build Go Backend
# =============================================================================
FROM golang:1.24-bookworm AS backend-builder

RUN apt-get update && apt-get install -y \
    git ca-certificates tzdata gcc libsqlite3-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -a -o otel-hive ./cmd/all-in-one

# =============================================================================
# Stage 2: Build React Frontend
# =============================================================================
FROM node:20-alpine AS frontend-builder

RUN npm install -g pnpm
WORKDIR /app

ARG VITE_BACKEND_URL=http://localhost:8080
ENV VITE_BACKEND_URL=${VITE_BACKEND_URL}

COPY ui/package.json ui/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY ui/ .
RUN pnpm build

# =============================================================================
# Stage 3: Production Image
# =============================================================================
FROM debian:bookworm-slim

LABEL org.opencontainers.image.source="https://github.com/storl0rd/otel-hive"
LABEL org.opencontainers.image.description="OpAMP-based OTel Collector fleet management"
LABEL org.opencontainers.image.licenses="Apache-2.0"

RUN apt-get update && apt-get install -y \
    ca-certificates tzdata curl libsqlite3-0 \
    && rm -rf /var/lib/apt/lists/*

RUN groupadd -g 1001 otel-hive && \
    useradd -u 1001 -g otel-hive -s /bin/bash -m otel-hive

WORKDIR /app
COPY --from=backend-builder /app/otel-hive .
COPY --from=frontend-builder /app/dist ./ui/dist
COPY otel-hive.yaml .
COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

RUN mkdir -p /app/data && chown -R otel-hive:otel-hive /app
USER otel-hive

EXPOSE 8080 4320

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

ENV GIN_MODE=release
ENV TZ=UTC

ENTRYPOINT ["/entrypoint.sh"]
CMD ["./otel-hive"]
