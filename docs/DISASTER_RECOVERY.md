# ClickFix backup and recovery (CF-129)

Run `scripts/backup-postgres.sh` daily from Dokploy (e.g. 01:00 UTC), with a
Postgres client matching the server major version, AWS CLI, and a private backup
bucket on an independent host/account. Root `.env` is parsed as data; environment
variables override it. Set DB_HOST/PORT/USER/PASSWORD/NAME/SSLMODE (require TLS
outside a private network), S3_ENDPOINT/REGION/ACCESS_KEY/SECRET_KEY,
BACKUP_BUCKET, BACKUP_PREFIX and BACKUP_KEEP_DAYS (default 30). Grant the backup
identity PutObject only for its prefix. Retention administration uses a separate
identity: export AWS_ACCESS_KEY_ID/SECRET_ACCESS_KEY and run
`scripts/backup-retention.py` once, and whenever retention changes. It preserves
unrelated lifecycle rules. SeaweedFS versions without lifecycle support require
the equivalent scheduled expiry; verify expiry on the deployed version before
accepting the backup policy. No pruning occurs before a successful upload.

Each run contains a custom-format dump, SHA256SUMS, and `complete.json` uploaded
last. Page if no complete backup appears for 26h; incomplete uploads are failures.
Monitor job exit status and elapsed time. Keep backup data encrypted at rest and
transported over TLS. Restrict access as production personal data.

## Restore drill (monthly and before launch)

1. Record incident time and freeze writes/Asynq consumers. Choose the latest
   **complete** prefix before the incident. Record backup age (RPO target 24h).
2. Download its three files to an encrypted incident directory with `aws
   --endpoint-url "$S3_ENDPOINT" s3 cp ...`. Run `shasum -a 256 -c SHA256SUMS`
   and `pg_restore --list postgres.dump`. Reject checksum or archive failures.
3. Create an isolated database, never restore over the live one in a drill:
   `createdb clickfix_restore_drill`; then
   `pg_restore --exit-on-error --single-transaction --no-owner --no-privileges
   --dbname=clickfix_restore_drill postgres.dump`. Review all errors.
4. Check `schema_migrations` is clean. Deploy the matching application version;
   apply newer migrations only after verifying the recorded version.
5. Restore the object bucket from its independent backup/versioned replica.
   Database dumps contain object keys, not file bytes. Replicate SeaweedFS data
   and volume metadata, or use S3 versioned replication to independent storage.
   Restore the encryption key from the secret manager: encrypted credentials
   cannot be recovered from Postgres alone. Restore versions before resuming
   attachment access. Apply completed erasure requests again after restoration.
6. Boot API on an isolated port; require `/healthz` and `/readyz` 200. Sign in,
   verify two organizations cannot access each other's contacts/attachments,
   read a conversation and attachment, inspect queue/outbox health. Keep payment,
   channel and email integrations disabled to prevent replaying side effects.
7. Record elapsed time (RTO target 4h), chosen backup, checksum, migration,
   representative row counts and attachment hashes, findings and operator.
   Destroy the drill database/files after review. A scheduled dump alone is
   not evidence that the RPO/RTO targets have been met.

## Security review

Go API adds nosniff, frame denial, no-referrer and a restrictive permissions
policy. TLS terminator owns HSTS and host allow-list; do not trust arbitrary
forwarded IP/proto headers. Frontend CSP must include the actual widget/media and
Sentry origins and be checked by the frontend owner. Channel credentials use
platform/crypto; keep the encryption key in Dokploy secrets and back it up
separately. Webhook signature verification and existing rate limits remain
required. CI `security` runs govulncheck, high-severity gosec and production pnpm
audit; the single required `ci` gate includes its result. Do not blanket-ignore
findings. Review advisories and document each narrowly scoped exception.
