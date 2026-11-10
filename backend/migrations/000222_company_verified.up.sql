-- CF-206: the company id in a widget identify call can be covered by
-- user_hash (hex(HMAC-SHA256(secret, user_id + "\n" + company_id))).
-- verified_at is the last time a user proved the company's external id
-- that way; NULL for companies created or matched without proof (by name,
-- domain, the API or an agent). The panel shows the Verified badge on it.
ALTER TABLE companies ADD COLUMN verified_at TIMESTAMPTZ NULL;
