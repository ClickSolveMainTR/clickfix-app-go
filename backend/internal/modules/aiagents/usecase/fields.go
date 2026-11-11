package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
	"github.com/jackc/pgx/v5"
)

// FieldSource lists custom field definitions (customfields usecase).
type FieldSource interface {
	Definitions(ctx context.Context, orgID int64, entity string, objectTypeID *int64) ([]customfields.Definition, error)
}

// maxFieldsBlock bounds the customer-fields header of the AI context.
const maxFieldsBlock = 2000

// customerFields renders the contact's and company's custom fields that the
// AI may read (CF-196): "kim görsün: AI" on and not sensitive. Sensitive
// and hidden fields never reach the model.
func customerFields(contactDefs []customfields.Definition, contact map[string]any, companyDefs []customfields.Definition, company map[string]any) string {
	var b strings.Builder
	write := func(title string, fvs []customfields.FieldValue) {
		if len(fvs) == 0 {
			return
		}
		b.WriteString(title + ":\n")
		for _, fv := range fvs {
			fmt.Fprintf(&b, "- %s (%s): %s\n", fv.Label, fv.Key, fv.Display)
		}
	}
	write("Customer fields", customfields.Project(contactDefs, contact, customfields.AudienceAI))
	write("Company fields", customfields.Project(companyDefs, company, customfields.AudienceAI))
	out := b.String()
	if len(out) > maxFieldsBlock {
		out = string([]rune(out)[:maxFieldsBlock/2]) + "\n"
	}
	return out
}

// fieldsContext loads and renders customerFields for a conversation's
// contact; "" when there is no field source, contact or visible field.
func (s *Service) fieldsContext(ctx context.Context, org int64, conv db.Conversation) (string, error) {
	if s.Fields == nil || conv.ContactID <= 0 {
		return "", nil
	}
	row, err := s.Repo.Q.ContactAndCompanyCustomFields(ctx, db.ContactAndCompanyCustomFieldsParams{OrganizationID: org, ID: conv.ContactID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	contactDefs, err := s.Fields.Definitions(ctx, org, customfields.EntityContact, nil)
	if err != nil {
		return "", err
	}
	companyDefs, err := s.Fields.Definitions(ctx, org, customfields.EntityCompany, nil)
	if err != nil {
		return "", err
	}
	contact, company := map[string]any{}, map[string]any{}
	_ = json.Unmarshal(row.ContactFields, &contact)
	_ = json.Unmarshal(row.CompanyFields, &company)
	return customerFields(contactDefs, contact, companyDefs, company), nil
}
