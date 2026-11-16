-- CF-234: masaüstü oturumlarını cihaz listesinde ayırt etmek için istemci
-- bilgisi. NULL = web / mobil (eski satırlar). Yenilemede yeni satıra taşınır.
ALTER TABLE refresh_tokens
    ADD COLUMN client      TEXT NULL,
    ADD COLUMN client_os   TEXT NULL,
    ADD COLUMN device_name TEXT NULL,
    ADD CONSTRAINT chk_refresh_tokens_client
        CHECK (client IS NULL OR client IN ('desktop')),
    ADD CONSTRAINT chk_refresh_tokens_client_os
        CHECK (client_os IS NULL OR client_os IN ('macos', 'windows', 'linux')),
    ADD CONSTRAINT chk_refresh_tokens_device_name
        CHECK (device_name IS NULL OR char_length(device_name) <= 64);
