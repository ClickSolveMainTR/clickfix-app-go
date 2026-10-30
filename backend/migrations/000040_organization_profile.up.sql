-- CF-3: organization profile (regional defaults used by SLA, reports and
-- e-mails), platform organization export / bulk permissions and the staff
-- invitation e-mail.
--
-- Company identity, business hours and branding live in organizations.settings
-- (validated by the organizations use case); the regional defaults are columns
-- because other modules read them in SQL.

ALTER TABLE organizations
    ADD COLUMN timezone     VARCHAR(64) NOT NULL DEFAULT 'Europe/Istanbul',
    ADD COLUMN locale       VARCHAR(8)  NOT NULL DEFAULT 'tr',
    ADD COLUMN currency     CHAR(3)     NOT NULL DEFAULT 'TRY',
    ADD COLUMN country_code CHAR(2)     NOT NULL DEFAULT 'TR',
    ADD CONSTRAINT chk_organizations_locale CHECK (locale IN ('tr', 'en')),
    ADD CONSTRAINT chk_organizations_currency CHECK (currency ~ '^[A-Z]{3}$'),
    ADD CONSTRAINT chk_organizations_country_code CHECK (country_code ~ '^[A-Z]{2}$');

INSERT INTO permissions (name, slug) VALUES
    ('Export platform organizations', 'platform.organizations.export'),
    ('Bulk activate platform organizations', 'platform.organizations.bulk.activate'),
    ('Bulk suspend platform organizations', 'platform.organizations.bulk.suspend')
ON CONFLICT (slug) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.slug IN (
    'platform.organizations.export',
    'platform.organizations.bulk.activate',
    'platform.organizations.bulk.suspend'
)
WHERE r.slug = 'super_admin' AND r.scope = 'platform'
ON CONFLICT DO NOTHING;

-- Invitation e-mail. {{inviter}} is the inviting person (organization member
-- or platform operator; the organization name when unknown), {{link}} the
-- ${PUBLIC_FRONTEND_URL}/invite/{token} page.
INSERT INTO notification_templates (code, channel, language, subject, body, active) VALUES
    ('organizations.invitation', 'email', 'tr', '{{organization}} sizi ClickFix ekibine davet ediyor',
     'Merhaba,

{{inviter}} sizi ClickFix üzerindeki {{organization}} ekibine {{roles}} rolüyle davet etti.

Daveti kabul etmek için bağlantıyı açın:
{{link}}

Bu davet {{expires_at}} tarihine kadar geçerlidir ve yalnızca {{email}} adresiyle kabul edilebilir. Bu daveti beklemiyorsanız e-postayı yok sayabilirsiniz.', TRUE),
    ('organizations.invitation', 'email', 'en', '{{organization}} invited you to their ClickFix team',
     'Hello,

{{inviter}} invited you to join {{organization}} on ClickFix as {{roles}}.

Open the link to accept the invitation:
{{link}}

The invitation is valid until {{expires_at}} and can only be accepted with {{email}}. If you were not expecting it, you can ignore this e-mail.', TRUE)
ON CONFLICT (code, channel, language) DO NOTHING;
