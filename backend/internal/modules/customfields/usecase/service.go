// Package usecase implements custom field definitions, custom object types
// and their records (CF-7). Other modules (contacts, companies,
// conversations, tickets) call Definitions, ValidateValues and ParseFilters
// to store and filter their own custom_fields column.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customfields/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customfields/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Errors the handler maps to HTTP statuses. Engine validation failures are
// *customfields.ValidationError.
var (
	ErrNotFound = errors.New("customfields: not found")
	ErrInvalid  = errors.New("customfields: invalid request")
	ErrConflict = errors.New("customfields: already exists")
	ErrLimit    = errors.New("customfields: limit reached")
)

// MaxObjectTypes caps custom object types per organization.
const MaxObjectTypes = 20

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Service is the custom field use case.
type Service struct {
	repo *repository.Repository
	q    *db.Queries
}

// New builds the service.
func New(repo *repository.Repository) *Service {
	return &Service{repo: repo, q: repo.Q}
}

// Meta returns the supported entities and field types.
func (s *Service) Meta() model.Meta {
	types := make([]model.TypeMeta, 0, len(customfields.Types))
	for _, t := range customfields.Types {
		types = append(types, model.TypeMeta{Type: t, Operators: customfields.Operators(t), Sortable: t != customfields.TypeMultiselect})
	}
	return model.Meta{
		EntityTypes: customfields.EntityTypes,
		FieldTypes:  types,
		Limits: model.MetaLimits{
			FieldsPerEntity: customfields.MaxDefinitionsPerEntity,
			Options:         customfields.MaxOptions,
			ObjectTypes:     MaxObjectTypes,
		},
	}
}

// resolveScope checks the entity type and resolves the object type id.
func (s *Service) resolveScope(ctx context.Context, orgID int64, scope model.DefinitionScope) (pgtype.Int8, error) {
	if !customfields.IsValidEntity(scope.EntityType) {
		return pgtype.Int8{}, invalid("entity_type must be one of %s", strings.Join(customfields.EntityTypes, ", "))
	}
	if scope.EntityType != customfields.EntityObject {
		if scope.ObjectTypeUUID != nil {
			return pgtype.Int8{}, invalid("object_type_uuid is only allowed with entity_type=object")
		}
		return pgtype.Int8{}, nil
	}
	if scope.ObjectTypeUUID == nil {
		return pgtype.Int8{}, invalid("object_type_uuid is required with entity_type=object")
	}
	ot, err := s.q.GetCustomObjectType(ctx, db.GetCustomObjectTypeParams{OrganizationID: orgID, Uuid: *scope.ObjectTypeUUID})
	if err != nil {
		return pgtype.Int8{}, notFound(err)
	}
	return pgtype.Int8{Int64: ot.ID, Valid: true}, nil
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (s *Service) listRows(ctx context.Context, orgID int64, entity string, objectTypeID pgtype.Int8, includeArchived bool) ([]db.CustomFieldDefinition, error) {
	return s.q.ListCustomFieldDefinitions(ctx, db.ListCustomFieldDefinitionsParams{
		OrganizationID:  orgID,
		EntityType:      entity,
		ObjectTypeID:    objectTypeID,
		IncludeArchived: includeArchived,
	})
}

// ListDefinitions returns the fields of one entity in display order.
func (s *Service) ListDefinitions(ctx context.Context, orgID int64, scope model.DefinitionScope, includeArchived bool) ([]model.FieldDefinition, error) {
	objectTypeID, err := s.resolveScope(ctx, orgID, scope)
	if err != nil {
		return nil, err
	}
	rows, err := s.listRows(ctx, orgID, scope.EntityType, objectTypeID, includeArchived)
	if err != nil {
		return nil, err
	}
	out := make([]model.FieldDefinition, 0, len(rows))
	for _, r := range rows {
		f, err := toDefinition(r, scope.ObjectTypeUUID)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// Definitions returns the engine definitions (archived included, so stored
// values of archived fields stay readable) for modules that own a
// custom_fields column. objectTypeID is nil except for custom objects.
func (s *Service) Definitions(ctx context.Context, orgID int64, entity string, objectTypeID *int64) ([]customfields.Definition, error) {
	ot := pgtype.Int8{}
	if objectTypeID != nil {
		ot = pgtype.Int8{Int64: *objectTypeID, Valid: true}
	}
	rows, err := s.listRows(ctx, orgID, entity, ot, true)
	if err != nil {
		return nil, err
	}
	out := make([]customfields.Definition, 0, len(rows))
	for _, r := range rows {
		f, err := toDefinition(r, nil)
		if err != nil {
			return nil, err
		}
		out = append(out, f.Engine())
	}
	return out, nil
}

// ValidateValues validates input against the entity's fields, merging into
// current (the stored map, nil on create), and checks that user fields point
// at members of the organization.
func (s *Service) ValidateValues(ctx context.Context, orgID int64, defs []customfields.Definition, current, input map[string]any, mode customfields.Mode) (map[string]any, error) {
	out, err := customfields.Validate(defs, current, input, mode)
	if err != nil {
		return nil, prefixed(err)
	}
	written := map[string]any{}
	for k := range input {
		if v, ok := out[k]; ok {
			written[k] = v
		}
	}
	ids := customfields.UserIDs(defs, written)
	if len(ids) == 0 {
		return out, nil
	}
	members, err := s.q.OrganizationMemberUserUUIDs(ctx, db.OrganizationMemberUserUUIDsParams{OrganizationID: orgID, UserUuids: ids})
	if err != nil {
		return nil, err
	}
	var errs []customfields.FieldError
	for _, d := range defs {
		if d.Type != customfields.TypeUser {
			continue
		}
		v, ok := written[d.Key].(string)
		if !ok {
			continue
		}
		id, _ := uuid.Parse(v)
		if !slices.Contains(members, id) {
			errs = append(errs, customfields.FieldError{Field: d.Key, Code: customfields.CodeInvalid, Message: "user is not a member of this organization"})
		}
	}
	if len(errs) > 0 {
		return nil, prefixed(&customfields.ValidationError{Errors: errs})
	}
	return out, nil
}

// prefixed names value errors "custom_fields.<key>" so API clients can map
// them onto form fields.
func prefixed(err error) error {
	var ve *customfields.ValidationError
	if !errors.As(err, &ve) {
		return err
	}
	out := &customfields.ValidationError{Errors: make([]customfields.FieldError, len(ve.Errors))}
	for i, e := range ve.Errors {
		e.Field = "custom_fields." + e.Field
		out.Errors[i] = e
	}
	return out
}

// ParseFilters reads `cf.*` list parameters against defs.
func (s *Service) ParseFilters(q url.Values, defs []customfields.Definition) ([]customfields.Filter, error) {
	return customfields.ParseFilters(q, defs)
}

func (s *Service) engineDefs(ctx context.Context, orgID int64, entity string, objectTypeID pgtype.Int8, skip uuid.UUID) ([]customfields.Definition, error) {
	rows, err := s.listRows(ctx, orgID, entity, objectTypeID, true)
	if err != nil {
		return nil, err
	}
	out := make([]customfields.Definition, 0, len(rows))
	for _, r := range rows {
		if r.Uuid == skip {
			continue
		}
		f, err := toDefinition(r, nil)
		if err != nil {
			return nil, err
		}
		out = append(out, f.Engine())
	}
	return out, nil
}

// CreateDefinition adds a field.
func (s *Service) CreateDefinition(ctx context.Context, orgID int64, in model.CreateDefinitionInput) (model.FieldDefinition, error) {
	scope := model.DefinitionScope{EntityType: in.EntityType, ObjectTypeUUID: in.ObjectTypeUUID}
	objectTypeID, err := s.resolveScope(ctx, orgID, scope)
	if err != nil {
		return model.FieldDefinition{}, err
	}
	in.Key = strings.TrimSpace(in.Key)
	in.Label = strings.TrimSpace(in.Label)
	others, err := s.engineDefs(ctx, orgID, in.EntityType, objectTypeID, uuid.Nil)
	if err != nil {
		return model.FieldDefinition{}, err
	}
	if len(others) >= customfields.MaxDefinitionsPerEntity {
		return model.FieldDefinition{}, fmt.Errorf("%w: at most %d fields per entity", ErrLimit, customfields.MaxDefinitionsPerEntity)
	}
	def := customfields.Definition{
		Key: in.Key, Label: in.Label, Type: in.Type, Required: in.Required, Options: in.Options,
		Config: in.Config, Default: in.Default, Condition: in.Condition,
	}
	if errs := customfields.ValidateDefinition(def, others); len(errs) > 0 {
		return model.FieldDefinition{}, &customfields.ValidationError{Errors: errs}
	}
	params := db.CreateCustomFieldDefinitionParams{
		OrganizationID: orgID,
		EntityType:     in.EntityType,
		ObjectTypeID:   objectTypeID,
		Key:            in.Key,
		Label:          in.Label,
		Description:    strings.TrimSpace(in.Description),
		FieldType:      string(in.Type),
		Required:       in.Required,
	}
	if params.Options, params.Config, params.DefaultValue, params.Condition, err = encodeDef(def); err != nil {
		return model.FieldDefinition{}, err
	}
	row, err := s.q.CreateCustomFieldDefinition(ctx, params)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return model.FieldDefinition{}, fmt.Errorf("%w: key %q is already used", ErrConflict, in.Key)
		}
		return model.FieldDefinition{}, err
	}
	return toDefinition(row, in.ObjectTypeUUID)
}

func (s *Service) getRow(ctx context.Context, orgID int64, id uuid.UUID) (db.CustomFieldDefinition, error) {
	row, err := s.q.GetCustomFieldDefinition(ctx, db.GetCustomFieldDefinitionParams{OrganizationID: orgID, Uuid: id})
	return row, notFound(err)
}

// GetDefinition returns one field.
func (s *Service) GetDefinition(ctx context.Context, orgID int64, id uuid.UUID) (model.FieldDefinition, error) {
	row, err := s.getRow(ctx, orgID, id)
	if err != nil {
		return model.FieldDefinition{}, err
	}
	return s.withObjectType(ctx, orgID, row)
}

func (s *Service) withObjectType(ctx context.Context, orgID int64, row db.CustomFieldDefinition) (model.FieldDefinition, error) {
	var otUUID *uuid.UUID
	if row.ObjectTypeID.Valid {
		var u uuid.UUID
		if err := s.repo.Pool.QueryRow(ctx, "SELECT uuid FROM custom_object_types WHERE organization_id = $1 AND id = $2", orgID, row.ObjectTypeID.Int64).Scan(&u); err != nil {
			return model.FieldDefinition{}, err
		}
		otUUID = &u
	}
	return toDefinition(row, otUUID)
}

// UpdateDefinition changes the mutable parts of a field.
func (s *Service) UpdateDefinition(ctx context.Context, orgID int64, id uuid.UUID, in model.UpdateDefinitionInput) (model.FieldDefinition, error) {
	row, err := s.getRow(ctx, orgID, id)
	if err != nil {
		return model.FieldDefinition{}, err
	}
	current, err := toDefinition(row, nil)
	if err != nil {
		return model.FieldDefinition{}, err
	}
	def := current.Engine()
	description := current.Description
	if in.Label != nil {
		def.Label = strings.TrimSpace(*in.Label)
	}
	if in.Description != nil {
		description = strings.TrimSpace(*in.Description)
	}
	if in.Required != nil {
		def.Required = *in.Required
	}
	if in.Options != nil {
		def.Options = *in.Options
	}
	if in.Config != nil {
		def.Config = *in.Config
	}
	if in.Default != nil {
		def.Default = nil
		if string(in.Default) != "null" {
			if err := json.Unmarshal(in.Default, &def.Default); err != nil {
				return model.FieldDefinition{}, invalid("default is not valid JSON")
			}
		}
	}
	if in.Condition != nil {
		def.Condition = nil
		if string(in.Condition) != "null" {
			if err := json.Unmarshal(in.Condition, &def.Condition); err != nil {
				return model.FieldDefinition{}, invalid("condition is not valid JSON")
			}
		}
	}
	others, err := s.engineDefs(ctx, orgID, row.EntityType, row.ObjectTypeID, row.Uuid)
	if err != nil {
		return model.FieldDefinition{}, err
	}
	if errs := customfields.ValidateDefinition(def, others); len(errs) > 0 {
		return model.FieldDefinition{}, &customfields.ValidationError{Errors: errs}
	}
	// Removing an option would orphan stored values: archive it instead.
	if in.Options != nil {
		for _, o := range current.Options {
			if !slices.ContainsFunc(def.Options, func(n customfields.Option) bool { return n.Value == o.Value }) {
				return model.FieldDefinition{}, &customfields.ValidationError{Errors: []customfields.FieldError{{
					Field: "options", Code: customfields.CodeInvalid, Message: fmt.Sprintf("option %q cannot be removed; archive it", o.Value),
				}}}
			}
		}
	}
	params := db.UpdateCustomFieldDefinitionParams{
		OrganizationID: orgID,
		ID:             row.ID,
		Label:          def.Label,
		Description:    description,
		Required:       def.Required,
	}
	if params.Options, params.Config, params.DefaultValue, params.Condition, err = encodeDef(def); err != nil {
		return model.FieldDefinition{}, err
	}
	updated, err := s.q.UpdateCustomFieldDefinition(ctx, params)
	if err != nil {
		return model.FieldDefinition{}, err
	}
	return s.withObjectType(ctx, orgID, updated)
}

// SetArchived archives (hides, keeps stored values) or restores a field.
// A field other fields depend on cannot be archived.
func (s *Service) SetArchived(ctx context.Context, orgID int64, id uuid.UUID, archived bool) (model.FieldDefinition, error) {
	row, err := s.getRow(ctx, orgID, id)
	if err != nil {
		return model.FieldDefinition{}, err
	}
	if archived {
		others, err := s.engineDefs(ctx, orgID, row.EntityType, row.ObjectTypeID, row.Uuid)
		if err != nil {
			return model.FieldDefinition{}, err
		}
		for _, o := range others {
			if !o.Archived && o.Condition != nil && o.Condition.Field == row.Key {
				return model.FieldDefinition{}, invalid("field %q depends on this field", o.Key)
			}
		}
	}
	updated, err := s.q.SetCustomFieldDefinitionArchived(ctx, db.SetCustomFieldDefinitionArchivedParams{
		OrganizationID: orgID, ID: row.ID, Archived: archived,
	})
	if err != nil {
		return model.FieldDefinition{}, err
	}
	return s.withObjectType(ctx, orgID, updated)
}

// Reorder sets positions; uuids must be exactly the entity's fields.
func (s *Service) Reorder(ctx context.Context, orgID int64, in model.ReorderInput) error {
	scope := model.DefinitionScope{EntityType: in.EntityType, ObjectTypeUUID: in.ObjectTypeUUID}
	objectTypeID, err := s.resolveScope(ctx, orgID, scope)
	if err != nil {
		return err
	}
	rows, err := s.listRows(ctx, orgID, in.EntityType, objectTypeID, true)
	if err != nil {
		return err
	}
	if len(rows) != len(in.UUIDs) {
		return invalid("uuids must list every field of the entity once")
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range in.UUIDs {
		if seen[id] || !slices.ContainsFunc(rows, func(r db.CustomFieldDefinition) bool { return r.Uuid == id }) {
			return invalid("uuids must list every field of the entity once")
		}
		seen[id] = true
	}
	tx, err := s.repo.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)
	for i, id := range in.UUIDs {
		if err := q.SetCustomFieldDefinitionPosition(ctx, db.SetCustomFieldDefinitionPositionParams{
			OrganizationID: orgID, Uuid: id, Position: safeconv.ToInt32(i),
		}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func encodeDef(d customfields.Definition) (options, config, def, cond []byte, err error) {
	if d.Options == nil {
		d.Options = []customfields.Option{}
	}
	if options, err = json.Marshal(d.Options); err != nil {
		return
	}
	if config, err = json.Marshal(d.Config); err != nil {
		return
	}
	if d.Default != nil {
		norm, nerr := customfields.Normalize(d, d.Default)
		if nerr != nil {
			err = invalid("default: %v", nerr)
			return
		}
		if def, err = json.Marshal(norm); err != nil {
			return
		}
	}
	if d.Condition != nil {
		cond, err = json.Marshal(d.Condition)
	}
	return
}

func toDefinition(r db.CustomFieldDefinition, objectTypeUUID *uuid.UUID) (model.FieldDefinition, error) {
	f := model.FieldDefinition{
		UUID:           r.Uuid,
		EntityType:     r.EntityType,
		ObjectTypeUUID: objectTypeUUID,
		Key:            r.Key,
		Label:          r.Label,
		Description:    r.Description,
		Type:           customfields.FieldType(r.FieldType),
		Required:       r.Required,
		Options:        []customfields.Option{},
		Position:       r.Position,
		Archived:       r.ArchivedAt.Valid,
		CreatedAt:      r.CreatedAt.Time,
		UpdatedAt:      r.UpdatedAt.Time,
	}
	if err := json.Unmarshal(r.Options, &f.Options); err != nil {
		return f, fmt.Errorf("customfields: options of %s: %w", r.Key, err)
	}
	if err := json.Unmarshal(r.Config, &f.Config); err != nil {
		return f, fmt.Errorf("customfields: config of %s: %w", r.Key, err)
	}
	if len(r.DefaultValue) > 0 {
		if err := json.Unmarshal(r.DefaultValue, &f.Default); err != nil {
			return f, fmt.Errorf("customfields: default of %s: %w", r.Key, err)
		}
	}
	if len(r.Condition) > 0 {
		if err := json.Unmarshal(r.Condition, &f.Condition); err != nil {
			return f, fmt.Errorf("customfields: condition of %s: %w", r.Key, err)
		}
	}
	return f, nil
}

// --- Custom object types ---

// ListObjectTypes returns the organization's object types.
func (s *Service) ListObjectTypes(ctx context.Context, orgID int64, includeArchived bool) ([]model.ObjectType, error) {
	rows, err := s.q.ListCustomObjectTypes(ctx, db.ListCustomObjectTypesParams{OrganizationID: orgID, IncludeArchived: includeArchived})
	if err != nil {
		return nil, err
	}
	out := make([]model.ObjectType, 0, len(rows))
	for _, r := range rows {
		out = append(out, toObjectType(r))
	}
	return out, nil
}

// GetObjectType returns one object type.
func (s *Service) GetObjectType(ctx context.Context, orgID int64, id uuid.UUID) (model.ObjectType, error) {
	row, err := s.q.GetCustomObjectType(ctx, db.GetCustomObjectTypeParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return model.ObjectType{}, notFound(err)
	}
	return toObjectType(row), nil
}

func validateObjectType(in *model.ObjectTypeInput, create bool) error {
	in.Key = strings.TrimSpace(in.Key)
	in.SingularLabel = strings.TrimSpace(in.SingularLabel)
	in.PluralLabel = strings.TrimSpace(in.PluralLabel)
	in.Description = strings.TrimSpace(in.Description)
	in.Icon = strings.TrimSpace(in.Icon)
	var errs []customfields.FieldError
	if create && !customfields.IsValidKey(in.Key) {
		errs = append(errs, customfields.FieldError{Field: "key", Code: customfields.CodeInvalid, Message: "key must start with a letter and use a-z, 0-9 or _"})
	}
	if in.SingularLabel == "" || len([]rune(in.SingularLabel)) > customfields.MaxLabelLength {
		errs = append(errs, customfields.FieldError{Field: "singular_label", Code: customfields.CodeInvalid, Message: "singular_label is required (max 100)"})
	}
	if in.PluralLabel == "" || len([]rune(in.PluralLabel)) > customfields.MaxLabelLength {
		errs = append(errs, customfields.FieldError{Field: "plural_label", Code: customfields.CodeInvalid, Message: "plural_label is required (max 100)"})
	}
	if len(in.Icon) > 64 || len([]rune(in.Description)) > 500 {
		errs = append(errs, customfields.FieldError{Field: "description", Code: customfields.CodeInvalid, Message: "description max 500, icon max 64"})
	}
	if len(errs) > 0 {
		return &customfields.ValidationError{Errors: errs}
	}
	return nil
}

// CreateObjectType adds an object type.
func (s *Service) CreateObjectType(ctx context.Context, orgID int64, in model.ObjectTypeInput) (model.ObjectType, error) {
	if err := validateObjectType(&in, true); err != nil {
		return model.ObjectType{}, err
	}
	n, err := s.q.CountCustomObjectTypes(ctx, orgID)
	if err != nil {
		return model.ObjectType{}, err
	}
	if n >= MaxObjectTypes {
		return model.ObjectType{}, fmt.Errorf("%w: at most %d object types", ErrLimit, MaxObjectTypes)
	}
	row, err := s.q.CreateCustomObjectType(ctx, db.CreateCustomObjectTypeParams{
		OrganizationID: orgID, Key: in.Key, SingularLabel: in.SingularLabel,
		PluralLabel: in.PluralLabel, Description: in.Description, Icon: in.Icon,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return model.ObjectType{}, fmt.Errorf("%w: key %q is already used", ErrConflict, in.Key)
		}
		return model.ObjectType{}, err
	}
	return toObjectType(row), nil
}

// UpdateObjectType changes labels, icon and the archived flag.
func (s *Service) UpdateObjectType(ctx context.Context, orgID int64, id uuid.UUID, in model.ObjectTypeInput) (model.ObjectType, error) {
	if err := validateObjectType(&in, false); err != nil {
		return model.ObjectType{}, err
	}
	row, err := s.q.GetCustomObjectType(ctx, db.GetCustomObjectTypeParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return model.ObjectType{}, notFound(err)
	}
	updated, err := s.q.UpdateCustomObjectType(ctx, db.UpdateCustomObjectTypeParams{
		OrganizationID: orgID, ID: row.ID, SingularLabel: in.SingularLabel, PluralLabel: in.PluralLabel,
		Description: in.Description, Icon: in.Icon, Archived: in.Archived,
	})
	if err != nil {
		return model.ObjectType{}, err
	}
	return toObjectType(updated), nil
}

func toObjectType(r db.CustomObjectType) model.ObjectType {
	return model.ObjectType{
		UUID: r.Uuid, Key: r.Key, SingularLabel: r.SingularLabel, PluralLabel: r.PluralLabel,
		Description: r.Description, Icon: r.Icon, Archived: r.ArchivedAt.Valid,
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
	}
}

// --- Custom object records ---

// RecordScope is a resolved object type with its field definitions.
type RecordScope struct {
	Type db.CustomObjectType
	Defs []customfields.Definition
}

// ResolveRecordScope loads an object type and its fields.
func (s *Service) ResolveRecordScope(ctx context.Context, orgID int64, typeUUID uuid.UUID) (RecordScope, error) {
	ot, err := s.q.GetCustomObjectType(ctx, db.GetCustomObjectTypeParams{OrganizationID: orgID, Uuid: typeUUID})
	if err != nil {
		return RecordScope{}, notFound(err)
	}
	defs, err := s.Definitions(ctx, orgID, customfields.EntityObject, &ot.ID)
	if err != nil {
		return RecordScope{}, err
	}
	return RecordScope{Type: ot, Defs: defs}, nil
}

// ListRecords returns one page of records.
func (s *Service) ListRecords(ctx context.Context, scope RecordScope, f model.RecordList) ([]model.ObjectRecord, int64, error) {
	f.OrganizationID = scope.Type.OrganizationID
	f.ObjectTypeID = scope.Type.ID
	f.Defs = scope.Defs
	rows, total, err := s.repo.ListRecords(ctx, f)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.ObjectRecord, 0, len(rows))
	for _, r := range rows {
		rec, err := toRecord(r, scope.Type.Uuid)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, rec)
	}
	return out, total, nil
}

func (s *Service) getRecord(ctx context.Context, scope RecordScope, id uuid.UUID) (db.CustomObjectRecord, error) {
	row, err := s.q.GetCustomObjectRecord(ctx, db.GetCustomObjectRecordParams{
		OrganizationID: scope.Type.OrganizationID, ObjectTypeID: scope.Type.ID, Uuid: id,
	})
	return row, notFound(err)
}

// GetRecord returns one record.
func (s *Service) GetRecord(ctx context.Context, scope RecordScope, id uuid.UUID) (model.ObjectRecord, error) {
	row, err := s.getRecord(ctx, scope, id)
	if err != nil {
		return model.ObjectRecord{}, err
	}
	return toRecord(row, scope.Type.Uuid)
}

func displayName(v *string) (string, error) {
	if v == nil {
		return "", invalid("display_name is required")
	}
	name := strings.TrimSpace(*v)
	if name == "" || len([]rune(name)) > 255 {
		return "", invalid("display_name is required (max 255)")
	}
	return name, nil
}

// CreateRecord adds a record to an active object type.
func (s *Service) CreateRecord(ctx context.Context, scope RecordScope, actorID int64, in model.RecordInput) (model.ObjectRecord, error) {
	if scope.Type.ArchivedAt.Valid {
		return model.ObjectRecord{}, invalid("object type is archived")
	}
	name, err := displayName(in.DisplayName)
	if err != nil {
		return model.ObjectRecord{}, err
	}
	values, err := s.ValidateValues(ctx, scope.Type.OrganizationID, scope.Defs, nil, in.CustomFields, customfields.ModeCreate)
	if err != nil {
		return model.ObjectRecord{}, err
	}
	body, err := json.Marshal(values)
	if err != nil {
		return model.ObjectRecord{}, err
	}
	row, err := s.q.CreateCustomObjectRecord(ctx, db.CreateCustomObjectRecordParams{
		OrganizationID: scope.Type.OrganizationID, ObjectTypeID: scope.Type.ID, DisplayName: name,
		CustomFields: body, CreatedBy: pgtype.Int8{Int64: actorID, Valid: actorID > 0},
	})
	if err != nil {
		return model.ObjectRecord{}, err
	}
	return toRecord(row, scope.Type.Uuid)
}

// UpdateRecord renames a record and/or merges custom field values.
func (s *Service) UpdateRecord(ctx context.Context, scope RecordScope, id uuid.UUID, in model.RecordInput) (model.ObjectRecord, error) {
	row, err := s.getRecord(ctx, scope, id)
	if err != nil {
		return model.ObjectRecord{}, err
	}
	name := row.DisplayName
	if in.DisplayName != nil {
		if name, err = displayName(in.DisplayName); err != nil {
			return model.ObjectRecord{}, err
		}
	}
	current := map[string]any{}
	if err := json.Unmarshal(row.CustomFields, &current); err != nil {
		return model.ObjectRecord{}, err
	}
	values, err := s.ValidateValues(ctx, scope.Type.OrganizationID, scope.Defs, current, in.CustomFields, customfields.ModeUpdate)
	if err != nil {
		return model.ObjectRecord{}, err
	}
	body, err := json.Marshal(values)
	if err != nil {
		return model.ObjectRecord{}, err
	}
	updated, err := s.q.UpdateCustomObjectRecord(ctx, db.UpdateCustomObjectRecordParams{
		OrganizationID: scope.Type.OrganizationID, ID: row.ID, DisplayName: name, CustomFields: body,
	})
	if err != nil {
		return model.ObjectRecord{}, err
	}
	return toRecord(updated, scope.Type.Uuid)
}

// DeleteRecord removes a record.
func (s *Service) DeleteRecord(ctx context.Context, scope RecordScope, id uuid.UUID) error {
	row, err := s.getRecord(ctx, scope, id)
	if err != nil {
		return err
	}
	_, err = s.q.DeleteCustomObjectRecord(ctx, db.DeleteCustomObjectRecordParams{OrganizationID: scope.Type.OrganizationID, ID: row.ID})
	return err
}

func toRecord(r db.CustomObjectRecord, typeUUID uuid.UUID) (model.ObjectRecord, error) {
	values := map[string]any{}
	if err := json.Unmarshal(r.CustomFields, &values); err != nil {
		return model.ObjectRecord{}, err
	}
	return model.ObjectRecord{
		UUID: r.Uuid, ObjectTypeUUID: typeUUID, DisplayName: r.DisplayName, CustomFields: values,
		CreatedAt: r.CreatedAt.Time.UTC().Truncate(time.Microsecond), UpdatedAt: r.UpdatedAt.Time.UTC().Truncate(time.Microsecond),
	}, nil
}
