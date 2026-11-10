-- Data-loss note: forgets when companies were last proved by a signed
-- company id (CF-206); the companies and their contacts stay.
ALTER TABLE companies DROP COLUMN verified_at;
