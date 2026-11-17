-- Masaüstü sürümleri (CF-240): ClickFix Masaüstü (agent) ve ClickFix Destek
-- (support) paketleri. Platform kapsamlıdır; organization_id yoktur. Paket
-- baytları nesne depolamada durur (desktop/releases/...), burada yalnız
-- anahtar, boyut, SHA-256 ve Tauri updater imzası tutulur. Bir satır bir
-- uygulamanın bir kanaldaki bir sürümünün bir hedefidir (ör. darwin-aarch64).
CREATE TABLE desktop_releases (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    app TEXT NOT NULL CHECK (app IN ('agent', 'support')),
    channel TEXT NOT NULL CHECK (channel IN ('stable', 'beta')),
    version TEXT NOT NULL CHECK (char_length(version) BETWEEN 5 AND 64),
    target TEXT NOT NULL CHECK (target ~ '^(darwin|windows|linux)-(x86_64|aarch64|i686|armv7)$'),
    status TEXT NOT NULL DEFAULT 'published' CHECK (status IN ('published', 'withdrawn')),
    notes TEXT NOT NULL DEFAULT '' CHECK (char_length(notes) <= 10000),
    -- Tauri updater imzası (.sig dosyasının içeriği, minisign).
    signature TEXT NOT NULL CHECK (char_length(signature) BETWEEN 1 AND 4096),
    -- İşletim sistemi imzası: macOS Developer ID + noter, Windows Authenticode.
    -- false: imzasız iç test paketi.
    os_signed BOOLEAN NOT NULL DEFAULT false,
    updater_key TEXT NOT NULL,
    updater_name TEXT NOT NULL,
    updater_size BIGINT NOT NULL CHECK (updater_size > 0),
    updater_sha256 TEXT NOT NULL CHECK (updater_sha256 ~ '^[0-9a-f]{64}$'),
    -- Kurulum paketi (macOS .dmg). Windows'ta NSIS kurulumu updater paketinin
    -- kendisidir; o zaman bu alanlar boştur ve indirme updater paketini verir.
    installer_key TEXT,
    installer_name TEXT,
    installer_size BIGINT CHECK (installer_size > 0),
    installer_sha256 TEXT CHECK (installer_sha256 ~ '^[0-9a-f]{64}$'),
    published_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    published_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    withdrawn_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    withdrawn_at TIMESTAMPTZ,
    withdrawn_reason TEXT NOT NULL DEFAULT '' CHECK (char_length(withdrawn_reason) <= 1000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT desktop_releases_installer_all CHECK (
        (installer_key IS NULL AND installer_name IS NULL AND installer_size IS NULL AND installer_sha256 IS NULL)
        OR (installer_key IS NOT NULL AND installer_name IS NOT NULL AND installer_size IS NOT NULL AND installer_sha256 IS NOT NULL)),
    CONSTRAINT desktop_releases_withdrawn CHECK ((status = 'withdrawn') = (withdrawn_at IS NOT NULL)),
    CONSTRAINT desktop_releases_unique UNIQUE (app, channel, target, version)
);

CREATE INDEX idx_desktop_releases_lookup ON desktop_releases (app, target, status);
CREATE TRIGGER trg_desktop_releases_updated BEFORE UPDATE ON desktop_releases FOR EACH ROW EXECUTE FUNCTION set_updated_at();

INSERT INTO permissions(name, slug) VALUES
    ('Publish and withdraw desktop releases', 'platform.desktop_releases.manage')
ON CONFLICT (slug) DO NOTHING;
INSERT INTO role_permissions(role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE p.slug = 'platform.desktop_releases.manage'
  AND r.scope = 'platform' AND r.slug = 'super_admin'
ON CONFLICT DO NOTHING;
