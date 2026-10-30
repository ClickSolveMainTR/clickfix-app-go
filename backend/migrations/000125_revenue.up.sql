-- CF-113: immutable MRR snapshots preserve price/seat/add-on history from installation onward.
CREATE TABLE billing_revenue_snapshots (
 id BIGSERIAL PRIMARY KEY,organization_id BIGINT NOT NULL REFERENCES organizations(id),
 subscription_id BIGINT,plan_code TEXT NOT NULL,currency TEXT NOT NULL,
 mrr_minor NUMERIC(24,4) NOT NULL CHECK(mrr_minor>=0),status TEXT NOT NULL,
 occurred_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(organization_id,subscription_id) REFERENCES billing_subscriptions(organization_id,id)
);
CREATE INDEX idx_billing_revenue_snapshots_org ON billing_revenue_snapshots(organization_id,occurred_at DESC,id DESC);
CREATE TABLE billing_revenue_coverage (singleton BOOLEAN PRIMARY KEY DEFAULT true CHECK(singleton),starts_at TIMESTAMPTZ NOT NULL DEFAULT now());
INSERT INTO billing_revenue_coverage(singleton) VALUES(true);
CREATE FUNCTION billing_capture_revenue(org_id BIGINT) RETURNS void LANGUAGE plpgsql AS $$
DECLARE s RECORD; revenue NUMERIC; seats BIGINT;
BEGIN
 -- Organization row serializes membership, subscription and add-on changes.
 PERFORM 1 FROM organizations WHERE id=org_id FOR UPDATE;
 SELECT x.*,p.currency,p.interval,p.amount_minor,p.seat_amount_minor,b.code INTO s
 FROM billing_subscriptions x JOIN billing_prices p ON p.id=x.price_id
 JOIN billing_plan_versions v ON v.id=x.plan_version_id JOIN billing_plans b ON b.id=v.plan_id
 WHERE x.organization_id=org_id ORDER BY x.id DESC LIMIT 1;
 IF NOT FOUND THEN RETURN; END IF;
 SELECT greatest(count(*),1) INTO seats FROM organization_members WHERE organization_id=org_id;
 revenue:=0;
 IF s.status IN ('active','past_due','grace') THEN
 revenue:=(s.amount_minor+s.seat_amount_minor*greatest(seats,s.contracted_seats)+COALESCE((SELECT sum(i.quantity*i.unit_amount_minor) FROM billing_subscription_items i WHERE i.organization_id=org_id AND i.subscription_id=s.id AND i.currency=s.currency),0))::NUMERIC;
 IF s.interval='yearly' THEN revenue:=revenue/12; END IF;
 END IF;
 INSERT INTO billing_revenue_snapshots(organization_id,subscription_id,plan_code,currency,mrr_minor,status)
 VALUES(org_id,s.id,s.code,s.currency,revenue,s.status);
END $$;
CREATE FUNCTION billing_revenue_changed() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN PERFORM billing_capture_revenue(OLD.organization_id); RETURN OLD; END IF;
 PERFORM billing_capture_revenue(NEW.organization_id); RETURN NEW;
END $$;
CREATE TRIGGER trg_revenue_subscription AFTER INSERT OR UPDATE ON billing_subscriptions FOR EACH ROW EXECUTE FUNCTION billing_revenue_changed();
CREATE TRIGGER trg_revenue_items AFTER INSERT OR UPDATE OR DELETE ON billing_subscription_items FOR EACH ROW EXECUTE FUNCTION billing_revenue_changed();
CREATE TRIGGER trg_revenue_seats AFTER INSERT OR UPDATE OR DELETE ON organization_members FOR EACH ROW EXECUTE FUNCTION billing_revenue_changed();
SELECT billing_capture_revenue(id) FROM organizations WHERE deleted_at IS NULL;
-- Baseline belongs to the coverage start, so the first report includes existing subscriptions.
UPDATE billing_revenue_snapshots SET occurred_at=(SELECT starts_at FROM billing_revenue_coverage);
