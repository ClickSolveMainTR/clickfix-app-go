package usecase

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// FieldSource lists custom field definitions (customfields usecase).
type FieldSource interface {
	Definitions(ctx context.Context, orgID int64, entity string, objectTypeID *int64) ([]customfields.Definition, error)
}

// addCustomFields puts the workflow-visible custom fields into the data
// map: conversation fields are filtered in place, contact and company
// fields land under contact.custom_fields and company.custom_fields.
func (s *Service) addCustomFields(ctx context.Context, org int64, out map[string]any, contact uuid.UUID, company *uuid.UUID) error {
	if s.Fields == nil {
		return nil
	}
	defsOf := func(entity string) ([]customfields.Definition, error) {
		return s.Fields.Definitions(ctx, org, entity, nil)
	}
	if raw, ok := out["custom_fields"].(map[string]any); ok {
		defs, err := defsOf(customfields.EntityConversation)
		if err != nil {
			return err
		}
		out["custom_fields"] = customfields.FilterFor(defs, raw, customfields.AudienceWorkflow)
	}
	put := func(entity, key string, stored []byte) error {
		values := map[string]any{}
		_ = json.Unmarshal(stored, &values)
		defs, err := defsOf(entity)
		if err != nil {
			return err
		}
		ref, _ := out[key].(map[string]any)
		if ref == nil {
			ref = map[string]any{}
			out[key] = ref
		}
		ref["custom_fields"] = customfields.FilterFor(defs, values, customfields.AudienceWorkflow)
		return nil
	}
	if contact != uuid.Nil {
		row, err := s.Store.CustomFieldsOfContact(ctx, db.CustomFieldsOfContactParams{OrganizationID: org, Uuid: contact})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err == nil {
			if err := put(customfields.EntityContact, "contact", row.CustomFields); err != nil {
				return err
			}
		}
	}
	if company != nil {
		row, err := s.Store.CustomFieldsOfCompany(ctx, db.CustomFieldsOfCompanyParams{OrganizationID: org, Uuid: *company})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err == nil {
			if err := put(customfields.EntityCompany, "company", row.CustomFields); err != nil {
				return err
			}
		}
	}
	return nil
}
