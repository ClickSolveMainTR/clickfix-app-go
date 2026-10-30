-- CF-143: platform commerce, sign-up mode and the end of the cms_user role.

-- Catalog default for plan versions that predate a feature. A feature added
-- after plans were published reaches their subscribers through this default,
-- so migrations never have to edit immutable published versions (the 000194
-- incident). Explicit plan grants and organization overrides still win.
ALTER TABLE billing_features
    ADD COLUMN default_bool BOOLEAN,
    ADD COLUMN default_limit BIGINT CHECK (default_limit IS NULL OR default_limit >= 0),
    ADD COLUMN default_unlimited BOOLEAN NOT NULL DEFAULT false;

-- Credit products: a buyer-facing description, the plans that may buy the
-- pack (empty = every plan) and the display order.
ALTER TABLE billing_credit_packages
    ADD COLUMN description TEXT NOT NULL DEFAULT '' CHECK (length(description) <= 500),
    ADD COLUMN plan_ids BIGINT[] NOT NULL DEFAULT '{}',
    ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;

-- Low AI credit alert: the threshold (NULL = a fifth of the plan allowance)
-- and when owners were last told, so the alert goes out once a day.
ALTER TABLE billing_wallet_settings
    ADD COLUMN alert_threshold BIGINT CHECK (alert_threshold IS NULL OR alert_threshold >= 0),
    ADD COLUMN low_balance_notified_at TIMESTAMPTZ;

INSERT INTO notification_templates(code,channel,language,subject,body,active) VALUES
 ('billing.wallet_low','email','tr','AI krediniz azalıyor','Kalan AI krediniz: {{balance}}. Krediler bittiğinde yapay zekâ yanıtları durur ve konuşmalar ekibinize aktarılır. Kredi paketi almak için: {{billing_path}}',true),
 ('billing.wallet_low','inapp','tr','AI krediniz azalıyor','Kalan AI krediniz: {{balance}}. Kredi paketi alabilir ya da otomatik yüklemeyi açabilirsiniz.',true),
 ('billing.wallet_low','email','en','Your AI credits are running low','AI credits left: {{balance}}. When they run out, AI replies stop and conversations are handed to your team. Buy a credit pack: {{billing_path}}',true),
 ('billing.wallet_low','inapp','en','Your AI credits are running low','AI credits left: {{balance}}. Buy a credit pack or turn on auto top-up.',true)
ON CONFLICT(code,channel,language) DO NOTHING;

-- Sign-up mode: open (anyone opens an organization), invite (accounts only
-- through invitations) or closed (only invitations a platform admin sent).
-- registration_enabled stays in sync for older readers.
ALTER TABLE auth_settings
    ADD COLUMN signup_mode TEXT NOT NULL DEFAULT 'invite' CHECK (signup_mode IN ('open', 'invite', 'closed'));
UPDATE auth_settings SET signup_mode = CASE WHEN registration_enabled THEN 'open' ELSE 'invite' END;

ALTER TABLE organization_invitations
    ADD COLUMN platform_issued BOOLEAN NOT NULL DEFAULT false;

-- cms_user is gone: workspace access comes from organization membership.
-- Its two permissions gate nothing a member needs; the default sign-up role
-- pointing at it is cleared.
UPDATE auth_settings SET default_role_id = NULL
 WHERE default_role_id IN (SELECT id FROM roles WHERE slug = 'cms_user' AND scope = 'platform');
DELETE FROM user_roles WHERE role_id IN (SELECT id FROM roles WHERE slug = 'cms_user' AND scope = 'platform');
DELETE FROM role_permissions WHERE role_id IN (SELECT id FROM roles WHERE slug = 'cms_user' AND scope = 'platform');
DELETE FROM roles WHERE slug = 'cms_user' AND scope = 'platform';

-- Manual wallet adjustments record why (category) and who (platform admin).
ALTER TABLE billing_credit_ledger
    ADD COLUMN category TEXT CHECK (category IS NULL OR category IN ('goodwill', 'correction', 'promotion', 'sales', 'migration', 'other')),
    ADD COLUMN actor_user_id BIGINT;
