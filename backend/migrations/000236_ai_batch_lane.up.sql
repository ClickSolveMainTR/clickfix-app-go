-- CF-228: batch AI jobs (history pipeline, re-embedding, evaluations) run in
-- their own lane. They may use at most batch_share_percent of the daily call
-- and budget limits, so live AI always keeps the rest. A history run that
-- hits a limit pauses and resumes when the limit resets instead of failing.

-- Lane of each call. Existing rows are live (the default), which only makes
-- today's batch share look emptier than it is for the rest of this day.
ALTER TABLE ai_runs ADD COLUMN lane TEXT NOT NULL DEFAULT 'live';
ALTER TABLE ai_runs ADD CONSTRAINT ai_runs_lane_check CHECK (lane IN ('live', 'batch')) NOT VALID;
ALTER TABLE ai_runs VALIDATE CONSTRAINT ai_runs_lane_check;

-- Share of the effective daily limits batch jobs may use (live AI keeps the
-- rest and may also use what batch jobs leave unused).
ALTER TABLE ai_policies ADD COLUMN batch_share_percent SMALLINT NOT NULL DEFAULT 50
 CHECK (batch_share_percent BETWEEN 10 AND 90);

-- Paused runs: why (error_code), when the sweep picks them up again
-- (resume_at) and where they continue (resume_offset: records, then
-- clusters to evaluate).
ALTER TABLE history_imports DROP CONSTRAINT history_imports_status_check;
ALTER TABLE history_imports ADD CONSTRAINT history_imports_status_check
 CHECK (status IN ('pending', 'running', 'paused', 'completed', 'failed', 'cancelled', 'rolled_back'));
ALTER TABLE history_imports
 ADD COLUMN error_code TEXT NOT NULL DEFAULT '' CHECK (error_code ~ '^[a-z_]{0,40}$'),
 ADD COLUMN resume_at TIMESTAMPTZ,
 ADD COLUMN resume_offset INTEGER NOT NULL DEFAULT 0 CHECK (resume_offset >= 0);

DROP INDEX idx_history_imports_due;
CREATE INDEX idx_history_imports_due ON history_imports (status, created_at) WHERE status IN ('pending', 'running', 'paused');
