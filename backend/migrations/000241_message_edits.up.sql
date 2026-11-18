-- Gönderilen mesajı düzenleme (CF-248): temsilci kendi giden mesajını kısa
-- bir süre içinde düzeltebilir. messages.content son hâli, messages.edited_at
-- son düzenleme zamanını tutar; her düzenlemenin önceki metni burada kalır
-- (düzenleme geçmişi). Mesaj silinirse geçmişi de silinir.
CREATE TABLE message_edits (
    id               BIGSERIAL    PRIMARY KEY,
    uuid             UUID         NOT NULL DEFAULT gen_random_uuid(),
    organization_id  BIGINT       NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    message_id       BIGINT       NOT NULL,
    editor_user_id   BIGINT       NULL REFERENCES users (id) ON DELETE SET NULL,
    previous_content TEXT         NOT NULL CHECK (char_length(previous_content) <= 65536),
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT message_edits_uuid_uq UNIQUE (uuid),
    CONSTRAINT message_edits_message_fk FOREIGN KEY (organization_id, message_id)
        REFERENCES messages (organization_id, id) ON DELETE CASCADE
);

CREATE INDEX message_edits_message_idx ON message_edits (organization_id, message_id, id);
