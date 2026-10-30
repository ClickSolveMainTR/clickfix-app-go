---
name: storage-media
description: >-
  Object storage and media URL rules for this boilerplate: SeaweedFS/S3 keys,
  API-owned public URLs, platform storage explorer, shares and versions.
  Use when adding uploads, logos, downloads, or the storage browser.
---

# Storage & media

**Hard rule:** the browser never builds SeaweedFS/S3 URLs from object keys. Only API-returned `public_url` / `logo_url` / stream paths.

## Packages

| Piece | Path |
|-------|------|
| Driver | `backend/internal/platform/storage` |
| Key helpers | `backend/internal/platform/storage/keys.go` |
| Explorer module | `backend/internal/modules/storage` |
| Frontend explorer | `frontend/src/features/storage` |
| URL guard | `frontend/src/lib/media/urls.ts` → `assertServiceMediaURL` |

## Keys

Use helpers; do not invent prefixes:

- Branding logo: `app/branding/logo.{ext}`
- Export artifact: `exports/{job_uuid}/export.{ext}`
- Import source: `imports/{job_uuid}/source.{ext}`

Object **bytes** live in SeaweedFS/S3. **Metadata** (name, size, versions, shares) stays in Postgres.

Env: `STORAGE_DRIVER` (`s3`), `S3_*`, `SEAWEEDFS_JWT_WRITE_KEY` / `SEAWEEDFS_JWT_READ_KEY`. The bucket is private; objects reach browsers only through API stream endpoints.

## Frontend

```ts
import { assertServiceMediaURL } from "@/lib/media/urls";

const src = assertServiceMediaURL(apiObject.public_url);
```

- Relative `/...` and absolute `http(s)://...` returned by the API are allowed.
- Local file preview may use `URL.createObjectURL`; revoke after use.
- Do not concatenate any bucket URL or bucket + key in React.

## Explorer API

Permission: `platform.storage.read` / `platform.storage.write`.

Mounted from `internal/modules/storage/routes.go`:

- List / get / usage / download / preview / versions / activity / shares / links
- Write: folders, uploads (+ session/complete), copy, move, rename, delete, restore, purge

`GET /v1/platform/storage/meta` is registered. This resource is **not** io/bulk-capable.

## Organization attachments (`modules/attachments`)

Files that belong to an organization (channel messages, tickets, agent replies) use the `attachments` table, not the platform explorer. Object keys: `orgs/{org_uuid}/attachments/{uuid}/original` (+ `thumb.jpg`), built by `platform/storage/keys.go`.

- Routes `/v1/tenant/attachments/*` need `X-Organization` + `tenant.attachments.read|write`. Every query filters by `organization_id`; another org's file is a 404.
- Link a file to a record with `owner_type` (`conversation`, `message`, `ticket`; add new types in `model` + `usecase.ValidOwnerType`) and `owner_id`. Owner modules call `usecase.Service.Upload` / `List` directly.
- Limits come from `usecase.LimitsProvider` (default: `ATTACHMENT_MAX_FILE_BYTES`, `ATTACHMENT_ORG_QUOTA_BYTES`); the plan/entitlement module plugs in per-plan limits with `WithLimits`. The quota is checked under a per-org advisory lock.
- The real media type is sniffed from the bytes; executables, HTML and SVG are rejected. `usecase.Scanner` is the virus-scan hook (`NoopScanner` today; an error fails the upload closed).
- Browsers get bytes only from the API: `download` (needs the org header) or a short-lived `signed-url` (`/v1/public/attachments/{token}`, HMAC, usable in `<img src>`). Never expose object keys.

## Logo / letterhead

Settings (`platform.settings.write`) stores letterhead fields. Logo upload goes through storage; settings responses expose `logo_url` from the API. PDF/XLSX export reads logo bytes server-side.
