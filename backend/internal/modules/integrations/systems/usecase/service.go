// Package usecase is the connected system ("Sistemini bağla", CF-197):
// an organization's own system described on one screen and used by the
// AI agent, members (panel, assistant, MCP) and workflows.
package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/integrations/systems/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/netguard"
)

// Store is the sqlc subset used by the service.
type Store interface {
	IntegrationConnection(ctx context.Context, organizationID int64) (db.IntegrationConnection, error)
	IntegrationSaveConnection(ctx context.Context, arg db.IntegrationSaveConnectionParams) (db.IntegrationConnection, error)
	IntegrationSources(ctx context.Context, organizationID int64) ([]db.IntegrationSourcesRow, error)
	IntegrationSourceByKey(ctx context.Context, arg db.IntegrationSourceByKeyParams) (db.IntegrationSourceByKeyRow, error)
	IntegrationSaveSource(ctx context.Context, arg db.IntegrationSaveSourceParams) (db.IntegrationSource, error)
	IntegrationDeleteSource(ctx context.Context, arg db.IntegrationDeleteSourceParams) (int64, error)
	IntegrationObjectTypeByUUID(ctx context.Context, arg db.IntegrationObjectTypeByUUIDParams) (db.CustomObjectType, error)
	IntegrationObjectTypeByKey(ctx context.Context, arg db.IntegrationObjectTypeByKeyParams) (db.CustomObjectType, error)
	IntegrationActions(ctx context.Context, organizationID int64) ([]db.IntegrationActionsRow, error)
	IntegrationActionByKey(ctx context.Context, arg db.IntegrationActionByKeyParams) (db.IntegrationActionByKeyRow, error)
	IntegrationSaveAction(ctx context.Context, arg db.IntegrationSaveActionParams) (db.IntegrationAction, error)
	IntegrationDeleteAction(ctx context.Context, arg db.IntegrationDeleteActionParams) (int64, error)
	IntegrationPlaybooks(ctx context.Context, organizationID int64) ([]db.IntegrationPlaybooksRow, error)
	IntegrationPlaybookByIntent(ctx context.Context, arg db.IntegrationPlaybookByIntentParams) (db.IntegrationPlaybookByIntentRow, error)
	IntegrationSavePlaybook(ctx context.Context, arg db.IntegrationSavePlaybookParams) (db.IntegrationPlaybook, error)
	IntegrationDeletePlaybook(ctx context.Context, arg db.IntegrationDeletePlaybookParams) (int64, error)
	IntegrationProcedureByUUID(ctx context.Context, arg db.IntegrationProcedureByUUIDParams) (db.IntegrationProcedureByUUIDRow, error)
	IntegrationLogCall(ctx context.Context, arg db.IntegrationLogCallParams) error
	IntegrationRecentCalls(ctx context.Context, organizationID int64) ([]db.IntegrationRecentCallsRow, error)
	IntegrationContactIdentity(ctx context.Context, arg db.IntegrationContactIdentityParams) (db.IntegrationContactIdentityRow, error)
	IntegrationContactByExternalID(ctx context.Context, arg db.IntegrationContactByExternalIDParams) (uuid.UUID, error)
	IntegrationConversationContact(ctx context.Context, arg db.IntegrationConversationContactParams) (uuid.UUID, error)
	IntegrationConversationContactByUUID(ctx context.Context, arg db.IntegrationConversationContactByUUIDParams) (uuid.UUID, error)
	IntegrationWidgetRejections(ctx context.Context, organizationID int64) ([]db.IntegrationWidgetRejectionsRow, error)
	ListCustomObjectTypes(ctx context.Context, arg db.ListCustomObjectTypesParams) ([]db.CustomObjectType, error)
}

// Service is the connected system.
type Service struct {
	Q      Store
	Box    *crypto.SecretBox
	Caller *Caller
	// ValidateBaseURL checks a connection's base URL on save.
	ValidateBaseURL func(string) error
}

// New wires the service. A nil box means credentials cannot be stored.
func New(q Store, box *crypto.SecretBox, limiter Limiter) *Service {
	return &Service{Q: q, Box: box, Caller: NewCaller(limiter),
		ValidateBaseURL: func(raw string) error { return netguard.ValidateURL(raw, false) }}
}

var (
	keyRxp    = regexp.MustCompile(`^[a-z][a-z0-9_]{1,49}$`)
	intentRxp = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)
	paramRxp  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
	headerRxp = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)
)

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", model.ErrInvalid, fmt.Sprintf(format, a...))
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return model.ErrNotFound
	}
	return err
}

// ---------------------------------------------------------------- read

// Overview is the whole screen: connection, concepts with their sources,
// actions, playbooks and the latest calls.
func (s *Service) Overview(ctx context.Context, org int64) (model.Overview, error) {
	out := model.Overview{Concepts: []model.Concept{}, Actions: []model.Action{}, Playbooks: []model.Playbook{}, Calls: []model.Call{}}
	conn, err := s.Q.IntegrationConnection(ctx, org)
	switch {
	case err == nil:
		c := connectionView(conn)
		out.Connection = &c
	case !errors.Is(err, pgx.ErrNoRows):
		return out, err
	}
	types, err := s.Q.ListCustomObjectTypes(ctx, db.ListCustomObjectTypesParams{OrganizationID: org, IncludeArchived: false})
	if err != nil {
		return out, err
	}
	sources, err := s.Q.IntegrationSources(ctx, org)
	if err != nil {
		return out, err
	}
	byType := map[uuid.UUID]db.IntegrationSourcesRow{}
	for _, src := range sources {
		byType[src.ObjectTypeUuid] = src
	}
	for _, t := range types {
		c := model.Concept{ObjectTypeUUID: t.Uuid, Key: t.Key, Label: t.SingularLabel, PluralLabel: t.PluralLabel, Icon: t.Icon}
		if src, ok := byType[t.Uuid]; ok {
			v := sourceView(src.Uuid, src.PathTemplate, src.IdentityKey, src.ItemsPath, src.Fields, src.Enabled)
			c.Source = &v
		}
		out.Concepts = append(out.Concepts, c)
	}
	actions, err := s.Q.IntegrationActions(ctx, org)
	if err != nil {
		return out, err
	}
	for _, a := range actions {
		out.Actions = append(out.Actions, actionView(db.IntegrationActionByKeyRow(a)))
	}
	playbooks, err := s.Q.IntegrationPlaybooks(ctx, org)
	if err != nil {
		return out, err
	}
	for _, p := range playbooks {
		out.Playbooks = append(out.Playbooks, playbookView(db.IntegrationPlaybookByIntentRow(p)))
	}
	calls, err := s.Q.IntegrationRecentCalls(ctx, org)
	if err != nil {
		return out, err
	}
	for _, c := range calls {
		out.Calls = append(out.Calls, model.Call{Kind: c.Kind, Target: c.Target, Channel: c.Channel, Method: c.Method, MaskedPath: c.MaskedPath,
			StatusCode: c.StatusCode, OK: c.Ok, ErrorCode: c.ErrorCode, DurationMS: c.DurationMs, CreatedAt: c.CreatedAt.Time})
	}
	return out, nil
}

func connectionView(c db.IntegrationConnection) model.Connection {
	v := model.Connection{UUID: c.Uuid, Name: c.Name, BaseURL: c.BaseUrl, AuthType: c.AuthType, AuthHeader: c.AuthHeader, HasSecret: c.AuthEnc != "", TimeoutMS: c.TimeoutMs}
	if c.UpdatedAt.Valid {
		t := c.UpdatedAt.Time
		v.UpdatedAt = &t
	}
	return v
}

func sourceView(id uuid.UUID, path, key, items string, rawFields []byte, enabled bool) model.Source {
	fields := []model.Field{}
	_ = json.Unmarshal(rawFields, &fields)
	return model.Source{UUID: id, PathTemplate: path, IdentityKey: key, ItemsPath: items, Fields: fields, Enabled: enabled}
}

func actionView(a db.IntegrationActionByKeyRow) model.Action {
	params := []model.Param{}
	_ = json.Unmarshal(a.Params, &params)
	allowed := a.AllowedFor
	if allowed == nil {
		allowed = []string{}
	}
	return model.Action{UUID: a.Uuid, Key: a.Key, Title: a.Title, Description: a.Description, ConceptKey: a.ObjectTypeKey.String, Method: a.Method,
		PathTemplate: a.PathTemplate, Params: params, RequiresApproval: a.RequiresApproval, AllowedFor: allowed, Enabled: a.Enabled}
}

func playbookView(p db.IntegrationPlaybookByIntentRow) model.Playbook {
	v := model.Playbook{UUID: p.Uuid, Intent: p.Intent, Title: p.Title, Instructions: p.Instructions, ConceptKeys: nonNil(p.ConceptKeys), ActionKeys: nonNil(p.ActionKeys), Enabled: p.Enabled}
	if p.ProcedureUuid.Valid {
		id := uuid.UUID(p.ProcedureUuid.Bytes)
		v.ProcedureUUID = &id
	}
	return v
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// ---------------------------------------------------------------- write

// SaveConnection stores the base URL and authentication. The secret is
// encrypted; it is never returned.
func (s *Service) SaveConnection(ctx context.Context, org, actor int64, in model.ConnectionInput) (model.Connection, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	if len([]rune(in.Name)) > 120 {
		return model.Connection{}, invalid("name is too long")
	}
	if len(in.BaseURL) < 8 || len(in.BaseURL) > 500 || s.ValidateBaseURL(in.BaseURL) != nil {
		return model.Connection{}, invalid("base_url must be a public https address")
	}
	if _, err := BuildURL(in.BaseURL, "/", nil); err != nil {
		return model.Connection{}, invalid("base_url must not have a query, fragment or user info")
	}
	if in.AuthType == "" {
		in.AuthType = model.AuthNone
	}
	if !slices.Contains([]string{model.AuthNone, model.AuthBearer, model.AuthHeader, model.AuthBasic}, in.AuthType) {
		return model.Connection{}, invalid("auth_type must be none, bearer, header or basic")
	}
	if in.AuthType == model.AuthHeader {
		if !headerRxp.MatchString(in.AuthHeader) || strings.EqualFold(in.AuthHeader, "host") || strings.EqualFold(in.AuthHeader, "content-length") {
			return model.Connection{}, invalid("auth_header must be a header name")
		}
	} else {
		in.AuthHeader = ""
	}
	if in.AuthType == model.AuthBasic && in.Secret != "" && !strings.Contains(in.Secret, ":") {
		return model.Connection{}, invalid("basic secret must be user:password")
	}
	if in.TimeoutMS == 0 {
		in.TimeoutMS = 8000
	}
	if in.TimeoutMS < 1000 || in.TimeoutMS > 15000 {
		return model.Connection{}, invalid("timeout_ms must be between 1000 and 15000")
	}
	if len(in.Secret) > 4000 {
		return model.Connection{}, invalid("secret is too long")
	}
	keep := in.KeepSecret && in.Secret == ""
	enc := ""
	if in.AuthType == model.AuthNone {
		keep = false
	} else if !keep && in.Secret != "" {
		if s.Box == nil {
			return model.Connection{}, invalid("credential storage is unavailable")
		}
		var err error
		if enc, err = s.Box.Encrypt(in.Secret); err != nil {
			return model.Connection{}, err
		}
	}
	row, err := s.Q.IntegrationSaveConnection(ctx, db.IntegrationSaveConnectionParams{OrganizationID: org, Name: in.Name, BaseUrl: in.BaseURL, AuthType: in.AuthType,
		AuthHeader: in.AuthHeader, AuthEnc: enc, TimeoutMs: in.TimeoutMS, UpdatedBy: pgtype.Int8{Int64: actor, Valid: actor > 0}, KeepAuth: keep})
	if err != nil {
		return model.Connection{}, err
	}
	return connectionView(row), nil
}

func validFields(fields []model.Field) ([]model.Field, error) {
	if len(fields) > maxFields {
		return nil, invalid("at most %d fields", maxFields)
	}
	out := []model.Field{}
	seen := map[string]bool{}
	for _, f := range fields {
		f.Path, f.Label = strings.TrimSpace(f.Path), strings.TrimSpace(f.Label)
		if f.Path == "" || !ValidPath(f.Path) {
			return nil, invalid("field path %q is invalid", f.Path)
		}
		if len([]rune(f.Label)) > 80 {
			return nil, invalid("field label is too long")
		}
		if seen[f.Path] {
			continue
		}
		seen[f.Path] = true
		out = append(out, f)
	}
	return out, nil
}

func validSourceTemplate(path, key string) error {
	if err := ValidTemplate(path); err != nil {
		return invalid("path_template: %s", strings.TrimPrefix(err.Error(), CodeTemplate+": "))
	}
	ph := Placeholders(path)
	if !slices.Contains(ph, key) {
		return invalid("path_template must contain {%s}: data is read by the customer's external id", key)
	}
	for _, p := range ph {
		if p != model.KeyExternalID && p != model.KeyCompanyExternalID {
			return invalid("unknown placeholder {%s}; use {external_id} or {company_external_id}", p)
		}
	}
	return nil
}

// SaveSource sets where a concept is read from.
func (s *Service) SaveSource(ctx context.Context, org int64, objectType uuid.UUID, in model.SourceInput) (model.Source, error) {
	t, err := s.Q.IntegrationObjectTypeByUUID(ctx, db.IntegrationObjectTypeByUUIDParams{OrganizationID: org, Uuid: objectType})
	if err != nil {
		return model.Source{}, notFound(err)
	}
	if in.IdentityKey == "" {
		in.IdentityKey = model.KeyExternalID
	}
	if in.IdentityKey != model.KeyExternalID && in.IdentityKey != model.KeyCompanyExternalID {
		return model.Source{}, invalid("identity_key must be external_id or company_external_id")
	}
	in.PathTemplate = strings.TrimSpace(in.PathTemplate)
	if err := validSourceTemplate(in.PathTemplate, in.IdentityKey); err != nil {
		return model.Source{}, err
	}
	in.ItemsPath = strings.TrimSpace(in.ItemsPath)
	if !ValidPath(in.ItemsPath) {
		return model.Source{}, invalid("items_path is invalid")
	}
	fields, err := validFields(in.Fields)
	if err != nil {
		return model.Source{}, err
	}
	raw, _ := json.Marshal(fields)
	row, err := s.Q.IntegrationSaveSource(ctx, db.IntegrationSaveSourceParams{OrganizationID: org, ObjectTypeID: t.ID, PathTemplate: in.PathTemplate,
		IdentityKey: in.IdentityKey, ItemsPath: in.ItemsPath, Fields: raw, Enabled: in.Enabled})
	if err != nil {
		return model.Source{}, err
	}
	return sourceView(row.Uuid, row.PathTemplate, row.IdentityKey, row.ItemsPath, row.Fields, row.Enabled), nil
}

// DeleteSource removes a concept's source.
func (s *Service) DeleteSource(ctx context.Context, org int64, objectType uuid.UUID) error {
	t, err := s.Q.IntegrationObjectTypeByUUID(ctx, db.IntegrationObjectTypeByUUIDParams{OrganizationID: org, Uuid: objectType})
	if err != nil {
		return notFound(err)
	}
	n, err := s.Q.IntegrationDeleteSource(ctx, db.IntegrationDeleteSourceParams{OrganizationID: org, ObjectTypeID: t.ID})
	if err == nil && n == 0 {
		return model.ErrNotFound
	}
	return err
}

func validParams(params []model.Param) ([]model.Param, error) {
	if len(params) > 20 {
		return nil, invalid("at most 20 params")
	}
	out := []model.Param{}
	seen := map[string]bool{}
	for _, p := range params {
		p.Key, p.Label = strings.TrimSpace(p.Key), strings.TrimSpace(p.Label)
		if !paramRxp.MatchString(p.Key) || p.Key == model.KeyExternalID || p.Key == model.KeyCompanyExternalID {
			return nil, invalid("param key %q is invalid", p.Key)
		}
		if seen[p.Key] {
			return nil, invalid("param %q is repeated", p.Key)
		}
		seen[p.Key] = true
		if p.Type == "" {
			p.Type = "string"
		}
		if !slices.Contains([]string{"string", "number", "boolean"}, p.Type) {
			return nil, invalid("param type must be string, number or boolean")
		}
		if len([]rune(p.Label)) > 80 {
			return nil, invalid("param label is too long")
		}
		out = append(out, p)
	}
	return out, nil
}

// SaveAction creates or updates an action.
func (s *Service) SaveAction(ctx context.Context, org int64, key string, in model.ActionInput) (model.Action, error) {
	if !keyRxp.MatchString(key) {
		return model.Action{}, invalid("key must look like resend_invoice")
	}
	in.Title, in.Description = strings.TrimSpace(in.Title), strings.TrimSpace(in.Description)
	if in.Title == "" || len([]rune(in.Title)) > 120 || len([]rune(in.Description)) > 1000 {
		return model.Action{}, invalid("title is required (120 characters) and description is at most 1000 characters")
	}
	in.Method = strings.ToUpper(strings.TrimSpace(in.Method))
	if in.Method == "" {
		in.Method = http.MethodPost
	}
	if !slices.Contains([]string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}, in.Method) {
		return model.Action{}, invalid("method must be POST, PUT, PATCH or DELETE")
	}
	params, err := validParams(in.Params)
	if err != nil {
		return model.Action{}, err
	}
	in.PathTemplate = strings.TrimSpace(in.PathTemplate)
	if err := ValidTemplate(in.PathTemplate); err != nil {
		return model.Action{}, invalid("path_template: %s", strings.TrimPrefix(err.Error(), CodeTemplate+": "))
	}
	for _, ph := range Placeholders(in.PathTemplate) {
		known := ph == model.KeyExternalID || ph == model.KeyCompanyExternalID || slices.ContainsFunc(params, func(p model.Param) bool { return p.Key == ph })
		if !known {
			return model.Action{}, invalid("unknown placeholder {%s}; add it as a param", ph)
		}
	}
	if len(in.AllowedFor) == 0 {
		in.AllowedFor = []string{model.ChannelAgent}
	}
	allowed := []string{}
	for _, c := range in.AllowedFor {
		if !slices.Contains(model.Channels, c) {
			return model.Action{}, invalid("allowed_for holds ai, agent or workflow")
		}
		if !slices.Contains(allowed, c) {
			allowed = append(allowed, c)
		}
	}
	slices.Sort(allowed)
	var objectType pgtype.Int8
	if k := strings.TrimSpace(in.ConceptKey); k != "" {
		t, err := s.Q.IntegrationObjectTypeByKey(ctx, db.IntegrationObjectTypeByKeyParams{OrganizationID: org, Key: k})
		if err != nil {
			return model.Action{}, invalid("concept %q does not exist", k)
		}
		objectType = pgtype.Int8{Int64: t.ID, Valid: true}
	}
	raw, _ := json.Marshal(params)
	if _, err := s.Q.IntegrationSaveAction(ctx, db.IntegrationSaveActionParams{OrganizationID: org, Key: key, Title: in.Title, Description: in.Description,
		ObjectTypeID: objectType, Method: in.Method, PathTemplate: in.PathTemplate, Params: raw, RequiresApproval: in.RequiresApproval, AllowedFor: allowed, Enabled: in.Enabled}); err != nil {
		return model.Action{}, err
	}
	row, err := s.Q.IntegrationActionByKey(ctx, db.IntegrationActionByKeyParams{OrganizationID: org, Key: key})
	if err != nil {
		return model.Action{}, err
	}
	return actionView(row), nil
}

// DeleteAction removes an action.
func (s *Service) DeleteAction(ctx context.Context, org int64, key string) error {
	n, err := s.Q.IntegrationDeleteAction(ctx, db.IntegrationDeleteActionParams{OrganizationID: org, Key: key})
	if err == nil && n == 0 {
		return model.ErrNotFound
	}
	return err
}

// SavePlaybook creates or updates a plain-language playbook.
func (s *Service) SavePlaybook(ctx context.Context, org int64, intent string, in model.PlaybookInput) (model.Playbook, error) {
	if !intentRxp.MatchString(intent) {
		return model.Playbook{}, invalid("intent must look like invoice_not_received")
	}
	in.Title, in.Instructions = strings.TrimSpace(in.Title), strings.TrimSpace(in.Instructions)
	if in.Title == "" || len([]rune(in.Title)) > 200 {
		return model.Playbook{}, invalid("title is required (200 characters)")
	}
	if in.Instructions == "" || len([]rune(in.Instructions)) > 3000 {
		return model.Playbook{}, invalid("instructions are required (3000 characters)")
	}
	if len(in.ConceptKeys) > 10 || len(in.ActionKeys) > 10 {
		return model.Playbook{}, invalid("at most 10 concepts and 10 actions")
	}
	concepts := []string{}
	for _, k := range in.ConceptKeys {
		if _, err := s.Q.IntegrationObjectTypeByKey(ctx, db.IntegrationObjectTypeByKeyParams{OrganizationID: org, Key: k}); err != nil {
			return model.Playbook{}, invalid("concept %q does not exist", k)
		}
		if !slices.Contains(concepts, k) {
			concepts = append(concepts, k)
		}
	}
	actions := []string{}
	for _, k := range in.ActionKeys {
		if _, err := s.Q.IntegrationActionByKey(ctx, db.IntegrationActionByKeyParams{OrganizationID: org, Key: k}); err != nil {
			return model.Playbook{}, invalid("action %q does not exist", k)
		}
		if !slices.Contains(actions, k) {
			actions = append(actions, k)
		}
	}
	var procedure pgtype.Int8
	if in.ProcedureUUID != nil && *in.ProcedureUUID != uuid.Nil {
		p, err := s.Q.IntegrationProcedureByUUID(ctx, db.IntegrationProcedureByUUIDParams{OrganizationID: org, Uuid: *in.ProcedureUUID})
		if err != nil {
			return model.Playbook{}, invalid("procedure does not exist")
		}
		procedure = pgtype.Int8{Int64: p.ID, Valid: true}
	}
	if _, err := s.Q.IntegrationSavePlaybook(ctx, db.IntegrationSavePlaybookParams{OrganizationID: org, Intent: intent, Title: in.Title, Instructions: in.Instructions,
		ConceptKeys: concepts, ActionKeys: actions, ProcedureID: procedure, Enabled: in.Enabled}); err != nil {
		return model.Playbook{}, err
	}
	row, err := s.Q.IntegrationPlaybookByIntent(ctx, db.IntegrationPlaybookByIntentParams{OrganizationID: org, Intent: intent})
	if err != nil {
		return model.Playbook{}, err
	}
	return playbookView(row), nil
}

// DeletePlaybook removes a playbook.
func (s *Service) DeletePlaybook(ctx context.Context, org int64, intent string) error {
	n, err := s.Q.IntegrationDeletePlaybook(ctx, db.IntegrationDeletePlaybookParams{OrganizationID: org, Intent: intent})
	if err == nil && n == 0 {
		return model.ErrNotFound
	}
	return err
}

// ---------------------------------------------------------------- calls

// connection loads the connection with its decrypted credentials.
func (s *Service) connection(ctx context.Context, org int64) (db.IntegrationConnection, Credentials, error) {
	c, err := s.Q.IntegrationConnection(ctx, org)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, Credentials{}, model.ErrNotConnected
	}
	if err != nil {
		return c, Credentials{}, err
	}
	auth := Credentials{Type: c.AuthType, Header: c.AuthHeader}
	if c.AuthEnc != "" && c.AuthType != model.AuthNone {
		if s.Box == nil {
			return c, auth, model.ErrNotConnected
		}
		secret, err := s.Box.Decrypt(c.AuthEnc)
		if err != nil {
			return c, auth, model.ErrNotConnected
		}
		auth.Secret = secret
	}
	return c, auth, nil
}

func (s *Service) log(ctx context.Context, org, actor int64, kind, target, channel, method string, resp Response, err error) {
	masked := resp.URL
	if len(masked) > 600 {
		masked = masked[:600]
	}
	// Logging is best effort; it must not turn a successful call into a failure.
	_ = s.Q.IntegrationLogCall(context.WithoutCancel(ctx), db.IntegrationLogCallParams{OrganizationID: org, Kind: kind, Target: clipText(target, 100), Channel: channel,
		Method: method, MaskedPath: masked, StatusCode: clampInt32(int64(resp.Status)), Ok: err == nil, ErrorCode: CodeOf(err), DurationMs: clampInt32(int64(resp.Duration / time.Millisecond)),
		ActorUserID: pgtype.Int8{Int64: actor, Valid: actor > 0}})
}

func message(err error) string {
	var ce *CallError
	if errors.As(err, &ce) {
		return ce.Message
	}
	if errors.Is(err, model.ErrNotConnected) {
		return "connected system is not configured"
	}
	return "call failed"
}

// TestSource is the "Dene" button: it calls the concept's endpoint for one
// external id and returns the real answer with pickable paths. Unsaved
// path / items edits can be tried.
func (s *Service) TestSource(ctx context.Context, org, actor int64, objectType uuid.UUID, in model.TestInput) (model.TestResult, error) {
	t, err := s.Q.IntegrationObjectTypeByUUID(ctx, db.IntegrationObjectTypeByUUIDParams{OrganizationID: org, Uuid: objectType})
	if err != nil {
		return model.TestResult{}, notFound(err)
	}
	path, items, key := "", "", model.KeyExternalID
	fields := []model.Field{}
	if src, err := s.Q.IntegrationSourceByKey(ctx, db.IntegrationSourceByKeyParams{OrganizationID: org, Key: t.Key}); err == nil {
		path, items, key = src.PathTemplate, src.ItemsPath, src.IdentityKey
		_ = json.Unmarshal(src.Fields, &fields)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return model.TestResult{}, err
	}
	if in.PathTemplate != nil {
		path = strings.TrimSpace(*in.PathTemplate)
		if slices.Contains(Placeholders(path), model.KeyCompanyExternalID) && !slices.Contains(Placeholders(path), model.KeyExternalID) {
			key = model.KeyCompanyExternalID
		}
	}
	if in.ItemsPath != nil {
		items = strings.TrimSpace(*in.ItemsPath)
	}
	if err := validSourceTemplate(path, key); err != nil {
		return model.TestResult{}, err
	}
	if !ValidPath(items) {
		return model.TestResult{}, invalid("items_path is invalid")
	}
	conn, auth, err := s.connection(ctx, org)
	if err != nil {
		return model.TestResult{OK: false, ErrorCode: CodeNotConnected, Message: message(err), Paths: []model.PathOption{}, Items: []map[string]any{}}, nil
	}
	id := model.Identity{ExternalID: strings.TrimSpace(in.ExternalID), CompanyExternalID: strings.TrimSpace(in.CompanyExternalID)}
	resp, callErr := s.Caller.Do(ctx, Request{Org: org, BaseURL: conn.BaseUrl, Path: path, Vars: vars(id), Method: http.MethodGet, Auth: auth, Timeout: time.Duration(conn.TimeoutMs) * time.Millisecond})
	s.log(ctx, org, actor, "test", t.Key, model.ChannelTest, http.MethodGet, resp, callErr)
	out := model.TestResult{OK: callErr == nil, StatusCode: resp.Status, ErrorCode: CodeOf(callErr), DurationMS: resp.Duration.Milliseconds(),
		Request: "GET " + resp.URL, Body: resp.Body, Paths: []model.PathOption{}, Items: []map[string]any{}}
	if callErr != nil {
		out.Message = message(callErr)
		return out, nil
	}
	recs := Items(resp.Body, items)
	if len(recs) > 0 {
		out.Paths = Paths(recs[0])
	}
	out.Items = Pick(recs, fields)
	// Arrays the items path may point at, as pseudo paths ("[] invoices").
	for _, a := range ArrayPaths(resp.Body) {
		out.Paths = append(out.Paths, model.PathOption{Path: a, Type: "array"})
	}
	return out, nil
}

// Lookup reads one concept for a customer. A missing source, connection
// or external id is a "skipped" result with the reason, not an error.
func (s *Service) Lookup(ctx context.Context, org, actor int64, concept string, id model.Identity, channel string) (model.Lookup, error) {
	out := model.Lookup{Concept: concept, Identity: id, Items: []map[string]any{}}
	src, err := s.Q.IntegrationSourceByKey(ctx, db.IntegrationSourceByKeyParams{OrganizationID: org, Key: concept})
	if errors.Is(err, pgx.ErrNoRows) {
		if _, typeErr := s.Q.IntegrationObjectTypeByKey(ctx, db.IntegrationObjectTypeByKeyParams{OrganizationID: org, Key: concept}); typeErr != nil {
			return out, model.ErrNotFound
		}
		out.Status, out.Reason = "skipped", "no source is set for this concept"
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Label = src.ObjectTypeLabel
	if !src.Enabled {
		out.Status, out.Reason = "skipped", "the source of this concept is turned off"
		return out, nil
	}
	if vars(id)[src.IdentityKey] == "" {
		out.Status, out.Reason = "skipped", CodeIdentityMissing+": the customer has no "+src.IdentityKey
		return out, nil
	}
	conn, auth, err := s.connection(ctx, org)
	if err != nil {
		out.Status, out.Reason = "skipped", message(err)
		return out, nil
	}
	resp, callErr := s.Caller.Do(ctx, Request{Org: org, BaseURL: conn.BaseUrl, Path: src.PathTemplate, Vars: vars(id), Method: http.MethodGet, Auth: auth, Timeout: time.Duration(conn.TimeoutMs) * time.Millisecond})
	s.log(ctx, org, actor, "source", concept, channel, http.MethodGet, resp, callErr)
	if callErr != nil {
		out.Status, out.Reason = "error", CodeOf(callErr)+": "+message(callErr)
		return out, nil
	}
	fields := []model.Field{}
	_ = json.Unmarshal(src.Fields, &fields)
	out.Items = Pick(Items(resp.Body, src.ItemsPath), fields)
	out.Status = "ok"
	return out, nil
}

// Action returns one action definition.
func (s *Service) Action(ctx context.Context, org int64, key string) (model.Action, error) {
	row, err := s.Q.IntegrationActionByKey(ctx, db.IntegrationActionByKeyParams{OrganizationID: org, Key: key})
	if err != nil {
		return model.Action{}, notFound(err)
	}
	return actionView(row), nil
}

func checkParams(params []model.Param, in map[string]any) (map[string]any, map[string]string, error) {
	body := map[string]any{}
	text := map[string]string{}
	for k := range in {
		if !slices.ContainsFunc(params, func(p model.Param) bool { return p.Key == k }) {
			return nil, nil, invalid("unknown param %q", k)
		}
	}
	for _, p := range params {
		v, ok := in[p.Key]
		if !ok || v == nil || v == "" {
			if p.Required {
				return nil, nil, invalid("param %q is required", p.Key)
			}
			continue
		}
		switch p.Type {
		case "number":
			f, ok := v.(float64)
			if !ok {
				return nil, nil, invalid("param %q must be a number", p.Key)
			}
			text[p.Key] = fmt.Sprint(f)
		case "boolean":
			b, ok := v.(bool)
			if !ok {
				return nil, nil, invalid("param %q must be true or false", p.Key)
			}
			text[p.Key] = fmt.Sprint(b)
		default:
			str, ok := v.(string)
			if !ok || len([]rune(str)) > 2000 {
				return nil, nil, invalid("param %q must be text (2000 characters)", p.Key)
			}
			text[p.Key] = str
		}
		body[p.Key] = v
	}
	return body, text, nil
}

// Run runs an action for a customer. The caller's channel must be in the
// action's allowed_for. An action that requires approval runs only for a
// member who approved it (assistant card, MCP client confirmation or the
// panel's confirm dialog); the AI agent and workflows cannot approve, so
// such actions are refused for them. A dry run never calls out.
func (s *Service) Run(ctx context.Context, org, actor int64, key, channel string, in model.RunInput) (model.RunResult, error) {
	a, err := s.Action(ctx, org, key)
	if err != nil {
		return model.RunResult{}, err
	}
	out := model.RunResult{Action: key, DryRun: in.DryRun}
	if !a.Enabled {
		return out, model.ErrDisabled
	}
	if !slices.Contains(a.AllowedFor, channel) {
		return out, fmt.Errorf("%w: %s may not run %s", model.ErrNotAllowed, channel, key)
	}
	if a.RequiresApproval && !in.DryRun && (channel != model.ChannelAgent || !in.Approved) {
		return out, model.ErrNeedsApproval
	}
	body, text, err := checkParams(a.Params, in.Params)
	if err != nil {
		return out, err
	}
	if in.ContactUUID == nil && in.ExternalID == "" && in.ConversationUUID != nil {
		contact, err := s.Q.IntegrationConversationContactByUUID(ctx, db.IntegrationConversationContactByUUIDParams{OrganizationID: org, Uuid: *in.ConversationUUID})
		if err != nil {
			return out, notFound(err)
		}
		in.ContactUUID = &contact
	}
	id, err := s.ResolveIdentity(ctx, org, in.ContactUUID, in.ExternalID)
	if err != nil {
		return out, err
	}
	values := vars(id)
	for k, v := range text {
		values[k] = v
		if slices.Contains(Placeholders(a.PathTemplate), k) {
			delete(body, k)
		}
	}
	if values[model.KeyExternalID] != "" {
		body[model.KeyExternalID] = values[model.KeyExternalID]
	}
	conn, auth, err := s.connection(ctx, org)
	if err != nil {
		return out, err
	}
	target, err := BuildURL(conn.BaseUrl, a.PathTemplate, values)
	if err != nil {
		return out, invalid("%s", message(err))
	}
	out.Request = a.Method + " " + MaskURL(target, auth.secrets()...)
	if in.DryRun {
		out.OK, out.Body = true, body
		out.Summary = fmt.Sprintf("%s would be sent to the connected system", a.Title)
		return out, nil
	}
	idem := uuid.NewString()
	resp, callErr := s.Caller.Do(ctx, Request{Org: org, BaseURL: conn.BaseUrl, Path: a.PathTemplate, Vars: values, Method: a.Method, Body: body, Auth: auth,
		Timeout: time.Duration(conn.TimeoutMs) * time.Millisecond, Idempote: idem})
	s.log(ctx, org, actor, "action", key, channel, a.Method, resp, callErr)
	out.OK, out.StatusCode, out.ErrorCode, out.Body = callErr == nil, resp.Status, CodeOf(callErr), resp.Body
	if callErr != nil {
		out.Summary = message(callErr)
	} else {
		out.Summary = a.Title + " done"
	}
	return out, nil
}

// clampInt32 narrows a counter for the call log without overflowing.
func clampInt32(v int64) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
}
