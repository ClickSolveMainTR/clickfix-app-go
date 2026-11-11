-- Data-loss note: forgets when companies and contacts were last proved by
-- a signed company id (CF-206); the companies and their contacts stay.
ALTER TABLE contacts DROP COLUMN company_verified_at;
ALTER TABLE companies DROP COLUMN verified_at;
