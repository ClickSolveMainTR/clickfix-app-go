// Package usecase ports Takwim's OAuth 2.1 authorization server: public-client
// registration, PKCE S256 codes, rotating refresh tokens with reuse detection,
// revocation and resource-bound access tokens. ClickFix mounts only the
// organization staff resource /mcp/business.

package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mcpauth/model"
)

// maxRedirectURIs bounds a registration (ck_mcp_oauth_clients_redirect_uris).
const maxRedirectURIs = 10

// clientIDPrefix marks the client ids this server issues.
const clientIDPrefix = "mcp_"

// ErrUnauthorized: the Bearer token is unknown, expired, revoked, of a
// revoked grant or client, of an inactive user or a changed customer phone,
// or of another resource. The MCP endpoint answers 401 with WWW-Authenticate.
var ErrUnauthorized = errors.New("mcpauth: invalid access token")

// DB is the connection the service runs on: a pool in production, a test
// transaction in tests (Begin opens a savepoint there).
type DB interface {
	db.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Limiter is the per-IP fixed window limiter (ratelimit.Limiter).
type Limiter interface {
	Allow(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration)
}

// Service is the authorization server.
type Service struct {
	db       DB
	q        *db.Queries
	limiter  Limiter
	gate     Gate
	activity ActivityRecorder
	issuer   string
	// unusedClientAge is how old a never connected registration must be
	// before Cleanup deletes it.
	unusedClientAge time.Duration
	log             *slog.Logger
	now             func() time.Time
}

// DefaultUnusedClientAge is the Cleanup age of never connected client
// registrations.
const DefaultUnusedClientAge = 7 * 24 * time.Hour

// New builds the service. issuer is the public origin clients reach (the
// frontend, which proxies /oauth/*, /.well-known/* and /mcp/* to Go).
func New(conn DB, limiter Limiter, issuer string, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		db: conn, q: db.New(conn), limiter: limiter,
		issuer: strings.TrimRight(issuer, "/"), unusedClientAge: DefaultUnusedClientAge, log: log, now: time.Now,
	}
}

// SetClock overrides the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// Issuer is the authorization server identifier (RFC 8414 issuer).
func (s *Service) Issuer() string { return s.issuer }

// ResourceURL is the absolute URI of an MCP endpoint path.
func (s *Service) ResourceURL(path string) string { return s.issuer + path }

// --- rate limits ---------------------------------------------------------------

func (s *Service) allow(ctx context.Context, action, ip string, limit int) error {
	if s.limiter == nil {
		return nil
	}
	ok, retry := s.limiter.Allow(ctx, "mcp_oauth_"+action, ip, limit, model.RateWindow)
	if ok {
		return nil
	}
	return &model.Error{
		Status: http.StatusTooManyRequests, Code: model.ErrSlowDown,
		Description: "too many requests, try again later", RetryAfter: retry,
	}
}

// --- helpers -------------------------------------------------------------------

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// HashToken is the stored form of codes and tokens (SHA-256, hex).
func HashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func serverError(err error) *model.Error {
	return &model.Error{Status: http.StatusInternalServerError, Code: model.ErrServerError, Description: "internal server error", Cause: err}
}

// validVerifier checks RFC 7636 §4.1 code_verifier syntax.
func validVerifier(v string) bool {
	if len(v) < 43 || len(v) > 128 {
		return false
	}
	for _, c := range v {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '.', c == '_', c == '~':
		default:
			return false
		}
	}
	return true
}

// S256 is the PKCE S256 code challenge of a verifier.
func S256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// validRedirect accepts https URLs and loopback http URLs (native clients),
// without a fragment (RFC 6749 §3.1.2, OAuth 2.1 §2.3.1).
func validRedirect(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Fragment != "" || u.Host == "" || u.User != nil {
		return false
	}
	switch u.Scheme {
	case "https":
		return true
	case "http":
		h := u.Hostname()
		return h == "localhost" || h == "127.0.0.1" || h == "::1"
	}
	return false
}

// resourcePath resolves an RFC 8707 resource indicator to an MCP endpoint
// path: an absolute URI on the issuer's origin naming one of the endpoints.
func (s *Service) resourcePath(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Fragment != "" || u.RawQuery != "" || !u.IsAbs() {
		return "", false
	}
	iss, err := url.Parse(s.issuer)
	if err != nil || !strings.EqualFold(u.Scheme, iss.Scheme) || !strings.EqualFold(u.Host, iss.Host) {
		return "", false
	}
	p := strings.TrimRight(u.Path, "/")
	if !model.IsResource(p) {
		return "", false
	}
	return p, true
}

// parseScopes resolves a requested scope string: known scopes only, read is
// always included; empty means read only.
func parseScopes(raw string) ([]string, bool) {
	out := []string{model.ScopeRead}
	for _, f := range strings.Fields(raw) {
		if !slices.Contains(model.Scopes, f) {
			return nil, false
		}
		if !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return out, true
}

func (s *Service) inTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func int8Of(v int64) pgtype.Int8 {
	if v <= 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: v, Valid: true}
}

func textOf(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// --- registration (RFC 7591) -----------------------------------------------------

// Register stores a public client. Only token_endpoint_auth_method none,
// the authorization_code and refresh_token grants and the code response type
// are accepted.
func (s *Service) Register(ctx context.Context, ip string, in model.RegisterInput) (model.Client, error) {
	if err := s.allow(ctx, "register", ip, model.RegisterPerHour); err != nil {
		return model.Client{}, err
	}
	if len(in.RedirectURIs) == 0 || len(in.RedirectURIs) > maxRedirectURIs {
		return model.Client{}, model.BadRequest(model.ErrInvalidRedirectURI, "1 to 10 redirect_uris are required")
	}
	for _, u := range in.RedirectURIs {
		if !validRedirect(u) {
			return model.Client{}, model.BadRequest(model.ErrInvalidRedirectURI, "redirect URIs must be https (or http on a loopback host) without a fragment")
		}
	}
	if in.TokenEndpointAuthMethod != "" && in.TokenEndpointAuthMethod != "none" {
		return model.Client{}, model.BadRequest(model.ErrInvalidClientMetadata, "only public clients (token_endpoint_auth_method=none) are supported")
	}
	for _, g := range in.GrantTypes {
		if g != model.GrantAuthorizationCode && g != model.GrantRefreshToken {
			return model.Client{}, model.BadRequest(model.ErrInvalidClientMetadata, "only the authorization_code and refresh_token grants are supported")
		}
	}
	for _, rt := range in.ResponseTypes {
		if rt != "code" {
			return model.Client{}, model.BadRequest(model.ErrInvalidClientMetadata, "only response_type code is supported")
		}
	}
	if _, ok := parseScopes(in.Scope); !ok {
		return model.Client{}, model.BadRequest(model.ErrInvalidClientMetadata, "supported scopes: mcp:read mcp:write")
	}
	name := strings.TrimSpace(in.ClientName)
	if name == "" {
		name = "MCP client"
	}
	if r := []rune(name); len(r) > 200 {
		name = string(r[:200])
	}
	ip = strings.TrimSpace(ip)
	if len(ip) > 64 {
		ip = ip[:64]
	}
	c, err := s.q.CreateMCPOAuthClient(ctx, db.CreateMCPOAuthClientParams{
		ClientID: clientIDPrefix + randomToken(18), ClientName: name,
		RedirectUris: in.RedirectURIs, CreatedIp: textOf(ip),
	})
	if err != nil {
		return model.Client{}, fmt.Errorf("mcpauth: register client: %w", err)
	}
	return model.Client{ClientID: c.ClientID, ClientName: c.ClientName, RedirectURIs: c.RedirectUris, IssuedAt: c.CreatedAt.Time}, nil
}

// --- authorization request -------------------------------------------------------

// CreateAuthRequest validates an authorize request and stores it for
// consent. An unknown client or an unregistered redirect_uri is a
// non-redirectable error; every later error is Redirectable. PKCE is
// required with code_challenge_method S256 and resource must name one of the
// MCP endpoints.
func (s *Service) CreateAuthRequest(ctx context.Context, in model.AuthorizeInput) (model.AuthRequest, error) {
	client, err := s.q.GetMCPOAuthClient(ctx, in.ClientID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && client.RevokedAt.Valid) {
		return model.AuthRequest{}, model.BadRequest(model.ErrInvalidClient, "unknown client_id")
	}
	if err != nil {
		return model.AuthRequest{}, fmt.Errorf("mcpauth: client: %w", err)
	}
	if !slices.Contains(client.RedirectUris, in.RedirectURI) {
		return model.AuthRequest{}, model.BadRequest(model.ErrInvalidRequest, "redirect_uri is not registered for this client")
	}
	fail := func(code, desc string) error {
		e := model.BadRequest(code, desc)
		e.Redirectable = true
		return e
	}
	if in.ResponseType != "code" {
		return model.AuthRequest{}, fail(model.ErrUnsupportedResponseType, "only response_type=code is supported")
	}
	if in.CodeChallengeMethod != "S256" {
		return model.AuthRequest{}, fail(model.ErrInvalidRequest, "PKCE with code_challenge_method=S256 is required")
	}
	if len(in.CodeChallenge) != 43 {
		return model.AuthRequest{}, fail(model.ErrInvalidRequest, "code_challenge must be a base64url SHA-256 digest")
	}
	resource, ok := s.resourcePath(in.Resource)
	if !ok {
		return model.AuthRequest{}, fail(model.ErrInvalidTarget, "resource must be one of the MCP endpoint URLs")
	}
	sc, ok := parseScopes(in.Scope)
	if !ok {
		return model.AuthRequest{}, fail(model.ErrInvalidScope, "supported scopes: mcp:read mcp:write")
	}
	req, err := s.q.CreateMCPOAuthAuthRequest(ctx, db.CreateMCPOAuthAuthRequestParams{
		ClientID: client.ClientID, RedirectUri: in.RedirectURI, CodeChallenge: in.CodeChallenge,
		State: textOf(in.State), Scopes: sc, Resource: resource, ExpiresAt: ts(s.now().Add(model.AuthRequestTTL)),
	})
	if err != nil {
		return model.AuthRequest{}, fmt.Errorf("mcpauth: auth request: %w", err)
	}
	return model.AuthRequest{
		ID: req.ID, ClientID: client.ClientID, ClientName: client.ClientName, RedirectURI: req.RedirectUri,
		State: in.State, Scopes: req.Scopes, Resource: resource, ExpiresAt: req.ExpiresAt.Time,
	}, nil
}

// GetAuthRequest returns a pending (unexpired) request; pgx.ErrNoRows when
// missing.
func (s *Service) GetAuthRequest(ctx context.Context, id uuid.UUID) (model.AuthRequest, error) {
	r, err := s.q.GetMCPOAuthAuthRequest(ctx, id)
	if err != nil {
		return model.AuthRequest{}, err
	}
	return model.AuthRequest{
		ID: r.ID, ClientID: r.ClientID, ClientName: r.ClientName, RedirectURI: r.RedirectUri,
		State: r.State.String, Scopes: r.Scopes, Resource: r.Resource, ExpiresAt: r.ExpiresAt.Time,
	}, nil
}

// issueCode approves a pending request: it claims the request, records the
// grant (connected app) with the granted scopes and returns the grant and
// the client redirect carrying the code, state and iss (RFC 9207). The
// caller has checked the layer rules for the subject.
func (s *Service) issueCode(ctx context.Context, requestID uuid.UUID, sub model.Subject, scopes []string) (db.McpOauthGrant, string, error) {
	code := randomToken(32)
	var (
		redirect string
		grant    db.McpOauthGrant
	)
	err := s.inTx(ctx, func(q *db.Queries) error {
		req, err := q.GetMCPOAuthAuthRequest(ctx, requestID)
		if err != nil {
			return err
		}
		n, err := q.DeleteMCPOAuthAuthRequest(ctx, requestID)
		if err != nil {
			return err
		}
		if n == 0 {
			return pgx.ErrNoRows
		}
		grant, err = q.UpsertMCPOAuthGrant(ctx, db.UpsertMCPOAuthGrantParams{
			UserID:         int8Of(sub.UserID),
			ClientID:       req.ClientID,
			OrganizationID: int8Of(sub.OrganizationID), Resource: req.Resource, Scopes: scopes,
		})
		if err != nil {
			return err
		}
		if err := q.CreateMCPOAuthCode(ctx, db.CreateMCPOAuthCodeParams{
			CodeHash: HashToken(code), GrantID: grant.ID, ClientID: req.ClientID, RedirectUri: req.RedirectUri,
			CodeChallenge: req.CodeChallenge, Resource: req.Resource, Scopes: scopes,
			ExpiresAt: ts(s.now().Add(model.CodeTTL)),
		}); err != nil {
			return err
		}
		u, err := url.Parse(req.RedirectUri)
		if err != nil {
			return err
		}
		v := u.Query()
		v.Set("code", code)
		if req.State.Valid && req.State.String != "" {
			v.Set("state", req.State.String)
		}
		v.Set("iss", s.issuer)
		u.RawQuery = v.Encode()
		redirect = u.String()
		return nil
	})
	if err != nil {
		return db.McpOauthGrant{}, "", err
	}
	return grant, redirect, nil
}

// --- token endpoint --------------------------------------------------------------

// Exchange serves the token endpoint: authorization_code (PKCE S256) and
// refresh_token (rotating). A replayed code or rotated refresh token revokes
// every token of its family.
func (s *Service) Exchange(ctx context.Context, ip string, in model.TokenInput) (model.TokenResponse, error) {
	if err := s.allow(ctx, "token", ip, model.TokenPerHour); err != nil {
		return model.TokenResponse{}, err
	}
	switch in.GrantType {
	case model.GrantAuthorizationCode:
		return s.exchangeCode(ctx, in)
	case model.GrantRefreshToken:
		return s.refresh(ctx, in)
	case "":
		return model.TokenResponse{}, model.BadRequest(model.ErrInvalidRequest, "grant_type is required")
	default:
		return model.TokenResponse{}, model.BadRequest(model.ErrUnsupportedGrantType, "use authorization_code or refresh_token")
	}
}

// activeClient loads a non-revoked client.
func (s *Service) activeClient(ctx context.Context, clientID string) error {
	if clientID == "" {
		return model.BadRequest(model.ErrInvalidRequest, "client_id is required")
	}
	c, err := s.q.GetMCPOAuthClient(ctx, clientID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && c.RevokedAt.Valid) {
		return &model.Error{Status: http.StatusUnauthorized, Code: model.ErrInvalidClient, Description: "unknown or revoked client"}
	}
	if err != nil {
		return serverError(err)
	}
	return nil
}

// grantAllowed checks the subject behind a grant at issue time: the user
// must be active (and still allowed on the layer), the customer's phone
// unchanged.
func (s *Service) grantAllowed(ctx context.Context, q *db.Queries, grantID int64, resource string) error {
	g, err := q.GetMCPOAuthGrantSubject(ctx, grantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.BadRequest(model.ErrInvalidGrant, "the grant was revoked")
	}
	if err != nil {
		return serverError(err)
	}
	sub, ok := subjectOf(g.UserID, g.OrganizationID, g.UserStatus)
	if !ok {
		return model.BadRequest(model.ErrInvalidGrant, "the account of this grant is not active")
	}
	if s.gate == nil {
		return nil
	}
	res, err := s.gate.Check(ctx, model.LayerOf(resource), sub)
	if err != nil {
		return serverError(err)
	}
	if !res.Allowed {
		return model.BadRequest(model.ErrInvalidGrant, "the account may no longer use this MCP layer")
	}
	return nil
}

// subjectOf builds the grant subject and reports whether it is live: an
// active user, or a customer identity whose phone is still the one proved on
// consent.
func subjectOf(userID, orgID pgtype.Int8, userStatus pgtype.Text) (model.Subject, bool) {
	return model.Subject{UserID: userID.Int64, OrganizationID: orgID.Int64}, userID.Valid && orgID.Valid && userStatus.Valid && userStatus.String == "active"
}

var (
	errCodeNotUsable = errors.New("mcpauth: code not usable")
	errRefreshRaced  = errors.New("mcpauth: refresh token already rotated")
)

func (s *Service) exchangeCode(ctx context.Context, in model.TokenInput) (model.TokenResponse, error) {
	if err := s.activeClient(ctx, in.ClientID); err != nil {
		return model.TokenResponse{}, err
	}
	if in.Code == "" || in.RedirectURI == "" || in.CodeVerifier == "" {
		return model.TokenResponse{}, model.BadRequest(model.ErrInvalidRequest, "code, redirect_uri and code_verifier are required")
	}
	codeHash := HashToken(in.Code)
	family := uuid.New()
	var out model.TokenResponse
	// Claim, check and issue in one transaction: a failed check leaves the
	// code unused, a successful exchange commits the claim with the tokens.
	err := s.inTx(ctx, func(q *db.Queries) error {
		code, err := q.UseMCPOAuthCode(ctx, db.UseMCPOAuthCodeParams{CodeHash: codeHash, Family: pgtype.UUID{Bytes: family, Valid: true}})
		if errors.Is(err, pgx.ErrNoRows) {
			return errCodeNotUsable
		}
		if err != nil {
			return serverError(err)
		}
		if code.ClientID != in.ClientID || code.RedirectUri != in.RedirectURI {
			return model.BadRequest(model.ErrInvalidGrant, "client_id or redirect_uri does not match the code")
		}
		if !validVerifier(in.CodeVerifier) || S256(in.CodeVerifier) != code.CodeChallenge {
			return model.BadRequest(model.ErrInvalidGrant, "PKCE verification failed")
		}
		// The resource parameter is optional at the token endpoint; when
		// sent it must name the endpoint of the authorization.
		if in.Resource != "" {
			if p, ok := s.resourcePath(in.Resource); !ok || p != code.Resource {
				return model.BadRequest(model.ErrInvalidTarget, "resource does not match the authorization")
			}
		}
		if err := s.grantAllowed(ctx, q, code.GrantID, code.Resource); err != nil {
			return err
		}
		out, err = s.issue(ctx, q, issueArgs{
			family: family, grantID: code.GrantID, clientID: code.ClientID, resource: code.Resource, scopes: code.Scopes,
		})
		return err
	})
	if errors.Is(err, errCodeNotUsable) {
		// A replayed code: revoke whatever was issued from it (RFC 6749 §4.1.2).
		if fam, ferr := s.q.GetUsedMCPOAuthCodeFamily(ctx, codeHash); ferr == nil && fam.Valid {
			if rerr := s.q.RevokeMCPOAuthFamily(ctx, uuid.UUID(fam.Bytes)); rerr != nil {
				s.log.Error("mcp_oauth_code_reuse_revoke_failed", "error", rerr)
			}
			s.log.Warn("mcp_oauth_code_reuse", "client_id", in.ClientID)
		}
		return model.TokenResponse{}, model.BadRequest(model.ErrInvalidGrant, "the code is invalid, expired or already used")
	}
	if err != nil {
		return model.TokenResponse{}, asOAuthError(err)
	}
	s.touchClient(ctx, in.ClientID)
	return out, nil
}

func (s *Service) refresh(ctx context.Context, in model.TokenInput) (model.TokenResponse, error) {
	if err := s.activeClient(ctx, in.ClientID); err != nil {
		return model.TokenResponse{}, err
	}
	if in.RefreshToken == "" {
		return model.TokenResponse{}, model.BadRequest(model.ErrInvalidRequest, "refresh_token is required")
	}
	h := HashToken(in.RefreshToken)
	tok, err := s.q.GetActiveMCPOAuthToken(ctx, db.GetActiveMCPOAuthTokenParams{TokenHash: h, Kind: model.KindRefresh})
	if errors.Is(err, pgx.ErrNoRows) {
		// A rotated (revoked) refresh token used again: revoke the family.
		if old, oerr := s.q.GetMCPOAuthTokenAnyState(ctx, db.GetMCPOAuthTokenAnyStateParams{TokenHash: h, Kind: model.KindRefresh}); oerr == nil && old.RevokedAt.Valid {
			if rerr := s.q.RevokeMCPOAuthFamily(ctx, old.Family); rerr != nil {
				s.log.Error("mcp_oauth_refresh_reuse_revoke_failed", "error", rerr)
			}
			s.log.Warn("mcp_oauth_refresh_reuse", "client_id", old.ClientID)
		}
		return model.TokenResponse{}, model.BadRequest(model.ErrInvalidGrant, "the refresh token is invalid or expired")
	}
	if err != nil {
		return model.TokenResponse{}, serverError(err)
	}
	if tok.ClientID != in.ClientID {
		return model.TokenResponse{}, model.BadRequest(model.ErrInvalidGrant, "the refresh token was issued to another client")
	}
	if in.Resource != "" {
		if p, ok := s.resourcePath(in.Resource); !ok || p != tok.Resource {
			return model.TokenResponse{}, model.BadRequest(model.ErrInvalidTarget, "resource does not match the grant")
		}
	}
	scopes := tok.Scopes
	if in.Scope != "" {
		// A refresh may narrow the scopes (RFC 6749 §6), never widen them.
		asked, ok := parseScopes(in.Scope)
		if !ok {
			return model.TokenResponse{}, model.BadRequest(model.ErrInvalidScope, "supported scopes: mcp:read mcp:write")
		}
		for _, sc := range asked {
			if !slices.Contains(tok.Scopes, sc) {
				return model.TokenResponse{}, model.BadRequest(model.ErrInvalidScope, "the scope exceeds the grant")
			}
		}
		scopes = asked
	}
	var out model.TokenResponse
	raced := false
	err = s.inTx(ctx, func(q *db.Queries) error {
		if err := s.grantAllowed(ctx, q, tok.GrantID, tok.Resource); err != nil {
			return err
		}
		// Revoking is the claim: of two concurrent refreshes only one wins,
		// and the loser is treated as reuse.
		n, err := q.RevokeMCPOAuthToken(ctx, tok.ID)
		if err != nil {
			return serverError(err)
		}
		if n == 0 {
			raced = true
			return errRefreshRaced
		}
		// Retire the old access token with the old refresh token.
		if err := q.RevokeMCPOAuthFamily(ctx, tok.Family); err != nil {
			return serverError(err)
		}
		out, err = s.issue(ctx, q, issueArgs{
			family: tok.Family, grantID: tok.GrantID, clientID: tok.ClientID, resource: tok.Resource, scopes: scopes,
		})
		return err
	})
	if raced {
		if rerr := s.q.RevokeMCPOAuthFamily(ctx, tok.Family); rerr != nil {
			s.log.Error("mcp_oauth_refresh_race_revoke_failed", "error", rerr)
		}
		s.log.Warn("mcp_oauth_refresh_race", "client_id", tok.ClientID)
		return model.TokenResponse{}, model.BadRequest(model.ErrInvalidGrant, "the refresh token is invalid or expired")
	}
	if err != nil {
		return model.TokenResponse{}, asOAuthError(err)
	}
	s.touchClient(ctx, in.ClientID)
	return out, nil
}

type issueArgs struct {
	family             uuid.UUID
	grantID            int64
	clientID, resource string
	scopes             []string
}

func (s *Service) issue(ctx context.Context, q *db.Queries, a issueArgs) (model.TokenResponse, error) {
	access, refresh := randomToken(32), randomToken(32)
	now := s.now()
	for _, t := range []struct {
		raw, kind string
		ttl       time.Duration
	}{{access, model.KindAccess, model.AccessTTL}, {refresh, model.KindRefresh, model.RefreshTTL}} {
		if err := q.CreateMCPOAuthToken(ctx, db.CreateMCPOAuthTokenParams{
			TokenHash: HashToken(t.raw), Kind: t.kind, Family: a.family, GrantID: a.grantID,
			ClientID: a.clientID, Resource: a.resource, Scopes: a.scopes, ExpiresAt: ts(now.Add(t.ttl)),
		}); err != nil {
			return model.TokenResponse{}, serverError(err)
		}
	}
	return model.TokenResponse{
		AccessToken: access, TokenType: "Bearer", ExpiresIn: int(model.AccessTTL.Seconds()),
		RefreshToken: refresh, Scope: strings.Join(a.scopes, " "),
	}, nil
}

func (s *Service) touchClient(ctx context.Context, clientID string) {
	if err := s.q.TouchMCPOAuthClient(ctx, clientID); err != nil {
		s.log.Warn("mcp_oauth_client_touch_failed", "error", err)
	}
}

func asOAuthError(err error) error {
	var oe *model.Error
	if errors.As(err, &oe) {
		return oe
	}
	return serverError(err)
}

// --- revocation (RFC 7009) ---------------------------------------------------------

// Revoke revokes the family of an access or refresh token. Unknown tokens
// and tokens of another client are ignored (the endpoint always answers
// 200).
func (s *Service) Revoke(ctx context.Context, ip, clientID, token string) error {
	if err := s.allow(ctx, "revoke", ip, model.RevokePerHour); err != nil {
		return err
	}
	if token == "" {
		return model.BadRequest(model.ErrInvalidRequest, "token is required")
	}
	h := HashToken(token)
	for _, kind := range []string{model.KindAccess, model.KindRefresh} {
		t, err := s.q.GetMCPOAuthTokenAnyState(ctx, db.GetMCPOAuthTokenAnyStateParams{TokenHash: h, Kind: kind})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return serverError(err)
		}
		if clientID != "" && t.ClientID != clientID {
			return nil
		}
		if err := s.q.RevokeMCPOAuthFamily(ctx, t.Family); err != nil {
			return serverError(err)
		}
	}
	return nil
}

// --- resource server ----------------------------------------------------------------

// ValidateAccessToken resolves a raw Bearer token for the MCP endpoint at
// resource (path, e.g. /mcp/business). The token must be live, of a live
// grant and client, of an active user (or a customer whose phone did not
// change) and issued for that endpoint (audience); otherwise
// ErrUnauthorized. The MCP server then resolves what the subject may do on
// the layer now.
func (s *Service) ValidateAccessToken(ctx context.Context, raw, resource string) (model.AccessToken, error) {
	if raw == "" {
		return model.AccessToken{}, ErrUnauthorized
	}
	t, err := s.q.GetActiveMCPOAuthToken(ctx, db.GetActiveMCPOAuthTokenParams{TokenHash: HashToken(raw), Kind: model.KindAccess})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.AccessToken{}, ErrUnauthorized
	}
	if err != nil {
		return model.AccessToken{}, fmt.Errorf("mcpauth: token: %w", err)
	}
	sub, live := subjectOf(t.UserID, t.OrganizationID, t.UserStatus)
	if !live || t.Resource != resource {
		return model.AccessToken{}, ErrUnauthorized
	}
	if err := s.q.TouchMCPOAuthToken(ctx, t.ID); err != nil {
		s.log.Warn("mcp_oauth_token_touch_failed", "error", err)
	}
	return model.AccessToken{
		ID: t.ID, Family: t.Family, GrantID: t.GrantID, GrantUUID: t.GrantUuid, ClientID: t.ClientID,
		Subject: sub, Resource: t.Resource, Scopes: t.Scopes, ExpiresAt: t.ExpiresAt.Time,
	}, nil
}

// --- metadata -------------------------------------------------------------------------

// AuthorizationServerMetadata is the RFC 8414 document.
func (s *Service) AuthorizationServerMetadata() map[string]any {
	return map[string]any{
		"issuer":                                         s.issuer,
		"authorization_endpoint":                         s.issuer + "/oauth/authorize",
		"token_endpoint":                                 s.issuer + "/oauth/token",
		"registration_endpoint":                          s.issuer + "/oauth/register",
		"revocation_endpoint":                            s.issuer + "/oauth/revoke",
		"response_types_supported":                       []string{"code"},
		"response_modes_supported":                       []string{"query"},
		"grant_types_supported":                          []string{model.GrantAuthorizationCode, model.GrantRefreshToken},
		"code_challenge_methods_supported":               []string{"S256"},
		"token_endpoint_auth_methods_supported":          []string{"none"},
		"revocation_endpoint_auth_methods_supported":     []string{"none"},
		"scopes_supported":                               model.Scopes,
		"authorization_response_iss_parameter_supported": true,
	}
}

// resourceNames are the display names of the layers.
var resourceNames = map[string]string{

	model.ResourceBusiness: "ClickFix MCP (organization)",
}

// ProtectedResourceMetadata is the RFC 9728 document of an MCP endpoint.
func (s *Service) ProtectedResourceMetadata(resource string) map[string]any {
	return map[string]any{
		"resource":                 s.ResourceURL(resource),
		"authorization_servers":    []string{s.issuer},
		"scopes_supported":         model.Scopes,
		"bearer_methods_supported": []string{"header"},
		"resource_name":            resourceNames[resource],
	}
}

// ResourceMetadataURL is the RFC 9728 metadata URL of an MCP endpoint (for
// the WWW-Authenticate challenge).
func (s *Service) ResourceMetadataURL(resource string) string {
	return s.issuer + "/.well-known/oauth-protected-resource" + resource
}

// --- maintenance ------------------------------------------------------------------------

// Cleanup removes expired requests, codes, tokens and client registrations
// that never connected (no grant, token or pending request) and are older
// than the unused client age (periodic worker task, idempotent).
func (s *Service) Cleanup(ctx context.Context) (int64, error) {
	createdBefore := ts(s.now().Add(-s.unusedClientAge))
	var total int64
	for _, f := range []func(context.Context) (int64, error){
		s.q.CleanupMCPOAuthAuthRequests, s.q.CleanupMCPOAuthCodes, s.q.CleanupMCPOAuthTokens,
		func(ctx context.Context) (int64, error) { return s.q.CleanupMCPOAuthClients(ctx, createdBefore) },
	} {
		n, err := f(ctx)
		if err != nil {
			return total, fmt.Errorf("mcpauth: cleanup: %w", err)
		}
		total += n
	}
	return total, nil
}
