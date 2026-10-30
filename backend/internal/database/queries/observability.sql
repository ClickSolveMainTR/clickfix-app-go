-- name: ObservabilityAICost :many
-- Internal operational aggregate: no org labels or customer text are exposed.
SELECT provider, COALESCE(SUM(cost_micros),0)::bigint AS cost_micros,
 (COALESCE(AVG(latency_ms),0) / 1000.0)::double precision AS latency_seconds
FROM ai_runs WHERE created_at > now() - interval '24 hours' GROUP BY provider;
