package usecase

import (
	"context"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forms/engine"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forms/model"
)

// ResolveFields resolves form fields stored outside request forms (CF-138
// appointment intake forms) against the organization's custom fields.
// strict rejects fields of archived or unknown custom fields (on save).
func (s *Service) ResolveFields(ctx context.Context, orgID int64, fields []engine.Field, strict bool) ([]engine.Resolved, error) {
	customs, err := s.customs(ctx, s.q, orgID)
	if err != nil {
		return nil, err
	}
	return engine.Resolve(fields, customs, strict)
}

// PublicFields is what an intake form shows to an invitee.
func PublicFields(fields []engine.Resolved) []model.PublicField {
	return toPublic(db.RequestForm{}, fields).Fields
}
