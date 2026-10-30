-- CF-62/63/69/71/73. Portable pgvector fallback: stock postgres:18 has no vector extension.
CREATE TABLE knowledge_collections (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id),
 parent_id BIGINT, name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 200),
 slug TEXT NOT NULL CHECK(slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
 kind TEXT NOT NULL CHECK(kind IN ('collection','category')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,id), UNIQUE(organization_id,slug),
 FOREIGN KEY(organization_id,parent_id) REFERENCES knowledge_collections(organization_id,id),
 CHECK(parent_id IS NULL OR parent_id<>id)
);
CREATE INDEX idx_knowledge_collections_org ON knowledge_collections(organization_id);
CREATE TRIGGER trg_knowledge_collections_updated BEFORE UPDATE ON knowledge_collections FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE knowledge_articles (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id), collection_id BIGINT,
 slug TEXT NOT NULL CHECK(slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
 locale TEXT NOT NULL CHECK(locale IN ('tr','en')), title TEXT NOT NULL CHECK(length(title) BETWEEN 1 AND 300),
 body TEXT NOT NULL CHECK(length(body)<=200000), tags TEXT[] NOT NULL DEFAULT '{}',
 visibility TEXT NOT NULL CHECK(visibility IN ('public','internal')),
 version INTEGER NOT NULL DEFAULT 1 CHECK(version>0), published_version INTEGER, publication_revision BIGINT NOT NULL DEFAULT 0,
 author_id BIGINT NOT NULL REFERENCES users(id), deleted_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(organization_id,id), UNIQUE(organization_id,locale,slug),
 FOREIGN KEY(organization_id,collection_id) REFERENCES knowledge_collections(organization_id,id)
);
CREATE INDEX idx_knowledge_articles_org ON knowledge_articles(organization_id,locale,published_version);
CREATE TRIGGER trg_knowledge_articles_updated BEFORE UPDATE ON knowledge_articles FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE knowledge_article_versions (
 id BIGSERIAL PRIMARY KEY, organization_id BIGINT NOT NULL REFERENCES organizations(id), article_id BIGINT NOT NULL,
 version INTEGER NOT NULL, title TEXT NOT NULL, body TEXT NOT NULL, visibility TEXT NOT NULL,
 author_id BIGINT NOT NULL REFERENCES users(id), created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,article_id) REFERENCES knowledge_articles(organization_id,id),
 UNIQUE(organization_id,article_id,version)
);
ALTER TABLE knowledge_articles ADD CONSTRAINT fk_knowledge_published_version FOREIGN KEY(organization_id,id,published_version)
 REFERENCES knowledge_article_versions(organization_id,article_id,version) DEFERRABLE INITIALLY DEFERRED;
CREATE FUNCTION knowledge_version_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 RAISE EXCEPTION 'knowledge versions are immutable' USING ERRCODE='23514'; END $$;
CREATE TRIGGER trg_knowledge_version_immutable BEFORE UPDATE OR DELETE ON knowledge_article_versions FOR EACH ROW EXECUTE FUNCTION knowledge_version_immutable();
CREATE TABLE knowledge_chunks (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id), article_id BIGINT NOT NULL,
 version INTEGER NOT NULL, ordinal INTEGER NOT NULL, content TEXT NOT NULL,
 embedding REAL[] NOT NULL CHECK(cardinality(embedding)>0), embedding_model TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,article_id,version) REFERENCES knowledge_article_versions(organization_id,article_id,version),
 UNIQUE(organization_id,article_id,ordinal)
);
CREATE INDEX idx_knowledge_chunks_org ON knowledge_chunks(organization_id,article_id,version);
CREATE FUNCTION ai_cosine(a REAL[], b REAL[]) RETURNS DOUBLE PRECISION LANGUAGE sql IMMUTABLE STRICT AS $$
 SELECT CASE WHEN cardinality(a)<>cardinality(b) THEN 0 ELSE COALESCE(sum(x::double precision*y)/nullif(sqrt(sum(x::double precision*x))*sqrt(sum(y::double precision*y)),0),0) END
 FROM unnest(a,b) t(x,y)
$$;
CREATE TABLE ai_policies (
 organization_id BIGINT PRIMARY KEY REFERENCES organizations(id), enabled BOOLEAN NOT NULL DEFAULT false,
 mask_pii BOOLEAN NOT NULL DEFAULT true, allow_unmask BOOLEAN NOT NULL DEFAULT false,
 daily_calls INTEGER NOT NULL DEFAULT 100 CHECK(daily_calls BETWEEN 1 AND 100000),
 concurrency INTEGER NOT NULL DEFAULT 3 CHECK(concurrency BETWEEN 1 AND 20),
 daily_budget_micros BIGINT NOT NULL DEFAULT 10000000 CHECK(daily_budget_micros>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_ai_policies_updated BEFORE UPDATE ON ai_policies FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TABLE ai_runs (
 id BIGSERIAL PRIMARY KEY, uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id), actor_id BIGINT REFERENCES users(id),
 task TEXT NOT NULL CHECK(task IN ('triage','reply','complex','embedding')), provider TEXT NOT NULL, model TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('running','completed','failed','unknown','blocked')),
 input TEXT NOT NULL, output TEXT NOT NULL DEFAULT '', retrieved_chunks JSONB NOT NULL DEFAULT '[]',
 tool_calls JSONB NOT NULL DEFAULT '[]', input_tokens BIGINT NOT NULL DEFAULT 0 CHECK(input_tokens>=0),
 output_tokens BIGINT NOT NULL DEFAULT 0 CHECK(output_tokens>=0),
 cost_micros BIGINT NOT NULL DEFAULT 0 CHECK(cost_micros>=0), reserved_cost_micros BIGINT NOT NULL DEFAULT 0 CHECK(reserved_cost_micros>=0),
 reserved_tokens BIGINT NOT NULL DEFAULT 0 CHECK(reserved_tokens>=0),
 reserved_credits BIGINT NOT NULL DEFAULT 0 CHECK(reserved_credits>=0), charged_credits BIGINT NOT NULL DEFAULT 0 CHECK(charged_credits>=0),
 latency_ms BIGINT NOT NULL DEFAULT 0, error_code TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), finished_at TIMESTAMPTZ,
 UNIQUE(organization_id,id)
);
CREATE INDEX idx_ai_runs_org ON ai_runs(organization_id,created_at DESC);
CREATE INDEX idx_ai_runs_active ON ai_runs(organization_id,status) WHERE status='running';

-- CF-37 compatibility: main's ledger works before commerce lands; when commerce's
-- expires_at/allocations exist, reserve FIFO lots and restore unused credits with
-- the original expiry/invoice lineage. Dynamic SQL avoids requiring migration 46
-- during sqlc generation and does not duplicate any commerce-owned schema.
CREATE TABLE ai_credit_reservation_lots (
 organization_id BIGINT NOT NULL REFERENCES organizations(id), debit_id BIGINT NOT NULL, credit_id BIGINT NOT NULL,
 ordinal INTEGER NOT NULL, amount BIGINT NOT NULL CHECK(amount>0), expires_at TIMESTAMPTZ, refundable BOOLEAN NOT NULL,
 PRIMARY KEY(debit_id,credit_id),
 FOREIGN KEY(organization_id,debit_id) REFERENCES billing_credit_ledger(organization_id,id),
 FOREIGN KEY(organization_id,credit_id) REFERENCES billing_credit_ledger(organization_id,id)
);
CREATE INDEX idx_ai_credit_reservation_lots_org ON ai_credit_reservation_lots(organization_id);
CREATE TRIGGER trg_ai_credit_reservation_lots_immutable BEFORE UPDATE OR DELETE ON ai_credit_reservation_lots
 FOR EACH ROW EXECUTE FUNCTION billing_guard_append_only();
CREATE FUNCTION ai_wallet_reserve(org_id BIGINT, run_uuid UUID, credits BIGINT) RETURNS BOOLEAN LANGUAGE plpgsql AS $$
DECLARE lot RECORD; debit BIGINT; sub BIGINT; balance BIGINT; remaining BIGINT; amount BIGINT; ordinal INTEGER:=0;
 compat BOOLEAN:=to_regclass('public.billing_credit_allocations') IS NOT NULL;
BEGIN
 IF credits<=0 THEN RAISE EXCEPTION 'invalid reservation' USING ERRCODE='23514'; END IF;
 PERFORM id FROM organizations WHERE id=org_id FOR UPDATE;
 SELECT id INTO debit FROM billing_credit_ledger WHERE organization_id=org_id AND unit='ai_credits' AND idempotency_key='ai:reserve:'||run_uuid;
 IF FOUND THEN RETURN true; END IF;
 IF compat THEN
  FOR lot IN EXECUTE 'SELECT l.*, (l.delta-COALESCE((SELECT sum(a.amount) FROM billing_credit_allocations a WHERE a.organization_id=l.organization_id AND a.credit_id=l.id),0))::bigint AS remaining FROM billing_credit_ledger l WHERE l.organization_id=$1 AND l.unit=''ai_credits'' AND l.delta>0 AND l.expires_at<=now() AND NOT EXISTS(SELECT 1 FROM billing_credit_ledger r WHERE r.reverses_id=l.id) ORDER BY l.expires_at,l.id' USING org_id LOOP
   IF lot.remaining>0 THEN
    INSERT INTO billing_credit_ledger(organization_id,subscription_id,unit,delta,reason,idempotency_key,reverses_id)
     VALUES(org_id,lot.subscription_id,'ai_credits',-lot.remaining,'Credit expiry','expiry:'||lot.id,lot.id);
   END IF;
  END LOOP;
 END IF;
 SELECT COALESCE(sum(delta),0) INTO balance FROM billing_credit_ledger WHERE organization_id=org_id AND unit='ai_credits';
 IF balance<credits THEN RETURN false; END IF;
 SELECT id INTO sub FROM billing_subscriptions WHERE organization_id=org_id ORDER BY id DESC LIMIT 1;
 IF sub IS NULL THEN RETURN false; END IF;
 INSERT INTO billing_credit_ledger(organization_id,subscription_id,unit,delta,reason,idempotency_key)
 VALUES(org_id,sub,'ai_credits',-credits,'AI run reservation','ai:reserve:'||run_uuid) RETURNING id INTO debit;
 IF compat THEN
  remaining:=credits;
  FOR lot IN EXECUTE 'SELECT l.*, (l.delta-COALESCE((SELECT sum(a.amount) FROM billing_credit_allocations a WHERE a.organization_id=l.organization_id AND a.credit_id=l.id),0))::bigint AS remaining FROM billing_credit_ledger l WHERE l.organization_id=$1 AND l.unit=''ai_credits'' AND l.delta>0 AND (l.expires_at IS NULL OR l.expires_at>now()) AND NOT EXISTS(SELECT 1 FROM billing_credit_ledger r WHERE r.reverses_id=l.id) ORDER BY l.expires_at NULLS LAST,l.id' USING org_id LOOP
   amount:=LEAST(remaining,lot.remaining);
   IF amount<=0 THEN CONTINUE; END IF;
   EXECUTE 'INSERT INTO billing_credit_allocations(organization_id,debit_id,credit_id,amount) VALUES($1,$2,$3,$4)' USING org_id,debit,lot.id,amount;
   INSERT INTO ai_credit_reservation_lots(organization_id,debit_id,credit_id,ordinal,amount,expires_at,refundable)
    VALUES(org_id,debit,lot.id,ordinal,amount,lot.expires_at,lot.idempotency_key LIKE 'purchase:%' OR lot.idempotency_key LIKE 'refund-restore:%');
   ordinal:=ordinal+1; remaining:=remaining-amount;
   EXIT WHEN remaining=0;
  END LOOP;
  IF remaining<>0 THEN RAISE EXCEPTION 'unfunded wallet lots' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN true;
END $$;
CREATE FUNCTION ai_wallet_release(org_id BIGINT, run_uuid UUID, credits BIGINT) RETURNS VOID LANGUAGE plpgsql AS $$
DECLARE lot RECORD; debit BIGINT; sub BIGINT; amount BIGINT; remaining BIGINT:=credits; entry_key TEXT;
BEGIN
 IF credits<=0 THEN RETURN; END IF;
 PERFORM id FROM organizations WHERE id=org_id FOR UPDATE;
 SELECT id,subscription_id INTO debit,sub FROM billing_credit_ledger
  WHERE organization_id=org_id AND unit='ai_credits' AND idempotency_key='ai:reserve:'||run_uuid AND -delta>=credits;
 IF NOT FOUND THEN RAISE EXCEPTION 'reservation not found' USING ERRCODE='23514'; END IF;
 FOR lot IN SELECT a.*,l.subscription_id,l.invoice_id FROM ai_credit_reservation_lots a
  JOIN billing_credit_ledger l ON l.organization_id=a.organization_id AND l.id=a.credit_id
  WHERE a.organization_id=org_id AND a.debit_id=debit ORDER BY a.ordinal DESC LOOP
  amount:=LEAST(remaining,lot.amount);
  entry_key:=CASE WHEN lot.refundable THEN 'refund-restore:ai:' ELSE 'ai:release:' END||run_uuid||':'||lot.credit_id;
  -- A credit whose original expiry passed during the run stays expired. Pair its
  -- restoration with an immediate expiry debit so the authoritative sum stays correct.
  EXECUTE 'INSERT INTO billing_credit_ledger(organization_id,subscription_id,invoice_id,unit,delta,reason,idempotency_key,expires_at) VALUES($1,$2,$3,''ai_credits'',$4,''Unused AI reservation release'',$5,$6) ON CONFLICT(organization_id,unit,idempotency_key) DO NOTHING'
   USING org_id,lot.subscription_id,lot.invoice_id,amount,entry_key,lot.expires_at;
  IF lot.expires_at IS NOT NULL AND lot.expires_at<=now() THEN
   INSERT INTO billing_credit_ledger(organization_id,subscription_id,unit,delta,reason,idempotency_key,reverses_id)
    SELECT org_id,lot.subscription_id,'ai_credits',-amount,'Credit expiry','expiry:'||id,id FROM billing_credit_ledger
     WHERE organization_id=org_id AND unit='ai_credits' AND idempotency_key=entry_key
    ON CONFLICT(organization_id,unit,idempotency_key) DO NOTHING;
  END IF;
  remaining:=remaining-amount; EXIT WHEN remaining=0;
 END LOOP;
 IF remaining>0 THEN
  INSERT INTO billing_credit_ledger(organization_id,subscription_id,unit,delta,reason,idempotency_key)
   VALUES(org_id,sub,'ai_credits',remaining,'Unused AI reservation release','ai:release:'||run_uuid)
   ON CONFLICT(organization_id,unit,idempotency_key) DO NOTHING;
 END IF;
END $$;
