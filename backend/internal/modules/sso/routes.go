// Package sso mounts organization sign-in, configuration and SCIM 2.0 endpoints.
package sso

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sso/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/entitlement"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// mapError turns SSO usecase errors into the shared support-operations errors
// the router writes as 4xx (an unmapped sentinel would become a 500).
func mapError(e error) error {
	switch {
	case errors.Is(e, usecase.ErrInvalid):
		return supportops.ErrInvalid
	case errors.Is(e, usecase.ErrDenied):
		return supportops.ErrDenied
	}
	return e
}

func RegisterRoutes(rt supportops.Router, s *usecase.Service) {
	rt.Tenant("GET /v1/tenant/security/sso", "tenant.settings.write", func(r *http.Request) (any, error) {
		v, e := s.Repo.Q.SSOConfig(r.Context(), orgctx.Must(r.Context()).ID)
		if errors.Is(e, pgx.ErrNoRows) {
			return usecase.Config{Protocol: "oidc", IPAllowlist: []string{}}, nil
		}
		return usecase.View(v), e
	})
	rt.Tenant("PUT /v1/tenant/security/sso", "tenant.settings.write", func(r *http.Request) (any, error) {
		in, e := supportops.Read[usecase.Config](r)
		if e != nil {
			return nil, e
		}
		// An allow-list that excludes the caller would lock this session out at once.
		if len(in.IPAllowlist) > 0 && !usecase.AllowedIP(s.ClientIP(r), in.IPAllowlist) {
			return nil, supportops.ErrConflict
		}
		v, e := s.Save(r.Context(), orgctx.Must(r.Context()).ID, in)
		return v, mapError(e)
	})
	// The address the allow-list is checked against (after trusted proxies).
	rt.Tenant("GET /v1/tenant/security/client-ip", "tenant.settings.write", func(r *http.Request) (any, error) {
		host, _, e := net.SplitHostPort(s.ClientIP(r))
		if e != nil {
			host = s.ClientIP(r)
		}
		return map[string]string{"ip": host}, nil
	})
	rt.Tenant("POST /v1/tenant/security/scim-token", "tenant.settings.write", func(r *http.Request) (any, error) {
		org := orgctx.Must(r.Context()).ID
		if e := entitlement.Require(r.Context(), s.Entitlements, org, "security.sso"); e != nil {
			return nil, e
		}
		token, e := s.RotateSCIM(r.Context(), org)
		if e != nil {
			return nil, mapError(e)
		}
		return map[string]string{"token": token, "base_url": strings.TrimRight(s.BaseURL, "/") + "/v1/scim/" + orgctx.Must(r.Context()).UUID.String() + "/v2"}, nil
	})
	rt.Mux.HandleFunc("GET /v1/public/sso/{organization}/start", func(w http.ResponseWriter, r *http.Request) {
		if s.Limit != nil {
			ok, _ := s.Limit(r.Context(), "sso_start", s.ClientIP(r))
			if !ok {
				response.Error(w, r, 429, "RATE_LIMITED", "Too many SSO requests")
				return
			}
		}
		id, _ := uuid.Parse(r.PathValue("organization"))
		out, e := s.Start(r.Context(), id, s.ClientIP(r))
		if e != nil {
			response.Forbidden(w, r, "Organization SSO unavailable")
			return
		}
		same := http.SameSiteLaxMode
		if out.Protocol == "saml" {
			same = http.SameSiteNoneMode
		}
		http.SetCookie(w, &http.Cookie{Name: "__Host-clickfix-sso", Value: out.State, Path: "/", MaxAge: 300, HttpOnly: true, Secure: true, SameSite: same})
		w.Header().Set("Cache-Control", "no-store")
		response.JSON(w, r, 200, out)
	})
	complete := func(protocol string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
			if e := r.ParseForm(); e != nil {
				response.BadRequest(w, r, "INVALID_SSO_RESPONSE", "Invalid SSO response")
				return
			}
			state := r.Form.Get("state")
			if protocol == "saml" {
				state = r.Form.Get("RelayState")
			}
			cookie, e := r.Cookie("__Host-clickfix-sso")
			if e != nil || state == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(state)) != 1 {
				response.Forbidden(w, r, "SSO state mismatch")
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "__Host-clickfix-sso", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteNoneMode})
			id, _ := uuid.Parse(r.PathValue("organization"))
			out, e := s.Complete(r.Context(), id, state, r.Form.Get("code"), protocol, s.ClientIP(r), r)
			if e != nil {
				response.Forbidden(w, r, "SSO authentication failed")
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			response.JSON(w, r, 200, out)
		}
	}
	rt.Mux.HandleFunc("GET /v1/public/sso/{organization}/oidc/callback", complete("oidc"))
	rt.Mux.HandleFunc("POST /v1/public/sso/{organization}/saml/callback", complete("saml"))
	rt.Mux.HandleFunc("GET /v1/public/sso/{organization}/saml/metadata", func(w http.ResponseWriter, r *http.Request) {
		id, _ := uuid.Parse(r.PathValue("organization"))
		b, e := s.Metadata(id)
		if e != nil {
			response.NotFound(w, r, "SSO metadata unavailable")
			return
		}
		w.Header().Set("Content-Type", "application/samlmetadata+xml")
		_, _ = w.Write(b)
	})
	registerSCIM(rt.Mux, s)
}

const listSchema = "urn:ietf:params:scim:api:messages:2.0:ListResponse"

func scimJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/scim+json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func scimError(w http.ResponseWriter, status int, detail string) {
	scimJSON(w, status, map[string]any{"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:Error"}, "status": strconv.Itoa(status), "detail": detail})
}
func scimFail(w http.ResponseWriter, e error) {
	var pe *pgconn.PgError
	if errors.Is(e, pgx.ErrNoRows) {
		scimError(w, 404, "Resource not found")
	} else if errors.Is(e, usecase.ErrDenied) {
		scimError(w, 403, "Operation denied")
	} else if errors.Is(e, usecase.ErrInvalid) || errors.Is(e, supportops.ErrInvalid) {
		scimError(w, 400, "Invalid SCIM input")
	} else if errors.As(e, &pe) && (pe.Code == "23505" || pe.Code == "23503") {
		scimError(w, 409, "Resource conflicts with existing data")
	} else {
		scimError(w, 500, "Provisioning operation failed")
	}
}
func scimNumber(raw string, fallback int32) (int32, error) {
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, err
	}
	return int32(value), nil
}
func page(r *http.Request) (int32, int32, error) {
	start, err := scimNumber(r.URL.Query().Get("startIndex"), 1)
	if err != nil {
		return 0, 0, err
	}
	count, err := scimNumber(r.URL.Query().Get("count"), 100)
	if err != nil {
		return 0, 0, err
	}
	if start < 1 || start > 1<<30 || count < 0 || count > 100 {
		return 0, 0, usecase.ErrInvalid
	}
	return count, start - 1, nil
}
func list(w http.ResponseWriter, items any, total int64, offset int32, count int) {
	scimJSON(w, 200, map[string]any{"schemas": []string{listSchema}, "totalResults": total, "startIndex": offset + 1, "itemsPerPage": count, "Resources": items})
}
func registerSCIM(mux *http.ServeMux, s *usecase.Service) {
	type endpoint func(http.ResponseWriter, *http.Request, int64)
	route := func(pattern string, fn endpoint) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			id, e := uuid.Parse(r.PathValue("organization"))
			if e != nil {
				scimError(w, 401, "Invalid credentials")
				return
			}
			o, e := s.Repo.Q.SSOOrganization(r.Context(), id)
			if e != nil {
				scimError(w, 401, "Invalid credentials")
				return
			}
			c, e := s.Repo.Q.SSOConfig(r.Context(), o.ID)
			parts := strings.Fields(r.Header.Get("Authorization"))
			if e != nil || len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || c.ScimTokenHash == "" || subtle.ConstantTimeCompare([]byte(usecase.Hash(parts[1])), []byte(c.ScimTokenHash)) != 1 || !usecase.AllowedIP(s.ClientIP(r), c.IpAllowlist) {
				scimError(w, 401, "Invalid credentials")
				return
			}
			if e = entitlement.Require(r.Context(), s.Entitlements, o.ID, "security.sso"); e != nil {
				scimError(w, 403, "SSO entitlement required")
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			if s.Limit != nil {
				ok, _ := s.Limit(r.Context(), "scim", strconv.FormatInt(o.ID, 10))
				if !ok {
					scimError(w, 429, "Provisioning rate limit exceeded")
					return
				}
			}
			audit := &scimWriter{ResponseWriter: w}
			fn(audit, r, o.ID)
			if r.Method != http.MethodGet && audit.status >= 200 && audit.status < 300 {
				s.Activity.Record(orgctx.WithOrganizationID(r.Context(), o.ID), nil, "security.scim_updated", "sso", nil, map[string]any{"method": r.Method, "route": r.Pattern}, nil)
			}
		})
	}
	route("GET /v1/scim/{organization}/v2/ServiceProviderConfig", func(w http.ResponseWriter, r *http.Request, org int64) {
		scimJSON(w, 200, map[string]any{"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"}, "patch": map[string]bool{"supported": true}, "bulk": map[string]any{"supported": false, "maxOperations": 0, "maxPayloadSize": 0}, "filter": map[string]any{"supported": true, "maxResults": 100}, "changePassword": map[string]bool{"supported": false}, "sort": map[string]bool{"supported": false}, "etag": map[string]bool{"supported": false}, "authenticationSchemes": []any{map[string]string{"type": "oauthbearertoken", "name": "Bearer token", "description": "Organization-scoped provisioning token"}}})
	})
	route("GET /v1/scim/{organization}/v2/Users", func(w http.ResponseWriter, r *http.Request, org int64) {
		count, offset, e := page(r)
		if e != nil {
			scimError(w, 400, "Invalid pagination")
			return
		}
		filter := r.URL.Query().Get("filter")
		if filter != "" { // Providers query userName/externalId before creating a resource.
			field, value, ok := strings.Cut(filter, " eq ")
			if !ok || (field != "userName" && field != "externalId") {
				scimError(w, 400, "Unsupported filter")
				return
			}
			var query string
			if json.Unmarshal([]byte(value), &query) != nil {
				scimError(w, 400, "Invalid filter")
				return
			}
			rows, e := s.Repo.Q.SCIMFilterUsers(r.Context(), db.SCIMFilterUsersParams{OrganizationID: org, Field: field, Value: query})
			if e != nil {
				scimFail(w, e)
				return
			}
			out := []usecase.SCIMUser{}
			for _, v := range rows {
				out = append(out, usecase.UserView(v.Uuid, v.ExternalID, v.Email, v.Name, v.Surname, v.Active))
			}
			list(w, out, int64(len(out)), 0, len(out))
			return
		}
		rows, e := s.Repo.Q.SCIMUsers(r.Context(), db.SCIMUsersParams{OrganizationID: org, Limit: count, Offset: offset})
		if e != nil {
			scimFail(w, e)
			return
		}
		total, e := s.Repo.Q.SCIMUserCount(r.Context(), org)
		if e != nil {
			scimFail(w, e)
			return
		}
		out := []usecase.SCIMUser{}
		for _, v := range rows {
			out = append(out, usecase.UserView(v.Uuid, v.ExternalID, v.Email, v.Name, v.Surname, v.Active))
		}
		list(w, out, total, offset, len(out))
	})
	route("POST /v1/scim/{organization}/v2/Users", func(w http.ResponseWriter, r *http.Request, org int64) {
		var in usecase.SCIMUser
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			scimError(w, 400, "Invalid user")
			return
		}
		v, e := s.Provision(r.Context(), org, in)
		if e != nil {
			scimFail(w, e)
			return
		}
		scimJSON(w, 201, v)
	})
	route("GET /v1/scim/{organization}/v2/Users/{uuid}", func(w http.ResponseWriter, r *http.Request, org int64) {
		v, e := s.Repo.Q.SCIMUser(r.Context(), db.SCIMUserParams{OrganizationID: org, Uuid: supportops.ID(r)})
		if e != nil {
			scimFail(w, e)
			return
		}
		scimJSON(w, 200, usecase.UserView(v.Uuid, v.ExternalID, v.Email, v.Name, v.Surname, v.Active))
	})
	route("PUT /v1/scim/{organization}/v2/Users/{uuid}", func(w http.ResponseWriter, r *http.Request, org int64) {
		var in usecase.SCIMUser
		if json.NewDecoder(r.Body).Decode(&in) != nil || in.Active == nil {
			scimError(w, 400, "Invalid user")
			return
		}
		old, e := s.Repo.Q.SCIMUser(r.Context(), db.SCIMUserParams{OrganizationID: org, Uuid: supportops.ID(r)})
		if e != nil {
			scimFail(w, e)
			return
		}
		if in.UserName != old.Email || in.ExternalID != "" && in.ExternalID != old.ExternalID {
			scimError(w, 400, "Global profile and subject are immutable through organization SCIM")
			return
		}
		v, e := s.SetActive(r.Context(), org, supportops.ID(r), *in.Active)
		if e != nil {
			scimFail(w, e)
			return
		}
		scimJSON(w, 200, v)
	})
	route("PATCH /v1/scim/{organization}/v2/Users/{uuid}", func(w http.ResponseWriter, r *http.Request, org int64) {
		var in struct {
			Operations []struct {
				Op    string          `json:"op"`
				Path  string          `json:"path"`
				Value json.RawMessage `json:"value"`
			} `json:"Operations"`
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil || len(in.Operations) != 1 {
			scimError(w, 400, "One active operation required")
			return
		}
		op := in.Operations[0]
		var active bool
		if !strings.EqualFold(op.Op, "replace") && !strings.EqualFold(op.Op, "add") {
			scimError(w, 400, "Unsupported operation")
			return
		}
		if strings.EqualFold(op.Path, "active") {
			if json.Unmarshal(op.Value, &active) != nil {
				scimError(w, 400, "Invalid active value")
				return
			}
		} else if op.Path == "" {
			var v struct {
				Active *bool `json:"active"`
			}
			if json.Unmarshal(op.Value, &v) != nil || v.Active == nil {
				scimError(w, 400, "Invalid active operation")
				return
			}
			active = *v.Active
		} else {
			scimError(w, 400, "Only organization membership active is mutable")
			return
		}
		v, e := s.SetActive(r.Context(), org, supportops.ID(r), active)
		if e != nil {
			scimFail(w, e)
			return
		}
		scimJSON(w, 200, v)
	})
	route("DELETE /v1/scim/{organization}/v2/Users/{uuid}", func(w http.ResponseWriter, r *http.Request, org int64) {
		_, e := s.SetActive(r.Context(), org, supportops.ID(r), false)
		if e != nil {
			scimFail(w, e)
			return
		}
		w.WriteHeader(204)
	})
	route("GET /v1/scim/{organization}/v2/Groups", func(w http.ResponseWriter, r *http.Request, org int64) {
		count, offset, e := page(r)
		if e != nil {
			scimError(w, 400, "Invalid pagination")
			return
		}
		ids := []uuid.UUID{}
		var total int64
		if filter := r.URL.Query().Get("filter"); filter != "" {
			field, raw, ok := strings.Cut(filter, " eq ")
			var value string
			if !ok || (field != "displayName" && field != "externalId") || json.Unmarshal([]byte(raw), &value) != nil {
				scimError(w, 400, "Unsupported group filter")
				return
			}
			ids, e = s.Repo.Q.SCIMFilterGroups(r.Context(), db.SCIMFilterGroupsParams{OrganizationID: org, Field: field, Value: value, Limit: count, Offset: offset})
			if e != nil {
				scimFail(w, e)
				return
			}
			total, e = s.Repo.Q.SCIMFilterGroupCount(r.Context(), db.SCIMFilterGroupCountParams{OrganizationID: org, Field: field, Value: value})
		} else {
			rows, err := s.Repo.Q.SCIMGroups(r.Context(), db.SCIMGroupsParams{OrganizationID: org, Limit: count, Offset: offset})
			if err != nil {
				scimFail(w, err)
				return
			}
			for _, v := range rows {
				ids = append(ids, v.Uuid)
			}
			total, e = s.Repo.Q.SCIMGroupCount(r.Context(), org)
		}
		if e != nil {
			scimFail(w, e)
			return
		}
		out := []usecase.SCIMGroup{}
		for _, id := range ids {
			v, e := s.Group(r.Context(), org, id)
			if e != nil {
				scimFail(w, e)
				return
			}
			out = append(out, v)
		}
		list(w, out, total, offset, len(out))
	})
	route("GET /v1/scim/{organization}/v2/Groups/{uuid}", func(w http.ResponseWriter, r *http.Request, org int64) {
		v, e := s.Group(r.Context(), org, supportops.ID(r))
		if e != nil {
			scimFail(w, e)
			return
		}
		scimJSON(w, 200, v)
	})
	for _, method := range []string{"POST", "PUT"} {
		pattern := method + " /v1/scim/{organization}/v2/Groups"
		if method == "PUT" {
			pattern += "/{uuid}"
		}
		route(pattern, func(w http.ResponseWriter, r *http.Request, org int64) {
			var in usecase.SCIMGroup
			if json.NewDecoder(r.Body).Decode(&in) != nil {
				scimError(w, 400, "Invalid group")
				return
			}
			v, e := s.SaveGroup(r.Context(), org, supportops.ID(r), in)
			if e != nil {
				scimFail(w, e)
				return
			}
			status := 200
			if r.Method == "POST" {
				status = 201
			}
			scimJSON(w, status, v)
		})
	}
	route("PATCH /v1/scim/{organization}/v2/Groups/{uuid}", func(w http.ResponseWriter, r *http.Request, org int64) { patchGroup(w, r, org, s) })
	route("DELETE /v1/scim/{organization}/v2/Groups/{uuid}", func(w http.ResponseWriter, r *http.Request, org int64) {
		v, e := s.Repo.Q.SCIMGroup(r.Context(), db.SCIMGroupParams{OrganizationID: org, Uuid: supportops.ID(r)})
		if e == nil {
			e = s.Repo.Q.SCIMDeleteGroup(r.Context(), db.SCIMDeleteGroupParams{OrganizationID: org, ID: v.TeamID})
		}
		if e != nil {
			scimFail(w, e)
			return
		}
		w.WriteHeader(204)
	})
}
func patchGroup(w http.ResponseWriter, r *http.Request, org int64, s *usecase.Service) {
	v, e := s.Group(r.Context(), org, supportops.ID(r))
	if e != nil {
		scimFail(w, e)
		return
	}
	var in struct {
		Operations []struct {
			Op    string          `json:"op"`
			Path  string          `json:"path"`
			Value json.RawMessage `json:"value"`
		} `json:"Operations"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || len(in.Operations) == 0 || len(in.Operations) > 100 {
		scimError(w, 400, "Invalid group patch")
		return
	}
	for _, op := range in.Operations {
		switch strings.ToLower(op.Path) {
		case "displayname":
			if !strings.EqualFold(op.Op, "replace") || json.Unmarshal(op.Value, &v.DisplayName) != nil {
				scimError(w, 400, "Invalid displayName")
				return
			}
		case "members":
			var members []usecase.Member
			if strings.EqualFold(op.Op, "remove") && len(op.Value) == 0 {
				v.Members = []usecase.Member{}
				continue
			}
			if json.Unmarshal(op.Value, &members) != nil {
				scimError(w, 400, "Invalid members")
				return
			}
			switch strings.ToLower(op.Op) {
			case "replace":
				v.Members = members
			case "add":
				v.Members = append(v.Members, members...)
			case "remove":
				kept := []usecase.Member{}
				for _, m := range v.Members {
					remove := false
					for _, n := range members {
						if m.Value == n.Value {
							remove = true
						}
					}
					if !remove {
						kept = append(kept, m)
					}
				}
				v.Members = kept
			default:
				scimError(w, 400, "Unsupported member operation")
				return
			}
		default:
			if strings.EqualFold(op.Op, "remove") && strings.HasPrefix(strings.ToLower(op.Path), "members[value eq ") && strings.HasSuffix(op.Path, "]") {
				raw := op.Path[len("members[value eq ") : len(op.Path)-1]
				var value string
				if json.Unmarshal([]byte(raw), &value) != nil {
					scimError(w, 400, "Invalid member filter")
					return
				}
				id, e := uuid.Parse(value)
				if e != nil {
					scimError(w, 400, "Invalid member id")
					return
				}
				kept := []usecase.Member{}
				for _, m := range v.Members {
					if m.Value != id {
						kept = append(kept, m)
					}
				}
				v.Members = kept
				continue
			}
			scimError(w, 400, "Unsupported group path")
			return
		}
	}
	out, e := s.SaveGroup(r.Context(), org, supportops.ID(r), v)
	if e != nil {
		scimFail(w, e)
		return
	}
	scimJSON(w, 200, out)
}

type scimWriter struct {
	http.ResponseWriter
	status int
}

func (w *scimWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
		w.ResponseWriter.WriteHeader(status)
	}
}
func (w *scimWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}
