-- CF-65/70/72/75/76/77: organization-scoped AI knowledge and evaluation workflow.
CREATE TABLE business_context_sections (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id), kind TEXT NOT NULL CHECK(kind IN ('company','products','pricing','refunds','tone','forbidden','other')),
 title TEXT NOT NULL CHECK(length(title) BETWEEN 1 AND 200), body TEXT NOT NULL CHECK(length(body)<=8000),
 active BOOLEAN NOT NULL DEFAULT true, sort_order INTEGER NOT NULL DEFAULT 0, revision INTEGER NOT NULL DEFAULT 1,
 updated_by BIGINT NOT NULL REFERENCES users(id), UNIQUE(organization_id,id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_business_context_sections_org ON business_context_sections(organization_id);
CREATE TRIGGER trg_business_context_sections_updated BEFORE UPDATE ON business_context_sections FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE business_context_versions (
 id BIGSERIAL PRIMARY KEY, organization_id BIGINT NOT NULL REFERENCES organizations(id), section_id BIGINT NOT NULL,
 revision INTEGER NOT NULL, kind TEXT NOT NULL, title TEXT NOT NULL, body TEXT NOT NULL, active BOOLEAN NOT NULL,
 sort_order INTEGER NOT NULL, updated_by BIGINT NOT NULL REFERENCES users(id),
 FOREIGN KEY(organization_id,section_id) REFERENCES business_context_sections(organization_id,id), UNIQUE(organization_id,section_id,revision),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_business_context_versions_org ON business_context_versions(organization_id);
CREATE TABLE knowledge_sources (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id), kind TEXT NOT NULL CHECK(kind IN ('url','file')),
 location TEXT NOT NULL, title TEXT NOT NULL CHECK(length(title) BETWEEN 1 AND 300), locale TEXT NOT NULL CHECK(locale IN ('tr','en')),
 author_id BIGINT NOT NULL REFERENCES users(id), enabled BOOLEAN NOT NULL DEFAULT true, max_depth INTEGER NOT NULL DEFAULT 1 CHECK(max_depth BETWEEN 0 AND 3),
 interval_hours INTEGER NOT NULL DEFAULT 24 CHECK(interval_hours BETWEEN 1 AND 8760), next_sync_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 lease_until TIMESTAMPTZ, status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','running','completed','failed')),
 last_error TEXT NOT NULL DEFAULT '', last_synced_at TIMESTAMPTZ, UNIQUE(organization_id,id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_knowledge_sources_org ON knowledge_sources(organization_id);
CREATE TRIGGER trg_knowledge_sources_updated BEFORE UPDATE ON knowledge_sources FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE knowledge_source_documents (
 id BIGSERIAL PRIMARY KEY, organization_id BIGINT NOT NULL REFERENCES organizations(id), source_id BIGINT NOT NULL,
 location TEXT NOT NULL, digest TEXT NOT NULL, article_id BIGINT NOT NULL, article_version INTEGER NOT NULL,
 FOREIGN KEY(organization_id,source_id) REFERENCES knowledge_sources(organization_id,id),
 FOREIGN KEY(organization_id,article_id) REFERENCES knowledge_articles(organization_id,id), UNIQUE(organization_id,source_id,location),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_knowledge_source_documents_org ON knowledge_source_documents(organization_id);
CREATE TRIGGER trg_knowledge_source_documents_updated BEFORE UPDATE ON knowledge_source_documents FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE knowledge_suggestions (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE, organization_id BIGINT NOT NULL REFERENCES organizations(id),
 topic TEXT NOT NULL CHECK(length(topic) BETWEEN 1 AND 200), conversation_id BIGINT NOT NULL, article_id BIGINT,
 title TEXT NOT NULL, body TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','rejected')),
 reviewed_by BIGINT REFERENCES users(id), FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id),
 FOREIGN KEY(organization_id,article_id) REFERENCES knowledge_articles(organization_id,id), UNIQUE(organization_id,conversation_id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_knowledge_suggestions_org ON knowledge_suggestions(organization_id);
CREATE TRIGGER trg_knowledge_suggestions_updated BEFORE UPDATE ON knowledge_suggestions FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE knowledge_translations (
 id BIGSERIAL PRIMARY KEY, organization_id BIGINT NOT NULL REFERENCES organizations(id), source_article_id BIGINT NOT NULL,
 target_article_id BIGINT NOT NULL, source_version INTEGER NOT NULL,
 FOREIGN KEY(organization_id,source_article_id) REFERENCES knowledge_articles(organization_id,id),
 FOREIGN KEY(organization_id,target_article_id) REFERENCES knowledge_articles(organization_id,id), UNIQUE(organization_id,target_article_id), CHECK(source_article_id<>target_article_id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_knowledge_translations_org ON knowledge_translations(organization_id);
CREATE TRIGGER trg_knowledge_translations_updated BEFORE UPDATE ON knowledge_translations FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE knowledge_reviews (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE, organization_id BIGINT NOT NULL REFERENCES organizations(id),
 article_id BIGINT NOT NULL, article_version INTEGER NOT NULL, reason TEXT NOT NULL CHECK(reason IN ('age','low_votes','contradiction','translation_outdated')),
 detail TEXT NOT NULL DEFAULT '', owner_id BIGINT NOT NULL REFERENCES users(id), status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open','done')),
 FOREIGN KEY(organization_id,article_id) REFERENCES knowledge_articles(organization_id,id), UNIQUE(organization_id,article_id,article_version,reason),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_knowledge_reviews_org ON knowledge_reviews(organization_id);
CREATE TRIGGER trg_knowledge_reviews_updated BEFORE UPDATE ON knowledge_reviews FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE ai_evaluation_cases (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE, organization_id BIGINT NOT NULL REFERENCES organizations(id),
 name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 200), transcript TEXT NOT NULL CHECK(length(transcript) BETWEEN 1 AND 18000),
 locale TEXT NOT NULL CHECK(locale IN ('tr','en')), expected JSONB NOT NULL, revision INTEGER NOT NULL DEFAULT 1,
 author_id BIGINT NOT NULL REFERENCES users(id), UNIQUE(organization_id,id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_ai_evaluation_cases_org ON ai_evaluation_cases(organization_id);
CREATE TRIGGER trg_ai_evaluation_cases_updated BEFORE UPDATE ON ai_evaluation_cases FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE ai_evaluations (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE, organization_id BIGINT NOT NULL REFERENCES organizations(id),
 config JSONB NOT NULL, config_revision INTEGER NOT NULL, baseline_uuid UUID, status TEXT NOT NULL DEFAULT 'running' CHECK(status IN ('running','completed','failed')),
 report JSONB NOT NULL DEFAULT '{}', author_id BIGINT NOT NULL REFERENCES users(id), UNIQUE(organization_id,uuid),
 FOREIGN KEY(organization_id,baseline_uuid) REFERENCES ai_evaluations(organization_id,uuid),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_ai_evaluations_org ON ai_evaluations(organization_id);
CREATE TRIGGER trg_ai_evaluations_updated BEFORE UPDATE ON ai_evaluations FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE ai_evaluation_results (
 id BIGSERIAL PRIMARY KEY, organization_id BIGINT NOT NULL REFERENCES organizations(id), evaluation_uuid UUID NOT NULL,
 case_uuid UUID NOT NULL, case_revision INTEGER NOT NULL, case_snapshot JSONB NOT NULL, output JSONB NOT NULL, score DOUBLE PRECISION NOT NULL CHECK(score BETWEEN 0 AND 1),
 run_uuid UUID REFERENCES ai_runs(uuid), error_code TEXT NOT NULL DEFAULT '',
 FOREIGN KEY(organization_id,evaluation_uuid) REFERENCES ai_evaluations(organization_id,uuid), UNIQUE(organization_id,evaluation_uuid,case_uuid),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_ai_evaluation_results_org ON ai_evaluation_results(organization_id);
CREATE TABLE ai_conversation_links (
 id BIGSERIAL PRIMARY KEY, organization_id BIGINT NOT NULL REFERENCES organizations(id), conversation_id BIGINT NOT NULL, similar_id BIGINT NOT NULL,
 score REAL NOT NULL CHECK(score BETWEEN 0 AND 1), FOREIGN KEY(organization_id,conversation_id) REFERENCES conversations(organization_id,id),
 FOREIGN KEY(organization_id,similar_id) REFERENCES conversations(organization_id,id), UNIQUE(organization_id,conversation_id,similar_id), CHECK(conversation_id<>similar_id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_ai_conversation_links_org ON ai_conversation_links(organization_id);
CREATE TRIGGER trg_ai_conversation_links_updated BEFORE UPDATE ON ai_conversation_links FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_business_context_immutable BEFORE UPDATE OR DELETE ON business_context_versions FOR EACH ROW EXECUTE FUNCTION knowledge_version_immutable();
