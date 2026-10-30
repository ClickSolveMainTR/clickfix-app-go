-- CF-21: metering ledger, immutable period settlement and invoice handoff.
INSERT INTO billing_features(key,kind,label_tr,label_en,unit,aggregation,period) VALUES
 ('ai.resolutions','limit','AI çözümü','AI resolutions','resolution','sum','month'),
 ('whatsapp.templates','limit','WhatsApp şablonları','WhatsApp templates','message','sum','month'),
 ('sms','limit','SMS','SMS','message','sum','month') ON CONFLICT(key) DO NOTHING;
CREATE INDEX idx_billing_usage_daily ON billing_usage_records(organization_id,occurred_at,feature_id);
CREATE TABLE billing_usage_settlements (
 id BIGSERIAL PRIMARY KEY,
 uuid UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 organization_id BIGINT NOT NULL REFERENCES organizations(id),
 usage_period_id BIGINT NOT NULL,
 feature_id BIGINT NOT NULL REFERENCES billing_features(id),
 quantity BIGINT NOT NULL CHECK(quantity>=0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY(organization_id,usage_period_id) REFERENCES billing_usage_periods(organization_id,id),
 UNIQUE(organization_id,usage_period_id,feature_id)
);
CREATE INDEX idx_billing_usage_settlements_org ON billing_usage_settlements(organization_id);
CREATE TRIGGER trg_billing_usage_settlement_immutable BEFORE UPDATE OR DELETE ON billing_usage_settlements
 FOR EACH ROW EXECUTE FUNCTION billing_guard_append_only();
CREATE FUNCTION billing_usage_open_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 -- Let the composite FK report missing/cross-organization periods.
 IF NOT EXISTS(SELECT 1 FROM billing_usage_periods WHERE organization_id=NEW.organization_id AND id=NEW.usage_period_id) THEN RETURN NEW; END IF;
 IF NOT EXISTS(SELECT 1 FROM billing_usage_periods WHERE organization_id=NEW.organization_id
 AND id=NEW.usage_period_id AND closed_at IS NULL AND NEW.occurred_at>=starts_at AND NEW.occurred_at<ends_at) THEN
 RAISE EXCEPTION 'usage period is closed or event is outside period' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER trg_billing_usage_open BEFORE INSERT ON billing_usage_records
 FOR EACH ROW EXECUTE FUNCTION billing_usage_open_guard();
CREATE TABLE billing_usage_denials (
 organization_id BIGINT NOT NULL REFERENCES organizations(id),
 event_key TEXT NOT NULL,
 feature TEXT NOT NULL,
 quantity BIGINT NOT NULL,
 code TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(organization_id,event_key)
);
