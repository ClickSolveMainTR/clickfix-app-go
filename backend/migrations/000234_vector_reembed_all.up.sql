-- CF-223: the re-embedding job rebuilds every vector table when the embedding
-- model changes. Category labels are append-only; besides the late first
-- vectorization they may now also get a new vector of another model. Every
-- other column stays immutable.
CREATE OR REPLACE FUNCTION category_labels_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='UPDATE' AND NEW.embedding IS NOT NULL
    AND (OLD.embedding IS NULL OR NEW.embedding_model IS DISTINCT FROM OLD.embedding_model)
    AND (OLD.id, OLD.uuid, OLD.organization_id, OLD.department_id, OLD.subject_type, OLD.subject_uuid, OLD.action, OLD.content,
         OLD.previous_category_id, OLD.previous_source, OLD.previous_confidence, OLD.category_id, OLD.labeled_by, OLD.created_at)
     IS NOT DISTINCT FROM
        (NEW.id, NEW.uuid, NEW.organization_id, NEW.department_id, NEW.subject_type, NEW.subject_uuid, NEW.action, NEW.content,
         NEW.previous_category_id, NEW.previous_source, NEW.previous_confidence, NEW.category_id, NEW.labeled_by, NEW.created_at) THEN
  RETURN NEW; -- (re)vectorization only
 END IF;
 RAISE EXCEPTION 'category labels are append-only' USING ERRCODE='23514'; END $$;
