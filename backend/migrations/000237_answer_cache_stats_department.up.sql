-- CF-225. Answer cache counters per department, so the AI knowledge panel
-- (CF-220) can show the cache hit rate of one department. department_id NULL
-- is a lookup without a department (organization wide); the organization
-- total is the sum over every row. Deleting a department deletes its
-- counters (a SET NULL would collide with the organization-wide row of the
-- same day).
ALTER TABLE ai_answer_cache_stats DROP CONSTRAINT ai_answer_cache_stats_pkey;
ALTER TABLE ai_answer_cache_stats ADD COLUMN department_id BIGINT NULL;
ALTER TABLE ai_answer_cache_stats ADD CONSTRAINT uq_ai_answer_cache_stats_day
    UNIQUE NULLS NOT DISTINCT (organization_id, day, audience, department_id);
ALTER TABLE ai_answer_cache_stats ADD CONSTRAINT fk_ai_answer_cache_stats_department FOREIGN KEY (organization_id, department_id)
    REFERENCES departments (organization_id, id) ON DELETE CASCADE;
