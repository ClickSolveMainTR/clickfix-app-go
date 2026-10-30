package usecase

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/apps/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/apps/repository"
	conv "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/apibridge"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const issuer = "clickfix:sidebar-apps"
const keyID = "sidebar-context-v1"

type Service struct {
	Store         *repository.Store
	Conversations *conv.Service
	Bridge        *apibridge.Bridge
	key           ed25519.PrivateKey
}

// New domain-separates the signing seed from the existing encryption secret.
// Iframe tokens use EdDSA and cannot be accepted as HS256 staff sessions.
func New(store *repository.Store, conversations *conv.Service, bridge *apibridge.Bridge, secret string) (*Service, error) {
	if len(secret) == 0 {
		return nil, fmt.Errorf("sidebar apps require a configured encryption key")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("clickfix:sidebar-context:ed25519:v1"))
	return &Service{Store: store, Conversations: conversations, Bridge: bridge, key: ed25519.NewKeyFromSeed(mac.Sum(nil))}, nil
}
func (s *Service) JWKS() map[string]any {
	public := s.key.Public().(ed25519.PublicKey)
	return map[string]any{"keys": []any{map[string]string{"kty": "OKP", "crv": "Ed25519", "alg": "EdDSA", "use": "sig", "kid": keyID, "x": base64.RawURLEncoding.EncodeToString(public)}}}
}
func (s *Service) Save(ctx context.Context, org, actor int64, id uuid.UUID, in model.Input) (db.SidebarApp, error) {
	if err := in.Validate(); err != nil {
		return db.SidebarApp{}, err
	}
	scopes := append([]string{}, in.Scopes...)
	if id == uuid.Nil {
		return s.Store.SidebarAppCreate(ctx, db.SidebarAppCreateParams{OrganizationID: org, Name: in.Name, Url: in.URL, Icon: in.Icon, Placement: in.Placement, Scopes: scopes, Enabled: in.Enabled, CreatedBy: actor})
	}
	if _, err := s.Store.SidebarAppGet(ctx, db.SidebarAppGetParams{OrganizationID: org, Uuid: id}); err != nil {
		return db.SidebarApp{}, err
	}
	row, err := s.Store.SidebarAppUpdate(ctx, db.SidebarAppUpdateParams{OrganizationID: org, Uuid: id, Name: in.Name, Url: in.URL, Icon: in.Icon, Placement: in.Placement, Scopes: scopes, Enabled: in.Enabled, Revision: in.Revision})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, model.ErrConflict
	}
	return row, err
}
func (s *Service) Launch(ctx context.Context, id uuid.UUID, in model.LaunchInput) (model.Launch, error) {
	org := orgctx.Must(ctx)
	actor := authctx.MustPrincipal(ctx)
	row, err := s.Store.SidebarAppGet(ctx, db.SidebarAppGetParams{OrganizationID: org.ID, Uuid: id})
	if err != nil {
		return model.Launch{}, err
	}
	if !row.Enabled {
		return model.Launch{}, model.ErrDenied
	}
	scopes := []string{}
	for _, scope := range row.Scopes {
		if org.Has(model.Scopes[scope]) {
			scopes = append(scopes, scope)
		}
	}
	if in.Conversation != nil {
		if !slices.Contains(scopes, "conversation.read") {
			return model.Launch{}, model.ErrDenied
		}
		if _, err = s.Conversations.Get(ctx, org.ID, actor.UserInternal, *in.Conversation); err != nil {
			return model.Launch{}, model.ErrDenied
		}
	}
	if in.Contact != nil {
		if !slices.Contains(scopes, "contact.read") {
			return model.Launch{}, model.ErrDenied
		}
		if _, err = s.Store.GetContactByUUID(ctx, db.GetContactByUUIDParams{OrganizationID: org.ID, Uuid: *in.Contact}); err != nil {
			return model.Launch{}, model.ErrDenied
		}
	}
	if row.Placement == "conversation_sidebar" && in.Conversation == nil || row.Placement == "contact_panel" && in.Contact == nil {
		return model.Launch{}, model.ErrInvalid
	}
	now := time.Now().UTC()
	claims := model.Claims{App: row.Uuid, Organization: org.UUID, Conversation: in.Conversation, Contact: in.Contact, Scopes: scopes, Revision: row.Revision,
		RegisteredClaims: jwt.RegisteredClaims{Issuer: issuer, Subject: actor.UserID.String(), Audience: jwt.ClaimStrings{row.Uuid.String()}, IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(2 * time.Minute)), ID: uuid.NewString()}}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = keyID
	signed, err := token.SignedString(s.key)
	if err != nil {
		return model.Launch{}, err
	}
	u, _ := url.Parse(row.Url)
	return model.Launch{URL: row.Url, Origin: u.Scheme + "://" + u.Host, Token: signed, ExpiresIn: 120, Scopes: scopes}, nil
}
func (s *Service) Action(ctx context.Context, id uuid.UUID, in model.ActionInput) (any, error) {
	org := orgctx.Must(ctx)
	actor := authctx.MustPrincipal(ctx)
	row, err := s.Store.SidebarAppGet(ctx, db.SidebarAppGetParams{OrganizationID: org.ID, Uuid: id})
	if err != nil {
		return nil, err
	}
	var claims model.Claims
	_, err = jwt.ParseWithClaims(in.Token, &claims, func(*jwt.Token) (any, error) { return s.key.Public(), nil },
		jwt.WithValidMethods([]string{"EdDSA"}), jwt.WithIssuer(issuer), jwt.WithAudience(id.String()), jwt.WithExpirationRequired())
	if err != nil || !row.Enabled || claims.App != id || claims.Organization != org.UUID || claims.Subject != actor.UserID.String() || claims.Revision != row.Revision {
		return nil, model.ErrDenied
	}
	perm, ok := model.Scopes[in.Action]
	if !ok || !org.Has(perm) || !slices.Contains(row.Scopes, in.Action) || !slices.Contains(claims.Scopes, in.Action) {
		return nil, model.ErrDenied
	}
	// Context permissions disclose identifiers only; there is no generic API proxy.
	if claims.Conversation == nil || in.RequestID == uuid.Nil {
		return nil, model.ErrInvalid
	}
	if _, err = s.Conversations.Get(ctx, org.ID, actor.UserInternal, *claims.Conversation); err != nil {
		return nil, model.ErrDenied
	}
	base := "/v1/tenant/conversations/" + claims.Conversation.String()
	method := http.MethodPatch
	path := base
	var body any
	switch in.Action {
	case "conversation.reply", "conversation.note":
		if len(in.Content) == 0 || len(in.Content) > 10000 {
			return nil, model.ErrInvalid
		}
		method = http.MethodPost
		path += "/messages"
		// App and user namespace the idempotency key so another app cannot collide with it.
		key := uuid.NewSHA1(id, []byte(actor.UserID.String()+":"+in.RequestID.String()))
		body = map[string]any{"content": in.Content, "private": in.Action == "conversation.note", "client_id": key}
	case "conversation.status":
		body = map[string]any{"status": in.Value}
	case "conversation.priority":
		body = map[string]any{"priority": in.Value}
	case "conversation.assign":
		if in.Target == uuid.Nil {
			return nil, model.ErrInvalid
		}
		body = map[string]any{"assignee": in.Target}
	case "conversation.tag":
		if in.Target == uuid.Nil {
			return nil, model.ErrInvalid
		}
		method = http.MethodPost
		path += "/tags"
		body = map[string]any{"uuid": in.Target}
	default:
		return nil, model.ErrDenied
	}
	res, err := s.Bridge.JSON(ctx, apibridge.Principal{OrganizationUUID: org.UUID, UserUUID: actor.UserID}, method, path, body)
	if err != nil {
		return nil, err
	}
	if res.Status >= 300 {
		return nil, model.ErrDenied
	}
	return map[string]bool{"applied": true}, nil
}
func IsMissing(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
