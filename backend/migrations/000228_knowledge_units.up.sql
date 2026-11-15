-- CF-216: knowledge units, answer cache, case memory and unanswered questions.
-- Vectors are pgvector vector(1536) (CF-214), compared with cosine distance
-- (<=>).
-- department_id has no foreign key until departments land (CF-215); NULL means
-- organization wide. brand_id NULL also means organization wide.

-- A unit is one question/answer pair taken from a published article or an
-- approved knowledge suggestion. A new source version replaces its units, and
-- deleting a unit deletes the cache entries that point at it.
CREATE TABLE knowledge_units (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 brand_id UUID, department_id BIGINT,
 article_id BIGINT, suggestion_id BIGINT,
 source_version INTEGER NOT NULL CHECK(source_version>0),
 source_hash TEXT NOT NULL CHECK(length(source_hash)=64),
 ordinal INTEGER NOT NULL CHECK(ordinal BETWEEN 0 AND 49),
 locale TEXT NOT NULL CHECK(locale IN ('tr','en')),
 visibility TEXT NOT NULL CHECK(visibility IN ('public','internal')),
 category TEXT NOT NULL DEFAULT '' CHECK(length(category)<=100),
 question TEXT NOT NULL CHECK(length(question) BETWEEN 1 AND 500),
 variants TEXT[] NOT NULL DEFAULT '{}' CHECK(cardinality(variants)<=8),
 answer TEXT NOT NULL CHECK(length(answer) BETWEEN 1 AND 4000),
 embedding_model TEXT NOT NULL CHECK(length(embedding_model) BETWEEN 1 AND 200),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,id),
 CHECK((article_id IS NULL) <> (suggestion_id IS NULL)),
 FOREIGN KEY(organization_id,brand_id) REFERENCES brands(organization_id,uuid) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,article_id) REFERENCES knowledge_articles(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(suggestion_id) REFERENCES knowledge_suggestions(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX uq_knowledge_units_article ON knowledge_units(organization_id,article_id,ordinal) WHERE article_id IS NOT NULL;
CREATE UNIQUE INDEX uq_knowledge_units_suggestion ON knowledge_units(organization_id,suggestion_id,ordinal) WHERE suggestion_id IS NOT NULL;
CREATE INDEX idx_knowledge_units_scope ON knowledge_units(organization_id,locale,brand_id,department_id);
CREATE TRIGGER trg_knowledge_units_updated BEFORE UPDATE ON knowledge_units FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- The question and each variant are embedded separately; a unit matches on
-- its best phrasing.
CREATE TABLE knowledge_unit_questions (
 id BIGSERIAL PRIMARY KEY,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 unit_id BIGINT NOT NULL, ordinal INTEGER NOT NULL CHECK(ordinal BETWEEN 0 AND 8),
 content TEXT NOT NULL CHECK(length(content) BETWEEN 1 AND 500),
 embedding vector(1536) NOT NULL,
 FOREIGN KEY(organization_id,unit_id) REFERENCES knowledge_units(organization_id,id) ON DELETE CASCADE,
 UNIQUE(organization_id,unit_id,ordinal)
);

-- Exact repeat questions skip even the embedding call. scope_key is
-- "<brand uuid or ->:<department id or ->", so NULL scopes compare equal.
CREATE TABLE ai_answer_cache (
 id BIGSERIAL PRIMARY KEY,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 unit_id BIGINT NOT NULL,
 audience TEXT NOT NULL CHECK(audience IN ('customer','agent')),
 locale TEXT NOT NULL CHECK(locale IN ('tr','en')),
 scope_key TEXT NOT NULL CHECK(length(scope_key) BETWEEN 3 AND 80),
 question_hash TEXT NOT NULL CHECK(length(question_hash)=64),
 score REAL NOT NULL,
 hits BIGINT NOT NULL DEFAULT 0 CHECK(hits>=0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), last_hit_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,unit_id) REFERENCES knowledge_units(organization_id,id) ON DELETE CASCADE,
 UNIQUE(organization_id,audience,locale,scope_key,question_hash)
);
CREATE INDEX idx_ai_answer_cache_unit ON ai_answer_cache(organization_id,unit_id);

-- Daily lookup counters: hit rate = (exact + semantic) / all lookups.
CREATE TABLE ai_answer_cache_stats (
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 day DATE NOT NULL,
 audience TEXT NOT NULL CHECK(audience IN ('customer','agent')),
 exact_hits BIGINT NOT NULL DEFAULT 0 CHECK(exact_hits>=0),
 semantic_hits BIGINT NOT NULL DEFAULT 0 CHECK(semantic_hits>=0),
 misses BIGINT NOT NULL DEFAULT 0 CHECK(misses>=0),
 PRIMARY KEY(organization_id,day,audience)
);

-- Anonymized problem/resolution summaries of closed conversations. The
-- customer AI reads only approved rows; approval requires the anonymizer pass.
CREATE TABLE ai_case_memories (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 conversation_id BIGINT NOT NULL,
 brand_id UUID, department_id BIGINT,
 locale TEXT NOT NULL CHECK(locale IN ('tr','en')),
 category TEXT NOT NULL DEFAULT '' CHECK(length(category)<=100),
 problem TEXT NOT NULL CHECK(length(problem) BETWEEN 1 AND 1000),
 resolution TEXT NOT NULL CHECK(length(resolution) BETWEEN 1 AND 2000),
 anonymized BOOLEAN NOT NULL DEFAULT false,
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','rejected')),
 embedding vector(1536) NOT NULL,
 embedding_model TEXT NOT NULL CHECK(length(embedding_model) BETWEEN 1 AND 200),
 reviewed_by BIGINT REFERENCES users(id), reviewed_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK(status<>'approved' OR anonymized),
 FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id) ON DELETE CASCADE,
 FOREIGN KEY(organization_id,brand_id) REFERENCES brands(organization_id,uuid) ON DELETE CASCADE,
 UNIQUE(organization_id,conversation_id)
);
CREATE INDEX idx_ai_case_memories_scope ON ai_case_memories(organization_id,status,locale);
CREATE TRIGGER trg_ai_case_memories_updated BEFORE UPDATE ON ai_case_memories FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Questions no unit, case or knowledge source answered (knowledge gaps).
CREATE TABLE knowledge_gap_questions (
 id BIGSERIAL PRIMARY KEY,
 organization_id BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
 audience TEXT NOT NULL CHECK(audience IN ('customer','agent')),
 locale TEXT NOT NULL CHECK(locale IN ('tr','en')),
 scope_key TEXT NOT NULL CHECK(length(scope_key) BETWEEN 3 AND 80),
 brand_id UUID, department_id BIGINT,
 question_hash TEXT NOT NULL CHECK(length(question_hash)=64),
 question TEXT NOT NULL CHECK(length(question) BETWEEN 1 AND 500),
 category TEXT NOT NULL DEFAULT '' CHECK(length(category)<=100),
 conversation_id BIGINT,
 occurrences BIGINT NOT NULL DEFAULT 1 CHECK(occurrences>0),
 first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(), last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id) ON DELETE SET NULL (conversation_id),
 UNIQUE(organization_id,audience,locale,scope_key,question_hash)
);
CREATE INDEX idx_knowledge_gap_questions_seen ON knowledge_gap_questions(organization_id,last_seen_at DESC);
