package usecase

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customfields/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// definitionChanges is the audit payload of an update: every changed part
// as {"from": …, "to": …}, renamed options as {"<id>": {"from","to"}}.
func definitionChanges(before, after model.FieldDefinition) map[string]any {
	out := map[string]any{"key": after.Key, "entity_type": after.EntityType}
	diff := map[string]any{}
	put := func(name string, a, b any) {
		if !reflect.DeepEqual(a, b) {
			diff[name] = map[string]any{"from": a, "to": b}
		}
	}
	put("label", before.Label, after.Label)
	put("description", before.Description, after.Description)
	put("required", before.Required, after.Required)
	put("visibility", before.Visibility, after.Visibility)
	put("sensitive", before.Sensitive, after.Sensitive)
	put("widget_access", before.WidgetAccess, after.WidgetAccess)
	put("default", before.Default, after.Default)
	put("condition", before.Condition, after.Condition)
	put("config", before.Config, after.Config)
	renamed := map[string]any{}
	var added, archived []string
	for _, o := range after.Options {
		i := slices.IndexFunc(before.Options, func(b customfields.Option) bool { return b.Value == o.Value })
		if i < 0 {
			added = append(added, o.Value)
			continue
		}
		if before.Options[i].Label != o.Label {
			renamed[o.Value] = map[string]any{"from": before.Options[i].Label, "to": o.Label}
		}
		if o.Archived && !before.Options[i].Archived {
			archived = append(archived, o.Value)
		}
	}
	if len(renamed) > 0 {
		diff["options_renamed"] = renamed
	}
	if len(added) > 0 {
		diff["options_added"] = added
	}
	if len(archived) > 0 {
		diff["options_archived"] = archived
	}
	out["changes"] = diff
	return out
}

// UnknownInput is one received key that no contact field defines.
type UnknownInput struct {
	OrganizationID    int64
	ChannelInstanceID int64
	ContactID         int64
	Key               string
	Value             any
	// Verified: an identified visitor (or a trusted source) sent it. Only
	// verified values are applied when the field is defined later, unless
	// the new field allows anonymous widget writes.
	Verified bool
}

// MaxUnknownKeyLength bounds a recorded key.
const MaxUnknownKeyLength = 100

// RecordUnknown keeps a key the widget sent for a contact although no field
// defines it: an organization-wide warning row (custom_field_unknown_keys)
// plus the value on the contact (contacts.unknown_fields, at most 50 keys)
// so defining the field later applies it. Nothing is dropped silently.
func (s *Service) RecordUnknown(ctx context.Context, in UnknownInput) error {
	key := strings.TrimSpace(in.Key)
	if key == "" || len(key) > MaxUnknownKeyLength {
		return nil
	}
	if err := s.q.RecordUnknownCustomFieldKey(ctx, db.RecordUnknownCustomFieldKeyParams{
		OrganizationID:    in.OrganizationID,
		EntityType:        customfields.EntityContact,
		Key:               key,
		Source:            "widget",
		ChannelInstanceID: pgtype.Int8{Int64: in.ChannelInstanceID, Valid: in.ChannelInstanceID > 0},
		ValueKind:         customfields.ValueKind(in.Value),
		LastContactID:     pgtype.Int8{Int64: in.ContactID, Valid: in.ContactID > 0},
	}); err != nil {
		return err
	}
	if in.ContactID <= 0 {
		return nil
	}
	v := in.Value
	if str, ok := v.(string); ok && len([]rune(str)) > customfields.MaxTextLength {
		v = string([]rune(str)[:customfields.MaxTextLength])
	}
	value, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.q.StoreContactUnknownField(ctx, db.StoreContactUnknownFieldParams{
		OrganizationID: in.OrganizationID, ContactID: in.ContactID, Key: key, Value: value, Verified: in.Verified,
	})
}

// ListUnknownKeys returns the open warnings, newest first; channelUUID
// narrows them to one widget (the channel setup check).
func (s *Service) ListUnknownKeys(ctx context.Context, orgID int64, channelUUID *uuid.UUID) ([]model.UnknownKey, error) {
	params := db.ListUnknownCustomFieldKeysParams{OrganizationID: orgID}
	if channelUUID != nil {
		params.ChannelUuid = pgtype.UUID{Bytes: *channelUUID, Valid: true}
	}
	rows, err := s.q.ListUnknownCustomFieldKeys(ctx, params)
	if err != nil {
		return nil, err
	}
	out := make([]model.UnknownKey, 0, len(rows))
	for _, r := range rows {
		k := model.UnknownKey{
			UUID: r.Uuid, EntityType: r.EntityType, Key: r.Key, SuggestedKey: customfields.SuggestKey(r.Key),
			Source: r.Source, ValueKind: r.ValueKind, Hits: r.Hits,
			FirstSeenAt: r.FirstSeenAt.Time.UTC(), LastSeenAt: r.LastSeenAt.Time.UTC(),
		}
		if r.ChannelUuid.Valid {
			id := uuid.UUID(r.ChannelUuid.Bytes)
			k.ChannelUUID = &id
		}
		if r.ChannelName.Valid {
			name := r.ChannelName.String
			k.ChannelName = &name
		}
		if r.LastContactUuid.Valid {
			id := uuid.UUID(r.LastContactUuid.Bytes)
			k.LastContactUUID = &id
		}
		out = append(out, k)
	}
	return out, nil
}

// DismissUnknownKey hides a warning until the key is received again.
func (s *Service) DismissUnknownKey(ctx context.Context, orgID int64, id uuid.UUID) error {
	n, err := s.q.DismissUnknownCustomFieldKey(ctx, db.DismissUnknownCustomFieldKeyParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Adopt runs after a contact field is created: the warnings for its key
// (and for the raw name it was suggested from) close, and the values
// contacts already received under that name move into custom_fields when
// they are valid for the new field. Returns how many contacts got a value.
// rawKeys are the received names (e.g. "Lisans Bitiş"); the field key is
// always included.
func (s *Service) Adopt(ctx context.Context, orgID int64, def model.FieldDefinition, rawKeys ...string) (int, error) {
	if def.EntityType != customfields.EntityContact {
		return 0, nil
	}
	keys := []string{def.Key}
	for _, k := range rawKeys {
		if k = strings.TrimSpace(k); k != "" && !slices.Contains(keys, k) && customfields.SuggestKey(k) == def.Key {
			keys = append(keys, k)
		}
	}
	engine := def.Engine()
	adopted := 0
	for _, key := range keys {
		if err := s.q.ResolveUnknownCustomFieldKey(ctx, db.ResolveUnknownCustomFieldKeyParams{
			OrganizationID: orgID, EntityType: customfields.EntityContact, Key: key,
		}); err != nil {
			return adopted, err
		}
		rows, err := s.q.ContactsWithUnknownField(ctx, db.ContactsWithUnknownFieldParams{OrganizationID: orgID, Key: key})
		if err != nil {
			return adopted, err
		}
		for _, r := range rows {
			var received any
			_ = json.Unmarshal(r.Value, &received)
			current := map[string]any{}
			_ = json.Unmarshal(r.CustomFields, &current)
			_, already := current[def.Key]
			allowed := r.Verified || def.WidgetAccess == customfields.WidgetAccessAnonymous
			if !already && allowed && !engine.Archived {
				if norm, err := customfields.Normalize(engine, received); err == nil && norm != nil {
					value, err := json.Marshal(norm)
					if err != nil {
						return adopted, err
					}
					if err := s.q.SetContactCustomField(ctx, db.SetContactCustomFieldParams{
						Key: def.Key, Value: value, OrganizationID: orgID, ID: r.ContactID,
					}); err != nil {
						return adopted, err
					}
					adopted++
				}
			}
			if err := s.q.DeleteContactUnknownField(ctx, db.DeleteContactUnknownFieldParams{
				OrganizationID: orgID, ContactID: r.ContactID, Key: key,
			}); err != nil {
				return adopted, err
			}
		}
	}
	return adopted, nil
}

// EntityValues returns the agent view of one contact or company: every
// live field the agent screens may show (sensitive ones masked) with its
// value, plus the undefined keys the contact received. It is the read model
// the contact/company card and the conversation panel render.
func (s *Service) EntityValues(ctx context.Context, orgID int64, entity string, id uuid.UUID) (model.EntityValues, error) {
	out := model.EntityValues{EntityType: entity, EntityUUID: id, Fields: []model.EntityFieldValue{}, Unknown: []model.UnknownValue{}}
	var raw []byte
	var contactID int64
	switch entity {
	case customfields.EntityContact:
		row, err := s.q.CustomFieldsOfContact(ctx, db.CustomFieldsOfContactParams{OrganizationID: orgID, Uuid: id})
		if err != nil {
			return out, notFound(err)
		}
		raw, contactID = row.CustomFields, row.ID
	case customfields.EntityCompany:
		row, err := s.q.CustomFieldsOfCompany(ctx, db.CustomFieldsOfCompanyParams{OrganizationID: orgID, Uuid: id})
		if err != nil {
			return out, notFound(err)
		}
		raw = row.CustomFields
	default:
		return out, invalid("entity_type must be contact or company")
	}
	values := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &values); err != nil {
			return out, err
		}
	}
	rows, err := s.listRows(ctx, orgID, entity, pgtype.Int8{}, false)
	if err != nil {
		return out, err
	}
	for _, r := range rows {
		f, err := toDefinition(r, nil)
		if err != nil {
			return out, err
		}
		if !f.Visibility.Agent {
			continue
		}
		fv := model.EntityFieldValue{
			UUID: f.UUID, Key: f.Key, Label: f.Label, Type: f.Type, Options: f.Options,
			Visibility: f.Visibility, Sensitive: f.Sensitive,
		}
		if v, ok := values[f.Key]; ok && v != nil {
			if p := customfields.Project([]customfields.Definition{f.Engine()}, map[string]any{f.Key: v}, customfields.AudienceAgent); len(p) == 1 {
				fv.Value, fv.Display, fv.Masked = p[0].Value, p[0].Display, p[0].Masked
			}
		}
		out.Fields = append(out.Fields, fv)
	}
	if contactID > 0 {
		unknown, err := s.q.ContactUnknownValues(ctx, db.ContactUnknownValuesParams{OrganizationID: orgID, ContactID: contactID})
		if err != nil {
			return out, err
		}
		for _, u := range unknown {
			var v any
			_ = json.Unmarshal(u.Value, &v)
			out.Unknown = append(out.Unknown, model.UnknownValue{
				Key: u.Key, SuggestedKey: customfields.SuggestKey(u.Key), Value: v, Verified: u.Verified, ReceivedAt: u.ReceivedAt.Time.UTC(),
			})
		}
	}
	return out, nil
}

// EntityDefinitions loads the live definitions of several entities at once
// for catalog consumers (SLA, workflows, article audiences, templates).
func (s *Service) EntityDefinitions(ctx context.Context, orgID int64, entities ...string) (map[string][]customfields.Definition, error) {
	out := map[string][]customfields.Definition{}
	for _, e := range entities {
		defs, err := s.Definitions(ctx, orgID, e, nil)
		if err != nil {
			return nil, err
		}
		out[e] = defs
	}
	return out, nil
}
