package usecase

import (
	"context"
	"encoding/json"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sla/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
)

// FieldSource lists custom field definitions (customfields usecase).
type FieldSource interface {
	Definitions(ctx context.Context, orgID int64, entity string, objectTypeID *int64) ([]customfields.Definition, error)
}

// WithFields enables custom field conditions (CF-196 catalog).
func (s *Service) WithFields(f FieldSource) *Service { s.fields = f; return s }

// normalizeFieldMatches checks custom field conditions against the catalog
// and stores option ids (a label typed by an older client resolves to its id).
func (s *Service) normalizeFieldMatches(ctx context.Context, orgID int64, c *model.Conditions) error {
	if len(c.CustomFields) == 0 {
		c.CustomFields = []customfields.FieldMatch{}
		return nil
	}
	if s.fields == nil {
		return invalidf("custom field conditions are not available")
	}
	defs := map[string][]customfields.Definition{}
	for _, m := range c.CustomFields {
		if _, done := defs[m.Entity]; done {
			continue
		}
		list, err := s.fields.Definitions(ctx, orgID, m.Entity, nil)
		if err != nil {
			return err
		}
		defs[m.Entity] = list
	}
	out, errs := customfields.NormalizeMatches(c.CustomFields, func(e string) []customfields.Definition { return defs[e] })
	if len(errs) > 0 {
		return invalidf("%s: %s", errs[0].Field, errs[0].Message)
	}
	c.CustomFields = out
	return nil
}

// fieldsMatch evaluates custom field conditions against the conversation,
// its contact and its company.
func (s *Service) fieldsMatch(ctx context.Context, q *db.Queries, conv db.Conversation, matches []customfields.FieldMatch) (bool, error) {
	values := map[string]map[string]any{}
	load := func(entity string) (map[string]any, error) {
		if v, ok := values[entity]; ok {
			return v, nil
		}
		var raw []byte
		var err error
		switch entity {
		case customfields.EntityConversation:
			raw = conv.CustomFields
		case customfields.EntityContact:
			if conv.ContactID > 0 {
				raw, err = q.ContactCustomFieldsByID(ctx, db.ContactCustomFieldsByIDParams{OrganizationID: conv.OrganizationID, ID: conv.ContactID})
			}
		case customfields.EntityCompany:
			if conv.CompanyID.Valid {
				raw, err = q.CompanyCustomFieldsByID(ctx, db.CompanyCustomFieldsByIDParams{OrganizationID: conv.OrganizationID, ID: conv.CompanyID.Int64})
			}
		}
		if err != nil && !isNoRows(err) {
			return nil, err
		}
		var v map[string]any
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &v)
		}
		values[entity] = v
		return v, nil
	}
	for _, m := range matches {
		if _, err := load(m.Entity); err != nil {
			return false, err
		}
	}
	return customfields.Matches(matches, func(e string) map[string]any { return values[e] }), nil
}
