package handler

import (
	"encoding/json"
	"errors"
	"mime"
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/diagnostics"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
)

// Capture accepts a JSON technical_context or a multipart capture and context.
func (h *Handler) Capture(w http.ResponseWriter, r *http.Request) {
	wd, claims, ok := h.visitor(w, r)
	if !ok || !h.allow(w, r, "diagnostic", wd.Instance.UUID.String()+":"+claims.Visitor, 10) {
		return
	}
	id, ok := conversationID(w, r)
	if !ok {
		return
	}
	var in usecase.DiagnosticInput
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid content type")
		return
	}
	if contentType == "application/json" {
		var body struct {
			Context diagnostics.Context `json:"context"`
			Consent bool                `json:"consent"`
		}
		if !decode(w, r, &body) {
			return
		}
		in.Kind = "technical_context"
		in.Context = body.Context
		in.Consent = body.Consent
	} else {
		if contentType != "multipart/form-data" {
			response.BadRequest(w, r, response.CodeValidationError, "Expected JSON or multipart capture")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, usecase.MaxRecordingBytes+multipartOverheadBytes)
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				h.fail(w, r, usecase.ErrFileTooLarge)
				return
			}
			response.BadRequest(w, r, response.CodeValidationError, "Invalid capture upload")
			return
		}
		if r.MultipartForm != nil {
			defer func() { _ = r.MultipartForm.RemoveAll() }()
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			response.BadRequest(w, r, response.CodeValidationError, "file is required")
			return
		}
		defer func() { _ = file.Close() }()
		in.Kind = r.FormValue("kind")
		in.Body = file
		in.Filename = header.Filename
		in.Consent = r.FormValue("consent") == "true"
		in.PIIMasked = r.FormValue("pii_masked") == "true"
		if raw := r.FormValue("context"); raw != "" {
			if len(raw) > maxJSONBytes || json.Unmarshal([]byte(raw), &in.Context) != nil {
				response.BadRequest(w, r, response.CodeValidationError, "Invalid technical context")
				return
			}
		}
	}
	out, err := h.svc.Capture(r.Context(), wd, claims, id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, out)
}
