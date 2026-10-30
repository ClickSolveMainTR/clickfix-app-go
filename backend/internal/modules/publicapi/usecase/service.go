// Package usecase adapts rentopia-app-go modules/publicapi key hashing and
// idempotent writes to ClickFix's authenticated tenant API.
package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/publicapi/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/publicapi/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/apibridge"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ratelimit"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrInvalid  = errors.New("invalid integration request")
	ErrConflict = errors.New("idempotency key is in progress or was used with a different request")
)

type Service struct {
	Store    *repository.Store
	Bridge   *apibridge.Bridge
	Box      *crypto.SecretBox
	Limiter  *ratelimit.Limiter
	Resolver orgctx.Resolver
}

func TokenHash(raw string) string { h := sha256.Sum256([]byte(raw)); return hex.EncodeToString(h[:]) }

func RandomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *Service) Create(ctx context.Context, org, user int64, in model.KeyInput) (map[string]any, error) {
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 120 || len(in.Scopes) == 0 {
		return nil, ErrInvalid
	}
	for _, v := range in.Scopes {
		if !slices.Contains(model.Scopes, v) {
			return nil, ErrInvalid
		}
	}
	if in.RateLimit == 0 {
		in.RateLimit = 120
	}
	if in.RateLimit < 1 || in.RateLimit > 6000 {
		return nil, ErrInvalid
	}
	var exp pgtype.Timestamptz
	if in.ExpiresAt != nil {
		if !in.ExpiresAt.After(time.Now()) {
			return nil, ErrInvalid
		}
		exp = pgtype.Timestamptz{Time: *in.ExpiresAt, Valid: true}
	}
	raw, err := RandomToken(32)
	if err != nil {
		return nil, err
	}
	raw = "cf_sk_" + raw
	k, err := s.Store.CreateIntegrationKey(ctx, db.CreateIntegrationKeyParams{OrganizationID: org, UserID: user, Name: strings.TrimSpace(in.Name), Scopes: in.Scopes, RateLimit: in.RateLimit, ExpiresAt: exp, TokenHash: TokenHash(raw), KeyPrefix: raw[:12]})
	if err != nil {
		return nil, err
	}
	return map[string]any{"uuid": k.Uuid, "name": k.Name, "key": raw, "scopes": k.Scopes, "rate_limit": k.RateLimit, "expires_at": k.ExpiresAt}, nil
}

func (s *Service) Authenticate(ctx context.Context, raw string) (db.AuthenticateIntegrationKeyRow, error) {
	k, err := s.Store.AuthenticateIntegrationKey(ctx, TokenHash(raw))
	if err != nil {
		return k, err
	}
	if s.Resolver == nil {
		return k, ErrInvalid
	}
	if _, err = s.Resolver.Resolve(ctx, k.OrganizationUuid.String(), k.UserID); err != nil {
		return k, err
	}
	return k, nil
}

// Route is a strict allowlist; integrations cannot reach platform/auth/key routes.
func Route(method, path string) (string, string, bool) {
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return "", "", false
	}
	resource := parts[0]
	scope := ""
	target := "/v1/tenant/" + path
	switch resource {
	case "triggers":
		if len(parts) == 1 && method == http.MethodGet {
			return "/v1/tenant/integration-triggers", "triggers:read", true
		}
	case "subscriptions":
		if len(parts) == 1 && method == http.MethodPost {
			return "/v1/tenant/webhooks", "webhooks:write", true
		}
		if len(parts) == 2 && method == http.MethodDelete {
			if _, err := uuid.Parse(parts[1]); err == nil {
				return "/v1/tenant/webhooks/" + parts[1], "webhooks:write", true
			}
		}
	case "conversations":
		if len(parts) == 1 && method == http.MethodPost {
			return target, "conversations:write", true
		}
		scope = "conversations:read"
		if len(parts) == 1 && method == http.MethodGet {
			return target, scope, true
		}
		if len(parts) < 2 {
			return "", "", false
		}
		if _, err := uuid.Parse(parts[1]); err != nil {
			return "", "", false
		}
		if len(parts) == 2 && (method == http.MethodGet || method == http.MethodPatch) {
			if method == http.MethodPatch {
				scope = "conversations:write"
			}
			return target, scope, true
		}
		if len(parts) == 3 && parts[2] == "messages" && (method == http.MethodGet || method == http.MethodPost) {
			if method == http.MethodPost {
				scope = "conversations:write"
			}
			return target, scope, true
		}
	case "contacts", "companies":
		if resource == "contacts" && len(parts) == 2 && method == http.MethodPatch {
			if _, err := uuid.Parse(parts[1]); err == nil {
				return target, "contacts:write", true
			}
		}

		if len(parts) == 1 && method == http.MethodGet {
			return "/v1/tenant/integration-" + resource, resource + ":read", true
		}
	case "company-revenue":
		// CF-86: billing systems push customer revenue (upsert by company
		// uuid, external id or domain).
		if len(parts) == 1 && method == http.MethodPut {
			return target, "revenue:write", true
		}
	case "company-usage":
		// CF-87: the organization's product reports a customer's usage (0-100).
		if len(parts) == 1 && method == http.MethodPut {
			return target, "health:write", true
		}
	case "company-events":
		// CF-85: the organization's product sends in-app events to the
		// company timeline.
		if len(parts) == 1 && method == http.MethodPost {
			return target, "events:write", true
		}
	case "webhooks":
		scope = "webhooks:read"
		if method != http.MethodGet {
			scope = "webhooks:write"
		}
		if len(parts) == 1 && (method == http.MethodGet || method == http.MethodPost) {
			return target, scope, true
		}
		if len(parts) == 2 {
			if _, err := uuid.Parse(parts[1]); err == nil && (method == http.MethodPut || method == http.MethodDelete) {
				return target, scope, true
			}
		}
	}
	return "", "", false
}

func (s *Service) Execute(ctx context.Context, k db.AuthenticateIntegrationKeyRow, method, target, requestKey string, body []byte) (apibridge.Result, error) {
	p := apibridge.Principal{UserUUID: k.UserUuid, OrganizationUUID: k.OrganizationUuid}
	if method == http.MethodGet {
		return s.Bridge.Do(ctx, p, method, target, body)
	}
	if requestKey == "" || len(requestKey) > 200 {
		return apibridge.Result{}, ErrInvalid
	}
	fingerprint := TokenHash(method + "\n" + target + "\n" + string(body))
	n, err := s.Store.ClaimIntegrationRequest(ctx, db.ClaimIntegrationRequestParams{OrganizationID: k.OrganizationID, KeyID: k.ID, RequestKey: requestKey, RequestHash: fingerprint})
	if err != nil {
		return apibridge.Result{}, err
	}
	if n == 0 {
		old, err := s.Store.GetIntegrationRequest(ctx, db.GetIntegrationRequestParams{OrganizationID: k.OrganizationID, KeyID: k.ID, RequestKey: requestKey})
		if err != nil {
			return apibridge.Result{}, err
		}
		if old.RequestHash != fingerprint || !old.StatusCode.Valid {
			return apibridge.Result{}, ErrConflict
		}
		raw, err := s.Box.Decrypt(old.ResponseEnc.String)
		return apibridge.Result{Status: int(old.StatusCode.Int32), Body: []byte(raw)}, err
	}
	res, err := s.Bridge.Do(ctx, p, method, target, body)
	if err != nil {
		return res, err
	}
	if res.Truncated {
		res = apibridge.Result{Status: 502, Body: []byte(`{"success":false,"error":{"code":"RESULT_TOO_LARGE","message":"Narrow the request"}}`)}
	}
	enc, err := s.Box.Encrypt(string(res.Body))
	if err != nil {
		return res, err
	}
	err = s.Store.FinishIntegrationRequest(ctx, db.FinishIntegrationRequestParams{OrganizationID: k.OrganizationID, KeyID: k.ID, RequestKey: requestKey, StatusCode: pgtype.Int4{Int32: safeconv.ToInt32(res.Status), Valid: true}, ResponseEnc: pgtype.Text{String: enc, Valid: true}})
	if err != nil {
		return apibridge.Result{}, fmt.Errorf("record integration response: %w", err)
	}
	return res, nil
}
