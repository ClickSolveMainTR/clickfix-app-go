-- Data-loss note: masaüstü oturumlarının istemci etiketi silinir; oturumlar
-- geçerli kalır ve cihaz listesinde web oturumu gibi görünür.
ALTER TABLE refresh_tokens
    DROP CONSTRAINT IF EXISTS chk_refresh_tokens_device_name,
    DROP CONSTRAINT IF EXISTS chk_refresh_tokens_client_os,
    DROP CONSTRAINT IF EXISTS chk_refresh_tokens_client,
    DROP COLUMN IF EXISTS device_name,
    DROP COLUMN IF EXISTS client_os,
    DROP COLUMN IF EXISTS client;
