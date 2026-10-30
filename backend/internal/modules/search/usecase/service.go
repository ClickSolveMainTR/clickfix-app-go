package usecase

import (
	"context"
	"log/slog"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/access"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/searchengine"
)

// Service coordinates search queries and spec listing.
type Service struct {
	client *searchengine.Client
	reg    *searchengine.Registry
	log    *slog.Logger
	// convAccess applies the inbox team visibility rule to tenant hits
	// (CF-44, WithConversationAccess).
	convAccess access.Queries
}

// New creates a search service.
func New(client *searchengine.Client, reg *searchengine.Registry, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{client: client, reg: reg, log: log}
}

// Enabled reports whether remote search is available.
func (s *Service) Enabled() bool {
	return s != nil && s.client != nil && s.client.Enabled()
}

// ListSpecs returns specs visible to the principal.
func (s *Service) ListSpecs(ctx context.Context) []searchengine.Spec {
	if s == nil || s.reg == nil {
		return nil
	}
	p, ok := authctx.PrincipalFrom(ctx)
	if !ok {
		return nil
	}
	out := make([]searchengine.Spec, 0, len(s.reg.Specs()))
	for _, spec := range s.reg.Specs() {
		// Tenant specs need an organization filter: /v1/tenant/search only.
		if spec.Tenant {
			continue
		}
		if spec.Permission == "" || p.HasPermission(spec.Permission) {
			out = append(out, spec)
		}
	}
	return out
}

// Search queries indexed records for allowed specs.
func (s *Service) Search(ctx context.Context, q, spec string, limit int) []searchengine.Hit {
	if !s.Enabled() {
		return nil
	}
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	specs := s.allowedSpecs(ctx, spec)
	if len(specs) == 0 {
		return nil
	}
	hits, err := s.client.Search(ctx, specs, q, limit)
	if err != nil {
		s.log.Warn("search_query_failed", "error", err)
		return nil
	}
	return hits
}

func (s *Service) allowedSpecs(ctx context.Context, requested string) []string {
	requested = strings.TrimSpace(requested)
	all := s.ListSpecs(ctx)
	if requested != "" {
		for _, spec := range all {
			if spec.ID == requested {
				return []string{requested}
			}
		}
		return nil
	}
	out := make([]string, 0, len(all))
	for _, spec := range all {
		out = append(out, spec.ID)
	}
	return out
}
