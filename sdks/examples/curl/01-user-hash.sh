# Adım 1: user_hash = hex(HMAC-SHA256(kimlik_anahtari, user_id)); yalnız sunucuda.
USER_ID="1001"
printf '%s' "$USER_ID" | openssl dgst -sha256 -hmac "$CLICKFIX_IDENTITY_SECRET" | sed 's/^.* //'
