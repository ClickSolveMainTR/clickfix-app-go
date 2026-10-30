package handler

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/resourcemeta"
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

func (h *Handler) BankStatementLines(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	query, ok := paymentQuery(w, r)
	if !ok {
		return
	}
	rows, total, err := h.svc.BankStatementLines(r.Context(), org, r.URL.Query().Get("status"), query.Limit, query.Offset)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": rows, "total": total, "limit": query.Limit, "offset": query.Offset})
}

func (h *Handler) ImportBankStatement(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	bank, err := uuid.Parse(r.URL.Query().Get("bank_account_uuid"))
	if err != nil {
		response.BadRequest(w, r, "VALIDATION_ERROR", "bank_account_uuid is required")
		return
	}
	rows, err := usecase.ParseBankStatement(r.URL.Query().Get("format"), http.MaxBytesReader(w, r.Body, 8<<20))
	if err != nil {
		fail(w, r, err)
		return
	}
	ids, err := h.svc.ImportBankStatement(r.Context(), org, actor(r), bank, rows)
	if err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, map[string]any{"line_uuids": ids})
}

func (h *Handler) ReviewBankStatement(w http.ResponseWriter, r *http.Request) {
	org, ok := h.organization(w, r)
	if !ok {
		return
	}
	id, ok := path(w, r, "line")
	if !ok {
		return
	}
	var in usecase.BankStatementReview
	if !decode(w, r, &in) {
		return
	}
	if err := h.svc.ReviewBankStatement(r.Context(), org, actor(r), id, in); err != nil {
		fail(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]string{"status": "reviewed"})
}

func (h *Handler) BankStatementMeta(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.organization(w, r); !ok {
		return
	}
	response.JSON(w, r, 200, resourcemeta.ResourceMeta{Resource: "billing.bank_statements", DefaultSort: "-id",
		DefaultFields:    []string{"uuid", "external_id", "amount_minor", "status", "confidence"},
		Capabilities:     resourcemeta.Capabilities{Read: true, Filter: true},
		SearchableFields: []string{}, SortableFields: []string{}, FilterableFields: []string{"status"},
		Columns: []resourcemeta.Column{}, Filters: []resourcemeta.Filter{}, Includes: []string{},
	})
}
