---
name: db-migration
description: >-
  Add or change the database schema: sequential golang-migrate files, sqlc
  queries and regenerated code, repository wiring. Use for any new table,
  column, index, seed row, permission slug or query in backend/.
---

# Database migration

Rules live in [backend/DATABASE_RULES.md](../../../backend/DATABASE_RULES.md). This is the checklist.

## 1. Create the files

```bash
cd backend && make migrate-create NAME=<snake_case_description>
```

- Sequential names only (`000025_x.up.sql` / `.down.sql`). Never timestamps.
- When porting from a fork (otopoly-go, comcode-app-go, …) **renumber** to the next free number here: `ls backend/migrations | tail -2`.
- Both `up` and `down`. A lossy `down` must say so in a comment (`-- Data-loss note: …`).

## 2. Write the SQL

- `BIGSERIAL id`, public `uuid UUID NOT NULL DEFAULT gen_random_uuid()` when exposed.
- `created_at` / `updated_at timestamptz`, plus the `set_updated_at()` trigger.
- Foreign keys required; `numeric` for money; `text` + `CHECK` for enums.
- Changing a CHECK constraint: `DROP CONSTRAINT` + `ADD CONSTRAINT` with the full list (see `000021`).
- Seed rows: `INSERT … ON CONFLICT … DO NOTHING` so re-runs are safe.
- New permission slug: migration insert **and** `internal/platform/rbac` **and** `frontend/src/config/permissions.ts` (hard rule in AGENTS.md).

## Billing features (plan entitlements)

Published plan versions are immutable (`trg_billing_entitlement_guard`). A migration that adds a `billing_features` row must NOT insert into `billing_plan_entitlements` for existing versions (that broke deploys in 000194, CF-124). Set the catalog default on the feature instead; subscribers on older versions resolve it automatically and new drafts are prefilled with it:

```sql
INSERT INTO billing_features(key,kind,label_tr,label_en,default_bool)            -- boolean feature
VALUES('module.x','boolean','X modülü','X module',true) ON CONFLICT(key) DO NOTHING;
INSERT INTO billing_features(key,kind,label_tr,label_en,unit,default_limit)      -- limit feature
VALUES('x.count','limit','X sayısı','X count','x',5) ON CONFLICT(key) DO NOTHING;
```

## 3. Queries + codegen

- Add queries to `internal/database/queries/<area>.sql` (`-- name: X :one|:many|:exec|:execrows`).
- `make sqlc` — never edit `internal/database/db/*.go` by hand (the edit hook blocks it).
- Merge conflicts in generated files: take either side, then `make sqlc` again.

## 4. Verify

```bash
cd backend && make migrate-up && go build ./... && go test ./...
```

Round-trip on a throwaway database before shipping:

```bash
docker run -d --rm --name mig-test -e POSTGRES_PASSWORD=t -p 55439:5432 pgvector/pgvector:0.8.7-pg18-trixie
migrate -path migrations -database "postgres://postgres:t@127.0.0.1:55439/postgres?sslmode=disable" up
migrate -path migrations -database "postgres://postgres:t@127.0.0.1:55439/postgres?sslmode=disable" down 1
migrate -path migrations -database "postgres://postgres:t@127.0.0.1:55439/postgres?sslmode=disable" up
docker stop mig-test
```

## 5. API surface

New `/v1` route → `backend/docs/openapi.yaml` in the same change, then `cd frontend && pnpm api:generate`.
