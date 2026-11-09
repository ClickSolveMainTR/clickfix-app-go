package usecase

import (
	"context"
	"encoding/json"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
	"github.com/google/uuid"
)

// HiddenArticles returns the help articles among ids whose audience
// (CF-196 catalog conditions by option id) the visitor behind token does
// not match. An article without an audience is offered to everyone; one
// with an audience needs a contact whose list fields match, so a visitor
// without a session or contact does not see targeted articles.
func (s *Service) HiddenArticles(ctx context.Context, w Widget, token string, ids []uuid.UUID) (map[uuid.UUID]bool, error) {
	hidden := map[uuid.UUID]bool{}
	if len(ids) == 0 {
		return hidden, nil
	}
	org := w.Instance.OrganizationID
	rows, err := s.q.KnowledgeAudiencesByUUID(ctx, db.KnowledgeAudiencesByUUIDParams{OrganizationID: org, Uuids: ids})
	if err != nil || len(rows) == 0 {
		return hidden, err
	}
	values := map[string]map[string]any{}
	if token != "" {
		if claims, err := s.Authenticate(w, token); err == nil {
			contact, ok, err := s.contactOf(ctx, w, claims)
			if err != nil {
				return nil, err
			}
			if ok {
				values[customfields.EntityContact] = decodeFields(contact.CustomFields)
				if contact.CompanyID.Valid {
					raw, err := s.q.CompanyCustomFieldsByID(ctx, db.CompanyCustomFieldsByIDParams{OrganizationID: org, ID: contact.CompanyID.Int64})
					if err == nil {
						values[customfields.EntityCompany] = decodeFields(raw)
					}
				}
			}
		}
	}
	for _, r := range rows {
		var matches []customfields.FieldMatch
		if err := json.Unmarshal(r.Conditions, &matches); err != nil || len(matches) == 0 {
			continue
		}
		if !customfields.Matches(matches, func(e string) map[string]any { return values[e] }) {
			hidden[r.Uuid] = true
		}
	}
	return hidden, nil
}

func decodeFields(raw []byte) map[string]any {
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}
