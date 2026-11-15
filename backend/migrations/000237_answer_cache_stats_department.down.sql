-- Data-loss note: department counters are folded into the organization-wide
-- row of the same day and audience; the department split is lost.
INSERT INTO ai_answer_cache_stats (organization_id, day, audience, exact_hits, semantic_hits, misses, department_id)
SELECT organization_id, day, audience, sum(exact_hits), sum(semantic_hits), sum(misses), NULL
FROM ai_answer_cache_stats WHERE department_id IS NOT NULL GROUP BY organization_id, day, audience
ON CONFLICT ON CONSTRAINT uq_ai_answer_cache_stats_day DO UPDATE SET
    exact_hits = ai_answer_cache_stats.exact_hits + excluded.exact_hits,
    semantic_hits = ai_answer_cache_stats.semantic_hits + excluded.semantic_hits,
    misses = ai_answer_cache_stats.misses + excluded.misses;
DELETE FROM ai_answer_cache_stats WHERE department_id IS NOT NULL;
ALTER TABLE ai_answer_cache_stats DROP CONSTRAINT fk_ai_answer_cache_stats_department;
ALTER TABLE ai_answer_cache_stats DROP CONSTRAINT uq_ai_answer_cache_stats_day;
ALTER TABLE ai_answer_cache_stats DROP COLUMN department_id;
ALTER TABLE ai_answer_cache_stats ADD PRIMARY KEY (organization_id, day, audience);
