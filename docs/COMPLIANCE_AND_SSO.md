# Organization compliance and SSO operations

## Data subject requests (CF-127)

`tenant.settings.write` grants access to `/v1/tenant/compliance/*`; requests require
`X-Organization`. Verify the requester's identity through your support process
before requesting erasure. POST `contacts/{uuid}/requests` with `kind: export` or
`erase`; poll `requests/{uuid}`. A durable request row survives Redis outages;
Asynq retries and a daily 02:00 UTC sweep recover unfinished work. Exports use
ioengine's streaming ZIP writer and private storage; attachments use UUID entry
names, never user filenames. Message JSON is paged into `messages/{uuid}.json`.
The archive includes contact, conversations, messages, linked AI runs, consents
and attachment bytes/manifest. The API download link is permission checked,
expires after seven days, and the daily sweep purges expired objects/versions.
ZIP uncompressed content is capped at 1 GiB; larger subjects require a separately
reviewed export job. Temp archives have private permissions and are removed.

Erasure purges attachment originals/thumbnails and their retained S3 versions,
old subject exports, message bodies/attributes, conversation free text/metadata,
identities, linked AI input/output/tool traces, AI job results, side conversations,
ratings, linked internal chat content and unpublished knowledge suggestions.
Conversation identity links are replaced with anonymous contacts; internal FK
rows remain as tombstones. Subject contacts are scrubbed and blocked. The request
record is the durable audit trail; activity events expose only request id/kind.
Late message/AI completion and stale contact/conversation edits are guarded by
Postgres triggers. AI runs from triage, reply, copilot and knowledge extraction
carry conversation_id at creation; ad-hoc AI input without a subject link cannot
be attributed automatically. Review copied data in manually authored knowledge,
engineering systems, outbox/dead-letter payloads, logs, external provider stores,
backups and legal holds in the controller's erasure procedure. This endpoint is
not a claim that every free-text copy in every integrated system can be located.

Retention is opt-in: PUT `retention` with conversation_days, attachment_days,
ai_log_days (0 disables a category) and action `anonymize` or `delete`. Only closed
conversations older than closed_at qualify. Both actions permanently dispose
content and unlink the subject; delete also hides the FK tombstone. Open
conversations are preserved. Attachment and AI trace retention are independent;
AI cost/token metadata stays for billing. Storage/search deletion errors leave a
request unfinished for retry. The same metadata keys permit idempotent retries.
Search documents are removed synchronously; future index rebuilds read scrubbed
records. Complete deployment acceptance must include a real S3 version purge and
Meilisearch check with the production versions and permissions.

Consent records are append-only through the API: purpose, granted,
notice_version, source and authenticated recorder. Revocation adds a false record;
it does not rewrite history. `/processing-inventory` is a Turkish VERBİS-friendly
draft. The controller must complete recipient/processor, transfer and legal basis
information and approve its actual privacy notice/DPA/subprocessor list.
Widget notice, cookie choices and legal page UI are owned by the frontend stream.

## Organization SSO/SCIM (CF-128)

Configure `/v1/tenant/security/sso` with tenant.settings.write. Grant the boolean
`security.sso` feature through an enterprise plan or entitlement override; it is
not enabled on every plan by migration. Client secrets are AES-GCM encrypted and
never returned. OIDC uses authorization code, S256 PKCE, a stored nonce and
RS256 signature/issuer/audience/expiry validation through platform/oidc.
SAML uses crewjam/saml v0.5.1 and uploaded IdP metadata, validates signed assertions
and InResponseTo; unsolicited IdP-initiated logins are rejected. Configure the
exact SP entity ID and ACS from the metadata endpoint. Encrypted SAML assertions
and signed SP AuthnRequests require an SP key/certificate extension; configure
signed, unencrypted IdP assertions for this version.

GET `/v1/public/sso/{organizationUUID}/start` sets a secure HttpOnly browser-bound
state cookie and returns authorization_url. OIDC uses the GET callback; SAML the
POST callback. A five-minute database state is consumed once. HTTPS is required,
including local IdP acceptance tests. PUBLIC callback URLs use AUTH_FRONTEND_URL
plus `/api/v1/public/sso/...` (same-origin BFF); preserve Set-Cookie, Cookie, form
bodies and query strings in that proxy. A frontend SSO entry flow must implement
this exchange before enabling enforced SSO. The API requires an SSO-authenticated
organization session to enable enforcement, proving an administrator can sign in first.

Successful sign-in requires an already provisioned active identity. SCIM
externalId must equal the OIDC `sub` or SAML NameID (stable immutable subject).
Tokens are deliberately organization-only and short-lived, with no global refresh
or platform/personal API privileges. They cannot authorize another organization
on the same global account. Reauthenticate through the IdP when they expire.
There is no email-only auto-link at callback. Provisioning credentials are a
trusted organization administration capability; audit issuance and rotation.

POST `/v1/tenant/security/scim-token` returns a high-entropy token once; only its
SHA-256 hash is stored. Rotation invalidates the old token immediately. The SCIM
base path is `/v1/scim/{organizationUUID}/v2`. Users and Groups support paged list,
GET, POST, PUT, PATCH and DELETE with SCIM envelopes. UserName/externalId equality
filters are supported. User mutations change organization membership active;
global names/email/password and external subject are not editable through an
organization. Deprovisioning removes membership and team memberships without
changing the global user. Owner/admin deprovision is rejected to prevent lockout.
Group PUT replaces provisioned members; PATCH supports displayName and member
arrays. Groups support displayName/externalId equality filters and member value
filters for PATCH removal. Other filter expressions return 400; configure the
IdP to send supported operations. Bulk, password changes, sorting and ETags are
explicitly unsupported. Deleting referenced teams can return conflict.

Enforced SSO and IP policies are checked after organization resolution, including
cache hits. IP rules use CIDRs and direct TCP peers by default; arbitrary forwarded
headers are ignored. For Dokploy/BFF, set SSO_TRUSTED_PROXY_CIDRS to the exact
trusted proxy subnet(s). Each proxy must append/overwrite X-Forwarded-For correctly.
A trusted hop walks the chain from right to left. Do not trust 0.0.0.0/0 or a
client-controlled network. Public start is rate limited (30/minute/peer), SCIM
120/minute/organization. Expired SSO state cleanup runs every 15 minutes.
