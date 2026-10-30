-- CF-144: marketing site tracking settings (platform admin, no redeploy) and
-- signup attribution (UTM / click ids / referrer, first and last touch).

-- Single row: tag ids, verification tokens, SEO defaults and the sanitized
-- custom snippets. Read by the public landing (`GET /v1/public/marketing`).
CREATE TABLE marketing_settings (
    id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    og_image_key TEXT,
    updated_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO marketing_settings(id) VALUES (1);
CREATE TRIGGER trg_marketing_settings_updated BEFORE UPDATE ON marketing_settings FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Where a signed-up user or organization came from. One row per user and one
-- per organization; source / medium / campaign are copied out of the first
-- touch (last touch when the first is empty) for the "Kaynaklar" report.
CREATE TABLE signup_attributions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    organization_id BIGINT UNIQUE REFERENCES organizations(id) ON DELETE CASCADE,
    first_touch JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_touch JSONB NOT NULL DEFAULT '{}'::jsonb,
    source TEXT NOT NULL DEFAULT '',
    medium TEXT NOT NULL DEFAULT '',
    campaign TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (user_id IS NOT NULL OR organization_id IS NOT NULL)
);
