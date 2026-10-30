package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	billingrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type BusinessContextInput struct {
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Active    bool   `json:"active"`
	SortOrder int32  `json:"sort_order"`
	Revision  int32  `json:"revision"`
}

func (s *Service) SaveBusinessContext(ctx context.Context, org, actor int64, id uuid.UUID, in BusinessContextInput) (out db.BusinessContextSection, err error) {
	if strings.ContainsRune(in.Body, 0) || strings.ContainsRune(in.Title, 0) {
		return out, model.ErrInvalid
	}
	if ai.Guard(in.Body) != nil {
		return out, ai.ErrInjection
	}
	if !slices.Contains([]string{"company", "products", "pricing", "refunds", "tone", "forbidden", "other"}, in.Kind) || strings.TrimSpace(in.Title) == "" || len(in.Title) > 200 || len(in.Body) > 8000 || actor <= 0 {
		return out, model.ErrInvalid
	}
	err = s.Repo.Within(ctx, func(r *billingrepo.Repository) error {
		if _, e := r.Q.BillingLockOrganizationID(ctx, org); e != nil {
			return e
		}
		if id == uuid.Nil {
			count, e := r.Q.BusinessContextCount(ctx, org)
			if e != nil {
				return e
			}
			if count >= 20 {
				return model.ErrConflict
			}
			out, err = r.Q.BusinessContextCreate(ctx, db.BusinessContextCreateParams{OrganizationID: org, Kind: in.Kind, Title: in.Title, Body: in.Body, Active: in.Active, SortOrder: in.SortOrder, UpdatedBy: actor})
		} else {
			out, err = r.Q.BusinessContextUpdate(ctx, db.BusinessContextUpdateParams{OrganizationID: org, Uuid: id, Kind: in.Kind, Title: in.Title, Body: in.Body, Active: in.Active, SortOrder: in.SortOrder, UpdatedBy: actor, ExpectedRevision: in.Revision})
			if errors.Is(err, pgx.ErrNoRows) {
				return model.ErrConflict
			}
		}
		if err != nil {
			return err
		}
		return r.Q.BusinessContextSnapshot(ctx, db.BusinessContextSnapshotParams{OrganizationID: org, ID: out.ID})
	})
	return
}

// InternalContext ports BusinessContextFormatter's ordering, enabled sections and
// bounded per-section/total budgets. It is used only for internal triage/complex
// tasks. Public reply generation and retrieval never receive these facts.
func (s *Service) InternalContext(ctx context.Context, org int64) (string, error) {
	rows, err := s.Repo.Q.BusinessContextList(ctx, db.BusinessContextListParams{OrganizationID: org, Limit: 20})
	if err != nil {
		return "", err
	}
	var out strings.Builder
	remaining := 6000
	for _, r := range rows {
		if !r.Active || strings.TrimSpace(r.Body) == "" {
			continue
		}
		text := fmt.Sprintf("\nSection %s (%s), revision %d:\n%s\n", r.Title, r.Kind, r.Revision, r.Body)
		text = ai.NewMasker().Mask(text)
		if ai.Guard(text) != nil {
			continue
		}
		runes := []rune(text)
		if len(runes) > 1500 {
			runes = runes[:1500]
		}
		for len(string(runes)) > remaining && len(runes) > 0 {
			runes = runes[:len(runes)-1]
		}
		out.WriteString(string(runes))
		remaining -= len(string(runes))
		if remaining <= 0 {
			break
		}
	}
	return out.String(), nil
}
