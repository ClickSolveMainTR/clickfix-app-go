-- CF-9: per-event notification preferences, quiet hours and e-mail digests.
-- event_channels: {"<event type>": {"inapp": bool, "email": bool, "push": bool}};
-- missing entries fall back to the catalog defaults in code.
ALTER TABLE notification_preferences
    ADD COLUMN event_channels      JSONB       NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN quiet_hours_enabled BOOLEAN     NOT NULL DEFAULT FALSE,
    ADD COLUMN quiet_hours_start   TEXT        NOT NULL DEFAULT '22:00',
    ADD COLUMN quiet_hours_end     TEXT        NOT NULL DEFAULT '08:00',
    ADD COLUMN timezone            TEXT        NOT NULL DEFAULT 'Europe/Istanbul',
    ADD COLUMN email_digest        TEXT        NOT NULL DEFAULT 'off',
    ADD COLUMN digest_hour         SMALLINT    NOT NULL DEFAULT 9,
    ADD COLUMN last_digest_at      TIMESTAMPTZ NULL,
    ADD CONSTRAINT notification_preferences_digest_chk CHECK (email_digest IN ('off', 'hourly', 'daily')),
    ADD CONSTRAINT notification_preferences_digest_hour_chk CHECK (digest_hour BETWEEN 0 AND 23),
    ADD CONSTRAINT notification_preferences_quiet_chk CHECK (
        quiet_hours_start ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'
        AND quiet_hours_end ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'
    );

-- E-mails held for the hourly/daily digest instead of being sent one by one.
CREATE TABLE notification_digest_items (
    id              BIGSERIAL    PRIMARY KEY,
    user_id         BIGINT       NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    organization_id BIGINT       NULL REFERENCES organizations (id) ON DELETE CASCADE,
    event_type      TEXT         NOT NULL,
    title           TEXT         NOT NULL,
    body            TEXT         NOT NULL DEFAULT '',
    action_url      TEXT         NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    digested_at     TIMESTAMPTZ  NULL
);

CREATE INDEX idx_notification_digest_items_pending
    ON notification_digest_items (user_id, created_at)
    WHERE digested_at IS NULL;

INSERT INTO notification_templates (code, channel, language, subject, body, active) VALUES
    ('conversation.assigned', 'inapp', 'tr', 'Konuşma size atandı', '#{{number}} {{subject}} konuşması size atandı.', TRUE),
    ('conversation.assigned', 'inapp', 'en', 'Conversation assigned to you', 'Conversation #{{number}} {{subject}} was assigned to you.', TRUE),
    ('conversation.assigned', 'email', 'tr', 'Konuşma size atandı: #{{number}}', E'Merhaba {{name}},\n\n#{{number}} {{subject}} konuşması size atandı.\n\n{{action_url}}', TRUE),
    ('conversation.assigned', 'email', 'en', 'Conversation assigned to you: #{{number}}', E'Hello {{name}},\n\nConversation #{{number}} {{subject}} was assigned to you.\n\n{{action_url}}', TRUE),
    ('message.received', 'inapp', 'tr', 'Yeni mesaj: #{{number}}', '{{preview}}', TRUE),
    ('message.received', 'inapp', 'en', 'New message: #{{number}}', '{{preview}}', TRUE),
    ('message.received', 'email', 'tr', 'Yeni mesaj: #{{number}}', E'Merhaba {{name}},\n\n#{{number}} konuşmasında yeni bir mesaj var:\n\n{{preview}}\n\n{{action_url}}', TRUE),
    ('message.received', 'email', 'en', 'New message: #{{number}}', E'Hello {{name}},\n\nThere is a new message in conversation #{{number}}:\n\n{{preview}}\n\n{{action_url}}', TRUE),
    ('conversation.mentioned', 'inapp', 'tr', '{{actor}} sizden bahsetti', '#{{number}} konuşmasındaki bir notta sizden bahsedildi.', TRUE),
    ('conversation.mentioned', 'inapp', 'en', '{{actor}} mentioned you', 'You were mentioned in a note on conversation #{{number}}.', TRUE),
    ('conversation.mentioned', 'email', 'tr', '{{actor}} sizden bahsetti: #{{number}}', E'Merhaba {{name}},\n\n{{actor}}, #{{number}} konuşmasındaki bir notta sizden bahsetti.\n\n{{action_url}}', TRUE),
    ('conversation.mentioned', 'email', 'en', '{{actor}} mentioned you: #{{number}}', E'Hello {{name}},\n\n{{actor}} mentioned you in a note on conversation #{{number}}.\n\n{{action_url}}', TRUE),
    ('ticket.assigned', 'inapp', 'tr', 'Talep size atandı', '{{ticket}} talebi size atandı.', TRUE),
    ('ticket.assigned', 'inapp', 'en', 'Ticket assigned to you', 'Ticket {{ticket}} was assigned to you.', TRUE),
    ('ticket.assigned', 'email', 'tr', 'Talep size atandı: {{ticket}}', E'Merhaba {{name}},\n\n{{ticket}} talebi size atandı.\n\n{{action_url}}', TRUE),
    ('ticket.assigned', 'email', 'en', 'Ticket assigned to you: {{ticket}}', E'Hello {{name}},\n\nTicket {{ticket}} was assigned to you.\n\n{{action_url}}', TRUE),
    ('ticket.commented', 'inapp', 'tr', 'Talebe yorum eklendi', '{{ticket}} talebine yeni bir yorum eklendi.', TRUE),
    ('ticket.commented', 'inapp', 'en', 'New comment on ticket', 'A new comment was added to ticket {{ticket}}.', TRUE),
    ('ticket.commented', 'email', 'tr', 'Talebe yorum eklendi: {{ticket}}', E'Merhaba {{name}},\n\n{{ticket}} talebine yeni bir yorum eklendi.\n\n{{action_url}}', TRUE),
    ('ticket.commented', 'email', 'en', 'New comment on ticket {{ticket}}', E'Hello {{name}},\n\nA new comment was added to ticket {{ticket}}.\n\n{{action_url}}', TRUE),
    ('sla.warning', 'inapp', 'tr', 'SLA süresi yaklaşıyor', '{{target}} için {{metric}} süresi {{due}} tarihinde doluyor.', TRUE),
    ('sla.warning', 'inapp', 'en', 'SLA due soon', '{{metric}} for {{target}} is due at {{due}}.', TRUE),
    ('sla.warning', 'email', 'tr', 'SLA süresi yaklaşıyor: {{target}}', E'Merhaba {{name}},\n\n{{target}} için {{metric}} süresi {{due}} tarihinde doluyor.\n\n{{action_url}}', TRUE),
    ('sla.warning', 'email', 'en', 'SLA due soon: {{target}}', E'Hello {{name}},\n\n{{metric}} for {{target}} is due at {{due}}.\n\n{{action_url}}', TRUE),
    ('sla.breached', 'inapp', 'tr', 'SLA ihlal edildi', '{{target}} için {{metric}} süresi aşıldı.', TRUE),
    ('sla.breached', 'inapp', 'en', 'SLA breached', '{{metric}} for {{target}} was breached.', TRUE),
    ('sla.breached', 'email', 'tr', 'SLA ihlal edildi: {{target}}', E'Merhaba {{name}},\n\n{{target}} için {{metric}} süresi aşıldı.\n\n{{action_url}}', TRUE),
    ('sla.breached', 'email', 'en', 'SLA breached: {{target}}', E'Hello {{name}},\n\n{{metric}} for {{target}} was breached.\n\n{{action_url}}', TRUE),
    ('csat.submitted', 'inapp', 'tr', 'Yeni memnuniyet puanı: {{rating}}/5', '{{comment}}', TRUE),
    ('csat.submitted', 'inapp', 'en', 'New satisfaction rating: {{rating}}/5', '{{comment}}', TRUE),
    ('csat.submitted', 'email', 'tr', 'Yeni memnuniyet puanı: {{rating}}/5', E'Merhaba {{name}},\n\nBir müşteri {{rating}}/5 puan verdi.\n\n{{comment}}\n\n{{action_url}}', TRUE),
    ('csat.submitted', 'email', 'en', 'New satisfaction rating: {{rating}}/5', E'Hello {{name}},\n\nA customer rated {{rating}}/5.\n\n{{comment}}\n\n{{action_url}}', TRUE),
    ('notifications.digest', 'email', 'tr', 'ClickFix özeti: {{count}} bildirim', E'Merhaba {{name}},\n\nSon özetten bu yana {{count}} bildiriminiz var:\n\n{{items}}', TRUE),
    ('notifications.digest', 'email', 'en', 'ClickFix digest: {{count}} notifications', E'Hello {{name}},\n\nYou have {{count}} notifications since the last digest:\n\n{{items}}', TRUE)
ON CONFLICT (code, channel, language) DO NOTHING;
