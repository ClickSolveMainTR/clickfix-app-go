# Agent entry

## ClickFix kuralları (önce bunları oku)

ClickFix, B2B müşteri destek platformudur. Bu repo nextjs-go-boilerplate üzerine kurulmuştur. Aşağıdaki kurallar boilerplate kurallarının önündedir.

### 1. İş takibi GitHub Projects'te

- Tek doğruluk kaynağı repo issue'ları ve org panosu **[ClickFix](https://github.com/orgs/ClickSolveMainTR/projects/1)**'tir (Status: Todo → In Progress → Done). itsplan artık kullanılmaz; eski `CF-<n>` numaraları itsplan arşivindeki maddelerdir ve taşınmadı.
- Yeni iş issue olarak açılır ve panoya eklenir: `gh issue create --repo ClickSolveMainTR/clickfix-app-go --project ClickFix`.
- **Durum değişikliğini herkes (insan ve AI ajanı) kendisi yapar:**
  - İşe başlarken issue'yu kendine ata (`gh issue edit <n> --add-assignee @me`) ve panoda **In Progress**'e al.
  - PR gövdesindeki `Fixes #<n>` issue'yu PR'a bağlar; ayrıca yorum gerekmez.
  - PR `main`'e birleşince issue kapanır ve pano onu **Done**'a alır. Kapanmazsa elle kapat.
  - Engel çıkarsa `Engelli` etiketini ekle ve nedenini issue'ya yorumla.
- PR gövdesi, tamamladığı her issue için `Fixes #<n>` satırı içerir; iş başka PR'da sürecekse `Refs #<n>` yazılır, bu sadece bağlar.
- Dal adı: `gh-<issue>-<kisa-ad>` (örnek `gh-14-inbox-filtre`). Commit ve PR başlığı `GH-<issue>: ...` ile başlar. Issue'su olmayan küçük düzeltmelerde `gh-<kisa-ad>` dalı ve `GH: ...` başlığı kullanılır.

### 2. Referans kullanımı zorunludur

- Bir işe başlamadan önce issue'nun **Referans** bölümündeki ve "Referans repo" alanındaki kod okunur.
- Aynı işi yapan kod referans repolarda varsa sıfırdan yazılmaz; taşınır ve bu reponun konvansiyonlarına uyarlanır.
- Referanstan sapma nedeni PR açıklamasına ve issue'ya yazılır.
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
