CREATE OR REPLACE FUNCTION category_labels_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF TG_OP='UPDATE' AND OLD.embedding IS NULL AND NEW.embedding IS NOT NULL
    AND (OLD.content, OLD.category_id, OLD.action, OLD.labeled_by, OLD.created_at) = (NEW.content, NEW.category_id, NEW.action, NEW.labeled_by, NEW.created_at) THEN
  RETURN NEW; -- late vectorization only
 END IF;
 RAISE EXCEPTION 'category labels are append-only' USING ERRCODE='23514'; END $$;
