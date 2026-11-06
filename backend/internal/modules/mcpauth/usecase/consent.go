package usecase

// Ported from takwim-app-go: staff consent and connected applications.
// ClickFix exposes only /mcp/business, bound to one organization. Consent
// requires tenant.settings.read; opting into mcp:write also requires
// tenant.settings.write. Every tool still checks its resource permissions.

import (
	"context"
	"errors"
	"fmt"
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

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
)

// ErrNotFound: the request, grant or client is missing, expired, foreign or
// already decided (404).
var ErrNotFound = errors.New("mcpauth: not found")

// ForbiddenError is a consent the caller may not give (403).
type ForbiddenError struct{ Message string }

func (e *ForbiddenError) Error() string { return "mcpauth: forbidden: " + e.Message }

func forbidden(msg string) error { return &ForbiddenError{Message: msg} }

func invalid(field, msg string) error {
	return &apiquery.ValidationError{Details: []apiquery.Detail{{Field: field, Message: msg, Code: "invalid"}}}
}

// GateResult is what a subject may do on a layer now.
type GateResult struct {
	// Allowed: the subject may connect (and keep using) the layer.
	Allowed bool
	// CanWrite: mcp:write may be granted / used.
	CanWrite bool
	// ToolCount is the number of tools the subject gets (read and, with
	// CanWrite, write tools).
	ToolCount int
}

// Gate resolves the layer rules for a subject (the MCP server's principal
// resolver): permissions, membership and organization state as of now.
type Gate interface {
	Check(ctx context.Context, layer string, sub model.Subject) (GateResult, error)
}

// SetGate attaches the layer rules. Without it consent offers nothing and
// tokens are not re-checked at refresh.
func (s *Service) SetGate(g Gate) { s.gate = g }

// ActivityRecorder writes audit events (*activity.Recorder).
type ActivityRecorder interface {
	Record(ctx context.Context, actorID *int64, action, resource string, resourceUUID *uuid.UUID, payload map[string]any, req *http.Request)
}

// SetActivity attaches the audit log.
func (s *Service) SetActivity(a ActivityRecorder) { s.activity = a }

// Actor is the signed-in person deciding on consent or managing grants: a
// staff user (UserID).
type Actor struct {
	UserID int64
	// Request is the HTTP request (audit IP and user agent).
	Request *http.Request
}

func (a Actor) actorID() *int64 {
	if a.UserID <= 0 {
		return nil
	}
	id := a.UserID
	return &id
}

func (s *Service) record(ctx context.Context, a Actor, orgID int64, action, resource string, id uuid.UUID, payload map[string]any) {
	if s.activity == nil {
		return
	}
	if orgID > 0 {
		ctx = orgctx.WithOrganizationID(ctx, orgID)
	}
	s.activity.Record(context.WithoutCancel(ctx), a.actorID(), action, resource, &id, payload, a.Request)
}

// --- authorize endpoint ------------------------------------------------------------

// Authorize serves GET /oauth/authorize: it applies the per-IP limit,
// validates and stores the request and returns where to send the browser:
// always the consent screen of this origin. Every error is returned as a
// *model.Error that must be shown to the user, never redirected.
//
// RFC 6749 §4.1.2.1 lets errors found after the client and redirect_uri are
// verified go back to the client's redirect_uri. Registration here is
// anonymous and accepts any https address, so that would make this endpoint
// an open redirect (register https://evil.example/, then link to
// /oauth/authorize with a deliberately invalid request). The browser leaves
// for a client's address only after an explicit decision on the consent
// screen (Decide).
func (s *Service) Authorize(ctx context.Context, ip string, in model.AuthorizeInput) (string, error) {
	if err := s.allow(ctx, "authorize", ip, model.AuthorizePerHour); err != nil {
		return "", err
	}
	req, err := s.CreateAuthRequest(ctx, in)
	if err != nil {
		var oe *model.Error
		if errors.As(err, &oe) {
			return "", oe
		}
		return "", serverError(err)
	}
	path := model.ConsentPath
	return s.issuer + path + "?request=" + req.ID.String(), nil
}

// errorRedirect builds the client redirect of an authorization error (RFC
// 6749 §4.1.2.1, iss per RFC 9207). Only a decision on the consent screen
// (a denial) uses it.
func (s *Service) errorRedirect(redirectURI, state, code, desc string) (string, error) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return "", serverError(err)
	}
	v := u.Query()
	v.Set("error", code)
	if desc != "" {
		v.Set("error_description", desc)
	}
	if state != "" {
		v.Set("state", state)
	}
	v.Set("iss", s.issuer)
	u.RawQuery = v.Encode()
	return u.String(), nil
}

// --- consent ------------------------------------------------------------------------

// checkSession enforces which session may decide on a request: customer
// panel sessions for the customer layer, staff sessions for the others.
func checkSession(a Actor, resource string) error {
	if a.UserID <= 0 || resource != model.ResourceBusiness {
		return forbidden("an authenticated staff account and organization resource are required")
	}
	return nil
}

func (a Actor) subject(orgID int64) model.Subject {
	return model.Subject{UserID: a.UserID, OrganizationID: orgID}
}

type option struct {
	org    model.ConsentOrganization
	id     int64
	result GateResult
}

// options lists what the actor may bind a token for resource to: one entry
// without an organization (platform, customer) or the businesses the user
// may connect (business).
func (s *Service) options(ctx context.Context, a Actor, resource string) ([]option, error) {
	if s.gate == nil {
		return nil, nil
	}
	layer := model.LayerOf(resource)
	if layer != model.LayerBusiness {
		res, err := s.gate.Check(ctx, layer, a.subject(0))
		if err != nil {
			return nil, fmt.Errorf("mcpauth: gate: %w", err)
		}
		if !res.Allowed {
			return nil, nil
		}
		return []option{{result: res}}, nil
	}
	rows, err := s.q.ListMCPOAuthUserOrganizations(ctx, a.UserID)
	if err != nil {
		return nil, fmt.Errorf("mcpauth: organizations: %w", err)
	}
	out := make([]option, 0, len(rows))
	for _, r := range rows {
		res, err := s.gate.Check(ctx, layer, a.subject(r.ID))
		if err != nil {
			return nil, fmt.Errorf("mcpauth: gate: %w", err)
		}
		if !res.Allowed {
			continue
		}
		n := res.ToolCount
		out = append(out, option{
			id: r.ID, result: res,
			org: model.ConsentOrganization{UUID: r.Uuid, Slug: r.Slug, Name: r.Name, ToolCount: &n, CanWrite: res.CanWrite},
		})
	}
	return out, nil
}

// Consent returns the consent screen data of a pending request: client,
// redirect host, layer, requested scopes and (business layer) the
// businesses the user may choose. A user who may not use the layer gets an
// empty choice (the screen explains it), not an error.
func (s *Service) Consent(ctx context.Context, a Actor, id uuid.UUID) (model.Consent, error) {
	req, err := s.GetAuthRequest(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Consent{}, ErrNotFound
	}
	if err != nil {
		return model.Consent{}, err
	}
	if err := checkSession(a, req.Resource); err != nil {
		return model.Consent{}, err
	}
	opts, err := s.options(ctx, a, req.Resource)
	if err != nil {
		return model.Consent{}, err
	}
	// The redirect URI passed validRedirect, so its host is ASCII (punycode
	// for an internationalized name): what is shown cannot imitate another
	// host with look-alike letters.
	host, loopback := req.RedirectURI, false
	if u, err := url.Parse(req.RedirectURI); err == nil {
		host, loopback = u.Host, u.Scheme == "http" && loopbackHost(u.Hostname())
	}
	out := model.Consent{
		RequestID: req.ID, ClientID: req.ClientID, ClientName: cleanClientName(req.ClientName), RedirectHost: host, RedirectLoopback: loopback,
		AccessTTLSeconds: int64(model.AccessTTL / time.Second), RefreshTTLSeconds: int64(model.RefreshTTL / time.Second),
		Resource: req.Resource, ResourceURL: s.ResourceURL(req.Resource), Layer: model.LayerOf(req.Resource),
		Scopes: req.Scopes, WriteRequested: model.HasScope(req.Scopes, model.ScopeWrite),
		Organizations: []model.ConsentOrganization{}, ExpiresAt: req.ExpiresAt,
	}
	for _, o := range opts {
		if out.Layer == model.LayerBusiness {
			out.Organizations = append(out.Organizations, o.org)
			continue
		}
		n := o.result.ToolCount
		out.ToolCount, out.CanWrite = &n, o.result.CanWrite
	}
	return out, nil
}

// Decide approves (binding the token to the chosen business on the business
// layer) or denies a pending request and returns the client redirect: the
// code on approval, error=access_denied on denial. The request is consumed
// either way.
func (s *Service) Decide(ctx context.Context, a Actor, id uuid.UUID, in model.DecideInput) (model.Decision, error) {
	if in.Decision != model.DecisionApprove && in.Decision != model.DecisionDeny {
		return model.Decision{}, invalid("decision", "must be approve or deny")
	}
	req, err := s.GetAuthRequest(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Decision{}, ErrNotFound
	}
	if err != nil {
		return model.Decision{}, err
	}
	if err := checkSession(a, req.Resource); err != nil {
		return model.Decision{}, err
	}
	payload := map[string]any{"client_id": req.ClientID, "client_name": req.ClientName, "resource": req.Resource}
	if in.Decision == model.DecisionDeny {
		return s.deny(ctx, a, req, payload)
	}

	layer := model.LayerOf(req.Resource)
	if layer == model.LayerBusiness && in.OrganizationUUID == nil {
		return model.Decision{}, invalid("organization_uuid", "required to approve")
	}
	opts, err := s.options(ctx, a, req.Resource)
	if err != nil {
		return model.Decision{}, err
	}
	var chosen *option
	for i := range opts {
		if layer != model.LayerBusiness || opts[i].org.UUID == *in.OrganizationUUID {
			chosen = &opts[i]
			break
		}
	}
	if chosen == nil {
		if layer == model.LayerBusiness {
			return model.Decision{}, forbidden("the organization cannot be connected with the current membership and permissions")
		}
		return model.Decision{}, forbidden("this account may not connect AI agents to this MCP layer")
	}
	scopes := []string{model.ScopeRead}
	if in.AllowWrite && model.HasScope(req.Scopes, model.ScopeWrite) {
		if !chosen.result.CanWrite {
			return model.Decision{}, forbidden("this account may not let AI agents change data here")
		}
		scopes = append(scopes, model.ScopeWrite)
	}
	payload["scopes"] = scopes
	if chosen.id > 0 {
		payload["organization_uuid"] = chosen.org.UUID.String()
		payload["organization_name"] = chosen.org.Name
	}
	grant, redirect, err := s.issueCode(ctx, req.ID, a.subject(chosen.id), scopes)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Decision{}, ErrNotFound
	}
	if err != nil {
		return model.Decision{}, err
	}
	s.record(ctx, a, chosen.id, model.ActionConsentApproved, "mcp_oauth_grants", grant.Uuid, payload)
	return model.Decision{RedirectURL: redirect}, nil
}

func (s *Service) deny(ctx context.Context, a Actor, req model.AuthRequest, payload map[string]any) (model.Decision, error) {
	n, err := s.q.DeleteMCPOAuthAuthRequest(ctx, req.ID)
	if err != nil {
		return model.Decision{}, err
	}
	if n == 0 {
		return model.Decision{}, ErrNotFound
	}
	s.record(ctx, a, 0, model.ActionConsentDenied, "mcp_oauth_auth_requests", req.ID, payload)
	redirect, err := s.errorRedirect(req.RedirectURI, req.State, model.ErrAccessDenied, "the user denied access")
	if err != nil {
		return model.Decision{}, err
	}
	return model.Decision{RedirectURL: redirect}, nil
}

// --- connected apps -------------------------------------------------------------------

// sortColumns are the sort keys of both lists.
var sortColumns = apiquery.SortColumns{"created_at": "created_at", "last_used_at": "last_used_at", "client_name": "client_name"}

// ListFilter is a parsed list query (grants: layer filter; clients: status).
type ListFilter struct {
	Q        string
	Layers   []string
	Status   string
	SortKey  string
	SortDesc bool
	Limit    int32
	Offset   int32
}

func parseList(values url.Values) (ListFilter, error) {
	q := apiquery.Parse(values)
	if err := apiquery.ValidateSort(q.Sort, sortColumns); err != nil {
		return ListFilter{}, err
	}
	f := ListFilter{Q: q.Q, Limit: q.Limit, Offset: q.Offset, SortKey: "created_at", SortDesc: true}
	if field, desc, ok := apiquery.PrimarySort(q.Sort, sortColumns); ok {
		f.SortKey, f.SortDesc = field, desc
	}
	return f, nil
}

// ParseGrantListFilter reads q, layer (CSV platform,business,customer),
// sort, limit and offset.
func ParseGrantListFilter(values url.Values) (ListFilter, error) {
	f, err := parseList(values)
	if err != nil {
		return f, err
	}
	for _, l := range apiquery.SplitCSV(values.Get("layer")) {
		if !slices.Contains(model.Layers, l) {
			return f, invalid("layer", "must be platform, business or customer")
		}
		f.Layers = append(f.Layers, l)
	}
	return f, nil
}

// ParseClientListFilter reads q, status (active | revoked), sort, limit and
// offset.
func ParseClientListFilter(values url.Values) (ListFilter, error) {
	f, err := parseList(values)
	if err != nil {
		return f, err
	}
	switch st := strings.TrimSpace(values.Get("status")); st {
	case "", model.ClientStatusActive, model.ClientStatusRevoked:
		f.Status = st
	default:
		return f, invalid("status", "must be active or revoked")
	}
	return f, nil
}

func (a Actor) subjectArgs() (pgtype.Int8, pgtype.Int8) {
	return int8Of(a.UserID), pgtype.Int8{}
}

// ListGrants lists the actor's connected apps (live grants of live clients).
func (s *Service) ListGrants(ctx context.Context, a Actor, f ListFilter) ([]model.Grant, int64, error) {
	var resources []string
	for _, l := range f.Layers {
		resources = append(resources, model.ResourceOf(l))
	}
	userID, _ := a.subjectArgs()
	rows, err := s.q.ListMCPOAuthGrants(ctx, db.ListMCPOAuthGrantsParams{
		UserID: userID, Resources: resources, Q: textOf(f.Q),
		SortKey: f.SortKey, SortDesc: f.SortDesc, RowLimit: f.Limit, RowOffset: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("mcpauth: grants: %w", err)
	}
	total, err := s.q.CountMCPOAuthGrants(ctx, db.CountMCPOAuthGrantsParams{
		UserID: userID, Resources: resources, Q: textOf(f.Q),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("mcpauth: grants count: %w", err)
	}
	out := make([]model.Grant, 0, len(rows))
	for _, r := range rows {
		g := model.Grant{
			UUID: r.Uuid, ClientID: r.ClientID, ClientName: r.ClientName, Resource: r.Resource,
			Layer: model.LayerOf(r.Resource), Scopes: r.Scopes, CreatedAt: r.CreatedAt.Time, LastUsedAt: timePtr(r.LastUsedAt),
		}
		if r.OrganizationUuid.Valid {
			id := uuid.UUID(r.OrganizationUuid.Bytes)
			name, slug := r.OrganizationName.String, r.OrganizationSlug.String
			g.OrganizationUUID, g.OrganizationName, g.OrganizationSlug = &id, &name, &slug
		}
		out = append(out, g)
	}
	return out, total, nil
}

// RevokeGrant disconnects one of the actor's apps: the grant and every
// token family issued under it (its access tokens answer 401 at once).
func (s *Service) RevokeGrant(ctx context.Context, a Actor, id uuid.UUID) error {
	userID, _ := a.subjectArgs()
	var g db.RevokeMCPOAuthGrantRow
	err := s.inTx(ctx, func(q *db.Queries) error {
		var err error
		g, err = q.RevokeMCPOAuthGrant(ctx, db.RevokeMCPOAuthGrantParams{Uuid: id, UserID: userID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return q.RevokeMCPOAuthGrantTokens(ctx, g.ID)
	})
	if err != nil {
		return err
	}
	s.record(ctx, a, g.OrganizationID.Int64, model.ActionGrantRevoked, "mcp_oauth_grants", g.Uuid,
		map[string]any{"client_id": g.ClientID, "client_name": g.ClientName, "resource": g.Resource})
	return nil
}

// --- platform clients -----------------------------------------------------------------

// ListClients lists every registered client (platform admin).
func (s *Service) ListClients(ctx context.Context, f ListFilter) ([]model.ClientSummary, int64, error) {
	rows, err := s.q.ListMCPOAuthClients(ctx, db.ListMCPOAuthClientsParams{
		Status: textOf(f.Status), Q: textOf(f.Q),
		SortKey: f.SortKey, SortDesc: f.SortDesc, RowLimit: f.Limit, RowOffset: f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("mcpauth: clients: %w", err)
	}
	total, err := s.q.CountMCPOAuthClients(ctx, db.CountMCPOAuthClientsParams{Status: textOf(f.Status), Q: textOf(f.Q)})
	if err != nil {
		return nil, 0, fmt.Errorf("mcpauth: clients count: %w", err)
	}
	out := make([]model.ClientSummary, 0, len(rows))
	for _, r := range rows {
		c := model.ClientSummary{
			UUID: r.Uuid, ClientID: r.ClientID, ClientName: r.ClientName, RedirectURIs: r.RedirectUris,
			Status: model.ClientStatusActive, ActiveGrants: r.ActiveGrants, Used: r.Used, CreatedAt: r.CreatedAt.Time,
			LastUsedAt: timePtr(r.LastUsedAt), RevokedAt: timePtr(r.RevokedAt),
		}
		if r.CreatedIp.Valid {
			ip := r.CreatedIp.String
			c.CreatedIP = &ip
		}
		if r.RevokedAt.Valid {
			c.Status = model.ClientStatusRevoked
		}
		if c.RedirectURIs == nil {
			c.RedirectURIs = []string{}
		}
		out = append(out, c)
	}
	return out, total, nil
}

// RevokeClient blocks a client and everything issued to it: grants, every
// token, pending requests and unused codes. Revoking a revoked client again
// is a no-op that still answers success.
func (s *Service) RevokeClient(ctx context.Context, a Actor, id uuid.UUID) error {
	c, err := s.q.GetMCPOAuthClientByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if err := s.q.RevokeMCPOAuthClient(ctx, c.ClientID); err != nil {
		return err
	}
	s.record(ctx, a, 0, model.ActionClientRevoked, "mcp_oauth_clients", c.Uuid,
		map[string]any{"client_id": c.ClientID, "client_name": c.ClientName, "already_revoked": c.RevokedAt.Valid})
	return nil
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}
