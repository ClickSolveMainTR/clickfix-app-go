package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/repository"
	core "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/usecase"
	billingrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/access"
	conv "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type toolAuth struct {
	Bearer string `json:"bearer"`
	HMAC   string `json:"hmac"`
}
type ToolInput struct {
	Name             string         `json:"name"`
	Description      string         `json:"description"`
	Schema           map[string]any `json:"schema"`
	URL              string         `json:"url"`
	Bearer           string         `json:"bearer"`
	HMAC             string         `json:"hmac"`
	KeepAuth         bool           `json:"keep_auth"`
	RequiresApproval bool           `json:"requires_approval"`
	Enabled          bool           `json:"enabled"`
	DailyLimit       int32          `json:"daily_limit"`
}
type Invocation struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Key       uuid.UUID       `json:"invocation_key"`
	DryRun    bool            `json:"dry_run"`
	Approve   bool            `json:"approve"`
}
type ToolService struct {
	Repo          *repository.Store
	Core          *core.Service
	Conversations *conv.Service
	Box           *crypto.SecretBox
	Issues        IssueGateway
	Status        StatusGateway
}

var toolName = regexp.MustCompile(`^[a-z][a-z0-9_]{2,49}$`)

func builtins() []ai.Tool {
	return []ai.Tool{
		{Name: "lookup_contact", Description: "Read this conversation's contact", Schema: objectSchema(map[string]any{})},
		{Name: "conversation_history", Description: "Read public conversation history", Schema: objectSchema(map[string]any{})},
		{Name: "knowledge_search", Description: "Search public organization knowledge", Schema: objectSchema(map[string]any{"query": textSchema(1000)})},
		{Name: "create_note", Description: "Create an internal note after human approval", RequiresApproval: true, Schema: objectSchema(map[string]any{"text": textSchema(4000)})},
		{Name: "assign_conversation", Description: "Assign to an organization member after human approval", RequiresApproval: true, Schema: objectSchema(map[string]any{"assignee_uuid": map[string]any{"type": "string", "format": "uuid"}})},
		{Name: "tag_conversation", Description: "Add an existing organization tag after human approval", RequiresApproval: true, Schema: objectSchema(map[string]any{"tag_uuid": map[string]any{"type": "string", "format": "uuid"}})},
		{Name: "set_status", Description: "Change conversation status after human approval", RequiresApproval: true, Schema: objectSchema(map[string]any{"status": enumSchema("open", "pending", "resolved", "closed")})},
	}
}
func (s *ToolService) Offered(ctx context.Context, org int64) ([]ai.Tool, error) {
	out := builtins()
	if s.Issues != nil {
		out = append(out, issueTools()...)
	}
	if s.Status != nil {
		out = append(out, statusTool())
	}
	rows, err := s.Repo.Q.AITools(ctx, db.AIToolsParams{OrganizationID: org, Limit: 100})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.Enabled {
			var schema map[string]any
			if err = json.Unmarshal(row.Schema, &schema); err != nil {
				return nil, err
			}
			out = append(out, ai.Tool{Name: row.Name, Description: row.Description, Schema: schema, RequiresApproval: row.RequiresApproval})
		}
	}
	return out, nil
}
func (s *ToolService) Save(ctx context.Context, org int64, in ToolInput) (uuid.UUID, error) {
	if !toolName.MatchString(in.Name) || len(in.Description) > 1000 || len(in.Bearer) > 4000 || len(in.HMAC) > 4000 || in.DailyLimit < 1 || in.DailyLimit > 1000 {
		return uuid.Nil, model.ErrInvalid
	}
	for _, b := range append(append(builtins(), issueTools()...), statusTool()) {
		if b.Name == in.Name {
			return uuid.Nil, model.ErrInvalid
		}
	}
	if _, err := toolURL(in.URL); err != nil {
		return uuid.Nil, err
	}
	if s.Box == nil {
		return uuid.Nil, model.ErrInvalid
	}
	schema, err := json.Marshal(in.Schema)
	if err != nil || len(schema) > 16000 || in.Schema["type"] != "object" {
		return uuid.Nil, model.ErrInvalid
	}
	// Schema compilation cannot fetch remote references. Compilation is independent
	// of required fields, unlike validating a dummy instance against the schema.
	if err = ai.ValidateSchema(in.Schema); err != nil {
		return uuid.Nil, model.ErrInvalid
	}
	auth, err := json.Marshal(toolAuth{Bearer: in.Bearer, HMAC: in.HMAC})
	if err != nil {
		return uuid.Nil, err
	}
	encrypted := ""
	if in.Bearer != "" || in.HMAC != "" {
		encrypted, err = s.Box.Encrypt(string(auth))
	}
	if err != nil {
		return uuid.Nil, err
	}
	return s.Repo.Q.AIToolSave(ctx, db.AIToolSaveParams{OrganizationID: org, Name: in.Name, Description: in.Description, Schema: schema, Url: in.URL, AuthEnc: encrypted, KeepAuth: in.KeepAuth, RequiresApproval: in.RequiresApproval, Enabled: in.Enabled, DailyLimit: in.DailyLimit})
}

func (s *ToolService) Invoke(ctx context.Context, org, actor int64, conversation uuid.UUID, in Invocation) (string, error) {
	if in.Key == uuid.Nil || len(in.Arguments) > 16000 {
		return "", model.ErrInvalid
	}
	c, err := s.Repo.Q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: org, Uuid: conversation})
	if err != nil {
		return "", err
	}
	if c.Status == "closed" || c.Status == "resolved" {
		return "", model.ErrConflict
	}
	if err = s.Repo.CheckConversation(ctx, org, c.ID); err != nil {
		return "", err
	}
	if scope, ok := orgctx.From(ctx); ok {
		viewer, e := access.For(ctx, s.Repo.Q, scope, actor)
		if e != nil {
			return "", e
		}
		ctx = access.WithViewer(ctx, viewer)
	}
	var definition ai.Tool
	var remote db.AiTool
	found := false
	available := builtins()
	if s.Issues != nil {
		available = append(available, issueTools()...)
	}
	if s.Status != nil {
		available = append(available, statusTool())
	}
	for _, b := range available {
		if b.Name == in.Name {
			definition = b
			found = true
		}
	}
	if !found {
		remote, err = s.Repo.Q.AIToolByName(ctx, db.AIToolByNameParams{OrganizationID: org, Name: in.Name})
		if err != nil || !remote.Enabled {
			return "", model.ErrInvalid
		}
		if err = json.Unmarshal(remote.Schema, &definition.Schema); err != nil {
			return "", err
		}
		definition.Name = remote.Name
		definition.RequiresApproval = remote.RequiresApproval
	}
	if err = ai.ValidateJSON(definition.Schema, in.Arguments); err != nil {
		return "", model.ErrInvalid
	}
	if scope, ok := orgctx.From(ctx); ok {
		required := map[string]string{"create_note": rbac.PermTenantConversationsNote, "assign_conversation": rbac.PermTenantConversationsAssign, "tag_conversation": rbac.PermTenantConversationsManage, "set_status": rbac.PermTenantConversationsManage, "create_issue": rbac.PermTenantConversationsManage, "link_issue": rbac.PermTenantConversationsManage}
		if remote.ID > 0 && !scope.Has(rbac.PermTenantConversationsManage) {
			return "", model.ErrConflict
		}
		if permission := required[in.Name]; permission != "" && !scope.Has(permission) {
			return "", model.ErrConflict
		}
	}
	if definition.RequiresApproval && !in.DryRun && (actor <= 0 || !in.Approve) {
		return "", model.ErrConflict
	}
	limit := int64(100)
	if remote.ID > 0 {
		limit = int64(remote.DailyLimit)
	}
	var invocation db.AiToolCall
	err = s.Repo.Within(ctx, func(r *billingrepo.Repository) error {
		if _, err := r.Q.BillingLockOrganizationID(ctx, org); err != nil {
			return err
		}
		calls, err := r.Q.AIToolDailyCalls(ctx, db.AIToolDailyCallsParams{OrganizationID: org, ToolName: in.Name})
		if err != nil {
			return err
		}
		if calls >= limit {
			return model.ErrConflict
		}
		invocation, err = r.Q.AIToolStart(ctx, db.AIToolStartParams{OrganizationID: org, ConversationID: c.ID, ToolName: in.Name, InvocationKey: in.Key, ActorID: pgtype.Int8{Int64: actor, Valid: actor > 0}, DryRun: in.DryRun})
		if errors.Is(err, pgx.ErrNoRows) {
			return model.ErrConflict
		}
		return err
	})
	if err != nil {
		return "", err
	}
	status, code := "completed", ""
	result := "Schema and authorization validated; no action executed"
	if in.DryRun {
		status = "dry_run"
	} else {
		result, err = s.execute(ctx, org, actor, c, remote, in)
		if err != nil {
			status, code = "failed", "TOOL_EXECUTION_FAILED"
		}
	}
	if !in.DryRun && err == nil && (len(result) > 4096 || ai.Guard(result) != nil) {
		status, code = "failed", "TOOL_UNSAFE_OUTPUT"
		result = ""
		err = ai.ErrInvalidOutput
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if finishErr := s.Repo.Q.AIToolFinish(finishCtx, db.AIToolFinishParams{OrganizationID: org, Uuid: invocation.Uuid, Status: status, ErrorCode: code}); finishErr != nil {
		return "", finishErr
	}
	if err != nil {
		return "", err
	}
	if ai.Guard(result) != nil {
		return "", ai.ErrInjection
	}
	return ai.NewMasker().Mask(result), nil
}
func (s *ToolService) execute(ctx context.Context, org, actor int64, c db.Conversation, remote db.AiTool, in Invocation) (string, error) {
	var args map[string]any
	if err := json.Unmarshal(in.Arguments, &args); err != nil {
		return "", model.ErrInvalid
	}
	text := func(key string) string { v, _ := args[key].(string); return v }
	var output any
	switch in.Name {
	case "lookup_contact":
		contact, err := s.Repo.Q.GetContactByID(ctx, db.GetContactByIDParams{OrganizationID: org, ID: c.ContactID})
		if err != nil {
			return "", err
		}
		output = map[string]any{"uuid": contact.Uuid, "name": contact.Name, "email": contact.Email, "phone": contact.Phone}
	case "conversation_history":
		rows, err := s.Repo.Q.AIAgentMessages(ctx, db.AIAgentMessagesParams{OrganizationID: org, ConversationID: c.ID})
		if err != nil {
			return "", err
		}
		output = rows
	case "knowledge_search":
		if s.Core == nil || s.Core.RAG == nil {
			return "", ai.ErrUnavailable
		}
		citations, err := s.Core.RAG.Search(ctx, org, text("query"), "tr", true)
		if err != nil {
			return "", err
		}
		output = citations
	case "create_note":
		if s.Conversations == nil || actor <= 0 {
			return "", model.ErrInvalid
		}
		posted, err := s.Conversations.Post(ctx, conv.PostInput{OrganizationID: org, ConversationUUID: c.Uuid, AuthorID: actor, Content: text("text"), Private: true, ClientID: in.Key})
		if err != nil {
			return "", err
		}
		output = map[string]any{"message_uuid": posted.Message.UUID}
	case "assign_conversation":
		if s.Conversations == nil || actor <= 0 {
			return "", model.ErrInvalid
		}
		id, err := uuid.Parse(text("assignee_uuid"))
		if err != nil {
			return "", model.ErrInvalid
		}
		target := &id
		row, err := s.Conversations.Update(ctx, conv.UpdateInput{OrganizationID: org, ConversationUUID: c.Uuid, Actor: conv.Actor{ID: actor}, Assignee: &target})
		if err != nil {
			return "", err
		}
		output = map[string]any{"conversation_uuid": row.Uuid, "assignee_uuid": id}
	case "tag_conversation":
		if s.Conversations == nil || actor <= 0 {
			return "", model.ErrInvalid
		}
		id, err := uuid.Parse(text("tag_uuid"))
		if err != nil {
			return "", model.ErrInvalid
		}
		tag, err := s.Conversations.TagConversation(ctx, org, c.Uuid, conv.Actor{ID: actor}, id, "", true)
		if err != nil {
			return "", err
		}
		output = tag
	case "create_issue":
		if s.Issues == nil || actor <= 0 {
			return "", model.ErrInvalid
		}
		id, err := s.Issues.Create(ctx, org, actor, c.Uuid, text("title"), text("body"))
		if err != nil {
			return "", err
		}
		output = map[string]any{"issue_uuid": id}
	case "link_issue":
		if s.Issues == nil || actor <= 0 {
			return "", model.ErrInvalid
		}
		id, err := uuid.Parse(text("issue_uuid"))
		if err != nil {
			return "", model.ErrInvalid
		}
		if err = s.Issues.Link(ctx, org, actor, c.Uuid, id); err != nil {
			return "", err
		}
		output = map[string]any{"issue_uuid": id, "conversation_uuid": c.Uuid}
	case "status_page_query":
		if s.Status == nil {
			return "", model.ErrInvalid
		}
		return s.Status.Query(ctx, org)
	case "set_status":
		if s.Conversations == nil || actor <= 0 {
			return "", model.ErrInvalid
		}
		status := text("status")
		row, err := s.Conversations.Update(ctx, conv.UpdateInput{OrganizationID: org, ConversationUUID: c.Uuid, Actor: conv.Actor{ID: actor}, Status: &status})
		if err != nil {
			return "", err
		}
		output = map[string]any{"status": row.Status}
	default:
		if s.Box == nil {
			return "", model.ErrInvalid
		}
		decrypted, err := s.Box.Decrypt(remote.AuthEnc)
		if err != nil {
			return "", err
		}
		var auth toolAuth
		if decrypted != "" {
			if err = json.Unmarshal([]byte(decrypted), &auth); err != nil {
				return "", err
			}
		}
		return invokeHTTP(ctx, remote.Url, auth, in.Key, map[string]any{"schema_version": 1, "event": "ai.custom_tool.invoke", "organization_id": org, "conversation_uuid": c.Uuid, "invocation_id": in.Key, "tool": in.Name, "arguments": args})
	}
	raw, err := json.Marshal(output)
	if err != nil {
		return "", err
	}
	if len(raw) > 4096 {
		return "", model.ErrInvalid
	}
	return strings.TrimSpace(string(raw)), nil
}

func issueTools() []ai.Tool {
	return []ai.Tool{
		{Name: "create_issue", Description: "Create and link an engineering issue after human approval", RequiresApproval: true, Schema: objectSchema(map[string]any{"title": textSchema(200), "body": textSchema(4000)})},
		{Name: "link_issue", Description: "Link an organization issue after human approval", RequiresApproval: true, Schema: objectSchema(map[string]any{"issue_uuid": map[string]any{"type": "string", "format": "uuid"}})},
	}
}
func statusTool() ai.Tool {
	return ai.Tool{Name: "status_page_query", Description: "Read organization service status", Schema: objectSchema(map[string]any{})}
}
