// Package model holds the custom field and custom object API types (CF-7).
package model

import (
	"encoding/json"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
	"github.com/google/uuid"
)

// FieldDefinition is a custom field as the API returns it.
type FieldDefinition struct {
	UUID           uuid.UUID               `json:"uuid"`
	EntityType     string                  `json:"entity_type"`
	ObjectTypeUUID *uuid.UUID              `json:"object_type_uuid"`
	Key            string                  `json:"key"`
	Label          string                  `json:"label"`
	Description    string                  `json:"description"`
	Type           customfields.FieldType  `json:"type"`
	Required       bool                    `json:"required"`
	Options        []customfields.Option   `json:"options"`
	Config         customfields.Config     `json:"config"`
	Default        any                     `json:"default"`
	Condition      *customfields.Condition `json:"condition"`
	Position       int32                   `json:"position"`
	Archived       bool                    `json:"archived"`
	// Visibility, Sensitive and WidgetAccess are CF-196: who reads the
	// value, whether it is masked and kept out of AI/exports, and whether
	// the widget SDK writes it by key without a mapping.
	Visibility   customfields.Visibility `json:"visibility"`
	Sensitive    bool                    `json:"sensitive"`
	WidgetAccess string                  `json:"widget_access"`
	CreatedAt    time.Time               `json:"created_at"`
	UpdatedAt    time.Time               `json:"updated_at"`
}

// Engine converts the API shape to the engine definition.
func (f FieldDefinition) Engine() customfields.Definition {
	return customfields.Definition{
		Key: f.Key, Label: f.Label, Type: f.Type, Required: f.Required, Options: f.Options,
		Config: f.Config, Default: f.Default, Condition: f.Condition, Archived: f.Archived,
		Visibility: f.Visibility, Sensitive: f.Sensitive, WidgetAccess: f.WidgetAccess,
	}
}

// DefinitionScope selects the definitions of one entity (or object type).
type DefinitionScope struct {
	EntityType     string
	ObjectTypeUUID *uuid.UUID
}

// CreateDefinitionInput is the POST body.
type CreateDefinitionInput struct {
	EntityType     string                  `json:"entity_type"`
	ObjectTypeUUID *uuid.UUID              `json:"object_type_uuid"`
	Key            string                  `json:"key"`
	Label          string                  `json:"label"`
	Description    string                  `json:"description"`
	Type           customfields.FieldType  `json:"type"`
	Required       bool                    `json:"required"`
	Options        []customfields.Option   `json:"options"`
	Config         customfields.Config     `json:"config"`
	Default        any                     `json:"default"`
	Condition      *customfields.Condition `json:"condition"`
	// Visibility defaults to every surface; WidgetAccess to verified.
	Visibility   *customfields.Visibility `json:"visibility"`
	Sensitive    bool                     `json:"sensitive"`
	WidgetAccess string                   `json:"widget_access"`
	// FromUnknownKey is the received key a "tanımsız alan" warning was
	// created from (e.g. "Lisans Bitiş"); its warning closes and the values
	// contacts received under it are applied to the new field.
	FromUnknownKey string `json:"from_unknown_key"`
}

// UpdateDefinitionInput is the PATCH body. Key, type and entity never change;
// omitted fields stay as they are, an explicit null clears default/condition.
type UpdateDefinitionInput struct {
	Label       *string                  `json:"label"`
	Description *string                  `json:"description"`
	Required    *bool                    `json:"required"`
	Options     *[]customfields.Option   `json:"options"`
	Config      *customfields.Config     `json:"config"`
	Default     json.RawMessage          `json:"default"`
	Condition   json.RawMessage          `json:"condition"`
	Visibility  *customfields.Visibility `json:"visibility"`
	Sensitive   *bool                    `json:"sensitive"`
	// WidgetAccess: none, verified or anonymous.
	WidgetAccess *string `json:"widget_access"`
}

// UnknownKey is a key the widget sent that no field defines (CF-196).
type UnknownKey struct {
	UUID            uuid.UUID  `json:"uuid"`
	EntityType      string     `json:"entity_type"`
	Key             string     `json:"key"`
	SuggestedKey    string     `json:"suggested_key"`
	Source          string     `json:"source"`
	ValueKind       string     `json:"value_kind"`
	Hits            int64      `json:"hits"`
	ChannelUUID     *uuid.UUID `json:"channel_uuid"`
	ChannelName     *string    `json:"channel_name"`
	LastContactUUID *uuid.UUID `json:"last_contact_uuid"`
	FirstSeenAt     time.Time  `json:"first_seen_at"`
	LastSeenAt      time.Time  `json:"last_seen_at"`
}

// EntityFieldValue is one field of a contact or company as an agent screen
// shows it: the definition, its visibility switches and the stored value.
type EntityFieldValue struct {
	UUID       uuid.UUID               `json:"uuid"`
	Key        string                  `json:"key"`
	Label      string                  `json:"label"`
	Type       customfields.FieldType  `json:"type"`
	Options    []customfields.Option   `json:"options"`
	Visibility customfields.Visibility `json:"visibility"`
	Sensitive  bool                    `json:"sensitive"`
	// Value is the stored value (option id for list fields); null when unset
	// or masked. Display is the text to show (option label, "Evet", ...).
	Value   any    `json:"value"`
	Display string `json:"display"`
	Masked  bool   `json:"masked"`
}

// UnknownValue is a value the widget sent for a key no field defines yet.
type UnknownValue struct {
	Key          string    `json:"key"`
	SuggestedKey string    `json:"suggested_key"`
	Value        any       `json:"value"`
	Verified     bool      `json:"verified"`
	ReceivedAt   time.Time `json:"received_at"`
}

// EntityValues is GET /v1/tenant/custom-fields/values: every agent-visible
// field of one contact or company plus the undefined keys it received.
type EntityValues struct {
	EntityType string             `json:"entity_type"`
	EntityUUID uuid.UUID          `json:"entity_uuid"`
	Fields     []EntityFieldValue `json:"fields"`
	Unknown    []UnknownValue     `json:"unknown"`
}

// ReorderInput sets the display order of one entity's fields.
type ReorderInput struct {
	EntityType     string      `json:"entity_type"`
	ObjectTypeUUID *uuid.UUID  `json:"object_type_uuid"`
	UUIDs          []uuid.UUID `json:"uuids"`
}

// TypeMeta describes one field type for the field editor.
type TypeMeta struct {
	Type      customfields.FieldType  `json:"type"`
	Operators []customfields.Operator `json:"operators"`
	Sortable  bool                    `json:"sortable"`
}

// Meta lists supported entities and types.
type Meta struct {
	EntityTypes []string   `json:"entity_types"`
	FieldTypes  []TypeMeta `json:"field_types"`
	Limits      MetaLimits `json:"limits"`
}

// MetaLimits are the engine limits the UI enforces up front.
type MetaLimits struct {
	FieldsPerEntity int `json:"fields_per_entity"`
	Options         int `json:"options"`
	ObjectTypes     int `json:"object_types"`
}

// ObjectType is an organization-defined record type (e.g. "Lisans").
type ObjectType struct {
	UUID          uuid.UUID `json:"uuid"`
	Key           string    `json:"key"`
	SingularLabel string    `json:"singular_label"`
	PluralLabel   string    `json:"plural_label"`
	Description   string    `json:"description"`
	Icon          string    `json:"icon"`
	Archived      bool      `json:"archived"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ObjectTypeInput creates (all fields) or updates (key ignored) a type.
type ObjectTypeInput struct {
	Key           string `json:"key"`
	SingularLabel string `json:"singular_label"`
	PluralLabel   string `json:"plural_label"`
	Description   string `json:"description"`
	Icon          string `json:"icon"`
	Archived      bool   `json:"archived"`
}

// ObjectRecord is one record of a custom object type.
type ObjectRecord struct {
	UUID           uuid.UUID      `json:"uuid"`
	ObjectTypeUUID uuid.UUID      `json:"object_type_uuid"`
	DisplayName    string         `json:"display_name"`
	CustomFields   map[string]any `json:"custom_fields"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// RecordInput creates or updates a record. On update CustomFields is merged
// into the stored values (null clears a key).
type RecordInput struct {
	DisplayName  *string        `json:"display_name"`
	CustomFields map[string]any `json:"custom_fields"`
}

// RecordList selects records.
type RecordList struct {
	OrganizationID int64
	ObjectTypeID   int64
	Q              string
	Filters        []customfields.Filter
	Sort           string // display_name, created_at, updated_at or cf.<key>
	SortDesc       bool
	Defs           []customfields.Definition
	Limit          int32
	Offset         int32
}
