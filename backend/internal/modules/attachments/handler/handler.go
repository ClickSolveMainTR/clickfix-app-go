// Package handler exposes organization attachments over HTTP.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

// Response error codes specific to attachments.
const (
	CodeFileTooLarge     = "FILE_TOO_LARGE"
	CodeQuotaExceeded    = "STORAGE_QUOTA_EXCEEDED"
	CodeUnsupportedType  = "UNSUPPORTED_FILE_TYPE"
	CodeFileRejected     = "FILE_REJECTED"
	CodeScanUnavailable  = "SCAN_UNAVAILABLE"
	multipartMemory      = 8 << 20
	multipartOverhead    = 1 << 20
	signedURLRequestSize = 4 << 10
)

// AttachmentSort lists the sortable columns of the listing.
var AttachmentSort = apiquery.SortColumns{"created_at": "created_at", "filename": "filename", "size_bytes": "size_bytes"}

// OwnerAccess decides whether a member may read files linked to an owner.
// The inbox supplies the team rule for message / conversation files
// (conversations/access.AttachmentGuard, docs/design/tenant-core.md §14).
type OwnerAccess interface {
	CanSeeOwner(ctx context.Context, scope orgctx.Scope, userID int64, ownerType string, ownerID uuid.UUID) (bool, error)
}

// Handler serves the attachment endpoints.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
	owners   OwnerAccess
}

// New builds the handler.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// WithOwnerAccess applies a visibility rule to owned files. Without one every
// member holding tenant.attachments.read reads every file of the organization.
func (h *Handler) WithOwnerAccess(a OwnerAccess) *Handler {
	h.owners = a
	return h
}

// canSee reports whether the caller may read a file linked to an owner.
func (h *Handler) canSee(r *http.Request, scope orgctx.Scope, ownerType *string, ownerID *uuid.UUID) (bool, error) {
	if h.owners == nil || ownerType == nil || ownerID == nil {
		return true, nil
	}
	p, ok := authctx.PrincipalFrom(r.Context())
	if !ok {
		return false, nil
	}
	return h.owners.CanSeeOwner(r.Context(), scope, p.UserInternal, *ownerType, *ownerID)
}

// visible loads an attachment and answers ErrNotFound when its owner is
// hidden from the caller (a hidden file looks like a missing one).
func (h *Handler) visible(r *http.Request, scope orgctx.Scope, id uuid.UUID) (model.Attachment, error) {
	att, err := h.svc.Get(r.Context(), scope.ID, id)
	if err != nil {
		return model.Attachment{}, err
	}
	ok, err := h.canSee(r, scope, att.OwnerType, att.OwnerID)
	if err != nil {
		return model.Attachment{}, err
	}
	if !ok {
		return model.Attachment{}, usecase.ErrNotFound
	}
	return att, nil
}

// Upload stores a multipart file ("file") with optional owner_type/owner_id.
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	scope, ok := orgctx.From(r.Context())
	if !ok {
		response.InternalErr(w, r, errors.New("attachments: missing organization scope"), "")
		return
	}
	limit, err := h.svc.MaxFileBytes(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit+multipartOverhead)
	if err := r.ParseMultipartForm(multipartMemory); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			response.Error(w, r, http.StatusRequestEntityTooLarge, CodeFileTooLarge, "File exceeds the size limit")
			return
		}
		response.BadRequest(w, r, response.CodeValidationError, "Invalid multipart form")
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()
	file, header, err := r.FormFile("file")
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "file is required")
		return
	}
	defer func() { _ = file.Close() }()

	in := model.UploadInput{
		OrganizationID:   scope.ID,
		OrganizationUUID: scope.UUID,
		UploaderID:       authctx.MustPrincipal(r.Context()).UserInternal,
		Filename:         header.Filename,
		DeclaredType:     header.Header.Get("Content-Type"),
		OwnerType:        strings.TrimSpace(r.FormValue("owner_type")),
		Body:             file,
	}
	if raw := strings.TrimSpace(r.FormValue("owner_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "owner_id is invalid")
			return
		}
		in.OwnerID = &id
	}
	att, err := h.svc.Upload(r.Context(), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "attachments.uploaded", att, map[string]any{
		"filename": att.Filename, "size_bytes": att.SizeBytes, "content_type": att.ContentType,
		"owner_type": att.OwnerType, "owner_id": att.OwnerID,
	})
	response.JSON(w, r, http.StatusCreated, att)
}

// List returns one page of attachments, optionally for one owner.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	if banned := apiquery.ForbiddenParams(r.URL.Query()); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{
			Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden",
		}})
		return
	}
	q := apiquery.Parse(r.URL.Query())
	if err := apiquery.ValidateSort(q.Sort, AttachmentSort); err != nil {
		writeError(w, r, err)
		return
	}
	filter := model.ListFilter{
		OrganizationID: scope.ID,
		OwnerType:      strings.TrimSpace(r.URL.Query().Get("owner_type")),
		Q:              q.Q,
		SortField:      "created_at",
		SortDesc:       true,
		Limit:          q.Limit,
		Offset:         q.Offset,
	}
	if len(q.Sort) > 0 {
		filter.SortField, filter.SortDesc = q.Sort[0].Field, q.Sort[0].Desc
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("owner_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "owner_id is invalid")
			return
		}
		filter.OwnerID = &id
		if filter.OwnerType != "" {
			ok, err := h.canSee(r, scope, &filter.OwnerType, &id)
			if err != nil {
				writeError(w, r, err)
				return
			}
			if !ok {
				response.JSON(w, r, http.StatusOK, apiquery.NewPage([]model.Attachment{}, 0, q.Limit, q.Offset))
				return
			}
		}
	}
	items, total, err := h.svc.List(r.Context(), filter)
	if err != nil {
		writeError(w, r, err)
		return
	}
	// Drop files of conversations the caller may not see. The total shrinks
	// by what this page dropped; the listing is a storage view, not the inbox.
	kept := items[:0]
	for _, it := range items {
		ok, err := h.canSee(r, scope, it.OwnerType, it.OwnerID)
		if err != nil {
			writeError(w, r, err)
			return
		}
		if ok {
			kept = append(kept, it)
		} else {
			total--
		}
	}
	items = kept
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// Get returns attachment metadata.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	att, err := h.visible(r, scope, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, att)
}

// Usage reports the organization's storage usage and limits.
func (h *Handler) Usage(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	u, err := h.svc.Usage(r.Context(), scope.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, u)
}

// Download streams the file (or ?variant=thumb) through the API.
func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	if _, err := h.visible(r, scope, id); err != nil {
		writeError(w, r, err)
		return
	}
	file, err := h.svc.Open(r.Context(), scope.ID, id, r.URL.Query().Get("variant"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeFile(w, file, r.URL.Query().Get("download") == "1", "private, no-store")
}

// SignedURL issues a short-lived public link.
func (h *Handler) SignedURL(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	var in struct {
		Variant    string `json:"variant"`
		TTLSeconds int    `json:"ttl_seconds"`
	}
	if r.ContentLength != 0 {
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, signedURLRequestSize))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&in); err != nil && !errors.Is(err, io.EOF) {
			response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
			return
		}
	}
	if _, err := h.visible(r, scope, id); err != nil {
		writeError(w, r, err)
		return
	}
	if in.TTLSeconds < 0 {
		response.BadRequest(w, r, response.CodeValidationError, "ttl_seconds must be positive")
		return
	}
	out, err := h.svc.SignedURL(r.Context(), scope.ID, id, in.Variant, time.Duration(in.TTLSeconds)*time.Second)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Delete removes an attachment.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	if _, err := h.visible(r, scope, id); err != nil {
		writeError(w, r, err)
		return
	}
	att, err := h.svc.Delete(r.Context(), scope.ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "attachments.deleted", att, map[string]any{
		"filename": att.Filename, "size_bytes": att.SizeBytes,
	})
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

// PublicDownload serves a signed URL. The token is the only credential, so
// every failure (forged, expired, deleted) answers the same 404.
func (h *Handler) PublicDownload(w http.ResponseWriter, r *http.Request) {
	file, err := h.svc.OpenSigned(r.Context(), r.PathValue("token"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeFile(w, file, r.URL.Query().Get("download") == "1", "private, max-age=60")
}

func (h *Handler) record(r *http.Request, scope orgctx.Scope, action string, att model.Attachment, payload map[string]any) {
	payload["organization_uuid"] = scope.UUID
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	h.activity.Record(r.Context(), &actor, action, "attachments", &att.UUID, payload, r)
}

func scopeOf(w http.ResponseWriter, r *http.Request) (orgctx.Scope, bool) {
	scope, ok := orgctx.From(r.Context())
	if !ok {
		// Route wiring bug: tenant routes always run RequireOrganization.
		response.InternalErr(w, r, errors.New("attachments: missing organization scope"), "")
		return orgctx.Scope{}, false
	}
	return scope, true
}

func scopeAndID(w http.ResponseWriter, r *http.Request) (orgctx.Scope, uuid.UUID, bool) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return orgctx.Scope{}, uuid.Nil, false
	}
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "attachment uuid is invalid")
		return orgctx.Scope{}, uuid.Nil, false
	}
	return scope, id, true
}

// writeFile streams a body. Types that can run script in a browser, and any
// type outside the inline-safe list, are always downloads; the sandbox CSP and
// nosniff cover everything else.
func writeFile(w http.ResponseWriter, f model.OpenFile, forceDownload bool, cacheControl string) {
	defer func() { _ = f.Body.Close() }()
	disposition := "inline"
	if forceDownload || !usecase.IsInlineSafe(f.ContentType) {
		disposition = "attachment"
	}
	h := w.Header()
	h.Set("Content-Type", f.ContentType)
	h.Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": f.Attachment.Filename}))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("Cache-Control", cacheControl)
	if f.Size >= 0 {
		h.Set("Content-Length", strconv.FormatInt(f.Size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f.Body)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	var ve *apiquery.ValidationError
	if errors.As(err, &ve) {
		details := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			details = append(details, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, details)
		return
	}
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		response.NotFound(w, r, "Resource was not found")
	case errors.Is(err, usecase.ErrInvalidRequest):
		response.BadRequest(w, r, response.CodeValidationError, trimPrefix(err))
	case errors.Is(err, usecase.ErrTooLarge):
		response.Error(w, r, http.StatusRequestEntityTooLarge, CodeFileTooLarge, trimPrefix(err))
	case errors.Is(err, usecase.ErrQuotaExceeded):
		response.Error(w, r, http.StatusRequestEntityTooLarge, CodeQuotaExceeded, "Organization storage quota exceeded")
	case errors.Is(err, usecase.ErrUnsupportedType):
		response.Error(w, r, http.StatusUnsupportedMediaType, CodeUnsupportedType, trimPrefix(err))
	case errors.Is(err, usecase.ErrInfected):
		response.Error(w, r, http.StatusUnprocessableEntity, CodeFileRejected, "File failed the security scan")
	case errors.Is(err, usecase.ErrScanUnavailable):
		response.ServiceUnavailable(w, r, CodeScanUnavailable, "File scanning is temporarily unavailable")
	default:
		response.InternalErr(w, r, err, "")
	}
}

// trimPrefix drops the "attachments: " package prefix from a client-facing
// error message.
func trimPrefix(err error) string {
	msg := err.Error()
	msg = strings.TrimPrefix(msg, "attachments: ")
	msg = strings.TrimPrefix(msg, "invalid request: ")
	return msg
}
