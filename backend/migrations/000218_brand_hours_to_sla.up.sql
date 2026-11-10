-- CF-198: working hours have one source, the SLA module's business_hours
-- schedule. Brands no longer carry their own widget hours.
--
-- An organization without a default schedule but with brand hours would
-- otherwise go from "closed outside the brand's hours" to "always open", so
-- the first brand's enabled hours become the organization's default
-- schedule first. Widget shape {"days": {"mon": [{"start","end"}]}} maps to
-- the SLA shape {"mon": {"closed": false, "open", "close"}} (first period of
-- each day; the SLA calendar holds one window a day).
INSERT INTO business_hours (organization_id, name, timezone, working_hours, use_public_holidays, holiday_country, is_default)
SELECT src.organization_id, 'Çalışma saatleri', src.timezone,
       (SELECT jsonb_object_agg(d.day, jsonb_build_object('closed', FALSE, 'open', d.period->>'start', 'close', d.period->>'end'))
        FROM (
            SELECT key AS day, value->0 AS period
            FROM jsonb_each(src.days)
            WHERE jsonb_typeof(value) = 'array' AND jsonb_array_length(value) > 0
              AND key IN ('sun', 'mon', 'tue', 'wed', 'thu', 'fri', 'sat')
        ) d),
       TRUE, 'TR', TRUE
FROM (
    SELECT DISTINCT ON (b.organization_id) b.organization_id,
           COALESCE(NULLIF(b.business_hours->>'timezone', ''), 'Europe/Istanbul') AS timezone,
           COALESCE(b.business_hours->'days', '{}'::jsonb) AS days
    FROM brands b
    WHERE COALESCE((b.business_hours->>'enabled')::boolean, FALSE)
      AND jsonb_typeof(b.business_hours->'days') = 'object'
    ORDER BY b.organization_id, b.id
) src
WHERE NOT EXISTS (SELECT 1 FROM business_hours bh WHERE bh.organization_id = src.organization_id AND bh.is_default)
  AND EXISTS (SELECT 1 FROM jsonb_each(src.days) e WHERE jsonb_typeof(e.value) = 'array' AND jsonb_array_length(e.value) > 0);

-- Data-loss note: per-brand hours are dropped after the copy above; brands
-- beyond the first of an organization are not carried over.
ALTER TABLE brands DROP COLUMN IF EXISTS business_hours;
