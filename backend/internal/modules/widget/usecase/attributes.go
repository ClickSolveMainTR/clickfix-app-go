package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
)

// FieldSource lists an organization's custom field definitions
// (customfields usecase).
type FieldSource interface {
	Definitions(ctx context.Context, orgID int64, entity string, objectTypeID *int64) ([]customfields.Definition, error)
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
// with the first message.
type AttributesResult struct {
	Applied []string `json:"applied"`
	Ignored []string `json:"ignored"`
	Pending bool     `json:"pending"`
}

// SetAttributes stores mapped SDK attributes on the visitor's contact.
// Unmapped keys are ignored; keys not marked anonymous need a verified
// (identified) visitor, so nobody can write a customer's plan by typing in
// the console.
func (s *Service) SetAttributes(ctx context.Context, w Widget, claims widget.Claims, attrs map[string]any) (AttributesResult, error) {
	if len(attrs) > MaxAttributes {
		return AttributesResult{}, fmt.Errorf("%w: at most %d attributes", ErrInvalid, MaxAttributes)
	}
	return s.applyAttributes(ctx, w, claims, attrs, false)
}

func (s *Service) applyAttributes(ctx context.Context, w Widget, claims widget.Claims, attrs map[string]any, trusted bool) (AttributesResult, error) {
	out := AttributesResult{Applied: []string{}, Ignored: []string{}}
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
	for key, value := range attrs {
		entry, found := mapping(w.Settings, key)
		if !found || (!trusted && !entry.Anonymous && !claims.Verified()) {
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
			if defs == nil {
				defs = map[string]customfields.Definition{}
				if s.fields != nil {
					list, err := s.fields.Definitions(ctx, org, customfields.EntityContact, nil)
					if err != nil {
						return out, err
					}
					for _, d := range list {
						defs[d.Key] = d
					}
				}
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

func mapping(set widget.Settings, key string) (widget.AttributeMapEntry, bool) {
	for _, m := range set.AttributeMapping {
		if m.Key == key {
			return m, true
		}
	}
	return widget.AttributeMapEntry{}, false
}

// checkPreChat validates the custom pre-chat answers and consent of a new
// conversation and returns the answers to keep on the message.
func checkPreChat(set widget.Settings, in SendInput) (map[string]any, error) {
	pc := set.PreChat
	if !pc.Enabled {
		return nil, nil
	}
	if pc.Consent.Enabled && pc.Consent.Required && (in.Consent == nil || !*in.Consent) {
		return nil, ErrConsentRequired
	}
	if pc.Phone == widget.FieldRequired && strings.TrimSpace(in.Profile.Phone) == "" {
		return nil, fmt.Errorf("%w: phone is required", ErrInvalid)
	}
	out := map[string]any{}
	for _, f := range pc.Fields {
		raw := in.PreChat[f.Key]
		var v any
		switch x := raw.(type) {
		case bool:
			if x {
				v = true
			}
		case string:
			if t := clip(strings.TrimSpace(x), 500); t != "" {
				v = t
			}
		case float64:
			v = x
		}
		if f.Type == "select" && v != nil {
			if s, _ := v.(string); !slices.Contains(f.Options, s) {
				return nil, fmt.Errorf("%w: %s must be one of its options", ErrInvalid, f.Key)
			}
		}
		if v == nil {
			if f.Required {
				return nil, fmt.Errorf("%w: %s is required", ErrInvalid, f.Key)
			}
			continue
		}
		out[f.Key] = v
	}
	return out, nil
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
