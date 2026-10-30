-- Ported from takwim-app-go 000085: only organization staff grants.
-- 1. Clients -------------------------------------------------------------------
CREATE TABLE mcp_oauth_clients (
    id            BIGSERIAL    PRIMARY KEY,
    uuid          UUID         NOT NULL DEFAULT gen_random_uuid(),
    client_id     VARCHAR(64)  NOT NULL,
    client_name   VARCHAR(200) NOT NULL,
    redirect_uris TEXT[]       NOT NULL,
    created_ip    VARCHAR(64)  NULL,
    last_used_at  TIMESTAMPTZ  NULL,
    revoked_at    TIMESTAMPTZ  NULL,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_mcp_oauth_clients_uuid UNIQUE (uuid),
    CONSTRAINT uq_mcp_oauth_clients_client_id UNIQUE (client_id),
    CONSTRAINT ck_mcp_oauth_clients_redirect_uris CHECK (cardinality(redirect_uris) BETWEEN 1 AND 10)
);

CREATE INDEX idx_mcp_oauth_clients_created_at ON mcp_oauth_clients (created_at);

-- 2. Pending authorization requests ---------------------------------------------
CREATE TABLE mcp_oauth_auth_requests (
    id             UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id      VARCHAR(64)  NOT NULL REFERENCES mcp_oauth_clients (client_id) ON DELETE CASCADE,
    redirect_uri   TEXT         NOT NULL,
    code_challenge VARCHAR(128) NOT NULL,
    state          TEXT         NULL,
    -- The scopes the client asked for; consent may drop mcp:write.
    scopes         TEXT[]       NOT NULL,
    resource       VARCHAR(32)  NOT NULL,
    expires_at     TIMESTAMPTZ  NOT NULL,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_mcp_oauth_auth_requests_resource CHECK (resource = '/mcp/business')
);

CREATE INDEX idx_mcp_oauth_auth_requests_expires ON mcp_oauth_auth_requests (expires_at);

-- 3. Grants ("connected apps") ----------------------------------------------------
CREATE TABLE mcp_oauth_grants (
    id                   BIGSERIAL   PRIMARY KEY,
    uuid                 UUID        NOT NULL DEFAULT gen_random_uuid(),
    -- Exactly one subject: a staff user (platform, business) or a customer
    -- panel identity (customer).
    user_id              BIGINT      NULL REFERENCES users (id) ON DELETE CASCADE,
    -- The phone the customer proved on consent: the grant stops working
    -- when the identity's number changes (same rule as panel sessions).
    client_id            VARCHAR(64) NOT NULL REFERENCES mcp_oauth_clients (client_id) ON DELETE CASCADE,
    -- The business of a /mcp/business grant.
    organization_id      BIGINT      NULL REFERENCES organizations (id) ON DELETE CASCADE,
    resource             VARCHAR(32) NOT NULL,
    scopes               TEXT[]      NOT NULL,
    last_used_at         TIMESTAMPTZ NULL,
    revoked_at           TIMESTAMPTZ NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_mcp_oauth_grants_uuid UNIQUE (uuid),
    CONSTRAINT uq_mcp_oauth_grants_key UNIQUE NULLS NOT DISTINCT (user_id, client_id, resource, organization_id),
    CONSTRAINT ck_mcp_oauth_grants_resource CHECK (resource = '/mcp/business'),
    CONSTRAINT ck_mcp_oauth_grants_subject CHECK (user_id IS NOT NULL AND organization_id IS NOT NULL)
);

CREATE INDEX idx_mcp_oauth_grants_client ON mcp_oauth_grants (client_id);
CREATE INDEX idx_mcp_oauth_grants_user ON mcp_oauth_grants (user_id) WHERE user_id IS NOT NULL;
CREATE INDEX idx_mcp_oauth_grants_organization ON mcp_oauth_grants (organization_id) WHERE organization_id IS NOT NULL;

CREATE TRIGGER trg_mcp_oauth_grants_updated_at
    BEFORE UPDATE ON mcp_oauth_grants
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- 4. Authorization codes ----------------------------------------------------------
CREATE TABLE mcp_oauth_codes (
    code_hash      VARCHAR(64)  PRIMARY KEY,
    grant_id       BIGINT       NOT NULL REFERENCES mcp_oauth_grants (id) ON DELETE CASCADE,
    client_id      VARCHAR(64)  NOT NULL REFERENCES mcp_oauth_clients (client_id) ON DELETE CASCADE,
    redirect_uri   TEXT         NOT NULL,
    code_challenge VARCHAR(128) NOT NULL,
    resource       VARCHAR(32)  NOT NULL,
    scopes         TEXT[]       NOT NULL,
    expires_at     TIMESTAMPTZ  NOT NULL,
    used_at        TIMESTAMPTZ  NULL,
    -- The token family issued from this code; a replayed code revokes it.
    family         UUID         NULL,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_mcp_oauth_codes_used CHECK ((used_at IS NULL) = (family IS NULL))
);

CREATE INDEX idx_mcp_oauth_codes_expires ON mcp_oauth_codes (expires_at);
CREATE INDEX idx_mcp_oauth_codes_grant ON mcp_oauth_codes (grant_id);

-- 5. Tokens ---------------------------------------------------------------------
CREATE TABLE mcp_oauth_tokens (
    id           BIGSERIAL   PRIMARY KEY,
    token_hash   VARCHAR(64) NOT NULL,
    kind         VARCHAR(10) NOT NULL,
    family       UUID        NOT NULL,
    grant_id     BIGINT      NOT NULL REFERENCES mcp_oauth_grants (id) ON DELETE CASCADE,
    client_id    VARCHAR(64) NOT NULL REFERENCES mcp_oauth_clients (client_id) ON DELETE CASCADE,
    resource     VARCHAR(32) NOT NULL,
    scopes       TEXT[]      NOT NULL,
    expires_at   TIMESTAMPTZ NOT NULL,
    revoked_at   TIMESTAMPTZ NULL,
    last_used_at TIMESTAMPTZ NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_mcp_oauth_tokens_hash UNIQUE (token_hash),
    CONSTRAINT ck_mcp_oauth_tokens_kind CHECK (kind IN ('access', 'refresh'))
);

CREATE INDEX idx_mcp_oauth_tokens_family ON mcp_oauth_tokens (family);
CREATE INDEX idx_mcp_oauth_tokens_grant ON mcp_oauth_tokens (grant_id);
CREATE INDEX idx_mcp_oauth_tokens_client ON mcp_oauth_tokens (client_id);
CREATE INDEX idx_mcp_oauth_tokens_expires ON mcp_oauth_tokens (expires_at);

