-- CF-218 category taxonomy, assignments and the label store.

-- name: ListCategories :many
-- department_id NULL lists the whole tree; a department lists the
-- organization-wide nodes plus that department's branches.
SELECT c.*, p.uuid AS parent_uuid
FROM categories c
LEFT JOIN categories p ON p.organization_id = c.organization_id AND p.id = c.parent_id
WHERE c.organization_id = sqlc.arg(organization_id)
  AND (sqlc.arg(include_archived)::boolean OR c.archived_at IS NULL)
  AND (sqlc.narg(department_id)::bigint IS NULL OR c.department_id IS NULL OR c.department_id = sqlc.narg(department_id)::bigint)
ORDER BY c.depth, c.position, c.name, c.id;

-- name: GetCategoryByUUID :one
SELECT c.*, p.uuid AS parent_uuid
FROM categories c
LEFT JOIN categories p ON p.organization_id = c.organization_id AND p.id = c.parent_id
WHERE c.organization_id = $1 AND c.uuid = $2;

-- name: GetCategoryByID :one
SELECT * FROM categories WHERE organization_id = $1 AND id = $2;

-- name: CategoryAncestorIDs :many
-- The node itself and every ancestor (cycle checks on re-parenting).
WITH RECURSIVE up AS (
  SELECT c.id, c.parent_id, 0 AS hops FROM categories c WHERE c.organization_id = sqlc.arg(organization_id) AND c.id = sqlc.arg(id)
  UNION ALL
  SELECT p.id, p.parent_id, up.hops + 1 FROM categories p JOIN up ON p.id = up.parent_id
  WHERE p.organization_id = sqlc.arg(organization_id) AND up.hops < 10
)
SELECT up.id::bigint FROM up;

-- name: CategorySubtreeHeight :one
-- Levels below the node (0 for a leaf).
WITH RECURSIVE down AS (
  SELECT c.id, 0 AS lvl FROM categories c WHERE c.organization_id = sqlc.arg(organization_id) AND c.id = sqlc.arg(id)
  UNION ALL
  SELECT ch.id, down.lvl + 1 FROM categories ch JOIN down ON ch.parent_id = down.id
  WHERE ch.organization_id = sqlc.arg(organization_id) AND down.lvl < 10
)
SELECT COALESCE(max(lvl), 0)::int FROM down;

-- name: CategoryChildDepartmentConflict :one
-- A descendant whose department differs from the given one (moving a branch
-- to another department would orphan its department-scoped children).
WITH RECURSIVE down AS (
  SELECT c.id FROM categories c WHERE c.organization_id = sqlc.arg(organization_id) AND c.parent_id = sqlc.arg(id)
  UNION ALL
  SELECT ch.id FROM categories ch JOIN down ON ch.parent_id = down.id WHERE ch.organization_id = sqlc.arg(organization_id)
)
SELECT EXISTS (
  SELECT 1 FROM categories x JOIN down ON down.id = x.id
  WHERE x.organization_id = sqlc.arg(organization_id) AND x.department_id IS DISTINCT FROM sqlc.narg(department_id)::bigint
)::boolean;

-- name: CreateCategory :one
INSERT INTO categories (organization_id, department_id, parent_id, name, slug, description, examples, position, depth)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: UpdateCategory :one
UPDATE categories SET department_id = $3, parent_id = $4, name = $5, slug = $6, description = $7,
  examples = $8, position = $9, depth = $10, archived_at = $11
WHERE organization_id = $1 AND id = $2
RETURNING *;

-- name: ShiftCategoryDescendantDepth :exec
WITH RECURSIVE down AS (
  SELECT c.id FROM categories c WHERE c.organization_id = sqlc.arg(organization_id) AND c.parent_id = sqlc.arg(id)
  UNION ALL
  SELECT ch.id FROM categories ch JOIN down ON ch.parent_id = down.id WHERE ch.organization_id = sqlc.arg(organization_id)
)
UPDATE categories u SET depth = u.depth + sqlc.arg(delta)::smallint
WHERE u.organization_id = sqlc.arg(organization_id) AND u.id IN (SELECT down.id FROM down);

-- name: CategoryInUse :one
SELECT (EXISTS (SELECT 1 FROM categories ch WHERE ch.organization_id = sqlc.arg(organization_id)::bigint AND ch.parent_id = sqlc.arg(id)::bigint)
     OR EXISTS (SELECT 1 FROM category_assignments a WHERE a.organization_id = sqlc.arg(organization_id)::bigint AND a.category_id = sqlc.arg(id)::bigint)
     OR EXISTS (SELECT 1 FROM category_labels l WHERE l.organization_id = sqlc.arg(organization_id)::bigint AND (l.category_id = sqlc.arg(id)::bigint OR l.previous_category_id = sqlc.arg(id)::bigint)))::boolean;

-- name: DeleteCategory :execrows
DELETE FROM categories WHERE organization_id = $1 AND id = $2;

-- name: CategoryClassificationCandidates :many
-- Active categories a record may take: a department's record takes the
-- organization-wide nodes and its branch; a record without a department
-- (NULL) may take any node.
SELECT c.id, c.uuid, c.parent_id, c.department_id, c.name, c.slug, c.description, c.examples, c.depth
FROM categories c
WHERE c.organization_id = sqlc.arg(organization_id) AND c.archived_at IS NULL
  AND (sqlc.narg(department_id)::bigint IS NULL OR c.department_id IS NULL OR c.department_id = sqlc.narg(department_id)::bigint)
ORDER BY c.depth, c.position, c.id;

-- name: CategoryExamplesMissing :many
-- Example texts of the candidate set that have no vector for the model.
SELECT x.category_id, x.content FROM (
  SELECT c.id AS category_id, unnest(ARRAY[c.name || CASE WHEN c.description <> '' THEN ': ' || c.description ELSE '' END] || c.examples)::text AS content
  FROM categories c
  WHERE c.organization_id = sqlc.arg(organization_id) AND c.archived_at IS NULL
    AND (sqlc.narg(department_id)::bigint IS NULL OR c.department_id IS NULL OR c.department_id = sqlc.narg(department_id)::bigint)
) x
WHERE length(x.content) BETWEEN 1 AND 4000 AND NOT EXISTS (
  SELECT 1 FROM category_examples e WHERE e.organization_id = sqlc.arg(organization_id) AND e.category_id = x.category_id
    AND e.embedding_model = sqlc.arg(embedding_model) AND e.content = x.content)
LIMIT 256;

-- name: InsertCategoryExample :exec
INSERT INTO category_examples (organization_id, category_id, content, embedding, embedding_model)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (organization_id, category_id, embedding_model, content) DO NOTHING;

-- name: PruneCategoryExamples :exec
-- Drops vectors whose text is no longer one of the category's texts.
DELETE FROM category_examples e USING categories c
WHERE e.organization_id = $1 AND c.organization_id = e.organization_id AND c.id = e.category_id AND c.id = $2
  AND e.content <> ALL (ARRAY[c.name || CASE WHEN c.description <> '' THEN ': ' || c.description ELSE '' END] || c.examples);

-- name: CategorySimilarity :many
-- Best cosine score per candidate category over its example vectors and the
-- organization's labelled texts (human labels are examples too).
SELECT s.category_id::bigint AS category_id, max(s.score)::double precision AS score FROM (
  SELECT e.category_id, ai_cosine(e.embedding, sqlc.arg(embedding)::real[]) AS score
  FROM category_examples e
  WHERE e.organization_id = sqlc.arg(organization_id) AND e.embedding_model = sqlc.arg(embedding_model)
  UNION ALL
  SELECT l.category_id, ai_cosine(l.embedding, sqlc.arg(embedding)::real[]) AS score
  FROM category_labels l
  WHERE l.organization_id = sqlc.arg(organization_id) AND l.embedding_model = sqlc.arg(embedding_model) AND l.embedding IS NOT NULL
    AND (sqlc.narg(department_id)::bigint IS NULL OR l.department_id IS NULL OR l.department_id = sqlc.narg(department_id)::bigint)
) s
JOIN categories c ON c.organization_id = sqlc.arg(organization_id) AND c.id = s.category_id AND c.archived_at IS NULL
  AND (sqlc.narg(department_id)::bigint IS NULL OR c.department_id IS NULL OR c.department_id = sqlc.narg(department_id)::bigint)
GROUP BY s.category_id
ORDER BY score DESC
LIMIT 10;

-- name: GetCategoryAssignment :one
SELECT a.*, c.uuid AS category_uuid, c.name AS category_name, c.slug AS category_slug
FROM category_assignments a
LEFT JOIN categories c ON c.organization_id = a.organization_id AND c.id = a.category_id
WHERE a.organization_id = $1 AND a.subject_type = $2 AND a.subject_id = $3;

-- name: UpsertCategoryAssignment :one
INSERT INTO category_assignments (organization_id, subject_type, subject_id, subject_uuid, department_id, category_id,
  confidence, source, classifier, needs_review, assigned_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (organization_id, subject_type, subject_id) DO UPDATE SET
  department_id = EXCLUDED.department_id, category_id = EXCLUDED.category_id, confidence = EXCLUDED.confidence,
  source = EXCLUDED.source, classifier = EXCLUDED.classifier, needs_review = EXCLUDED.needs_review,
  assigned_by = EXCLUDED.assigned_by
RETURNING *;

-- name: InsertCategoryAssignmentIfAbsent :execrows
-- Automatic classification never overwrites an existing (human) decision.
INSERT INTO category_assignments (organization_id, subject_type, subject_id, subject_uuid, department_id, category_id,
  confidence, source, classifier, needs_review)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (organization_id, subject_type, subject_id) DO NOTHING;

-- name: ReplaceAutoCategoryAssignment :execrows
-- Re-classification replaces a machine decision, never a human one.
UPDATE category_assignments SET department_id = $4, category_id = $5, confidence = $6, source = $7,
  classifier = $8, needs_review = $9, assigned_by = NULL
WHERE organization_id = $1 AND subject_type = $2 AND subject_id = $3 AND source <> 'human';

-- name: DeleteCategoryAssignment :execrows
DELETE FROM category_assignments WHERE organization_id = $1 AND subject_type = $2 AND subject_id = $3;

-- name: ListCategoryReviewQueue :many
SELECT a.*, c.uuid AS category_uuid, c.name AS category_name, c.slug AS category_slug
FROM category_assignments a
LEFT JOIN categories c ON c.organization_id = a.organization_id AND c.id = a.category_id
WHERE a.organization_id = sqlc.arg(organization_id) AND a.needs_review
  AND (sqlc.arg(subject_type)::text = '' OR a.subject_type = sqlc.arg(subject_type)::text)
  AND (sqlc.narg(department_id)::bigint IS NULL OR a.department_id IS NULL OR a.department_id = sqlc.narg(department_id)::bigint)
ORDER BY a.updated_at DESC, a.id DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountCategoryReviewQueue :one
SELECT count(*) FROM category_assignments a
WHERE a.organization_id = sqlc.arg(organization_id) AND a.needs_review
  AND (sqlc.arg(subject_type)::text = '' OR a.subject_type = sqlc.arg(subject_type)::text)
  AND (sqlc.narg(department_id)::bigint IS NULL OR a.department_id IS NULL OR a.department_id = sqlc.narg(department_id)::bigint);

-- name: InsertCategoryLabel :one
INSERT INTO category_labels (organization_id, department_id, subject_type, subject_uuid, action, content, embedding,
  embedding_model, previous_category_id, previous_source, previous_confidence, category_id, labeled_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: CountCategoryLabels :one
SELECT count(*) FROM category_labels WHERE organization_id = $1;

-- name: ExportCategoryLabels :many
-- Keyset page of the training set (CF-219): masked text, vector, old and
-- new category, who and when.
SELECT l.uuid, l.department_id, l.subject_type, l.action, l.content, l.embedding, l.embedding_model,
  l.previous_source, l.previous_confidence, l.created_at, l.id,
  c.uuid AS category_uuid, c.slug AS category_slug, c.name AS category_name,
  pc.uuid AS previous_category_uuid, pc.slug AS previous_category_slug, pc.name AS previous_category_name,
  u.uuid AS labeled_by_uuid
FROM category_labels l
JOIN categories c ON c.organization_id = l.organization_id AND c.id = l.category_id
LEFT JOIN categories pc ON pc.organization_id = l.organization_id AND pc.id = l.previous_category_id
JOIN users u ON u.id = l.labeled_by
WHERE l.organization_id = sqlc.arg(organization_id) AND l.id > sqlc.arg(after_id)::bigint
  AND (sqlc.narg(since)::timestamptz IS NULL OR l.created_at >= sqlc.narg(since)::timestamptz)
ORDER BY l.id
LIMIT sqlc.arg(limit_count);

-- name: CategorySuggestionSubject :one
SELECT s.id, s.uuid, s.topic, s.title, s.body FROM knowledge_suggestions s
WHERE s.organization_id = $1 AND s.uuid = $2;

-- name: CategoryCategoryRef :one
SELECT c.uuid, c.name, c.slug FROM categories c WHERE c.organization_id = $1 AND c.id = $2;

-- name: ArchiveCategoryDescendants :exec
WITH RECURSIVE down AS (
  SELECT c.id FROM categories c WHERE c.organization_id = sqlc.arg(organization_id) AND c.parent_id = sqlc.arg(id)::bigint
  UNION ALL
  SELECT ch.id FROM categories ch JOIN down ON ch.parent_id = down.id WHERE ch.organization_id = sqlc.arg(organization_id)
)
UPDATE categories u SET archived_at = now()
WHERE u.organization_id = sqlc.arg(organization_id) AND u.archived_at IS NULL AND u.id IN (SELECT down.id FROM down);
