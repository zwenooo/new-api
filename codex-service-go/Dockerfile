# syntax=docker/dockerfile:1

FROM golang:1.24 AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o /app/bin/codex-service-go ./cmd/server

FROM debian:bookworm-slim AS runtime
WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/bin/codex-service-go /app/codex-service-go

ENV DATABASE_PATH=/app/db/codex.db \
    AUTH_DIR=/app/auth \
    LOG_DIR=/app/log \
    SERVER_HOST=0.0.0.0 \
    SERVER_PORT=8080

VOLUME ["/app/auth", "/app/log", "/app/db"]

EXPOSE 8080

ENTRYPOINT ["/app/codex-service-go"]
