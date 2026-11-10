package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	cfusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customfields/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
)

// FieldSource lists an organization's custom field definitions
// (customfields usecase).
type FieldSource interface {
	Definitions(ctx context.Context, orgID int64, entity string, objectTypeID *int64) ([]customfields.Definition, error)
}

// UnknownRecorder keeps keys no contact field defines (CF-196); the
// customfields usecase implements it next to FieldSource.
type UnknownRecorder interface {
	RecordUnknown(ctx context.Context, in cfusecase.UnknownInput) error
}

// WithFields enables attribute mapping to contact custom fields (CF-150).
func (s *Service) WithFields(f FieldSource) *Service {
	s.fields = f
	return s
}

// MaxAttributes bounds one setAttributes call.
const MaxAttributes = 50

// AttributesResult tells the SDK what was stored. Pending is true when the
// visitor has no contact yet: the SDK keeps the attributes and sends them
// with the first message. Unknown lists keys no field defines: they are
// also in Ignored (not stored as custom fields) but recorded as
// "tanımsız alan" warnings and kept on the contact until a field is defined.
type AttributesResult struct {
	Applied []string `json:"applied"`
	Ignored []string `json:"ignored"`
	Unknown []string `json:"unknown"`
	Pending bool     `json:"pending"`
}

// SetAttributes stores SDK attributes on the visitor's contact. A key is
// accepted when it is a contact field key (CF-196: defining the field is
// the only step) or an attribute mapping entry (an alias, or name/email/
// phone). Writes need a verified (identified) visitor unless the field's
// widget access, or the mapping, allows anonymous visitors, so nobody can
// write a customer's plan by typing in the console.
func (s *Service) SetAttributes(ctx context.Context, w Widget, claims widget.Claims, attrs map[string]any) (AttributesResult, error) {
	if len(attrs) > MaxAttributes {
		return AttributesResult{}, fmt.Errorf("%w: at most %d attributes", ErrInvalid, MaxAttributes)
	}
	return s.applyAttributes(ctx, w, claims, attrs, false)
}

func (s *Service) applyAttributes(ctx context.Context, w Widget, claims widget.Claims, attrs map[string]any, trusted bool) (AttributesResult, error) {
	out := AttributesResult{Applied: []string{}, Ignored: []string{}, Unknown: []string{}}
	contact, ok, err := s.contactOf(ctx, w, claims)
	if err != nil {
		return out, err
	}
	if !ok {
		out.Pending = true
		return out, nil
	}
	org := w.Instance.OrganizationID
	var defs map[string]customfields.Definition
	profile := ProfileInput{}
	patch := map[string]any{}
	loadDefs := func() error {
		if defs != nil {
			return nil
		}
		defs = map[string]customfields.Definition{}
		if s.fields == nil {
			return nil
		}
		list, err := s.fields.Definitions(ctx, org, customfields.EntityContact, nil)
		if err != nil {
			return err
		}
		for _, d := range list {
			defs[d.Key] = d
		}
		return nil
	}
	for key, value := range attrs {
		entry, found := mapping(w.Settings, key)
		if !found {
			if err := loadDefs(); err != nil {
				return out, err
			}
			def, defined := defs[key]
			if !defined {
				out.Ignored = append(out.Ignored, key)
				out.Unknown = append(out.Unknown, key)
				if rec, ok := s.fields.(UnknownRecorder); ok {
					if err := rec.RecordUnknown(ctx, cfusecase.UnknownInput{
						OrganizationID: org, ChannelInstanceID: w.Instance.ID, ContactID: contact.ID,
						Key: key, Value: value, Verified: trusted || claims.Verified(),
					}); err != nil {
						return out, err
					}
				}
				continue
			}
			if !widgetMayWrite(def, trusted || claims.Verified()) {
				out.Ignored = append(out.Ignored, key)
				continue
			}
			v, err := customfields.Normalize(def, value)
			if err != nil || v == nil {
				out.Ignored = append(out.Ignored, key)
				continue
			}
			patch[def.Key] = v
			out.Applied = append(out.Applied, key)
			continue
		}
		if !trusted && !entry.Anonymous && !claims.Verified() {
			out.Ignored = append(out.Ignored, key)
			continue
		}
		text := fmt.Sprint(value)
		switch entry.Target {
		case "name":
			profile.Name = clip(text, 255)
		case "email":
			profile.Email = text
		case "phone":
			profile.Phone = text
		default:
			if err := loadDefs(); err != nil {
				return out, err
			}
			def, ok := defs[strings.TrimPrefix(entry.Target, "custom:")]
			if !ok || def.Archived {
				out.Ignored = append(out.Ignored, key)
				continue
			}
			v, err := customfields.Normalize(def, value)
			if err != nil || v == nil {
				out.Ignored = append(out.Ignored, key)
				continue
			}
			patch[def.Key] = v
		}
		out.Applied = append(out.Applied, key)
	}
	if profile != (ProfileInput{}) {
		if err := s.applyProfile(ctx, org, contact, claims.Verified(), profile); err != nil {
			return out, err
		}
	}
	if len(patch) > 0 {
		raw, err := json.Marshal(patch)
		if err != nil {
			return out, err
		}
		if err := s.q.MergeWidgetContactCustomFields(ctx, db.MergeWidgetContactCustomFieldsParams{Patch: raw, OrganizationID: org, ID: contact.ID}); err != nil {
			return out, err
		}
	}
	return out, nil
}

// widgetMayWrite applies a field's widget access to a key sent without a
// mapping: anonymous fields take any visitor, verified ones (the default)
// an identified visitor or a trusted source, none never.
func widgetMayWrite(d customfields.Definition, verified bool) bool {
	if d.Archived {
		return false
	}
	switch d.WidgetAccess {
	case customfields.WidgetAccessAnonymous:
		return true
	case customfields.WidgetAccessNone:
		return false
	}
	return verified
}

func mapping(set widget.Settings, key string) (widget.AttributeMapEntry, bool) {
	for _, m := range set.AttributeMapping {
		if m.Key == key {
			return m, true
		}
	}
	return widget.AttributeMapEntry{}, false
}

// checkPreChat enforces the consent box of a new conversation (CF-198: the
// pre-chat questions themselves are the widget's request form, checked by
// the forms module).
func checkPreChat(set widget.Settings, in SendInput) error {
	pc := set.PreChat
	if !pc.Enabled {
		return nil
	}
	if pc.Consent.Enabled && pc.Consent.Required && (in.Consent == nil || !*in.Consent) {
		return ErrConsentRequired
	}
	return nil
}

func handoffKeyword(keywords []string, text string) bool {
	low := strings.ToLower(text)
	for _, k := range keywords {
		if k != "" && strings.Contains(low, k) {
			return true
		}
	}
	return false
}
