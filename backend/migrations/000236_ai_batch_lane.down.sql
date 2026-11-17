-- Data-loss note: paused history runs become failed (their reason stays in
-- error), and the lane of past AI calls and the batch share are dropped.
DROP INDEX idx_history_imports_due;
CREATE INDEX idx_history_imports_due ON history_imports (status, created_at) WHERE status IN ('pending', 'running');

UPDATE history_imports SET status = 'failed', finished_at = now(), lease_until = NULL,
 error = CASE WHEN error = '' THEN 'paused at an AI limit; upload it again' ELSE error END
WHERE status = 'paused';
ALTER TABLE history_imports DROP COLUMN resume_offset, DROP COLUMN resume_at, DROP COLUMN error_code;
ALTER TABLE history_imports DROP CONSTRAINT history_imports_status_check;
ALTER TABLE history_imports ADD CONSTRAINT history_imports_status_check
 CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled', 'rolled_back'));

ALTER TABLE ai_policies DROP COLUMN batch_share_percent;
ALTER TABLE ai_runs DROP COLUMN lane;
