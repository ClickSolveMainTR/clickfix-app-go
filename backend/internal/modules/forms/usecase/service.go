// Package usecase implements dynamic request forms (CF-52): form CRUD, the
// form a channel offers, validating and applying a submission onto the
// conversation and contact it started, editing a conversation's form values
// and the "required before resolving" check.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forms/engine"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forms/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/dbtx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Errors returned by the service. Value validation failures are
// *customfields.ValidationError.
var (
	ErrNotFound = errors.New("forms: not found")
	ErrInvalid  = errors.New("forms: invalid request")
)

func invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Service is the request form use case.
type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// New builds the service.
func New(pool *pgxpool.Pool, q *db.Queries) *Service { return &Service{pool: pool, q: q} }

// customs loads the conversation and contact custom fields of the organization.
func (s *Service) customs(ctx context.Context, q *db.Queries, orgID int64) ([]engine.Custom, error) {
	rows, err := q.ListFormCustomFieldDefinitions(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]engine.Custom, 0, len(rows))
	for _, r := range rows {
		d := customfields.Definition{
			Key: r.Key, Label: r.Label, Type: customfields.FieldType(r.FieldType), Required: r.Required,
			Archived: r.ArchivedAt.Valid,
		}
		if err := json.Unmarshal(r.Options, &d.Options); err != nil {
			return nil, fmt.Errorf("forms: options of %s: %w", r.Key, err)
		}
		if err := json.Unmarshal(r.Config, &d.Config); err != nil {
			return nil, fmt.Errorf("forms: config of %s: %w", r.Key, err)
		}
		out = append(out, engine.Custom{UUID: r.Uuid, Entity: r.EntityType, Def: d})
	}
	return out, nil
}

func decodeFields(raw []byte) []engine.Field {
	var f []engine.Field
	if err := json.Unmarshal(raw, &f); err != nil || f == nil {
		return []engine.Field{}
	}
	return f
}

// resolved loads a stored form's fields with their definitions (fields of
// removed custom fields are skipped).
func (s *Service) resolved(ctx context.Context, q *db.Queries, r db.RequestForm) ([]engine.Resolved, error) {
	customs, err := s.customs(ctx, q, r.OrganizationID)
	if err != nil {
		return nil, err
	}
	return engine.Resolve(decodeFields(r.Fields), customs, false)
}

func toForm(r db.RequestForm, fields []engine.Field) model.Form {
	return model.Form{
		UUID: r.Uuid, Name: r.Name, Description: r.Description, IsActive: r.IsActive, Channels: nonNil(r.Channels),
		Position: r.Position, Fields: fields, CreatedAt: r.CreatedAt.Time.UTC(), UpdatedAt: r.UpdatedAt.Time.UTC(),
	}
}

func toPublic(r db.RequestForm, fields []engine.Resolved) *model.PublicForm {
	out := &model.PublicForm{UUID: r.Uuid, Name: r.Name, Description: r.Description, Fields: []model.PublicField{}}
	for _, f := range fields {
		opts := []customfields.Option{}
		for _, o := range f.Def.Options {
			if !o.Archived {
				opts = append(opts, o)
			}
		}
		out.Fields = append(out.Fields, model.PublicField{
			Key: f.Key, Source: f.Source, Type: f.Def.Type, Label: f.Def.Label, Help: f.Help, Required: f.Required,
			Options: opts, Placeholder: f.Def.Config.Placeholder, Condition: f.Condition,
		})
	}
	return out
}

// List returns the organization's forms.
func (s *Service) List(ctx context.Context, orgID int64) ([]model.Form, error) {
	rows, err := s.q.ListRequestForms(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]model.Form, 0, len(rows))
	for _, r := range rows {
		out = append(out, toForm(r, decodeFields(r.Fields)))
	}
	return out, nil
}

// Get returns one form.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (model.Form, error) {
	r, err := s.q.GetRequestFormByUUID(ctx, db.GetRequestFormByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return model.Form{}, notFound(err)
	}
	return toForm(r, decodeFields(r.Fields)), nil
}

func (s *Service) normalize(ctx context.Context, orgID int64, in *model.FormInput) ([]byte, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len([]rune(in.Name)) > 120 {
		return nil, invalidf("name is required (max 120 characters)")
	}
	in.Description = strings.TrimSpace(in.Description)
	if len([]rune(in.Description)) > 1000 {
		return nil, invalidf("description is too long")
	}
	channels := []string{}
	for _, c := range in.Channels {
		if !slices.Contains(model.Channels, c) {
			return nil, invalidf("unknown channel %q", c)
		}
		if !slices.Contains(channels, c) {
			channels = append(channels, c)
		}
	}
	in.Channels = channels
	if len(in.Fields) == 0 {
		return nil, invalidf("a form needs at least one field")
	}
	customs, err := s.customs(ctx, s.q, orgID)
	if err != nil {
		return nil, err
	}
	res, err := engine.Resolve(in.Fields, customs, true)
	if err != nil {
		return nil, invalidf("%s", strings.TrimPrefix(err.Error(), engine.ErrInvalid.Error()+": "))
	}
	fields := make([]engine.Field, 0, len(res))
	for _, r := range res {
		fields = append(fields, r.Field)
	}
	in.Fields = fields
	return json.Marshal(fields)
}

// Create adds a form.
func (s *Service) Create(ctx context.Context, orgID int64, in model.FormInput) (model.Form, error) {
	fields, err := s.normalize(ctx, orgID, &in)
	if err != nil {
		return model.Form{}, err
	}
	pos := int32(0)
	if in.Position != nil {
		pos = *in.Position
	} else if pos, err = s.q.NextRequestFormPosition(ctx, orgID); err != nil {
		return model.Form{}, err
	}
	r, err := s.q.InsertRequestForm(ctx, db.InsertRequestFormParams{
		OrganizationID: orgID, Name: in.Name, Description: in.Description, IsActive: in.IsActive == nil || *in.IsActive,
		Channels: in.Channels, Fields: fields, Position: pos,
	})
	if err != nil {
		return model.Form{}, err
	}
	return toForm(r, in.Fields), nil
}

// Update replaces a form.
func (s *Service) Update(ctx context.Context, orgID int64, id uuid.UUID, in model.FormInput) (model.Form, error) {
	cur, err := s.q.GetRequestFormByUUID(ctx, db.GetRequestFormByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return model.Form{}, notFound(err)
	}
	fields, err := s.normalize(ctx, orgID, &in)
	if err != nil {
		return model.Form{}, err
	}
	pos, active := cur.Position, cur.IsActive
	if in.Position != nil {
		pos = *in.Position
	}
	if in.IsActive != nil {
		active = *in.IsActive
	}
	r, err := s.q.UpdateRequestForm(ctx, db.UpdateRequestFormParams{
		OrganizationID: orgID, ID: cur.ID, Name: in.Name, Description: in.Description, IsActive: active,
		Channels: in.Channels, Fields: fields, Position: pos,
	})
	if err != nil {
		return model.Form{}, err
	}
	return toForm(r, in.Fields), nil
}

// Delete removes a form. Conversations keep the values they were given.
func (s *Service) Delete(ctx context.Context, orgID int64, id uuid.UUID) error {
	n, err := s.q.DeleteRequestForm(ctx, db.DeleteRequestFormParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// offered picks the form a channel shows: preferred when it is active and
// lists the channel (CF-198: the widget settings pick their pre-chat form),
// else the first active form of the channel. ok is false without one.
func (s *Service) offered(ctx context.Context, orgID int64, channel string, preferred uuid.UUID) (db.RequestForm, bool, error) {
	if preferred != uuid.Nil {
		r, err := s.q.GetRequestFormByUUID(ctx, db.GetRequestFormByUUIDParams{OrganizationID: orgID, Uuid: preferred})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return db.RequestForm{}, false, err
		}
		if err == nil && r.IsActive && slices.Contains(r.Channels, channel) {
			return r, true, nil
		}
	}
	r, err := s.q.FirstActiveRequestFormForChannel(ctx, db.FirstActiveRequestFormForChannelParams{OrganizationID: orgID, Channel: channel})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.RequestForm{}, false, nil
		}
		return db.RequestForm{}, false, err
	}
	return r, true, nil
}

// ForChannel returns the form a channel offers (nil when none is active).
func (s *Service) ForChannel(ctx context.Context, orgID int64, channel string) (*model.PublicForm, error) {
	return s.Offered(ctx, orgID, channel, uuid.Nil)
}

// Offered returns the form a channel offers, preferring one by id (nil when
// none is active).
func (s *Service) Offered(ctx context.Context, orgID int64, channel string, preferred uuid.UUID) (*model.PublicForm, error) {
	r, ok, err := s.offered(ctx, orgID, channel, preferred)
	if err != nil || !ok {
		return nil, err
	}
	fields, err := s.resolved(ctx, s.q, r)
	if err != nil {
		return nil, err
	}
	return toPublic(r, fields), nil
}

// Check validates a submission of the form a channel offers. optional keys
// are never required (the widget's message comes from its composer). A
// channel without an active form accepts no submission (nil, nil).
func (s *Service) Check(ctx context.Context, orgID int64, channel string, sub *model.Submission, optional ...string) (*engine.Submission, uuid.UUID, error) {
	return s.CheckOffered(ctx, orgID, channel, uuid.Nil, sub, optional...)
}

// CheckOffered is Check for the form Offered picks.
func (s *Service) CheckOffered(ctx context.Context, orgID int64, channel string, preferred uuid.UUID, sub *model.Submission, optional ...string) (*engine.Submission, uuid.UUID, error) {
	r, ok, err := s.offered(ctx, orgID, channel, preferred)
	if err != nil || !ok {
		return nil, uuid.Nil, err
	}
	values := map[string]any{}
	if sub != nil {
		if sub.FormUUID != uuid.Nil && sub.FormUUID != r.Uuid {
			return nil, uuid.Nil, invalidf("this form is no longer offered; reload and try again")
		}
		values = sub.Values
	}
	fields, err := s.resolved(ctx, s.q, r)
	if err != nil {
		return nil, uuid.Nil, err
	}
	out, err := engine.Check(fields, values, optional...)
	if err != nil {
		return nil, uuid.Nil, err
	}
	return &out, r.Uuid, nil
}

// Apply writes a checked submission onto the conversation it started and
// its contact: subject, priority, custom fields and the form reference.
func (s *Service) Apply(ctx context.Context, orgID, conversationID int64, formUUID uuid.UUID, sub engine.Submission) error {
	return s.inTx(ctx, func(ctx context.Context, q *db.Queries) error {
		conv, err := q.LockConversation(ctx, db.LockConversationParams{OrganizationID: orgID, ID: conversationID})
		if err != nil {
			return notFound(err)
		}
		values := map[string]any{}
		_ = json.Unmarshal(conv.CustomFields, &values)
		for k, v := range sub.Conversation {
			values[k] = v
		}
		raw, err := json.Marshal(values)
		if err != nil {
			return err
		}
		subject, priority := conv.Subject, conv.Priority
		if sub.Subject != "" {
			subject = sub.Subject
		}
		if sub.Priority != "" {
			priority = sub.Priority
		}
		if err := q.SetConversationFormData(ctx, db.SetConversationFormDataParams{
			CustomFields: raw, Subject: subject, Priority: priority, FormUuid: formUUID.String(),
			OrganizationID: orgID, ID: conv.ID,
		}); err != nil {
			return err
		}
		contactFields, err := json.Marshal(sub.Contact)
		if err != nil {
			return err
		}
		return q.SetContactFormData(ctx, db.SetContactFormDataParams{
			Name: sub.Name, Email: strings.ToLower(sub.Email), Phone: sub.Phone, CustomFields: contactFields,
			OrganizationID: orgID, ID: conv.ContactID,
		})
	})
}

// formOf returns the form a conversation started with (nil when none or it
// was deleted).
func (s *Service) formOf(ctx context.Context, q *db.Queries, conv db.Conversation) (*db.RequestForm, []engine.Resolved, error) {
	var meta struct {
		RequestForm string `json:"request_form"`
	}
	_ = json.Unmarshal(conv.Meta, &meta)
	id, err := uuid.Parse(meta.RequestForm)
	if err != nil {
		return nil, nil, nil
	}
	r, err := q.GetRequestFormByUUID(ctx, db.GetRequestFormByUUIDParams{OrganizationID: conv.OrganizationID, Uuid: id})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	fields, err := s.resolved(ctx, q, r)
	if err != nil {
		return nil, nil, err
	}
	return &r, fields, nil
}

// ConversationForm returns the form of a conversation with its values.
func (s *Service) ConversationForm(ctx context.Context, orgID int64, id uuid.UUID) (model.ConversationForm, error) {
	conv, err := s.q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return model.ConversationForm{}, notFound(err)
	}
	return s.conversationForm(ctx, s.q, conv)
}

func (s *Service) conversationForm(ctx context.Context, q *db.Queries, conv db.Conversation) (model.ConversationForm, error) {
	out := model.ConversationForm{Values: map[string]any{}, RequiredToResolve: []string{}, MissingToResolve: []string{}}
	r, fields, err := s.formOf(ctx, q, conv)
	if err != nil || r == nil {
		return out, err
	}
	stored := map[string]any{}
	_ = json.Unmarshal(conv.CustomFields, &stored)
	out.Form = toPublic(*r, fields)
	for _, f := range fields {
		if f.Source != engine.SourceConversation {
			continue
		}
		if v, ok := stored[f.StoredKey]; ok {
			out.Values[f.Key] = v
		}
		if f.RequiredToResolve {
			out.RequiredToResolve = append(out.RequiredToResolve, f.Key)
		}
	}
	for _, f := range engine.MissingToResolve(fields, stored) {
		out.MissingToResolve = append(out.MissingToResolve, f.Key)
	}
	return out, nil
}

// UpdateConversationValues lets an agent fill or change the conversation
// fields of the conversation's form (keys are form keys; null clears).
func (s *Service) UpdateConversationValues(ctx context.Context, orgID int64, id uuid.UUID, values map[string]any) (model.ConversationForm, error) {
	var out model.ConversationForm
	err := s.inTx(ctx, func(ctx context.Context, q *db.Queries) error {
		conv, err := q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: orgID, Uuid: id})
		if err != nil {
			return notFound(err)
		}
		if conv, err = q.LockConversation(ctx, db.LockConversationParams{OrganizationID: orgID, ID: conv.ID}); err != nil {
			return err
		}
		r, fields, err := s.formOf(ctx, q, conv)
		if err != nil {
			return err
		}
		if r == nil {
			return invalidf("this conversation was not started with a form")
		}
		stored := map[string]any{}
		_ = json.Unmarshal(conv.CustomFields, &stored)
		// Validate only the conversation fields, none required here (the
		// resolve check enforces them), against the current values.
		defs := []customfields.Definition{}
		current := map[string]any{}
		byKey := map[string]engine.Resolved{}
		for _, f := range fields {
			if f.Source != engine.SourceConversation {
				continue
			}
			d := f.Def
			d.Required = false
			d.Condition = nil
			defs = append(defs, d)
			byKey[f.Key] = f
			if v, ok := stored[f.StoredKey]; ok {
				current[f.Key] = v
			}
		}
		next, err := customfields.Validate(defs, current, values, customfields.ModeUpdate)
		if err != nil {
			return err
		}
		for key, f := range byKey {
			if v, ok := next[key]; ok {
				stored[f.StoredKey] = v
			} else {
				delete(stored, f.StoredKey)
			}
		}
		raw, err := json.Marshal(stored)
		if err != nil {
			return err
		}
		if err := q.SetConversationCustomFields(ctx, db.SetConversationCustomFieldsParams{CustomFields: raw, OrganizationID: orgID, ID: conv.ID}); err != nil {
			return err
		}
		conv.CustomFields = raw
		out, err = s.conversationForm(ctx, q, conv)
		return err
	})
	return out, err
}

// CheckResolve fails when the conversation's form has fields required
// before resolving that are still empty. It runs inside the conversation
// update transaction (ctx carries it).
func (s *Service) CheckResolve(ctx context.Context, conv db.Conversation) error {
	q := s.q
	if tx, ok := dbtx.Tx(ctx); ok {
		q = s.q.WithTx(tx)
	}
	_, fields, err := s.formOf(ctx, q, conv)
	if err != nil || len(fields) == 0 {
		return err
	}
	stored := map[string]any{}
	_ = json.Unmarshal(conv.CustomFields, &stored)
	missing := engine.MissingToResolve(fields, stored)
	if len(missing) == 0 {
		return nil
	}
	labels := make([]string, 0, len(missing))
	for _, f := range missing {
		labels = append(labels, f.Def.Label)
	}
	return fmt.Errorf("fill in before resolving: %s", strings.Join(labels, ", "))
}

func (s *Service) inTx(ctx context.Context, fn func(ctx context.Context, q *db.Queries) error) error {
	if tx, ok := dbtx.Tx(ctx); ok {
		return fn(ctx, s.q.WithTx(tx))
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("forms: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(dbtx.WithTx(ctx, tx), s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func nonNil[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}
