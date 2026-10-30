@AGENTS.md

## Claude Code

Project setup checked into the repo (works the same in every app cloned from this boilerplate):

- **Skills**: `.claude/skills/*` are symlinks to `.agents/skills/*`, so Claude and other agents share one copy. Add a skill under `.agents/skills/<name>/SKILL.md`, then link it: `ln -s ../../.agents/skills/<name> .claude/skills/<name>`.
- **Hooks** (`.claude/settings.json`):
  - before an edit, `.claude/hooks/guard-edit.sh` blocks real `.env*` files, lock files / `go.sum`, sqlc output (`backend/internal/database/db`) and `frontend/src/generated/api.d.ts`; regenerate those instead.
  - after an edit, `.claude/hooks/format-edit.sh` runs `gofmt` on Go files and Prettier on frontend files.
- **MCP servers** (`.mcp.json`, approve once per machine): `shadcn` (component registry, runs in `frontend/`), `context7` (current library docs), `playwright` (browser checks).
- **Subagent**: `boilerplate-reviewer` checks a diff against the hard rules above before a PR.
- **Before saying done**: follow the `verify-changes` skill. Schema work: `db-migration` skill.
- **Shipping**: push a `cf-<numara>-<kisa-ad>` branch and open a PR to `main` using the PR template. The single required check is `ci`; merge when it is green. Every green `main` run is tagged `v0.N` (Dokploy deploys on that tag) and ships `mobile/` changes through EAS.

## Repo kuralları

Repo `ClickSolveMainTR/clickfix-app-go` (private). Eski `furkanmeclis/clickfix-app-go` artık kullanılmaz; oraya push ve PR yapılmaz.

GitHub'ın Free planında private repoda branch protection yok, bu yüzden aşağıdaki kuralları GitHub zorlamaz; herkes (insan ve ajan) kendisi uyar:

- `main`'e doğrudan push yapılmaz. Her değişiklik `cf-<numara>-<kisa-ad>` dalından PR ile girer.
- `main`'e force-push yapılmaz, `main` ve `v0.N` etiketleri silinmez veya taşınmaz.
- PR'lar sadece **squash** ile birleşir; dal birleşince silinir. Squash commit başlığı PR başlığıdır (`CF-<numara>: ...`).
- `ci` kontrolü yeşil olmadan birleştirme yapılmaz. CI kapalıysa (Actions dakikası nedeniyle) birleştirmeden önce `verify-changes` yerelde çalıştırılır ve PR'a yazılır.
- `v0.N` etiketini sadece `ci.yml` içindeki `release-tag` işi basar; elle etiket basılmaz.
- Commit mesajlarına ve PR gövdelerine Claude/AI atıf satırı (`Co-Authored-By`, "Generated with ...") eklenmez.
- Sır, gerçek `.env`, müşteri verisi commit edilmez (repo private olsa da).
- **itsplan**: move the CF issue yourself (In Progress → Done) through the itsplan MCP and comment the PR link and "Kullanılan referanslar". See the ClickFix rules at the top of `AGENTS.md`.
