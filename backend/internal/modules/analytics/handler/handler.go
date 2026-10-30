package handler

import (
	"encoding/csv"
	"errors"
	"fmt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/analytics/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/analytics/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
	"net/http"
	"time"
)

type Handler struct {
	svc     *usecase.Service
	enqueue func(*http.Request, int64) error
}

func New(s *usecase.Service, enq func(*http.Request, int64) error) *Handler { return &Handler{s, enq} }
func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, e1 := time.Parse(time.RFC3339, q.Get("from"))
	to, e2 := time.Parse(time.RFC3339, q.Get("to"))
	f := model.Filter{From: from, To: to, Bucket: q.Get("bucket"), Timezone: q.Get("timezone"), Channel: q.Get("channel")}
	if e1 != nil || e2 != nil {
		response.Error(w, r, 400, "VALIDATION_ERROR", "from and to must be RFC3339")
		return
	}
	for key, dst := range map[string]**uuid.UUID{"team": &f.Team, "agent": &f.Agent, "tag": &f.Tag} {
		if v := q.Get(key); v != "" {
			id, err := uuid.Parse(v)
			if err != nil {
				response.Error(w, r, 400, "VALIDATION_ERROR", "Invalid "+key)
				return
			}
			*dst = &id
		}
	}
	out, err := h.svc.Dashboard(r.Context(), orgctx.Must(r.Context()).ID, f)
	if errors.Is(err, usecase.ErrInvalid) {
		response.Error(w, r, 400, "VALIDATION_ERROR", "Invalid analytics filter")
		return
	}
	if err != nil {
		response.Internal(w, r, "Analytics unavailable")
		return
	}
	if r.PathValue("format") == "" {
		response.JSON(w, r, 200, out)
		return
	}
	cards, _ := out["cards"].(map[string]any)
	keys := []string{"volume", "resolved", "backlog", "first_response_seconds", "resolution_seconds", "sla_total", "sla_breached", "sla_hit_percent", "csat_average", "csat_responses"}
	rows := [][]string{{"metric", "value"}}
	for _, k := range keys {
		value := ""
		if cards[k] != nil {
			value = fmt.Sprint(cards[k])
		}
		rows = append(rows, []string{k, value})
	}
	switch r.PathValue("format") {
	case "csv":
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="analytics.csv"`)
		cw := csv.NewWriter(w)
		_ = cw.WriteAll(rows)
	case "xlsx":
		file := excelize.NewFile()
		defer func() { _ = file.Close() }()
		for i, row := range rows {
			values := []any{row[0], row[1]}
			cell, _ := excelize.CoordinatesToCellName(1, i+1)
			if err = file.SetSheetRow("Sheet1", cell, &values); err != nil {
				response.Internal(w, r, "Analytics unavailable")
				return
			}
		}
		buf, err := file.WriteToBuffer()
		if err != nil {
			response.Internal(w, r, "Analytics unavailable")
			return
		}
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", `attachment; filename="analytics.xlsx"`)
		_, _ = w.Write(buf.Bytes())
	default:
		response.NotFound(w, r, "Unknown export format")
	}
}
func (h *Handler) Backfill(w http.ResponseWriter, r *http.Request) {
	if h.enqueue == nil {
		response.Error(w, r, 503, "UNAVAILABLE", "Queue unavailable")
		return
	}
	if err := h.enqueue(r, orgctx.Must(r.Context()).ID); err != nil {
		response.Internal(w, r, "Analytics unavailable")
		return
	}
	response.JSON(w, r, 202, map[string]string{"status": "queued"})
}
