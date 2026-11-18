-- CF-249: custom conversation statuses get a settings screen. Every
-- organization starts with "İşleniyor" (base category open); organizations
-- created later get it from the trigger below. Custom statuses map onto the
-- open, pending and resolved categories (the API enforces that; the CHECK of
-- 000034 stays wider for rows written before).
INSERT INTO conversation_statuses (organization_id, name, slug, category, color, sort_order)
SELECT o.id, 'İşleniyor', 'isleniyor', 'open', '#3b82f6', 0
FROM organizations o
ON CONFLICT (organization_id, slug) DO NOTHING;

CREATE OR REPLACE FUNCTION organizations_default_conversation_statuses()
RETURNS trigger AS $$
BEGIN
    INSERT INTO conversation_statuses (organization_id, name, slug, category, color, sort_order)
    VALUES (NEW.id, 'İşleniyor', 'isleniyor', 'open', '#3b82f6', 0)
    ON CONFLICT (organization_id, slug) DO NOTHING;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_organizations_default_conversation_statuses
    AFTER INSERT ON organizations
    FOR EACH ROW
    EXECUTE FUNCTION organizations_default_conversation_statuses();
