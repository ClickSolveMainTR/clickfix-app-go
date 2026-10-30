package usecase

import (
	"context"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/access"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/searchengine"
	searchadapters "github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/searchengine/adapters"
	"github.com/google/uuid"
)

// WithConversationAccess lets the tenant search apply the inbox team
// visibility rule (CF-44) to conversation and message hits.
func (s *Service) WithConversationAccess(q access.Queries) *Service {
	s.convAccess = q
	return s
}

// TenantSpecs returns the organization specs the member's tenant
// permissions allow (CF-45).
func (s *Service) TenantSpecs(scope orgctx.Scope) []searchengine.Spec {
	if s == nil || s.reg == nil {
		return nil
	}
	out := make([]searchengine.Spec, 0, 4)
	for _, spec := range s.reg.Specs() {
		if !spec.Tenant {
			continue
		}
		if spec.Permission == "" || scope.Has(spec.Permission) {
			out = append(out, spec)
		}
	}
	return out
}

// restrictedOverfetch widens the index query of a member without read_all:
// hidden conversation hits are dropped after the query.
const restrictedOverfetch = 3

// TenantSearch queries the organization's indexes. The organization filter
// is always applied; spec narrows to one allowed spec. For a member without
// tenant.conversations.read_all, conversation and message hits are kept only
// when the database says the member may see the conversation (CF-44). The
// rule is checked against the database rather than indexed as a filter:
// assignment changes would otherwise have to reindex every message of the
// conversation, and a stale index must not widen visibility.
func (s *Service) TenantSearch(ctx context.Context, scope orgctx.Scope, userID int64, q, spec string, limit int) []searchengine.Hit {
	if !s.Enabled() || scope.ID <= 0 {
		return nil
	}
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	specs := TenantSpecIDs(s.TenantSpecs(scope), spec)
	if len(specs) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = searchengine.DefaultLimit
	}
	if limit > searchengine.MaxPaletteLimit {
		limit = searchengine.MaxPaletteLimit
	}
	restricted := !scope.CanSeeAll(access.Resource)
	fetch := limit
	if restricted {
		fetch = limit * restrictedOverfetch
	}
	hits, err := s.client.Query(ctx, searchengine.Query{
		Specs: specs, Q: q, Limit: fetch, Filter: searchengine.OrgFilter(scope.ID), Snippet: true,
	})
	if err != nil {
		s.log.Warn("tenant_search_query_failed", "organization", scope.ID, "error", err)
		return nil
	}
	if restricted {
		hits = s.visibleHits(ctx, scope, userID, hits)
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

// visibleHits drops conversation and message hits of conversations the
// member may not see. Without the access queries (or on an error) every
// such hit is dropped: the filter fails closed.
func (s *Service) visibleHits(ctx context.Context, scope orgctx.Scope, userID int64, hits []searchengine.Hit) []searchengine.Hit {
	ids := make([]uuid.UUID, 0, len(hits))
	for _, h := range hits {
		if id, ok := hitConversation(h); ok {
			ids = append(ids, id)
		}
	}
	visible := map[uuid.UUID]bool{}
	if len(ids) > 0 && s.convAccess != nil {
		v, err := access.For(ctx, s.convAccess, scope, userID)
		if err == nil {
			visible, err = v.Visible(ctx, s.convAccess, scope.ID, ids)
		}
		if err != nil {
			s.log.Warn("tenant_search_visibility_failed", "organization", scope.ID, "error", err)
			visible = map[uuid.UUID]bool{}
		}
	}
	out := hits[:0]
	for _, h := range hits {
		if id, ok := hitConversation(h); ok && !visible[id] {
			continue
		}
		out = append(out, h)
	}
	return out
}

// hitConversation returns the conversation of a conversation or message hit.
// A hit of those specs without a parsable conversation id is treated as one
// (uuid.Nil, never visible).
func hitConversation(h searchengine.Hit) (uuid.UUID, bool) {
	if h.Spec != searchadapters.SpecConversations && h.Spec != searchadapters.SpecMessages {
		return uuid.Nil, false
	}
	raw := h.ConversationID
	if raw == "" && h.Spec == searchadapters.SpecConversations {
		raw = h.ID
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, true
	}
	return id, true
}

// TenantSpecIDs narrows allowed specs to requested (empty: all allowed).
func TenantSpecIDs(allowed []searchengine.Spec, requested string) []string {
	requested = strings.TrimSpace(requested)
	out := make([]string, 0, len(allowed))
	for _, spec := range allowed {
		if requested == "" || spec.ID == requested {
			out = append(out, spec.ID)
		}
	}
	return out
}
