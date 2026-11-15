-- Data-loss note: drops derived knowledge units, the answer cache, its
-- counters, case memories and recorded unanswered questions.
DROP TABLE IF EXISTS knowledge_gap_questions;
DROP TABLE IF EXISTS ai_case_memories;
DROP TABLE IF EXISTS ai_answer_cache_stats;
DROP TABLE IF EXISTS ai_answer_cache;
DROP TABLE IF EXISTS knowledge_unit_questions;
DROP TABLE IF EXISTS knowledge_units;
