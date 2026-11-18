// Package handler exposes the inbox (/v1/tenant/conversations, /v1/tenant/tags).
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/access"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes of the inbox endpoints.
const (
	CodeNoChannel   = "CONVERSATION_NO_CHANNEL"
	CodeTagExists   = "TAG_EXISTS"
	maxBodyBytes    = 128 << 10
	activityResouce = "conversations"
)

// Handler serves the inbox.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New builds the handler.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// List returns one page of the inbox.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := who(w, r)
	if !ok {
		return
	}
	v := r.URL.Query()
	f := usecase.ListFilter{
		Statuses: apiquery.SplitCSV(v.Get("status")), Priorities: apiquery.SplitCSV(v.Get("priority")),
		ChannelTypes: apiquery.SplitCSV(v.Get("channel_type")), Assignee: strings.TrimSpace(v.Get("assignee")),
		Mentioned: v.Get("mentioned") == "1" || v.Get("mentioned") == "true",
		Unread:    v.Get("unread") == "1" || v.Get("unread") == "true",
		Q:         strings.TrimSpace(v.Get("q")), Sort: "last_message_at", Desc: true,
	}
	var err error
	if f.Inboxes, err = uuids(v.Get("inbox")); err != nil {
		invalid(w, r, "inbox must be a list of uuids")
		return
	}
	if f.Tags, err = uuids(v.Get("tag")); err != nil {
		invalid(w, r, "tag must be a list of uuids")
		return
	}
	if f.Contact, err = optUUID(v.Get("contact")); err != nil {
		invalid(w, r, "contact must be a uuid")
		return
	}
	if f.Company, err = optUUID(v.Get("company")); err != nil {
		invalid(w, r, "company must be a uuid")
		return
	}
	f.IdentityState = strings.TrimSpace(v.Get("identity"))
	f.ExternalThreadID = strings.TrimSpace(v.Get("external_thread_id"))
	if raw := v.Get("sla_breached"); raw != "" {
		b := raw == "1" || raw == "true"
		f.SLABreached = &b
	}
	if raw := strings.TrimSpace(v.Get("sort")); raw != "" {
		f.Desc = strings.HasPrefix(raw, "-")
		f.Sort = strings.TrimPrefix(raw, "-")
	}
	switch raw := strings.TrimSpace(v.Get("team")); raw {
	case "":
	case "mine":
		f.MyTeams = true
	default:
		id, err := uuid.Parse(raw)
		if err != nil {
			invalid(w, r, "team must be mine or a team uuid")
			return
		}
		f.Team = &id
	}
	if raw := v.Get("brand_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			invalid(w, r, "brand_id must be a brand uuid")
			return
		}
		f.BrandID = &id
	}
	limit, offset := apiquery.Clamp(atoi(v.Get("limit")), atoi(v.Get("offset")), 50, usecase.MaxListLimit)
	f.Limit, f.Offset = limit, offset
	items, total, err := h.svc.List(r.Context(), scope.ID, actor.ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, limit, offset))
}

// Counts returns the inbox badges of the caller.
func (h *Handler) Counts(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := who(w, r)
	if !ok {
		return
	}
	c, err := h.svc.Counts(r.Context(), scope.ID, actor.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, c)
}

// Agents lists assignable members.
func (h *Handler) Agents(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := who(w, r)
	if !ok {
		return
	}
	items, err := h.svc.Agents(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// Get returns one conversation.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope, actor, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	d, err := h.svc.Get(r.Context(), scope.ID, actor.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, d)
}

// Messages returns a page of messages (oldest first; before pages back).
func (h *Handler) Messages(w http.ResponseWriter, r *http.Request) {
	scope, _, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	before, err := optUUID(r.URL.Query().Get("before"))
	if err != nil {
		invalid(w, r, "before must be a message uuid")
		return
	}
	b := uuid.Nil
	if before != nil {
		b = *before
	}
	page, err := h.svc.Messages(r.Context(), scope.ID, id, b, atoi(r.URL.Query().Get("limit")))
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, page)
}

type postRequest struct {
	Content     string      `json:"content"`
	Private     bool        `json:"private"`
	ClientID    *uuid.UUID  `json:"client_id"`
	ReplyTo     *uuid.UUID  `json:"reply_to"`
	Mentions    []uuid.UUID `json:"mentions"`
	Attachments []uuid.UUID `json:"attachments"`
	Template    *struct {
		Name       string   `json:"name"`
		Language   string   `json:"language"`
		Parameters []string `json:"parameters"`
	} `json:"template"`
}

// Post sends a reply or adds a private note.
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	scope, actor, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	var req postRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Private {
		if !scope.Has(rbac.PermTenantConversationsNote) && !scope.Has(rbac.PermTenantConversationsReply) {
			response.Forbidden(w, r, "Missing permission: "+rbac.PermTenantConversationsNote)
			return
		}
	} else if !scope.Has(rbac.PermTenantConversationsReply) {
		response.Forbidden(w, r, "Missing permission: "+rbac.PermTenantConversationsReply)
		return
	}
	actor = h.svc.Named(r.Context(), actor)
	in := usecase.PostInput{
		OrganizationID: scope.ID, ConversationUUID: id, AuthorID: actor.ID, AuthorUUID: actor.UUID, AuthorName: actor.Name,
		Content: req.Content, Private: req.Private, Attachments: req.Attachments,
	}
	if req.ClientID != nil {
		in.ClientID = *req.ClientID
	}
	if req.ReplyTo != nil {
		in.ReplyToUUID = *req.ReplyTo
	}
	if req.Template != nil {
		in.Template = &channels.Template{Name: req.Template.Name, Language: req.Template.Language, Parameters: req.Template.Parameters}
	}
	ids, err := h.svc.AgentIDs(r.Context(), scope.ID, req.Mentions)
	if err != nil {
		writeError(w, r, err)
		return
	}
	in.MentionedUserIDs = ids
	posted, err := h.svc.Post(r.Context(), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := http.StatusCreated
	if posted.Duplicate {
		status = http.StatusOK
	}
	response.JSON(w, r, status, posted.Message)
}

// Retry queues a failed outbound message again.
func (h *Handler) Retry(w http.ResponseWriter, r *http.Request) {
	scope, _, _, ok := whoAndID(w, r)
	if !ok {
		return
	}
	msg, err := uuid.Parse(r.PathValue("message"))
	if err != nil {
		response.NotFound(w, r, "Message not found")
		return
	}
	if err := h.svc.Retry(r.Context(), scope.ID, msg); err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusAccepted, map[string]any{"queued": true})
}

// Read marks the conversation read up to its newest message.
func (h *Handler) Read(w http.ResponseWriter, r *http.Request) {
	scope, actor, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	if err := h.svc.MarkRead(r.Context(), scope.ID, actor.ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"read": true})
}

// Unread marks the conversation unread for the caller (CF-247).
func (h *Handler) Unread(w http.ResponseWriter, r *http.Request) {
	scope, actor, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	if err := h.svc.MarkUnread(r.Context(), scope.ID, actor, id); err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"unread": true})
}

// updateRequest uses raw JSON for assignee so null (unassign) and absent differ.
type updateRequest struct {
	Status   *string `json:"status"`
	Priority *string `json:"priority"`
	Subject  *string `json:"subject"`
	// Assignee, Team, CustomStatus: absent = unchanged, null = remove
	// (a value type so a JSON null is seen, unlike a pointer).
	Assignee     json.RawMessage `json:"assignee"`
	Team         json.RawMessage `json:"team"`
	CustomStatus json.RawMessage `json:"custom_status"`
	SnoozedUntil *time.Time      `json:"snoozed_until"`
}

// Update changes status, priority, subject or assignee.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	scope, actor, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	var req updateRequest
	if !decode(w, r, &req) {
		return
	}
	stateChange := req.Status != nil || req.Priority != nil || req.Subject != nil || req.SnoozedUntil != nil || len(req.CustomStatus) > 0
	if stateChange && !scope.Has(rbac.PermTenantConversationsManage) {
		response.Forbidden(w, r, "Missing permission: "+rbac.PermTenantConversationsManage)
		return
	}
	if (len(req.Assignee) > 0 || len(req.Team) > 0) && !scope.Has(rbac.PermTenantConversationsAssign) && !scope.Has(rbac.PermTenantConversationsManage) {
		response.Forbidden(w, r, "Missing permission: "+rbac.PermTenantConversationsAssign)
		return
	}
	actor = h.svc.Named(r.Context(), actor)
	in := usecase.UpdateInput{
		OrganizationID: scope.ID, ConversationUUID: id, Actor: actor,
		Status: req.Status, Priority: req.Priority, Subject: req.Subject, SnoozedUntil: req.SnoozedUntil,
	}
	if len(req.Assignee) > 0 {
		var target *uuid.UUID
		if string(req.Assignee) != "null" {
			var u uuid.UUID
			if err := json.Unmarshal(req.Assignee, &u); err != nil {
				invalid(w, r, "assignee must be a user uuid or null")
				return
			}
			target = &u
		}
		in.Assignee = &target
	}
	if len(req.Team) > 0 {
		var team *uuid.UUID
		if string(req.Team) != "null" {
			var u uuid.UUID
			if err := json.Unmarshal(req.Team, &u); err != nil {
				invalid(w, r, "team must be a team uuid or null")
				return
			}
			team = &u
		}
		in.Team = &team
	}
	if len(req.CustomStatus) > 0 {
		var st *uuid.UUID
		if string(req.CustomStatus) != "null" {
			var u uuid.UUID
			if err := json.Unmarshal(req.CustomStatus, &u); err != nil {
				invalid(w, r, "custom_status must be a custom status uuid or null")
				return
			}
			st = &u
		}
		in.CustomStatus = &st
	}
	if _, err := h.svc.Update(r.Context(), in); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, actor, "conversations.updated", id, map[string]any{
		"status": req.Status, "priority": req.Priority, "assignee_changed": len(req.Assignee) > 0, "custom_status_changed": len(req.CustomStatus) > 0,
	})
	d, err := h.svc.Get(r.Context(), scope.ID, actor.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, d)
}

type tagRequest struct {
	UUID  *uuid.UUID `json:"uuid"`
	Name  string     `json:"name"`
	Color string     `json:"color"`
}

// AddTag tags a conversation (by tag uuid, or by name creating the tag).
func (h *Handler) AddTag(w http.ResponseWriter, r *http.Request) {
	scope, actor, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	var req tagRequest
	if !decode(w, r, &req) {
		return
	}
	tagID := uuid.Nil
	if req.UUID != nil {
		tagID = *req.UUID
	}
	t, err := h.svc.TagConversation(r.Context(), scope.ID, id, h.svc.Named(r.Context(), actor), tagID, req.Name, true)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, t)
}

// RemoveTag removes a tag from a conversation.
func (h *Handler) RemoveTag(w http.ResponseWriter, r *http.Request) {
	scope, actor, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	tagID, err := uuid.Parse(r.PathValue("tag"))
	if err != nil {
		response.NotFound(w, r, "Tag not found")
		return
	}
	t, err := h.svc.TagConversation(r.Context(), scope.ID, id, h.svc.Named(r.Context(), actor), tagID, "", false)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, t)
}

// Tags lists the organization's tags.
func (h *Handler) Tags(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := who(w, r)
	if !ok {
		return
	}
	items, err := h.svc.Tags(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// CreateTag creates a tag.
func (h *Handler) CreateTag(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := who(w, r)
	if !ok {
		return
	}
	var req tagRequest
	if !decode(w, r, &req) {
		return
	}
	t, err := h.svc.CreateTag(r.Context(), scope.ID, req.Name, req.Color)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, t)
}

// UpdateTag renames or recolors a tag.
func (h *Handler) UpdateTag(w http.ResponseWriter, r *http.Request) {
	scope, _, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	var req tagRequest
	if !decode(w, r, &req) {
		return
	}
	t, err := h.svc.UpdateTag(r.Context(), scope.ID, id, req.Name, req.Color)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, t)
}

// DeleteTag deletes a tag.
func (h *Handler) DeleteTag(w http.ResponseWriter, r *http.Request) {
	scope, _, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteTag(r.Context(), scope.ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

func (h *Handler) record(r *http.Request, scope orgctx.Scope, actor usecase.Actor, action string, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	payload["organization_uuid"] = scope.UUID
	h.activity.Record(r.Context(), &actor.ID, action, activityResouce, &id, payload, r)
}

// WithViewer resolves the caller's team visibility (CF-44) once per request
// and stores it on the context; every inbox route runs behind it so the
// usecase filters lists, badges and single conversations by it.
func (h *Handler) WithViewer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scope, actor, ok := who(w, r)
		if !ok {
			return
		}
		v, err := h.svc.Viewer(r.Context(), scope, actor.ID)
		if err != nil {
			response.InternalErr(w, r, err, "")
			return
		}
		next.ServeHTTP(w, r.WithContext(access.WithViewer(r.Context(), v)))
	})
}

func who(w http.ResponseWriter, r *http.Request) (orgctx.Scope, usecase.Actor, bool) {
	scope, ok := orgctx.From(r.Context())
	if !ok {
		response.InternalErr(w, r, errors.New("conversations: missing organization scope"), "")
		return orgctx.Scope{}, usecase.Actor{}, false
	}
	p := authctx.MustPrincipal(r.Context())
	return scope, usecase.Actor{ID: p.UserInternal, UUID: p.UserID}, true
}

func whoAndID(w http.ResponseWriter, r *http.Request) (orgctx.Scope, usecase.Actor, uuid.UUID, bool) {
	scope, actor, ok := who(w, r)
	if !ok {
		return orgctx.Scope{}, usecase.Actor{}, uuid.Nil, false
	}
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Not found")
		return orgctx.Scope{}, usecase.Actor{}, uuid.Nil, false
	}
	return scope, actor, id, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid JSON body")
		return false
	}
	return true
}

func invalid(w http.ResponseWriter, r *http.Request, msg string) {
	response.ValidationError(w, r, []response.Detail{{Message: msg}})
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Not found")
	case errors.Is(err, usecase.ErrNoChannel):
		response.Conflict(w, r, CodeNoChannel, "This conversation has no channel to reply on")
	case errors.Is(err, usecase.ErrTagExists):
		response.Conflict(w, r, CodeTagExists, "A tag with this name already exists")
	case errors.Is(err, usecase.ErrStatusExists):
		response.Conflict(w, r, CodeStatusExists, "A status with this name already exists")
	case errors.Is(err, usecase.ErrInvalid):
		invalid(w, r, strings.TrimPrefix(err.Error(), usecase.ErrInvalid.Error()+": "))
	default:
		response.InternalErr(w, r, err, "")
	}
}

func uuids(raw string) ([]uuid.UUID, error) {
	out := []uuid.UUID{}
	for _, s := range apiquery.SplitCSV(raw) {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func optUUID(raw string) (*uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func atoi(s string) int32 {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 32)
	if err != nil {
		return 0
	}
	return int32(n)
}

// Presence records that the caller views the conversation (typing: they
// are composing) and returns everyone viewing it (CF-49). Clients call it
// every ~10 seconds while the conversation is open.
func (h *Handler) Presence(w http.ResponseWriter, r *http.Request) {
	scope, actor, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	var req struct {
		Typing bool `json:"typing"`
	}
	if !decode(w, r, &req) {
		return
	}
	viewers, err := h.svc.Heartbeat(r.Context(), scope.ID, id, actor, req.Typing)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"viewers": viewers})
}

// Leave removes the caller from the conversation's viewers.
func (h *Handler) Leave(w http.ResponseWriter, r *http.Request) {
	scope, actor, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	if err := h.svc.LeaveConversation(r.Context(), scope.ID, id, actor); err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"left": true})
}

// Views lists the caller's saved views.
func (h *Handler) Views(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := who(w, r)
	if !ok {
		return
	}
	items, err := h.svc.Views(r.Context(), scope.ID, actor.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// ViewCounts returns open conversation counts per saved view.
func (h *Handler) ViewCounts(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := who(w, r)
	if !ok {
		return
	}
	counts, err := h.svc.ViewCounts(r.Context(), scope.ID, actor.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"counts": counts})
}

type viewRequest struct {
	Scope    string              `json:"scope"`
	Name     string              `json:"name"`
	Icon     string              `json:"icon"`
	Filters  usecase.ViewFilters `json:"filters"`
	Position int32               `json:"position"`
}

func (v viewRequest) input() usecase.ViewInput {
	return usecase.ViewInput{Scope: v.Scope, Name: v.Name, Icon: v.Icon, Filters: v.Filters, Position: v.Position}
}

// CreateView saves a view (organization views need tenant.conversations.manage).
func (h *Handler) CreateView(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := who(w, r)
	if !ok {
		return
	}
	var req viewRequest
	if !decode(w, r, &req) {
		return
	}
	v, err := h.svc.CreateView(r.Context(), scope.ID, actor.ID, scope.Has(rbac.PermTenantConversationsManage), req.input())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, v)
}

// UpdateView edits a view.
func (h *Handler) UpdateView(w http.ResponseWriter, r *http.Request) {
	scope, actor, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	var req viewRequest
	if !decode(w, r, &req) {
		return
	}
	v, err := h.svc.UpdateView(r.Context(), scope.ID, actor.ID, scope.Has(rbac.PermTenantConversationsManage), id, req.input())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// DeleteView deletes a view.
func (h *Handler) DeleteView(w http.ResponseWriter, r *http.Request) {
	scope, actor, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteView(r.Context(), scope.ID, actor.ID, scope.Has(rbac.PermTenantConversationsManage), id); err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

// Reminders lists the pending reminders of a conversation.
func (h *Handler) Reminders(w http.ResponseWriter, r *http.Request) {
	scope, _, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	items, err := h.svc.Reminders(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// AddReminder schedules a follow-up for the caller.
func (h *Handler) AddReminder(w http.ResponseWriter, r *http.Request) {
	scope, actor, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	var req struct {
		DueAt     time.Time `json:"due_at"`
		Condition string    `json:"condition"`
		Note      string    `json:"note"`
	}
	if !decode(w, r, &req) {
		return
	}
	rem, err := h.svc.AddReminder(r.Context(), scope.ID, id, actor, req.DueAt, req.Condition, req.Note)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, rem)
}

// CancelReminder cancels a reminder.
func (h *Handler) CancelReminder(w http.ResponseWriter, r *http.Request) {
	scope, _, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	if err := h.svc.CancelReminder(r.Context(), scope.ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"cancelled": true})
}

func bulkPerms(scope orgctx.Scope) usecase.BulkPermissions {
	return usecase.BulkPermissions{
		Manage: scope.Has(rbac.PermTenantConversationsManage),
		Assign: scope.Has(rbac.PermTenantConversationsAssign),
	}
}

// Bulk applies one action to many conversations.
func (h *Handler) Bulk(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := who(w, r)
	if !ok {
		return
	}
	var req struct {
		Conversations []uuid.UUID `json:"conversations"`
		Action        string      `json:"action"`
		Value         string      `json:"value"`
	}
	if !decode(w, r, &req) {
		return
	}
	res, err := h.svc.Bulk(r.Context(), usecase.BulkInput{
		OrganizationID: scope.ID, Actor: actor, Perms: bulkPerms(scope), Conversations: req.Conversations,
		Action: req.Action, Value: req.Value,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, actor, "conversations.bulk", res.Operation, map[string]any{
		"action": req.Action, "applied": res.Applied, "failed": res.Failed,
	})
	response.JSON(w, r, http.StatusOK, res)
}

// UndoBulk reverts a bulk operation (24 hours).
func (h *Handler) UndoBulk(w http.ResponseWriter, r *http.Request) {
	scope, actor, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	res, err := h.svc.UndoBulk(r.Context(), scope.ID, actor, bulkPerms(scope), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, actor, "conversations.bulk_undone", res.Operation, map[string]any{"applied": res.Applied})
	response.JSON(w, r, http.StatusOK, res)
}
