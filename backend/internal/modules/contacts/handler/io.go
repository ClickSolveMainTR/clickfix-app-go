package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/contacts/ioadapter"
	importusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/imports/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/activity"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/i18n"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
)

const maxImportBytes = 10 << 20

// sampler is an adapter that also ships template rows.
type sampler interface {
	ioengine.ResourceAdapter
	SampleRows() []map[string]any
}

// IOHandler serves contact and company import / export (CF-84). Imports run
// through the shared import engine (upload → mapping → preview → confirm →
// rollback within 24 hours); exports are streamed synchronously.
type IOHandler struct {
	imports   *importusecase.Service
	contacts  sampler
	companies sampler
	activity  *activity.Recorder
}

// NewIO builds the import / export handler.
func NewIO(imports *importusecase.Service, contacts *ioadapter.Contacts, companies *ioadapter.Companies, rec *activity.Recorder) *IOHandler {
	return &IOHandler{imports: imports, contacts: contacts, companies: companies, activity: rec}
}

func (h *IOHandler) adapter(resource string) sampler {
	if resource == ioadapter.ResourceCompanies {
		return h.companies
	}
	return h.contacts
}

// ExportContacts streams the contact list as CSV or XLSX.
func (h *IOHandler) ExportContacts(w http.ResponseWriter, r *http.Request) {
	h.export(w, r, ioadapter.ResourceContacts, "contacts")
}

// ExportCompanies streams the company list as CSV or XLSX.
func (h *IOHandler) ExportCompanies(w http.ResponseWriter, r *http.Request) {
	h.export(w, r, ioadapter.ResourceCompanies, "companies")
}

func (h *IOHandler) export(w http.ResponseWriter, r *http.Request, resource, name string) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	v := r.URL.Query()
	format := ioengine.ExportFormat(strings.ToLower(strings.TrimSpace(v.Get("format"))))
	if format == "" {
		format = ioengine.ExportCSV
	}
	if format != ioengine.ExportCSV && format != ioengine.ExportXLSX {
		invalid(w, r, "format must be csv or xlsx")
		return
	}
	locale := localeOf(r)
	query := ioengine.ExportQuery{"q": v.Get("q"), "company": v.Get("company")}
	ds, err := h.adapter(resource).Export(r.Context(), query, i18n.Normalize(locale))
	if err != nil {
		writeError(w, r, err)
		return
	}
	data, err := ioengine.EncodeExport(format, ds, locale, nil, name)
	if err != nil {
		response.InternalErr(w, r, err, "")
		return
	}
	if h.activity != nil {
		p := authctx.MustPrincipal(r.Context())
		h.activity.Record(r.Context(), &p.UserInternal, "contacts.exported", resource, nil, map[string]any{
			"format": format, "rows": len(ds.Rows), "organization": scope.UUID.String(),
		}, r)
	}
	ct := "text/csv; charset=utf-8"
	if format == ioengine.ExportXLSX {
		ct = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fmt.Sprintf("%s-%s.%s", name, time.Now().UTC().Format("20060102"), format)))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// ImportContacts uploads a contacts file and opens an import job.
func (h *IOHandler) ImportContacts(w http.ResponseWriter, r *http.Request) {
	h.upload(w, r, ioadapter.ResourceContacts)
}

// ImportCompanies uploads a companies file and opens an import job.
func (h *IOHandler) ImportCompanies(w http.ResponseWriter, r *http.Request) {
	h.upload(w, r, ioadapter.ResourceCompanies)
}

func (h *IOHandler) upload(w http.ResponseWriter, r *http.Request, resource string) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes+(1<<20))
	if err := r.ParseMultipartForm(maxImportBytes); err != nil {
		invalid(w, r, "invalid multipart form (max 10 MB)")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		invalid(w, r, "file is required")
		return
	}
	defer func() { _ = file.Close() }()
	format := ioengine.ImportFormat(strings.ToLower(strings.TrimSpace(r.FormValue("format"))))
	if format == "" {
		format = formatFromName(header.Filename)
	}
	if format != ioengine.ImportCSV && format != ioengine.ImportXLSX && format != ioengine.ImportTSV {
		invalid(w, r, "format must be csv, tsv or xlsx")
		return
	}
	locale := strings.TrimSpace(r.FormValue("locale"))
	if locale == "" {
		locale = localeOf(r)
	}
	job, err := h.imports.UploadTenant(r.Context(), scope.ID, p.UserInternal, resource, format, locale, header.Filename, file)
	if err != nil {
		writeImportError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusCreated, h.withSchema(job))
}

// ImportSample downloads a template for {resource} (contacts|companies).
func (h *IOHandler) ImportSample(resource string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		format := ioengine.ImportFormat(strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format"))))
		if format == "" {
			format = ioengine.ImportXLSX
		}
		if format != ioengine.ImportCSV && format != ioengine.ImportXLSX {
			invalid(w, r, "format must be csv or xlsx")
			return
		}
		a := h.adapter(resource)
		data, ct, err := ioengine.EncodeSample(format, localeOf(r), a.ImportSchema(), a.SampleRows())
		if err != nil {
			response.InternalErr(w, r, err, "")
			return
		}
		name := strings.TrimPrefix(resource, "tenant.")
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name+"-sample."+string(format)))
		_, _ = w.Write(data)
	}
}

// ImportJobView is an import job plus the target schema for the mapping UI.
type ImportJobView struct {
	importusecase.ImportJobView
	Fields []ioengine.ImportField `json:"fields"`
}

func (h *IOHandler) withSchema(job importusecase.ImportJobView) ImportJobView {
	return ImportJobView{ImportJobView: job, Fields: h.adapter(job.Resource).ImportSchema()}
}

// ListImports lists the organization's contact / company import jobs.
func (h *IOHandler) ListImports(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	v := r.URL.Query()
	limit, offset := apiquery.Clamp(atoi(v.Get("limit")), atoi(v.Get("offset")), 20, 100)
	resource := ""
	switch v.Get("resource") {
	case "":
	case "contacts":
		resource = ioadapter.ResourceContacts
	case "companies":
		resource = ioadapter.ResourceCompanies
	default:
		invalid(w, r, "resource must be contacts or companies")
		return
	}
	items, total, err := h.imports.ListTenantJobs(r.Context(), scope.ID, resource, limit, offset)
	if err != nil {
		writeImportError(w, r, err)
		return
	}
	out := make([]ImportJobView, 0, len(items))
	for _, it := range items {
		if it.Resource != ioadapter.ResourceContacts && it.Resource != ioadapter.ResourceCompanies {
			continue
		}
		out = append(out, h.withSchema(it))
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(out, total, limit, offset))
}

// GetImport returns one import job.
func (h *IOHandler) GetImport(w http.ResponseWriter, r *http.Request) {
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	job, err := h.imports.GetTenantJob(r.Context(), scope.ID, id)
	if err != nil {
		writeImportError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, h.withSchema(job))
}

// UpdateImportMapping saves the column mapping and defaults.
func (h *IOHandler) UpdateImportMapping(w http.ResponseWriter, r *http.Request) {
	_, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	var in struct {
		Mapping  map[string]string `json:"mapping"`
		Defaults map[string]string `json:"defaults"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Mapping == nil {
		in.Mapping = map[string]string{}
	}
	if in.Defaults == nil {
		in.Defaults = map[string]string{}
	}
	h.jobAction(w, r, http.StatusOK, func(p authctx.Principal) (importusecase.ImportJobView, error) {
		return h.imports.UpdateMapping(r.Context(), id, p.UserInternal, in.Mapping, in.Defaults)
	})
}

// PreviewImport dry-runs the mapped rows (create / update / invalid).
func (h *IOHandler) PreviewImport(w http.ResponseWriter, r *http.Request) {
	_, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	h.jobAction(w, r, http.StatusOK, func(p authctx.Principal) (importusecase.ImportJobView, error) {
		return h.imports.Preview(r.Context(), id, p.UserInternal)
	})
}

// ConfirmImport queues the apply step.
func (h *IOHandler) ConfirmImport(w http.ResponseWriter, r *http.Request) {
	_, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	h.jobAction(w, r, http.StatusAccepted, func(p authctx.Principal) (importusecase.ImportJobView, error) {
		return h.imports.Confirm(r.Context(), id, p.UserInternal)
	})
}

// RollbackImport reverts an applied import (24 hours).
func (h *IOHandler) RollbackImport(w http.ResponseWriter, r *http.Request) {
	_, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	h.jobAction(w, r, http.StatusOK, func(p authctx.Principal) (importusecase.ImportJobView, error) {
		return h.imports.Rollback(r.Context(), id, p.UserInternal)
	})
}

func (h *IOHandler) jobAction(w http.ResponseWriter, r *http.Request, status int, fn func(authctx.Principal) (importusecase.ImportJobView, error)) {
	job, err := fn(authctx.MustPrincipal(r.Context()))
	if err != nil {
		writeImportError(w, r, err)
		return
	}
	response.JSON(w, r, status, h.withSchema(job))
}

func writeImportError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, importusecase.ErrNotFound), errors.Is(err, importusecase.ErrForbidden):
		response.NotFound(w, r, "Import not found")
	case errors.Is(err, importusecase.ErrInvalidRequest):
		invalid(w, r, strings.TrimPrefix(err.Error(), importusecase.ErrInvalidRequest.Error()+": "))
	default:
		response.InternalErr(w, r, err, "")
	}
}

func formatFromName(name string) ioengine.ImportFormat {
	n := strings.ToLower(name)
	switch {
	case strings.HasSuffix(n, ".xlsx"):
		return ioengine.ImportXLSX
	case strings.HasSuffix(n, ".tsv"):
		return ioengine.ImportTSV
	default:
		return ioengine.ImportCSV
	}
}

func localeOf(r *http.Request) string {
	if l := strings.TrimSpace(r.URL.Query().Get("locale")); l != "" {
		return string(i18n.Normalize(l))
	}
	al := strings.ToLower(r.Header.Get("Accept-Language"))
	if strings.HasPrefix(al, "en") {
		return "en"
	}
	return "tr"
}
