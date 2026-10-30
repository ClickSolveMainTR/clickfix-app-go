#!/usr/bin/env bash
# CF-129: adapted from takwim-app-go/scripts/backup{,-lib}.sh.
# Run from a secured Dokploy scheduled host job with matching-major pg_dump.
set -euo pipefail
umask 077
# Parse KEY=VALUE, never source a potentially executable env file. Environment wins.
backup_env="${ENV_FILE:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/.env}"
if [ -f "$backup_env" ]; then
  while IFS= read -r line || [ -n "$line" ]; do
    if [[ "$line" =~ ^([A-Za-z_][A-Za-z0-9_]*)=(.*)$ ]]; then
      key="${BASH_REMATCH[1]}"; value="${BASH_REMATCH[2]}"
      value="${value%\"}"; value="${value#\"}"
      if [ -z "${!key+x}" ]; then export "$key=$value"; fi
    fi
  done < "$backup_env"
fi
: "${DB_NAME:?required}" "${DB_USER:?required}" "${BACKUP_BUCKET:?use a separate private backup bucket}"
: "${S3_ENDPOINT:?required}" "${S3_ACCESS_KEY:?required}" "${S3_SECRET_KEY:?required}"
backup_keep="${BACKUP_KEEP_DAYS:-30}"
[[ "$backup_keep" =~ ^[1-9][0-9]*$ ]] || { echo 'BACKUP_KEEP_DAYS must be positive' >&2; exit 1; }
backup_prefix="${BACKUP_PREFIX:-clickfix/postgres}"
[[ "$backup_prefix" =~ ^[a-zA-Z0-9][a-zA-Z0-9/_-]+$ ]] || { echo 'invalid BACKUP_PREFIX' >&2; exit 1; }
for tool in pg_dump pg_restore aws; do command -v "$tool" >/dev/null; done
backup_tmp="$(mktemp -d)"
trap 'rm -rf "$backup_tmp"' EXIT
export PGHOST="${DB_HOST:-127.0.0.1}" PGPORT="${DB_PORT:-5432}" PGUSER="$DB_USER" PGPASSWORD="${DB_PASSWORD:-}" PGSSLMODE="${DB_SSLMODE:-require}"
export AWS_ACCESS_KEY_ID="$S3_ACCESS_KEY" AWS_SECRET_ACCESS_KEY="$S3_SECRET_KEY" AWS_DEFAULT_REGION="${S3_REGION:-us-east-1}" AWS_PAGER=''
backup_stamp="$(date -u +%Y%m%dT%H%M%SZ)"
pg_dump -Fc --no-password --dbname="$DB_NAME" --file="$backup_tmp/postgres.dump"
[ -s "$backup_tmp/postgres.dump" ]
pg_restore --list "$backup_tmp/postgres.dump" >/dev/null
(cd "$backup_tmp"; shasum -a 256 postgres.dump > SHA256SUMS)
backup_dest="s3://$BACKUP_BUCKET/$backup_prefix/$backup_stamp"
aws --endpoint-url "$S3_ENDPOINT" s3 cp "$backup_tmp/postgres.dump" "$backup_dest/postgres.dump" --only-show-errors
aws --endpoint-url "$S3_ENDPOINT" s3 cp "$backup_tmp/SHA256SUMS" "$backup_dest/SHA256SUMS" --only-show-errors
# Completion marker is uploaded last. Consumers ignore incomplete prefixes.
printf '{"format":1,"created_at":"%s","retention_days":%s}\n' "$backup_stamp" "$backup_keep" > "$backup_tmp/complete.json"
aws --endpoint-url "$S3_ENDPOINT" s3 cp "$backup_tmp/complete.json" "$backup_dest/complete.json" --only-show-errors
# Apply a prefix-scoped expiry rule in a dedicated backup bucket. Do not replace
# unrelated lifecycle rules: configure this once with the companion command below.
printf 'Backup complete: %s (retention policy: %s days; configure bucket lifecycle)\n' "$backup_dest" "$backup_keep"
