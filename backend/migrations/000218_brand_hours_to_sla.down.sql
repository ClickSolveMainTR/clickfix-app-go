-- The column comes back empty: the brand hours copied into the default SLA
-- schedule by the up migration are not moved back (the schedule stays).
ALTER TABLE brands ADD COLUMN IF NOT EXISTS business_hours JSONB NOT NULL DEFAULT '{}';
