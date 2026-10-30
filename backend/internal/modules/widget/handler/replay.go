package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

func (h *Handler) ReplayPolicy(w http.ResponseWriter, r *http.Request) {
	wd, _, ok := h.visitor(w, r)
	if !ok {
		return
	}
	response.JSON(w, r, http.StatusOK, usecase.Policy(wd))
}
func (h *Handler) StartReplay(w http.ResponseWriter, r *http.Request) {
	wd, c, ok := h.visitor(w, r)
	if !ok || !h.allow(w, r, "replay_start", wd.Instance.UUID.String()+":"+c.Visitor, 5) {
		return
	}
	id, ok := conversationID(w, r)
	if !ok {
		return
	}
	var in struct {
		Consent bool `json:"consent"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.StartReplay(r.Context(), wd, c, id, in.Consent)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}
func sessionID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("session"))
	if err != nil {
		response.NotFound(w, r, "")
		return id, false
	}
	return id, true
}
func (h *Handler) visitorSession(w http.ResponseWriter, r *http.Request) (usecase.Widget, widget.Claims, db.WidgetSession, bool) {
	wd, c, ok := h.visitor(w, r)
	if !ok {
		return wd, c, db.WidgetSession{}, false
	}
	if !h.allow(w, r, "session", wd.Instance.UUID.String()+":"+c.Visitor, 120) {
		return wd, c, db.WidgetSession{}, false
	}
	conv, ok := conversationID(w, r)
	if !ok {
		return wd, c, db.WidgetSession{}, false
	}
	id, ok := sessionID(w, r)
	if !ok {
		return wd, c, db.WidgetSession{}, false
	}
	row, err := h.svc.VisitorSession(r.Context(), wd, c, conv, id)
	if err != nil {
		h.fail(w, r, err)
		return wd, c, row, false
	}
	return wd, c, row, true
}
func (h *Handler) SessionConsent(w http.ResponseWriter, r *http.Request) {
	wd, _, row, ok := h.visitorSession(w, r)
	if !ok {
		return
	}
	var in struct {
		Action string `json:"action"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.ChangeSession(r.Context(), wd, row, in.Action)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
func (h *Handler) ReplayChunk(w http.ResponseWriter, r *http.Request) {
	wd, _, row, ok := h.visitorSession(w, r)
	if !ok {
		return
	}
	var in struct {
		Sequence int    `json:"sequence"`
		Data     []byte `json:"data"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2*usecase.MaxReplayChunk)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		h.fail(w, r, usecase.ErrInvalid)
		return
	}
	if err := h.svc.UploadReplay(r.Context(), wd, row, in.Sequence, in.Data); err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, map[string]bool{"accepted": true})
}
func (h *Handler) VisitorLiveToken(w http.ResponseWriter, r *http.Request) {
	_, _, row, ok := h.visitorSession(w, r)
	if !ok {
		return
	}
	out, err := h.svc.LiveTokens(row)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
func (h *Handler) agentSession(w http.ResponseWriter, r *http.Request) (usecase.Widget, db.WidgetSession, bool) {
	if !h.allow(w, r, "agent_session", orgctx.Must(r.Context()).UUID.String(), 600) {
		return usecase.Widget{}, db.WidgetSession{}, false
	}
	conv, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		h.fail(w, r, usecase.ErrNotFound)
		return usecase.Widget{}, db.WidgetSession{}, false
	}
	id, ok := sessionID(w, r)
	if !ok {
		return usecase.Widget{}, db.WidgetSession{}, false
	}
	wd, row, err := h.svc.AgentSession(r.Context(), conv, id)
	if err != nil {
		h.fail(w, r, err)
		return wd, row, false
	}
	return wd, row, true
}
func (h *Handler) AgentSessions(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		h.fail(w, r, usecase.ErrNotFound)
		return
	}
	out, err := h.svc.AgentSessions(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": out})
}
func (h *Handler) RequestCobrowse(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r, "cobrowse_request", orgctx.Must(r.Context()).UUID.String(), 20) {
		return
	}
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		h.fail(w, r, usecase.ErrNotFound)
		return
	}
	var in struct {
		Control bool `json:"control_enabled"`
	}
	if !decode(w, r, &in) {
		return
	}
	out, err := h.svc.RequestCobrowse(r.Context(), id, in.Control)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}
func (h *Handler) ReplayManifest(w http.ResponseWriter, r *http.Request) {
	_, row, ok := h.agentSession(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Manifest(r.Context(), row)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response.JSON(w, r, http.StatusOK, out)
}
func (h *Handler) AgentLiveToken(w http.ResponseWriter, r *http.Request) {
	_, row, ok := h.agentSession(w, r)
	if !ok {
		return
	}
	out, err := h.svc.LiveTokens(row)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
func (h *Handler) AgentPointer(w http.ResponseWriter, r *http.Request) {
	wd, row, ok := h.agentSession(w, r)
	if !ok {
		return
	}
	var in usecase.Pointer
	if !decode(w, r, &in) {
		return
	}
	if err := h.svc.AgentPointer(r.Context(), wd, row, in); err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]bool{"accepted": true})
}
func (h *Handler) AgentStop(w http.ResponseWriter, r *http.Request) {
	wd, row, ok := h.agentSession(w, r)
	if !ok {
		return
	}
	out, err := h.svc.ChangeSession(r.Context(), wd, row, "stop")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
func (h *Handler) ReplayDownload(w http.ResponseWriter, r *http.Request) {
	body, _, err := h.svc.ReplayDownload(r.Context(), r.PathValue("token"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	defer func() { _ = body.Close() }()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", `attachment; filename="replay.json.gz"`)
	_, _ = io.Copy(w, body)
}
