package handler

// CF-98: merge, split, links and side conversations.

import (
	"errors"
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes of the CF-98 endpoints.
const (
	CodeLinkConflict = "CONVERSATION_LINK_CONFLICT"
	CodeNoEmailInbox = "NO_EMAIL_INBOX"
)

type mergeRequest struct {
	SourceUUID uuid.UUID `json:"source_uuid"`
}

type splitRequest struct {
	MessageUUID uuid.UUID `json:"message_uuid"`
	Following   bool      `json:"following"`
	Subject     string    `json:"subject"`
}

type linkRequest struct {
	ConversationUUID uuid.UUID `json:"conversation_uuid"`
	Relation         string    `json:"relation"`
}

type sideStartRequest struct {
	To        []string   `json:"to"`
	Subject   string     `json:"subject"`
	Body      string     `json:"body"`
	InboxUUID *uuid.UUID `json:"inbox_uuid"`
}

type sideReplyRequest struct {
	Body string `json:"body"`
}

type sideUpdateRequest struct {
	Status string `json:"status"`
}

// Merge folds the conversation named in the body into the one in the path.
func (h *Handler) Merge(w http.ResponseWriter, r *http.Request) {
	scope, actor, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	var req mergeRequest
	if !decode(w, r, &req) {
		return
	}
	if req.SourceUUID == uuid.Nil {
		invalid(w, r, "source_uuid is required")
		return
	}
	res, err := h.svc.Merge(r.Context(), scope.ID, actor, id, req.SourceUUID)
	if err != nil {
		writeLinkError(w, r, err)
		return
	}
	h.record(r, scope, actor, "conversations.merged", id, map[string]any{
		"source_uuid": req.SourceUUID, "messages": res.Messages,
	})
	h.detail(w, r, scope.ID, actor.ID, res.Target.Uuid, http.StatusOK)
}

// Split moves a message (and optionally the later ones) into a new child
// conversation.
func (h *Handler) Split(w http.ResponseWriter, r *http.Request) {
	scope, actor, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	var req splitRequest
	if !decode(w, r, &req) {
		return
	}
	if req.MessageUUID == uuid.Nil {
		invalid(w, r, "message_uuid is required")
		return
	}
	child, err := h.svc.Split(r.Context(), usecase.SplitInput{
		OrganizationID: scope.ID, Actor: actor, ConversationUUID: id, MessageUUID: req.MessageUUID,
		Following: req.Following, Subject: req.Subject,
	})
	if err != nil {
		writeLinkError(w, r, err)
		return
	}
	h.record(r, scope, actor, "conversations.split", id, map[string]any{
		"child_uuid": child.Uuid, "message_uuid": req.MessageUUID, "following": req.Following,
	})
	h.detail(w, r, scope.ID, actor.ID, child.Uuid, http.StatusCreated)
}

// Links lists the conversations linked to one.
func (h *Handler) Links(w http.ResponseWriter, r *http.Request) {
	scope, _, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Links(r.Context(), scope.ID, id)
	if err != nil {
		writeLinkError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Link adds a parent, child or related link.
func (h *Handler) Link(w http.ResponseWriter, r *http.Request) {
	scope, actor, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	var req linkRequest
	if !decode(w, r, &req) {
		return
	}
	if req.ConversationUUID == uuid.Nil {
		invalid(w, r, "conversation_uuid is required")
		return
	}
	if err := h.svc.Link(r.Context(), usecase.LinkInput{
		OrganizationID: scope.ID, Actor: actor, ConversationUUID: id, OtherUUID: req.ConversationUUID, Relation: req.Relation,
	}); err != nil {
		writeLinkError(w, r, err)
		return
	}
	h.record(r, scope, actor, "conversations.linked", id, map[string]any{"other_uuid": req.ConversationUUID, "relation": req.Relation})
	out, err := h.svc.Links(r.Context(), scope.ID, id)
	if err != nil {
		writeLinkError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Unlink removes a link.
func (h *Handler) Unlink(w http.ResponseWriter, r *http.Request) {
	scope, actor, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	other, err := uuid.Parse(r.PathValue("other"))
	if err != nil {
		response.NotFound(w, r, "Not found")
		return
	}
	if err := h.svc.Unlink(r.Context(), scope.ID, actor, id, other); err != nil {
		writeLinkError(w, r, err)
		return
	}
	h.record(r, scope, actor, "conversations.unlinked", id, map[string]any{"other_uuid": other})
	out, err := h.svc.Links(r.Context(), scope.ID, id)
	if err != nil {
		writeLinkError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// SideConversations lists the side conversations of a conversation.
func (h *Handler) SideConversations(w http.ResponseWriter, r *http.Request) {
	scope, _, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.ListSide(r.Context(), scope.ID, id)
	if err != nil {
		writeLinkError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// StartSideConversation opens a side conversation and sends its first mail.
func (h *Handler) StartSideConversation(w http.ResponseWriter, r *http.Request) {
	scope, actor, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	var req sideStartRequest
	if !decode(w, r, &req) {
		return
	}
	out, err := h.svc.StartSide(r.Context(), usecase.StartSideInput{
		OrganizationID: scope.ID, Actor: actor, ConversationUUID: id, InboxUUID: req.InboxUUID,
		To: req.To, Subject: req.Subject, Body: req.Body,
	})
	if err != nil {
		writeLinkError(w, r, err)
		return
	}
	h.record(r, scope, actor, "conversations.side_started", id, map[string]any{"side_conversation_uuid": out.UUID, "to": out.Participants})
	response.JSON(w, r, http.StatusCreated, out)
}

// SideConversation returns one side conversation with its messages.
func (h *Handler) SideConversation(w http.ResponseWriter, r *http.Request) {
	scope, _, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.GetSide(r.Context(), scope.ID, id)
	if err != nil {
		writeLinkError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ReplySideConversation sends another mail on a side conversation.
func (h *Handler) ReplySideConversation(w http.ResponseWriter, r *http.Request) {
	scope, actor, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	var req sideReplyRequest
	if !decode(w, r, &req) {
		return
	}
	out, err := h.svc.ReplySide(r.Context(), scope.ID, actor, id, req.Body)
	if err != nil {
		writeLinkError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}

// UpdateSideConversation opens or closes a side conversation.
func (h *Handler) UpdateSideConversation(w http.ResponseWriter, r *http.Request) {
	scope, _, id, ok := whoAndID(w, r)
	if !ok {
		return
	}
	var req sideUpdateRequest
	if !decode(w, r, &req) {
		return
	}
	out, err := h.svc.SetSideStatus(r.Context(), scope.ID, id, req.Status)
	if err != nil {
		writeLinkError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) detail(w http.ResponseWriter, r *http.Request, orgID, viewerID int64, id uuid.UUID, status int) {
	d, err := h.svc.Get(r.Context(), orgID, viewerID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, status, d)
}

func writeLinkError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, usecase.ErrLinkConflict):
		response.Conflict(w, r, CodeLinkConflict, "The conversation already has a parent, or the link would create a cycle")
	case errors.Is(err, usecase.ErrNoEmailInbox):
		response.Conflict(w, r, CodeNoEmailInbox, "Connect an e-mail inbox to send side conversations")
	default:
		writeError(w, r, err)
	}
}
