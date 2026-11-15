-- CF-224: department foreign keys on the knowledge layer.
--
-- The knowledge layer branches (CF-216 to CF-220) were written before the
-- departments table (CF-215, 000226) existed, so their department_id BIGINT
-- columns had no foreign key: a deleted department's id or another
-- organization's department could be written. Their comments said CF-215
-- would add the reference; it did not. This migration does.
--
-- Every column gets the same composite key as 000226:
--   (organization_id, department_id) -> departments(organization_id, id)
-- so a department of another organization cannot be stored.
--
-- Delete behaviour: these rows are records about knowledge (history imports,
-- cases, clusters, decisions, category data, evaluation sets and runs, AI
-- units, case memories, unanswered questions), not knowledge a department
-- owns. When the department is deleted they fall back to organization-wide:
-- ON DELETE SET NULL (department_id). Owned knowledge (knowledge_articles,
-- knowledge_sources, business_context_sections, knowledge_suggestions) keeps
-- NO ACTION from 000226: the department cannot be deleted while it owns any,
-- and the delete use case moves it first.
--
-- Existing values that point at no department of the row's organization are
-- set to NULL (organization-wide) before the key is added.
--
-- Out of scope: knowledge_chunks.department_id is still UUID and is converted
-- by CF-222.

-- History pipeline (CF-217, 000229)
UPDATE history_imports x SET department_id = NULL
WHERE x.department_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM departments d WHERE d.organization_id = x.organization_id AND d.id = x.department_id);
ALTER TABLE history_imports ADD CONSTRAINT fk_history_imports_department FOREIGN KEY (organization_id, department_id)
    REFERENCES departments (organization_id, id) ON DELETE SET NULL (department_id);
CREATE INDEX idx_history_imports_department_fk ON history_imports (organization_id, department_id) WHERE department_id IS NOT NULL;
UPDATE history_clusters x SET department_id = NULL
WHERE x.department_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM departments d WHERE d.organization_id = x.organization_id AND d.id = x.department_id);
ALTER TABLE history_clusters ADD CONSTRAINT fk_history_clusters_department FOREIGN KEY (organization_id, department_id)
    REFERENCES departments (organization_id, id) ON DELETE SET NULL (department_id);
CREATE INDEX idx_history_clusters_department_fk ON history_clusters (organization_id, department_id) WHERE department_id IS NOT NULL;
UPDATE history_cases x SET department_id = NULL
WHERE x.department_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM departments d WHERE d.organization_id = x.organization_id AND d.id = x.department_id);
ALTER TABLE history_cases ADD CONSTRAINT fk_history_cases_department FOREIGN KEY (organization_id, department_id)
    REFERENCES departments (organization_id, id) ON DELETE SET NULL (department_id);
CREATE INDEX idx_history_cases_department_fk ON history_cases (organization_id, department_id) WHERE department_id IS NOT NULL;
UPDATE history_decisions x SET department_id = NULL
WHERE x.department_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM departments d WHERE d.organization_id = x.organization_id AND d.id = x.department_id);
ALTER TABLE history_decisions ADD CONSTRAINT fk_history_decisions_department FOREIGN KEY (organization_id, department_id)
    REFERENCES departments (organization_id, id) ON DELETE SET NULL (department_id);
CREATE INDEX idx_history_decisions_department_fk ON history_decisions (organization_id, department_id) WHERE department_id IS NOT NULL;

-- Categories (CF-218, 000227)
UPDATE categories x SET department_id = NULL
WHERE x.department_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM departments d WHERE d.organization_id = x.organization_id AND d.id = x.department_id);
ALTER TABLE categories ADD CONSTRAINT fk_categories_department FOREIGN KEY (organization_id, department_id)
    REFERENCES departments (organization_id, id) ON DELETE SET NULL (department_id);
UPDATE category_assignments x SET department_id = NULL
WHERE x.department_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM departments d WHERE d.organization_id = x.organization_id AND d.id = x.department_id);
ALTER TABLE category_assignments ADD CONSTRAINT fk_category_assignments_department FOREIGN KEY (organization_id, department_id)
    REFERENCES departments (organization_id, id) ON DELETE SET NULL (department_id);
CREATE INDEX idx_category_assignments_department_fk ON category_assignments (organization_id, department_id) WHERE department_id IS NOT NULL;
UPDATE category_labels x SET department_id = NULL
WHERE x.department_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM departments d WHERE d.organization_id = x.organization_id AND d.id = x.department_id);
ALTER TABLE category_labels ADD CONSTRAINT fk_category_labels_department FOREIGN KEY (organization_id, department_id)
    REFERENCES departments (organization_id, id) ON DELETE SET NULL (department_id);
CREATE INDEX idx_category_labels_department_fk ON category_labels (organization_id, department_id) WHERE department_id IS NOT NULL;

-- Measurement (CF-220, 000230)
UPDATE ai_evaluation_cases x SET department_id = NULL
WHERE x.department_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM departments d WHERE d.organization_id = x.organization_id AND d.id = x.department_id);
ALTER TABLE ai_evaluation_cases ADD CONSTRAINT fk_ai_evaluation_cases_department FOREIGN KEY (organization_id, department_id)
    REFERENCES departments (organization_id, id) ON DELETE SET NULL (department_id);
CREATE INDEX idx_ai_evaluation_cases_department_fk ON ai_evaluation_cases (organization_id, department_id) WHERE department_id IS NOT NULL;
UPDATE ai_evaluations x SET department_id = NULL
WHERE x.department_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM departments d WHERE d.organization_id = x.organization_id AND d.id = x.department_id);
ALTER TABLE ai_evaluations ADD CONSTRAINT fk_ai_evaluations_department FOREIGN KEY (organization_id, department_id)
    REFERENCES departments (organization_id, id) ON DELETE SET NULL (department_id);
CREATE INDEX idx_ai_evaluations_department_fk ON ai_evaluations (organization_id, department_id) WHERE department_id IS NOT NULL;
UPDATE ai_evaluation_results x SET department_id = NULL
WHERE x.department_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM departments d WHERE d.organization_id = x.organization_id AND d.id = x.department_id);
ALTER TABLE ai_evaluation_results ADD CONSTRAINT fk_ai_evaluation_results_department FOREIGN KEY (organization_id, department_id)
    REFERENCES departments (organization_id, id) ON DELETE SET NULL (department_id);
CREATE INDEX idx_ai_evaluation_results_department_fk ON ai_evaluation_results (organization_id, department_id) WHERE department_id IS NOT NULL;

-- Knowledge units and memory (CF-216, 000228)
UPDATE knowledge_units x SET department_id = NULL
WHERE x.department_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM departments d WHERE d.organization_id = x.organization_id AND d.id = x.department_id);
ALTER TABLE knowledge_units ADD CONSTRAINT fk_knowledge_units_department FOREIGN KEY (organization_id, department_id)
    REFERENCES departments (organization_id, id) ON DELETE SET NULL (department_id);
CREATE INDEX idx_knowledge_units_department_fk ON knowledge_units (organization_id, department_id) WHERE department_id IS NOT NULL;
UPDATE ai_case_memories x SET department_id = NULL
WHERE x.department_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM departments d WHERE d.organization_id = x.organization_id AND d.id = x.department_id);
ALTER TABLE ai_case_memories ADD CONSTRAINT fk_ai_case_memories_department FOREIGN KEY (organization_id, department_id)
    REFERENCES departments (organization_id, id) ON DELETE SET NULL (department_id);
CREATE INDEX idx_ai_case_memories_department_fk ON ai_case_memories (organization_id, department_id) WHERE department_id IS NOT NULL;
UPDATE knowledge_gap_questions x SET department_id = NULL
WHERE x.department_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM departments d WHERE d.organization_id = x.organization_id AND d.id = x.department_id);
ALTER TABLE knowledge_gap_questions ADD CONSTRAINT fk_knowledge_gap_questions_department FOREIGN KEY (organization_id, department_id)
    REFERENCES departments (organization_id, id) ON DELETE SET NULL (department_id);
CREATE INDEX idx_knowledge_gap_questions_department_fk ON knowledge_gap_questions (organization_id, department_id) WHERE department_id IS NOT NULL;
