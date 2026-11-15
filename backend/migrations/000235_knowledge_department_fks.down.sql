-- CF-224: drop the department foreign keys and indexes added in 000235.
-- Data-loss note: department ids that up set to NULL (they pointed at no
-- department of the row's organization) are not restored.
DROP INDEX IF EXISTS idx_knowledge_gap_questions_department_fk;
ALTER TABLE knowledge_gap_questions DROP CONSTRAINT IF EXISTS fk_knowledge_gap_questions_department;
DROP INDEX IF EXISTS idx_ai_case_memories_department_fk;
ALTER TABLE ai_case_memories DROP CONSTRAINT IF EXISTS fk_ai_case_memories_department;
DROP INDEX IF EXISTS idx_knowledge_units_department_fk;
ALTER TABLE knowledge_units DROP CONSTRAINT IF EXISTS fk_knowledge_units_department;
DROP INDEX IF EXISTS idx_ai_evaluation_results_department_fk;
ALTER TABLE ai_evaluation_results DROP CONSTRAINT IF EXISTS fk_ai_evaluation_results_department;
DROP INDEX IF EXISTS idx_ai_evaluations_department_fk;
ALTER TABLE ai_evaluations DROP CONSTRAINT IF EXISTS fk_ai_evaluations_department;
DROP INDEX IF EXISTS idx_ai_evaluation_cases_department_fk;
ALTER TABLE ai_evaluation_cases DROP CONSTRAINT IF EXISTS fk_ai_evaluation_cases_department;
DROP INDEX IF EXISTS idx_category_labels_department_fk;
ALTER TABLE category_labels DROP CONSTRAINT IF EXISTS fk_category_labels_department;
DROP INDEX IF EXISTS idx_category_assignments_department_fk;
ALTER TABLE category_assignments DROP CONSTRAINT IF EXISTS fk_category_assignments_department;
ALTER TABLE categories DROP CONSTRAINT IF EXISTS fk_categories_department;
DROP INDEX IF EXISTS idx_history_decisions_department_fk;
ALTER TABLE history_decisions DROP CONSTRAINT IF EXISTS fk_history_decisions_department;
DROP INDEX IF EXISTS idx_history_cases_department_fk;
ALTER TABLE history_cases DROP CONSTRAINT IF EXISTS fk_history_cases_department;
DROP INDEX IF EXISTS idx_history_clusters_department_fk;
ALTER TABLE history_clusters DROP CONSTRAINT IF EXISTS fk_history_clusters_department;
DROP INDEX IF EXISTS idx_history_imports_department_fk;
ALTER TABLE history_imports DROP CONSTRAINT IF EXISTS fk_history_imports_department;
