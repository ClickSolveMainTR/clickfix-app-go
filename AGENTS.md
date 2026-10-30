# Agent entry

## ClickFix kuralları (önce bunları oku)

ClickFix, B2B müşteri destek platformudur. Bu repo nextjs-go-boilerplate üzerine kurulmuştur. Aşağıdaki kurallar boilerplate kurallarının önündedir.

### 1. İş takibi sadece itsplan'de

- Tek doğruluk kaynağı itsplan'deki **ClickFix (CF)** projesidir. GitHub Issues kapalıdır; madde, durum ve tartışma itsplan'de tutulur.
- **Durum değişikliğini herkes (insan ve AI ajanı) itsplan'de kendisi yapar:**
  - İşe başlarken maddeyi kendine ata ve **In Progress**'e al.
  - PR açınca PR bağlantısını maddeye yorum olarak yaz.
  - PR `main`'e birleşince maddeyi **Done**'a al, kapanış yorumuna "Kullanılan referanslar" listesini ekle.
  - Engel çıkarsa "Engelli" etiketini ekle ve nedenini yorumla.
- AI ajanları bunu itsplan MCP'si üzerinden yapar (`update_issue`, `add_comment`).
- PR gövdesi, tamamladığı her madde için `Fixes CF-<n>` satırı içerir (itsplan birleşince kapatır); madde başka PR'da sürecekse `Refs CF-<n>` yazılır, bu sadece bağlar.
- Dal adı: `cf-<numara>-<kisa-ad>` (örnek `cf-26-paytr`). Commit ve PR başlığı `CF-<numara>: ...` ile başlar.

### 2. Referans kullanımı zorunludur

- Bir maddeye başlamadan önce maddenin **Referans** bölümündeki ve "Referans repo" alanındaki kod okunur.
- Aynı işi yapan kod referans repolarda varsa sıfırdan yazılmaz; taşınır ve bu reponun konvansiyonlarına uyarlanır.
- Referanstan sapma nedeni PR açıklamasına ve maddeye yazılır.
- PR açıklamasında "Kullanılan referanslar" bölümü zorunludur (repo + yol + ne alındı). Şablon: `.github/pull_request_template.md`.
- Referans haritası: [docs/REFERANSLAR.md](./docs/REFERANSLAR.md).

### 3. Çok kiracılık

ClickFix çok kiracılıdır: her işletme bir organizasyondur ve tüm veri organizasyon kapsamındadır. Bu bilinçli bir ürün kararıdır; boilerplate'teki `tenant` / `organizations` modülleri ve `orgctx` kullanılır. Aşağıdaki "tenant icat etme" uyarısı bu repo için geçerli değildir.

### 4. Repo herkese açıktır

Geliştirme bitene kadar repo public'tir. Gerçek `.env` dosyası, anahtar, token, müşteri verisi veya ekran görüntüsünde kişisel veri commit edilmez. Sırlar sadece GitHub Actions secrets ve Dokploy ortam değişkenlerindedir.

---

Read this first, then the matching skill. Human docs live under `docs/`. Do not invent a `/business` shell.

Canonical env is the **repo-root** `.env`. Compose files are `compose.local.yml` (infra) and `compose.prod.yml` (full stack).

## Skills (`.agents/skills/`)

| Skill | When |
|-------|------|
| [boilerplate-first-setup](./.agents/skills/boilerplate-first-setup/SKILL.md) | Clone, `.env`, Docker names, Dokploy, `make local-dev` |
| [monorepo-playbooks](./.agents/skills/monorepo-playbooks/SKILL.md) | Cross-app work; repo map |
| [add-platform-resource](./.agents/skills/add-platform-resource/SKILL.md) | New list resource (module → RBAC → OpenAPI → UI → nav) |
| [nav-engine](./.agents/skills/nav-engine/SKILL.md) | Sidebar catalog, badges, info |
| [io-engine](./.agents/skills/io-engine/SKILL.md) | Export / import adapters |
| [bulk-engine](./.agents/skills/bulk-engine/SKILL.md) | Bulk actions, jobs, rollback |
| [search-engine](./.agents/skills/search-engine/SKILL.md) | Cmd+K palette, Meilisearch adapters |
| [storage-media](./.agents/skills/storage-media/SKILL.md) | Object storage, media URLs |
| [github-integration](./.agents/skills/github-integration/SKILL.md) | GitHub App OAuth, encrypted credentials |
| [step-up-engine](./.agents/skills/step-up-engine/SKILL.md) | Re-auth gates (password/passkey) on sensitive actions |
| [admin-date-picker](./.agents/skills/admin-date-picker/SKILL.md) | Any date field in `frontend/` |
| [db-migration](./.agents/skills/db-migration/SKILL.md) | New table/column/seed/query: migration → sqlc → OpenAPI |
| [verify-changes](./.agents/skills/verify-changes/SKILL.md) | Checks to run before commit / PR (mirrors CI) |

## Human docs

| Doc | When |
|-----|------|
| [docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md) | Shells, request flow, stores, RBAC |
| [docs/DEVELOPMENT.md](./docs/DEVELOPMENT.md) | Local / prod commands |
| [docs/EXTENDING.md](./docs/EXTENDING.md) | How to add a feature (narrative) |
| [backend/API_CONVENTIONS.md](./backend/API_CONVENTIONS.md) | Envelope, lists, meta |
| [backend/DATABASE_RULES.md](./backend/DATABASE_RULES.md) | Migrations + sqlc |
| [backend/docs/auth.md](./backend/docs/auth.md) | JWT, roles, permissions |

## Hard rules

- Browser talks to same-origin `/api/v1/*`, never the Go host.
- Media URLs are API-owned. Never assemble SeaweedFS/S3 keys in the browser.
- Never expose Meilisearch to the browser; search goes through `/v1/search`.
- New `/v1` routes update `backend/docs/openapi.yaml` in the same change.
- Permission slugs are added only via migration + `internal/platform/rbac` + `frontend/src/config/permissions.ts`.
- After OpenAPI changes: `cd frontend && pnpm api:generate` and `cd mobile && npm ci && npm run api:generate` (CI checks both are up to date).
