-- Data-loss note: removes recorded replay channel/category/time; case text remains.
ALTER TABLE ai_evaluation_cases DROP COLUMN scenario_at,DROP COLUMN category,DROP COLUMN channel;
