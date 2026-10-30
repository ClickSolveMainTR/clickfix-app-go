// Package usecase implements organization OIDC/SAML and SCIM provisioning.
package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/crewjam/saml"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sso/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/oidc"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
)

var ErrDenied = errors.New("sso: denied")
var ErrInvalid = errors.New("sso: invalid input")

type Service struct {
	Activity       *activity.Recorder
	TrustedProxies []netip.Prefix
	Limit          func(context.Context, string, string) (bool, time.Duration)
	Repo           *repository.Postgres
	Box            *crypto.SecretBox
	Tokens         *jwt.Manager
	Resolver       orgctx.Resolver
	Entitlements   entitlement.Engine
	BaseURL        string
	HTTP           *http.Client
}

func (s *Service) client() *http.Client {
	if s.HTTP != nil {
		return s.HTTP
	}
	return oidc.OrganizationClient()
}
func Random() (string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func Hash(v string) string { b := sha256.Sum256([]byte(v)); return hex.EncodeToString(b[:]) }
func (s *Service) Resolve(ctx context.Context, ref string, user int64) (orgctx.Scope, error) {
	return s.Resolver.Resolve(ctx, ref, user)
}
func (s *Service) ResolveSupport(ctx context.Context, ref string, user int64) (orgctx.Scope, error) {
	if r, ok := s.Resolver.(orgctx.SupportResolver); ok {
		return r.ResolveSupport(ctx, ref, user)
	}
	return orgctx.Scope{}, orgctx.ErrNotMember
}
func (s *Service) CheckOrganizationAccess(r *http.Request, scope orgctx.Scope, p authctx.Principal) error {
	c, err := s.Repo.Q.SSOConfig(r.Context(), scope.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if !AllowedIP(s.ClientIP(r), c.IpAllowlist) {
		return fmt.Errorf("%w: %w", ErrDenied, orgctx.ErrIPNotAllowed)
	}
	if c.Enforced && p.SSOOrganizationID != scope.ID {
		return fmt.Errorf("%w: %w", ErrDenied, orgctx.ErrSSORequired)
	}
	// Check fresh membership even when the resolver served a cached entry.
	if !scope.Support {
		ok, e := s.Repo.Q.SSOMembership(r.Context(), db.SSOMembershipParams{OrganizationID: scope.ID, UserID: p.UserInternal})
		if e != nil || !ok {
			return ErrDenied
		}
	}
	if p.SSOOrganizationID > 0 {
		ok, e := s.Repo.Q.SSOActiveUser(r.Context(), db.SSOActiveUserParams{OrganizationID: scope.ID, UserID: p.UserInternal})
		if e != nil || !ok || !c.Enabled {
			return ErrDenied
		}
	}
	return nil
}
func AllowedIP(remote string, allow []string) bool {
	if len(allow) == 0 {
		return true
	}
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	ip = ip.Unmap()
	for _, v := range allow {
		p, e := netip.ParsePrefix(v)
		if e == nil && p.Contains(ip) {
			return true
		}
	}
	return false
}

type Config struct {
	Protocol         string   `json:"protocol"`
	Enabled          bool     `json:"enabled"`
	Enforced         bool     `json:"enforced"`
	Issuer           string   `json:"issuer"`
	ClientID         string   `json:"client_id"`
	ClientSecret     string   `json:"client_secret,omitempty"`
	AuthorizationURL string   `json:"authorization_url"`
	TokenURL         string   `json:"token_url"`
	JWKSURL          string   `json:"jwks_url"`
	SAMLMetadata     string   `json:"saml_metadata"`
	IPAllowlist      []string `json:"ip_allowlist"`
}

func View(c db.OrganizationSso) Config {
	return Config{Protocol: c.Protocol, Enabled: c.Enabled, Enforced: c.Enforced, Issuer: c.Issuer, ClientID: c.ClientID, AuthorizationURL: c.AuthorizationUrl, TokenURL: c.TokenUrl, JWKSURL: c.JwksUrl, SAMLMetadata: c.SamlMetadata, IPAllowlist: c.IpAllowlist}
}
func validURL(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Fragment == ""
}
func (s *Service) Save(ctx context.Context, org int64, in Config) (Config, error) {
	if in.Enforced {
		p, ok := authctx.PrincipalFrom(ctx)
		if !ok || p.SSOOrganizationID != org {
			return Config{}, ErrDenied
		}
	}

	if err := entitlement.Require(ctx, s.Entitlements, org, "security.sso"); err != nil {
		return Config{}, err
	}
	if (in.Protocol != "oidc" && in.Protocol != "saml") || in.Enforced && !in.Enabled || len(in.IPAllowlist) > 100 {
		return Config{}, ErrInvalid
	}
	for _, p := range in.IPAllowlist {
		if _, e := netip.ParsePrefix(p); e != nil {
			return Config{}, ErrInvalid
		}
	}
	if in.Enabled {
		if in.Protocol == "oidc" {
			if !validURL(in.Issuer) || !validURL(in.AuthorizationURL) || !validURL(in.TokenURL) || !validURL(in.JWKSURL) || in.ClientID == "" {
				return Config{}, ErrInvalid
			}
		} else {
			if _, e := s.provider(in.SAMLMetadata, uuid.Nil); e != nil {
				return Config{}, ErrInvalid
			}
		}
	}
	secret, err := s.Box.Encrypt(in.ClientSecret)
	if err != nil {
		return Config{}, err
	}
	ips := in.IPAllowlist
	if ips == nil {
		ips = []string{}
	}
	c, err := s.Repo.Q.SSOConfigSave(ctx, db.SSOConfigSaveParams{OrganizationID: org, Protocol: in.Protocol, Enabled: in.Enabled, Enforced: in.Enforced, Issuer: in.Issuer, ClientID: in.ClientID, ClientSecretEnc: secret, AuthorizationUrl: in.AuthorizationURL, TokenUrl: in.TokenURL, JwksUrl: in.JWKSURL, SamlMetadata: in.SAMLMetadata, IpAllowlist: ips})
	if err == nil {
		s.Activity.Record(orgctx.WithOrganizationID(ctx, org), nil, "security.sso_configured", "sso", nil, map[string]any{"enabled": in.Enabled, "enforced": in.Enforced, "protocol": in.Protocol}, nil)
	}
	return View(c), err
}
func (s *Service) callback(id uuid.UUID, protocol string) string {
	return strings.TrimRight(s.BaseURL, "/") + "/v1/public/sso/" + id.String() + "/" + protocol + "/callback"
}
func (s *Service) provider(metadata string, id uuid.UUID) (*saml.ServiceProvider, error) {
	var md saml.EntityDescriptor
	if len(metadata) > 1<<20 || xml.Unmarshal([]byte(metadata), &md) != nil || md.EntityID == "" || len(md.IDPSSODescriptors) == 0 {
		return nil, ErrInvalid
	}
	acs, e := url.Parse(s.callback(id, "saml"))
	if e != nil {
		return nil, e
	}
	meta, _ := url.Parse(strings.TrimRight(s.BaseURL, "/") + "/v1/public/sso/" + id.String() + "/saml/metadata")
	sp := &saml.ServiceProvider{EntityID: meta.String(), MetadataURL: *meta, AcsURL: *acs, IDPMetadata: &md, AllowIDPInitiated: false}
	if !validURL(sp.GetSSOBindingLocation(saml.HTTPRedirectBinding)) {
		return nil, ErrInvalid
	}
	return sp, nil
}

type Start struct {
	AuthorizationURL string `json:"authorization_url"`
	State            string `json:"-"`
	Protocol         string `json:"-"`
}

func (s *Service) Start(ctx context.Context, id uuid.UUID, remote string) (Start, error) {
	o, e := s.Repo.Q.SSOOrganization(ctx, id)
	if e != nil {
		return Start{}, ErrDenied
	}
	c, e := s.Repo.Q.SSOConfig(ctx, o.ID)
	if e != nil || !c.Enabled || !AllowedIP(remote, c.IpAllowlist) {
		return Start{}, ErrDenied
	}
	if e = entitlement.Require(ctx, s.Entitlements, o.ID, "security.sso"); e != nil {
		return Start{}, e
	}
	state, e := Random()
	if e != nil {
		return Start{}, e
	}
	nonce, e := Random()
	if e != nil {
		return Start{}, e
	}
	verifier, e := Random()
	if e != nil {
		return Start{}, e
	}
	target := ""
	requestID := ""
	if c.Protocol == "oidc" {
		u, e := url.Parse(c.AuthorizationUrl)
		if e != nil {
			return Start{}, e
		}
		v := u.Query()
		v.Set("response_type", "code")
		v.Set("client_id", c.ClientID)
		v.Set("redirect_uri", s.callback(id, "oidc"))
		v.Set("scope", "openid email profile")
		v.Set("state", state)
		v.Set("nonce", nonce)
		challenge := sha256.Sum256([]byte(verifier))
		v.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:]))
		v.Set("code_challenge_method", "S256")
		u.RawQuery = v.Encode()
		target = u.String()
	} else {
		sp, e := s.provider(c.SamlMetadata, id)
		if e != nil {
			return Start{}, e
		}
		req, e := sp.MakeAuthenticationRequest(sp.GetSSOBindingLocation(saml.HTTPRedirectBinding), saml.HTTPRedirectBinding, saml.HTTPPostBinding)
		if e != nil {
			return Start{}, e
		}
		requestID = req.ID
		u, e := req.Redirect(state, sp)
		if e != nil {
			return Start{}, e
		}
		target = u.String()
	}
	enc, e := s.Box.Encrypt(verifier)
	if e != nil {
		return Start{}, e
	}
	e = s.Repo.Q.SSOState(ctx, db.SSOStateParams{OrganizationID: o.ID, StateHash: Hash(state), Nonce: nonce, VerifierEnc: enc, RequestID: requestID})
	return Start{target, state, c.Protocol}, e
}

type Token struct {
	AccessToken      string    `json:"access_token"`
	TokenType        string    `json:"token_type"`
	ExpiresIn        int64     `json:"expires_in"`
	OrganizationUUID uuid.UUID `json:"organization_uuid"`
}

func (s *Service) Complete(ctx context.Context, id uuid.UUID, state, code, protocol, remote string, r *http.Request) (Token, error) {
	o, e := s.Repo.Q.SSOOrganization(ctx, id)
	if e != nil {
		return Token{}, ErrDenied
	}
	c, e := s.Repo.Q.SSOConfig(ctx, o.ID)
	if e != nil || !c.Enabled || c.Protocol != protocol || !AllowedIP(remote, c.IpAllowlist) {
		return Token{}, ErrDenied
	}
	if e = entitlement.Require(ctx, s.Entitlements, o.ID, "security.sso"); e != nil {
		return Token{}, e
	}
	stored, e := s.Repo.Q.SSOConsumeState(ctx, db.SSOConsumeStateParams{OrganizationID: o.ID, StateHash: Hash(state)})
	if e != nil {
		return Token{}, ErrDenied
	}
	subject := ""
	if protocol == "oidc" {
		verifier, e := s.Box.Decrypt(stored.VerifierEnc)
		if e != nil {
			return Token{}, e
		}
		secret, e := s.Box.Decrypt(c.ClientSecretEnc)
		if e != nil {
			return Token{}, e
		}
		raw, e := oidc.ExchangeOrganizationCode(ctx, s.client(), c.TokenUrl, c.ClientID, secret, code, s.callback(id, protocol), verifier)
		if e != nil {
			return Token{}, ErrDenied
		}
		v := oidc.New(map[string]oidc.ProviderConfig{"organization": {Issuers: []string{c.Issuer}, JWKSURL: c.JwksUrl}}, s.client())
		claims, e := v.Verify(ctx, "organization", raw, []string{c.ClientID}, stored.Nonce)
		if e != nil {
			return Token{}, ErrDenied
		}
		subject = claims.Subject
	} else {
		sp, e := s.provider(c.SamlMetadata, id)
		if e != nil {
			return Token{}, e
		}
		clone := r.Clone(ctx)
		u := sp.AcsURL
		clone.URL = &u
		assertion, e := sp.ParseResponse(clone, []string{stored.RequestID})
		if e != nil || assertion.Subject == nil || assertion.Subject.NameID == nil {
			return Token{}, ErrDenied
		}
		subject = assertion.Subject.NameID.Value
	}
	user, e := s.Repo.Q.SSOUser(ctx, db.SSOUserParams{OrganizationID: o.ID, ExternalID: subject})
	if e != nil {
		return Token{}, ErrDenied
	}
	s.Activity.Record(orgctx.WithOrganizationID(ctx, o.ID), &user.ID, "security.sso_login", "sso", nil, map[string]any{"protocol": protocol}, nil)
	access, expires, e := s.Tokens.IssueAccess(jwt.AccessInput{UserID: user.Uuid, SSOOrganizationID: o.ID})
	return Token{AccessToken: access, TokenType: "Bearer", ExpiresIn: int64(time.Until(expires).Seconds()), OrganizationUUID: id}, e
}
func (s *Service) Metadata(id uuid.UUID) ([]byte, error) {
	o, e := s.Repo.Q.SSOOrganization(context.Background(), id)
	if e != nil {
		return nil, e
	}
	c, e := s.Repo.Q.SSOConfig(context.Background(), o.ID)
	if e != nil || !c.Enabled || c.Protocol != "saml" {
		return nil, ErrDenied
	}
	sp, e := s.provider(c.SamlMetadata, id)
	if e != nil {
		return nil, e
	}
	return xml.Marshal(sp.Metadata())
}
func (s *Service) RotateSCIM(ctx context.Context, org int64) (string, error) {
	token, e := Random()
	if e != nil {
		return "", e
	}
	n, e := s.Repo.Q.SSOSetSCIMToken(ctx, db.SSOSetSCIMTokenParams{OrganizationID: org, ScimTokenHash: Hash(token)})
	if e != nil {
		return "", e
	}
	if n != 1 {
		return "", ErrInvalid
	}
	s.Activity.Record(orgctx.WithOrganizationID(ctx, org), nil, "security.scim_token_rotated", "sso", nil, map[string]any{}, nil)
	return token, nil
}

// ClientIP walks the forwarded chain from the trusted immediate peer towards
// the client. Without explicit trusted proxy CIDRs, forwarded headers are ignored.
func (s *Service) ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return r.RemoteAddr
	}
	trusted := func(ip netip.Addr) bool {
		for _, v := range s.TrustedProxies {
			if v.Contains(ip.Unmap()) {
				return true
			}
		}
		return false
	}
	if !trusted(peer) {
		return r.RemoteAddr
	}
	parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(parts) - 1; i >= 0; i-- {
		ip, e := netip.ParseAddr(strings.TrimSpace(parts[i]))
		if e != nil {
			return r.RemoteAddr
		}
		peer = ip
		if !trusted(ip) {
			break
		}
	}
	return net.JoinHostPort(peer.String(), "0")
}

const TaskCleanup = "app:sso:cleanup"

func (s *Service) Cleanup(ctx context.Context, _ *asynq.Task) error {
	return s.Repo.Q.SSOExpireStates(ctx)
}
