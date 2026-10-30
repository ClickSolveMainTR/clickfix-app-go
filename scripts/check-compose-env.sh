#!/usr/bin/env bash
# Every environment variable the Go backend reads must be passed through the
# backend environment block of compose.prod.yml; otherwise a value set in
# Dokploy never reaches the container. Run from the repo root.
set -euo pipefail

# Read only in local development (or derived from another variable in prod).
# DEMO_PASSWORD is given inline to the one-off seed-demo command (CI / E2E) and
# must never live in the Dokploy environment.
allow='^(STORAGE_LOCAL_PATH|SMTP_FROM|APP_HTTP_ADDR|REDIS_DB|DEMO_PASSWORD)$'

read_vars=$(find backend -name '*.go' ! -name '*_test.go' -print0 |
  xargs -0 grep -hoE '(os\.Getenv|os\.LookupEnv|price|get(Env|Int|Int64|Bool|Duration|Float|CSV|List)[A-Za-z]*)\("[A-Z][A-Z0-9_]+"' |
  sed -E 's/.*"([A-Z0-9_]+)"/\1/' | sort -u)

block=$(awk '/^x-backend-environment:/{on=1;next} on&&/^[^ #]/{exit} on' compose.prod.yml)
missing=0
for v in $read_vars; do
  [[ $v =~ $allow || $v == *_ ]] && continue  # *_ = dynamic prefix, checked below
  if ! grep -qE "^  $v:" <<<"$block"; then
    echo "missing in compose.prod.yml x-backend-environment: $v"
    missing=1
  fi
done
# Variables built from a prefix at runtime (payment providers, AI task routes) are
# listed explicitly in .env.example and must reach compose too.
for v in $(grep -oE '^(PAYMENTS|AI)_[A-Z0-9_]+' .env.example); do
  grep -qE "^  $v:" <<<"$block" || { echo "missing in compose.prod.yml x-backend-environment: $v"; missing=1; }
done
exit $missing
