// Package model holds the types of the MCP OAuth 2.1 authorization server
// (docs/design/mcp.md): the three MCP layers, their scopes and the OAuth
// protocol shapes.
package model

import (
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/google/uuid"
)

// Lifetimes of the OAuth artifacts.
const (
	AccessTTL      = time.Hour
	RefreshTTL     = 30 * 24 * time.Hour
	CodeTTL        = 5 * time.Minute
	AuthRequestTTL = 15 * time.Minute
)

// Per-IP limits per hour of the public endpoints.
const (
	RegisterPerHour = 20
	// RegisterGlobalPerHour caps anonymous client registration across all
	// addresses, so a distributed caller cannot fill the client table.
	RegisterGlobalPerHour = 300
	AuthorizePerHour      = 60
	TokenPerHour          = 120
	RevokePerHour         = 120
	RateWindow            = time.Hour
)

// Scopes. mcp:read is always granted; mcp:write is opt-in on the consent
// screen and lets the write tools of a layer run.
const (
	ScopeRead  = "mcp:read"
	ScopeWrite = "mcp:write"
)

// Scopes lists every scope the server knows, in display order.
var Scopes = []string{ScopeRead, ScopeWrite}

// HasScope reports whether scopes include s.
func HasScope(scopes []string, s string) bool { return slices.Contains(scopes, s) }

// Token kinds (mcp_oauth_tokens.kind).
const (
	KindAccess  = "access"
	KindRefresh = "refresh"
)

// Grant types of the token endpoint.
const (
	GrantAuthorizationCode = "authorization_code"
	GrantRefreshToken      = "refresh_token"
)

// Layers of the MCP server: who connects and what the token is bound to.
const (
	// LayerPlatform: Takwim management, platform users with
	// platform.mcp.connect. The token is bound to the user.
	LayerPlatform = "platform"
	// LayerBusiness: one business, members holding tenant.mcp.connect there.
	// The token is bound to the user and the organization chosen on consent.
	LayerBusiness = "business"
	// LayerCustomer: the customer panel person (customer identity), their
	// own records across businesses.
	LayerCustomer = "customer"
)

// Layers lists the layers in display order.
var Layers = []string{LayerBusiness}

// The protected resources (RFC 8707 audiences): one MCP endpoint per layer.
const (
	ResourcePlatform = "/mcp/platform"
	ResourceBusiness = "/mcp/business"
	ResourceCustomer = "/mcp/customer"
)

// Resources lists every MCP endpoint path.
var Resources = []string{ResourceBusiness}

// IsResource reports whether path is one of the MCP endpoints.
func IsResource(path string) bool { return slices.Contains(Resources, path) }

// LayerOf maps an MCP endpoint path to its layer ("" for unknown paths).
func LayerOf(resource string) string {
	switch resource {
	case ResourcePlatform:
		return LayerPlatform
	case ResourceBusiness:
		return LayerBusiness
	case ResourceCustomer:
		return LayerCustomer
	}
	return ""
}

// ResourceOf maps a layer to its MCP endpoint path ("" for unknown layers).
func ResourceOf(layer string) string {
	switch layer {
	case LayerPlatform:
		return ResourcePlatform
	case LayerBusiness:
		return ResourceBusiness
	case LayerCustomer:
		return ResourceCustomer
	}
	return ""
}

// OAuth error codes (RFC 6749 §5.2, RFC 7591 §3.2.2, RFC 8707 §2).
const (
	ErrInvalidRequest          = "invalid_request"
	ErrInvalidClient           = "invalid_client"
	ErrInvalidGrant            = "invalid_grant"
	ErrUnsupportedGrantType    = "unsupported_grant_type"
	ErrUnsupportedResponseType = "unsupported_response_type"
	ErrInvalidScope            = "invalid_scope"
	ErrInvalidTarget           = "invalid_target"
	ErrInvalidRedirectURI      = "invalid_redirect_uri"
	ErrInvalidClientMetadata   = "invalid_client_metadata"
	ErrAccessDenied            = "access_denied"
	ErrSlowDown                = "slow_down"
	ErrServerError             = "server_error"
)

// Error is an OAuth protocol error: the handler writes it as
// {"error", "error_description"} with Status.
type Error struct {
	Status      int
	Code        string
	Description string
	// Redirectable is set by authorize request validation once client and
	// redirect_uri are known good: the error may go back to the client's
	// redirect_uri. Otherwise it must be shown to the user (RFC 6749 §4.1.2.1).
	Redirectable bool
	// RetryAfter is set on rate limit errors.
	RetryAfter time.Duration
	// Cause is the internal error behind a server_error (logged, never sent).
	Cause error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("mcpauth: %s: %v", e.Code, e.Cause)
	}
	return fmt.Sprintf("mcpauth: %s: %s", e.Code, e.Description)
}

// Unwrap returns the internal cause.
func (e *Error) Unwrap() error { return e.Cause }

// BadRequest builds a 400 OAuth error.
func BadRequest(code, desc string) *Error {
	return &Error{Status: http.StatusBadRequest, Code: code, Description: desc}
}

// Client is a registered (RFC 7591) public client.
type Client struct {
	ClientID     string
	ClientName   string
	RedirectURIs []string
	IssuedAt     time.Time
}

// RegisterInput is the client metadata of a registration request.
type RegisterInput struct {
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	Scope                   string   `json:"scope"`
}

// AuthorizeInput is an /oauth/authorize request.
type AuthorizeInput struct {
	ClientID            string
	RedirectURI         string
	ResponseType        string
	CodeChallenge       string
	CodeChallengeMethod string
	State               string
	Scope               string
	Resource            string
}

// AuthRequest is a validated authorize request waiting for consent.
type AuthRequest struct {
	ID          uuid.UUID
	ClientID    string
	ClientName  string
	RedirectURI string
	State       string
	Scopes      []string
	Resource    string
	ExpiresAt   time.Time
}

// Subject is who a grant acts for: a staff user (platform, business) or a
// customer identity (customer). OrganizationID is set on the business layer.
type Subject struct {
	UserID         int64
	OrganizationID int64
}

// TokenInput is the form of a token endpoint request.
type TokenInput struct {
	GrantType    string
	ClientID     string
	Code         string
	RedirectURI  string
	CodeVerifier string
	RefreshToken string
	Resource     string
	Scope        string
}

// TokenResponse is the RFC 6749 §5.1 success body.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

// AccessToken is a validated Bearer token of an MCP endpoint.
type AccessToken struct {
	ID        int64
	Family    uuid.UUID
	GrantID   int64
	GrantUUID uuid.UUID
	ClientID  string
	Subject   Subject
	Resource  string
	Scopes    []string
	ExpiresAt time.Time
}

// Layer is the token's layer.
func (t AccessToken) Layer() string { return LayerOf(t.Resource) }

// --- consent and connected apps ---------------------------------------------

// Consent decisions.
const (
	DecisionApprove = "approve"
	DecisionDeny    = "deny"
)

// Consent screens /oauth/authorize redirects to: the panel one for staff
// layers, the customer panel one for the customer layer.
const (
	ConsentPath         = "/oauth/consent"
	CustomerConsentPath = "/panel/oauth/consent"
)

// Activity log actions.
const (
	ActionConsentApproved = "mcp.consent_approved"
	ActionConsentDenied   = "mcp.consent_denied"
	ActionGrantRevoked    = "mcp.grant_revoked"
	ActionClientRevoked   = "mcp.client_revoked"
)

// ConsentOrganization is a business a business-layer token may be bound to.
type ConsentOrganization struct {
	UUID uuid.UUID `json:"uuid"`
	Slug string    `json:"slug"`
	Name string    `json:"name"`
	// ToolCount is the number of tools the user gets there; nil when not
	// computed.
	ToolCount *int `json:"tool_count"`
	// CanWrite: the member may let the agent change data here
	// (tenant.mcp.write).
	CanWrite bool `json:"can_write"`
}

// Consent is what the consent screen shows for a pending request.
type Consent struct {
	RequestID    uuid.UUID `json:"request_uuid"`
	ClientID     string    `json:"client_id"`
	ClientName   string    `json:"client_name"`
	RedirectHost string    `json:"redirect_host"`
	// RedirectLoopback: the answer goes to a program on the user's own
	// computer (http on a loopback address), not to a website.
	RedirectLoopback bool `json:"redirect_loopback"`
	// AccessTTLSeconds / RefreshTTLSeconds: how long an access token lasts
	// and for how long the connection can renew itself without asking again.
	AccessTTLSeconds  int64  `json:"access_ttl_seconds"`
	RefreshTTLSeconds int64  `json:"refresh_ttl_seconds"`
	Resource          string `json:"resource"`
	ResourceURL       string `json:"resource_url"`
	Layer             string `json:"layer"`
	// Scopes the client asked for.
	Scopes []string `json:"scopes"`
	// WriteRequested: the client asked for mcp:write; the user may grant it
	// on approval.
	WriteRequested bool `json:"write_requested"`
	// CanWrite: on the platform and customer layers, whether write may be
	// granted (the business layer decides per organization).
	CanWrite bool `json:"can_write"`
	// ToolCount: the tools of the platform / customer layer; nil on the
	// business layer (per organization).
	ToolCount *int `json:"tool_count"`
	// Organizations: the businesses the user may choose (business layer).
	Organizations []ConsentOrganization `json:"organizations"`
	ExpiresAt     time.Time             `json:"expires_at"`
}

// DecideInput is the body of a consent decision.
type DecideInput struct {
	Decision         string     `json:"decision"`
	OrganizationUUID *uuid.UUID `json:"organization_uuid"`
	// AllowWrite grants mcp:write when the client asked for it and the user
	// may give it.
	AllowWrite bool `json:"allow_write"`
}

// Decision is the result of a consent decision: where to send the browser.
type Decision struct {
	RedirectURL string `json:"redirect_url"`
}

// Grant is one connected app.
type Grant struct {
	UUID             uuid.UUID  `json:"uuid"`
	ClientID         string     `json:"client_id"`
	ClientName       string     `json:"client_name"`
	Resource         string     `json:"resource"`
	Layer            string     `json:"layer"`
	Scopes           []string   `json:"scopes"`
	OrganizationUUID *uuid.UUID `json:"organization_uuid"`
	OrganizationName *string    `json:"organization_name"`
	OrganizationSlug *string    `json:"organization_slug"`
	CreatedAt        time.Time  `json:"created_at"`
	LastUsedAt       *time.Time `json:"last_used_at"`
}

// ClientSummary is a registered client in the platform list.
type ClientSummary struct {
	UUID         uuid.UUID  `json:"uuid"`
	ClientID     string     `json:"client_id"`
	ClientName   string     `json:"client_name"`
	RedirectURIs []string   `json:"redirect_uris"`
	CreatedIP    *string    `json:"created_ip"`
	Status       string     `json:"status"`
	ActiveGrants int64      `json:"active_grants"`
	Used         bool       `json:"used"`
	CreatedAt    time.Time  `json:"created_at"`
	LastUsedAt   *time.Time `json:"last_used_at"`
	RevokedAt    *time.Time `json:"revoked_at"`
}

// Client statuses of the platform list.
const (
	ClientStatusActive  = "active"
	ClientStatusRevoked = "revoked"
)
