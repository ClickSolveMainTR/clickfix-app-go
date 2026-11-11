# syntax=docker/dockerfile:1

FROM golang:1.26.11-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/create-super-admin ./cmd/create-super-admin && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/search-reindex ./cmd/search-reindex && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/backfill-plan-credits ./cmd/backfill-plan-credits && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/seed-demo ./cmd/seed-demo

FROM migrate/migrate:v4.19.1 AS migrate

FROM alpine:3.22 AS runtime

RUN apk add --no-cache ca-certificates tzdata poppler-utils \
    && adduser -D -H -u 10001 appuser

WORKDIR /app

COPY --from=migrate /usr/local/bin/migrate /usr/local/bin/migrate
COPY --from=builder /out/server /app/server
COPY --from=builder /out/worker /app/worker
# One-off operator commands (run from the container terminal).
COPY --from=builder /out/create-super-admin /app/create-super-admin
COPY --from=builder /out/search-reindex /app/search-reindex
COPY --from=builder /out/backfill-plan-credits /app/backfill-plan-credits
COPY --from=builder /out/seed-demo /app/seed-demo
COPY migrations /app/migrations
COPY deploy/docker/entrypoint.sh /app/entrypoint.sh

RUN chmod +x /app/entrypoint.sh \
    && chown -R appuser:appuser /app

USER appuser

EXPOSE 8080

ENV APP_HTTP_ADDR=:8080
ENV APP_ROLE=server
ENV AUTO_MIGRATE=true

ENTRYPOINT ["/app/entrypoint.sh"]
