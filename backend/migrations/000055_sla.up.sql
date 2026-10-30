-- CF-46: SLA policies and business hours.
--
-- business_hours: weekly schedules of an organization in a time zone. One
-- is the organization default; a schedule may belong to a team (team_id is a
-- plain id until the teams table exists). Public holidays of holiday_country
-- close the schedule unless use_public_holidays is off; business_holidays are
-- the organization's own closed or short days.
--
-- sla_policies: conditions (priority, channel, tag, company, company tier) →
-- per-priority targets (sla_policy_targets). A company's contracted policy
-- (companies.sla_policy_id) applies first; otherwise the first active policy
-- by position whose conditions match.
--
-- conversation_sla_metrics: one clock per conversation and metric
-- (first_response, next_response, resolution), ported from the Laravel
-- clickfix-app ticket_sla_states table. A running clock has resumed_at set
-- and paused_at NULL; consumed_seconds is the working time spent before the
-- last pause, so due_at = business_add(resumed_at, target - consumed).

CREATE TABLE business_hours (
    id                  BIGSERIAL    PRIMARY KEY,
    uuid                UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id     BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name                TEXT         NOT NULL,
    timezone            TEXT         NOT NULL DEFAULT 'Europe/Istanbul',
    -- {"mon": {"closed": false, "open": "09:00", "close": "18:00"}, ...}
    working_hours       JSONB        NOT NULL DEFAULT '{}'::jsonb,
    use_public_holidays BOOLEAN      NOT NULL DEFAULT TRUE,
    holiday_country     TEXT         NOT NULL DEFAULT 'TR',
    team_id             BIGINT       NULL,
    is_default          BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT business_hours_uuid_uq UNIQUE (uuid),
    CONSTRAINT business_hours_org_id_uq UNIQUE (organization_id, id),
    CONSTRAINT business_hours_name_chk CHECK (char_length(name) BETWEEN 1 AND 120),
    CONSTRAINT business_hours_country_chk CHECK (holiday_country ~ '^[A-Z]{2}$')
);

CREATE UNIQUE INDEX uq_business_hours_default ON business_hours (organization_id) WHERE is_default;
CREATE UNIQUE INDEX uq_business_hours_team ON business_hours (organization_id, team_id) WHERE team_id IS NOT NULL;

CREATE TRIGGER trg_business_hours_set_updated_at
    BEFORE UPDATE ON business_hours FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Public holidays, shared by every organization. half_day: the eve (arife)
-- closes at 13:00.
CREATE TABLE public_holidays (
    id           BIGSERIAL PRIMARY KEY,
    country_code TEXT      NOT NULL,
    date         DATE      NOT NULL,
    name         TEXT      NOT NULL,
    name_en      TEXT      NOT NULL DEFAULT '',
    half_day     BOOLEAN   NOT NULL DEFAULT FALSE,
    CONSTRAINT public_holidays_uq UNIQUE (country_code, date)
);

CREATE TABLE business_holidays (
    id                BIGSERIAL    PRIMARY KEY,
    uuid              UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id   BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    -- NULL: every schedule of the organization.
    business_hours_id BIGINT       NULL,
    date              DATE         NOT NULL,
    name              TEXT         NOT NULL,
    -- closed all day, or open only between open_time and close_time.
    closed            BOOLEAN      NOT NULL DEFAULT TRUE,
    open_time         TIME         NULL,
    close_time        TIME         NULL,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT business_holidays_uuid_uq UNIQUE (uuid),
    CONSTRAINT business_holidays_schedule_fk FOREIGN KEY (organization_id, business_hours_id)
        REFERENCES business_hours (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT business_holidays_name_chk CHECK (char_length(name) BETWEEN 1 AND 120),
    CONSTRAINT business_holidays_hours_chk CHECK (
        closed OR (open_time IS NOT NULL AND close_time IS NOT NULL AND close_time > open_time))
);

CREATE INDEX idx_business_holidays_org_date ON business_holidays (organization_id, date);

CREATE TABLE sla_policies (
    id                BIGSERIAL    PRIMARY KEY,
    uuid              UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id   BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name              TEXT         NOT NULL,
    description       TEXT         NOT NULL DEFAULT '',
    is_active         BOOLEAN      NOT NULL DEFAULT TRUE,
    position          INTEGER      NOT NULL DEFAULT 0,
    -- NULL: the team's schedule, else the organization default, else 24/7.
    business_hours_id BIGINT       NULL,
    -- {"priorities": [], "channel_types": [], "tag_uuids": [], "company_uuids": [],
    --  "company_tiers": []};
    -- an empty list matches everything.
    conditions        JSONB        NOT NULL DEFAULT '{}'::jsonb,
    warning_percent   INTEGER      NOT NULL DEFAULT 80,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT sla_policies_uuid_uq UNIQUE (uuid),
    CONSTRAINT sla_policies_org_id_uq UNIQUE (organization_id, id),
    CONSTRAINT sla_policies_schedule_fk FOREIGN KEY (organization_id, business_hours_id)
        REFERENCES business_hours (organization_id, id) ON DELETE SET NULL (business_hours_id),
    CONSTRAINT sla_policies_name_chk CHECK (char_length(name) BETWEEN 1 AND 120),
    CONSTRAINT sla_policies_warning_chk CHECK (warning_percent BETWEEN 1 AND 99)
);

CREATE INDEX idx_sla_policies_org_position ON sla_policies (organization_id, position, id);

CREATE TRIGGER trg_sla_policies_set_updated_at
    BEFORE UPDATE ON sla_policies FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE sla_policy_targets (
    organization_id        BIGINT  NOT NULL,
    policy_id              BIGINT  NOT NULL,
    priority               TEXT    NOT NULL,
    first_response_minutes INTEGER NULL,
    next_response_minutes  INTEGER NULL,
    resolution_minutes     INTEGER NULL,
    PRIMARY KEY (policy_id, priority),
    CONSTRAINT sla_policy_targets_policy_fk FOREIGN KEY (organization_id, policy_id)
        REFERENCES sla_policies (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT sla_policy_targets_priority_chk CHECK (priority IN ('low', 'normal', 'high', 'urgent')),
    CONSTRAINT sla_policy_targets_minutes_chk CHECK (
        COALESCE(first_response_minutes, 1) BETWEEN 1 AND 525600
        AND COALESCE(next_response_minutes, 1) BETWEEN 1 AND 525600
        AND COALESCE(resolution_minutes, 1) BETWEEN 1 AND 525600)
);

CREATE TABLE conversation_sla_metrics (
    organization_id  BIGINT       NOT NULL,
    conversation_id  BIGINT       NOT NULL,
    metric           TEXT         NOT NULL,
    policy_id        BIGINT       NULL,
    target_seconds   BIGINT       NOT NULL,
    -- next_response restarts on every new customer turn; cycle counts them.
    cycle            INTEGER      NOT NULL DEFAULT 1,
    started_at       TIMESTAMPTZ  NOT NULL,
    resumed_at       TIMESTAMPTZ  NOT NULL,
    consumed_seconds BIGINT       NOT NULL DEFAULT 0,
    paused_at        TIMESTAMPTZ  NULL,
    warn_at          TIMESTAMPTZ  NULL,
    due_at           TIMESTAMPTZ  NULL,
    warned_at        TIMESTAMPTZ  NULL,
    breached_at      TIMESTAMPTZ  NULL,
    completed_at     TIMESTAMPTZ  NULL,
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (conversation_id, metric),
    CONSTRAINT conversation_sla_metrics_conversation_fk FOREIGN KEY (organization_id, conversation_id)
        REFERENCES conversations (organization_id, id) ON DELETE CASCADE,
    CONSTRAINT conversation_sla_metrics_policy_fk FOREIGN KEY (organization_id, policy_id)
        REFERENCES sla_policies (organization_id, id) ON DELETE SET NULL (policy_id),
    CONSTRAINT conversation_sla_metrics_metric_chk CHECK (metric IN ('first_response', 'next_response', 'resolution'))
);

CREATE INDEX idx_conversation_sla_metrics_org ON conversation_sla_metrics (organization_id, conversation_id);
-- Sweep: running clocks whose warning or due time has come.
CREATE INDEX idx_conversation_sla_metrics_due
    ON conversation_sla_metrics (due_at)
    WHERE completed_at IS NULL AND paused_at IS NULL AND breached_at IS NULL;
CREATE INDEX idx_conversation_sla_metrics_warn
    ON conversation_sla_metrics (warn_at)
    WHERE completed_at IS NULL AND paused_at IS NULL AND warned_at IS NULL AND breached_at IS NULL;

ALTER TABLE conversations
    ADD CONSTRAINT conversations_sla_policy_fk FOREIGN KEY (organization_id, sla_policy_id)
        REFERENCES sla_policies (organization_id, id) ON DELETE SET NULL (sla_policy_id);

-- B2B: a company's contracted SLA (companies.sla_policy_id, CF-84) wins over
-- the condition match.
ALTER TABLE companies
    ADD CONSTRAINT companies_sla_policy_fk FOREIGN KEY (organization_id, sla_policy_id)
        REFERENCES sla_policies (organization_id, id) ON DELETE SET NULL (sla_policy_id);
CREATE INDEX idx_companies_sla_policy ON companies (organization_id, sla_policy_id) WHERE sla_policy_id IS NOT NULL;

-- Turkish public holidays. Fixed national days for 2026-2035; religious
-- holidays (Ramazan / Kurban Bayramı and their eves) as announced by
-- Diyanet for 2026-2027. Later years are added by a migration once the
-- official calendar is published.
INSERT INTO public_holidays (country_code, date, name, name_en, half_day)
SELECT 'TR', make_date(y, d.m, d.dd), d.name, d.name_en, d.half
FROM generate_series(2026, 2035) AS y
CROSS JOIN (VALUES
    (1, 1, 'Yılbaşı', 'New Year''s Day', FALSE),
    (4, 23, 'Ulusal Egemenlik ve Çocuk Bayramı', 'National Sovereignty and Children''s Day', FALSE),
    (5, 1, 'Emek ve Dayanışma Günü', 'Labour and Solidarity Day', FALSE),
    (5, 19, 'Atatürk''ü Anma, Gençlik ve Spor Bayramı', 'Commemoration of Atatürk, Youth and Sports Day', FALSE),
    (7, 15, 'Demokrasi ve Milli Birlik Günü', 'Democracy and National Unity Day', FALSE),
    (8, 30, 'Zafer Bayramı', 'Victory Day', FALSE),
    (10, 28, 'Cumhuriyet Bayramı Arifesi', 'Republic Day Eve', TRUE),
    (10, 29, 'Cumhuriyet Bayramı', 'Republic Day', FALSE)
) AS d (m, dd, name, name_en, half)
ON CONFLICT DO NOTHING;

INSERT INTO public_holidays (country_code, date, name, name_en, half_day) VALUES
    ('TR', '2026-03-19', 'Ramazan Bayramı Arifesi', 'Eid al-Fitr Eve', TRUE),
    ('TR', '2026-03-20', 'Ramazan Bayramı 1. Gün', 'Eid al-Fitr Day 1', FALSE),
    ('TR', '2026-03-21', 'Ramazan Bayramı 2. Gün', 'Eid al-Fitr Day 2', FALSE),
    ('TR', '2026-03-22', 'Ramazan Bayramı 3. Gün', 'Eid al-Fitr Day 3', FALSE),
    ('TR', '2026-05-26', 'Kurban Bayramı Arifesi', 'Eid al-Adha Eve', TRUE),
    ('TR', '2026-05-27', 'Kurban Bayramı 1. Gün', 'Eid al-Adha Day 1', FALSE),
    ('TR', '2026-05-28', 'Kurban Bayramı 2. Gün', 'Eid al-Adha Day 2', FALSE),
    ('TR', '2026-05-29', 'Kurban Bayramı 3. Gün', 'Eid al-Adha Day 3', FALSE),
    ('TR', '2026-05-30', 'Kurban Bayramı 4. Gün', 'Eid al-Adha Day 4', FALSE),
    ('TR', '2027-03-08', 'Ramazan Bayramı Arifesi', 'Eid al-Fitr Eve', TRUE),
    ('TR', '2027-03-09', 'Ramazan Bayramı 1. Gün', 'Eid al-Fitr Day 1', FALSE),
    ('TR', '2027-03-10', 'Ramazan Bayramı 2. Gün', 'Eid al-Fitr Day 2', FALSE),
    ('TR', '2027-03-11', 'Ramazan Bayramı 3. Gün', 'Eid al-Fitr Day 3', FALSE),
    ('TR', '2027-05-15', 'Kurban Bayramı Arifesi', 'Eid al-Adha Eve', TRUE),
    ('TR', '2027-05-16', 'Kurban Bayramı 1. Gün', 'Eid al-Adha Day 1', FALSE),
    ('TR', '2027-05-17', 'Kurban Bayramı 2. Gün', 'Eid al-Adha Day 2', FALSE),
    ('TR', '2027-05-18', 'Kurban Bayramı 3. Gün', 'Eid al-Adha Day 3', FALSE),
    ('TR', '2027-05-19', 'Kurban Bayramı 4. Gün', 'Eid al-Adha Day 4', FALSE)
ON CONFLICT DO NOTHING;
