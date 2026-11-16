-- Masaüstü sürümleri (CF-240). Platform tablosu: organization_id yoktur.

-- name: CreateDesktopRelease :one
INSERT INTO desktop_releases (
    app, channel, version, target, notes, signature, os_signed,
    updater_key, updater_name, updater_size, updater_sha256,
    installer_key, installer_name, installer_size, installer_sha256,
    published_by_user_id
) VALUES (
    @app, @channel, @version, @target, @notes, @signature, @os_signed,
    @updater_key, @updater_name, @updater_size, @updater_sha256,
    sqlc.narg(installer_key), sqlc.narg(installer_name), sqlc.narg(installer_size), sqlc.narg(installer_sha256),
    sqlc.narg(published_by_user_id)
)
RETURNING *;

-- name: GetDesktopReleaseByUUID :one
SELECT * FROM desktop_releases WHERE uuid = @uuid;

-- name: ListPublishedDesktopReleases :many
-- Bir uygulamanın bir hedefteki yayındaki sürümleri; en yenisi Go'da semver
-- ile seçilir (metin sıralaması 0.10.0 < 0.9.0 yapar).
SELECT * FROM desktop_releases
WHERE app = @app AND target = @target AND status = 'published'
  AND channel = ANY(@channels::text[]);

-- name: ListAllPublishedDesktopReleases :many
-- İndirme sayfası: tüm uygulama ve hedeflerin yayındaki sürümleri.
SELECT * FROM desktop_releases
WHERE status = 'published' AND channel = ANY(@channels::text[]);

-- name: ListDesktopReleases :many
SELECT * FROM desktop_releases
WHERE (sqlc.narg(app)::text IS NULL OR app = sqlc.narg(app))
  AND (sqlc.narg(channel)::text IS NULL OR channel = sqlc.narg(channel))
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
ORDER BY published_at DESC, id DESC
LIMIT @lim OFFSET @off;

-- name: CountDesktopReleases :one
SELECT count(*) FROM desktop_releases
WHERE (sqlc.narg(app)::text IS NULL OR app = sqlc.narg(app))
  AND (sqlc.narg(channel)::text IS NULL OR channel = sqlc.narg(channel))
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status));

-- name: WithdrawDesktopRelease :one
UPDATE desktop_releases
SET status = 'withdrawn', withdrawn_at = now(), withdrawn_reason = @reason,
    withdrawn_by_user_id = sqlc.narg(withdrawn_by_user_id)
WHERE uuid = @uuid AND status = 'published'
RETURNING *;
