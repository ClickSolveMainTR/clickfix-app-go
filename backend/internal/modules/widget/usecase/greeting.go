package usecase

import (
	"context"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
)

// PersonalizeGreeting renders {{contact.<key>}}, {{company.<key>}},
// {{contact.name}} and {{contact.first_name}} in the greeting title and
// text of a bootstrap (CF-196: the AI greeting picks the same catalog as
// SLA, workflows and articles). List fields render their option label;
// only fields the AI may read are filled, so sensitive or hidden fields
// stay empty. Without a contact every variable renders empty.
func (s *Service) PersonalizeGreeting(ctx context.Context, w Widget, b *Bootstrap) error {
	set := &b.Widget.Settings
	if !needsVars(set) {
		return nil
	}
	vars, err := s.greetingVars(ctx, w, b.Visitor.Token)
	if err != nil {
		return err
	}
	set.GreetingTitle = customfields.RenderTemplate(set.GreetingTitle, vars)
	set.GreetingText = customfields.RenderTemplate(set.GreetingText, vars)
	// Texts is shared with the widget's cached settings: render into a copy.
	texts := make(map[string]widget.Texts, len(set.Texts))
	for locale, t := range set.Texts {
		t.GreetingTitle = customfields.RenderTemplate(t.GreetingTitle, vars)
		t.GreetingText = customfields.RenderTemplate(t.GreetingText, vars)
		texts[locale] = t
	}
	set.Texts = texts
	return nil
}

func needsVars(set *PublicSettings) bool {
	if strings.Contains(set.GreetingTitle, "{{") || strings.Contains(set.GreetingText, "{{") {
		return true
	}
	for _, t := range set.Texts {
		if strings.Contains(t.GreetingTitle, "{{") || strings.Contains(t.GreetingText, "{{") {
			return true
		}
	}
	return false
}

func (s *Service) greetingVars(ctx context.Context, w Widget, token string) (map[string]string, error) {
	vars := map[string]string{}
	if token == "" {
		return vars, nil
	}
	claims, err := s.Authenticate(w, token)
	if err != nil {
		return vars, nil
	}
	contact, ok, err := s.contactOf(ctx, w, claims)
	if err != nil || !ok {
		return vars, err
	}
	vars["contact.name"] = contact.Name
	if f := strings.Fields(contact.Name); len(f) > 0 {
		vars["contact.first_name"] = f[0]
	}
	if s.fields == nil {
		return vars, nil
	}
	org := w.Instance.OrganizationID
	defs, err := s.fields.Definitions(ctx, org, customfields.EntityContact, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range customfields.TemplateVars("contact", defs, decodeFields(contact.CustomFields), customfields.AudienceAI) {
		if _, builtin := vars[k]; !builtin {
			vars[k] = v
		}
	}
	if contact.CompanyID.Valid {
		raw, err := s.q.CompanyCustomFieldsByID(ctx, db.CompanyCustomFieldsByIDParams{OrganizationID: org, ID: contact.CompanyID.Int64})
		if err == nil {
			companyDefs, err := s.fields.Definitions(ctx, org, customfields.EntityCompany, nil)
			if err != nil {
				return nil, err
			}
			for k, v := range customfields.TemplateVars("company", companyDefs, decodeFields(raw), customfields.AudienceAI) {
				vars[k] = v
			}
		}
	}
	return vars, nil
}
