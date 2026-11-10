-- CF-198: working hours have one source, the SLA module's business_hours
-- schedule. Brands no longer carry their own widget hours.
-- Data-loss note: per-brand widget hours are dropped; the organization's
-- default SLA schedule drives the widget's online state from now on.
ALTER TABLE brands DROP COLUMN IF EXISTS business_hours;
