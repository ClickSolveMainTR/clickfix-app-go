package usecase

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	coremodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	core "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/usecase"
	deptmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/departments/model"
	deptusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/departments/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type deptEngine struct {
	in   coremodel.RunInput
	opts core.AgentOptions
	text string
}

func (e *deptEngine) RunAgent(_ context.Context, _, _ int64, in coremodel.RunInput, opts core.AgentOptions) (core.RunResult, error) {
	e.in, e.opts = in, opts
	return core.RunResult{Result: ai.Result{Text: e.text}}, nil
}

type deptRouter struct {
	candidates []deptmodel.Candidate
	ai         db.GetDepartmentAIRow
	transfers  []deptmodel.TransferInput
	actors     []deptusecase.Actor
}

func (r *deptRouter) Route(context.Context, int64, int64, string, string) (*deptmodel.Prediction, error) {
	return nil, nil
}
func (r *deptRouter) AISettings(context.Context, int64, pgtype.Int8) (db.GetDepartmentAIRow, bool, error) {
	return r.ai, true, nil
}
func (r *deptRouter) Candidates(context.Context, int64, pgtype.UUID) ([]deptmodel.Candidate, error) {
	return r.candidates, nil
}
func (r *deptRouter) Transfer(_ context.Context, _ int64, _ uuid.UUID, actor deptusecase.Actor, in deptmodel.TransferInput) (deptmodel.ConversationDepartment, error) {
	r.transfers, r.actors = append(r.transfers, in), append(r.actors, actor)
	return deptmodel.ConversationDepartment{}, nil
}

// TestClassifyDepartment: the model sees the candidates as untrusted labels
// and answers through the structured schema; a non-uuid answer is no
// department.
func TestClassifyDepartment(t *testing.T) {
	sales := uuid.New()
	eng := &deptEngine{text: `{"department_uuid":"` + sales.String() + `","confidence":0.82,"reason":"fiyat soruyor"}`}
	s := &Service{Engine: eng}
	p, err := s.ClassifyDepartment(context.Background(), 1, 2, "contact: Paket fiyatı nedir?", "tr", []deptmodel.Candidate{{UUID: sales, Name: "Satış", RoutingHint: "fiyat"}})
	if err != nil || p.DepartmentUUID == nil || *p.DepartmentUUID != sales || p.Confidence != 0.82 {
		t.Fatalf("prediction = %+v %v", p, err)
	}
	if eng.in.Task != "triage" || !strings.Contains(eng.in.Input, sales.String()) || eng.opts.Schema["required"] == nil || eng.opts.ConversationID != 2 {
		t.Fatalf("run = %+v %+v", eng.in, eng.opts)
	}
	eng.text = `{"department_uuid":null,"confidence":0.1,"reason":""}`
	if p, err = s.ClassifyDepartment(context.Background(), 1, 2, "x", "tr", nil); err != nil || p.DepartmentUUID != nil {
		t.Fatalf("null = %+v %v", p, err)
	}
	eng.text = "not json"
	if _, err = s.ClassifyDepartment(context.Background(), 1, 2, "x", "tr", nil); err == nil {
		t.Fatal("invalid output accepted")
	}
}

// TestDepartmentReplyTools: a department's AI settings shape the reply and
// the transfer tool hands the conversation over as the AI.
func TestDepartmentReplyTools(t *testing.T) {
	sales, support := uuid.New(), uuid.New()
	router := &deptRouter{
		candidates: []deptmodel.Candidate{{UUID: sales, Name: "Satış"}, {UUID: support, Name: "Destek"}},
		ai:         db.GetDepartmentAIRow{Name: "Destek", AiEnabled: true, AiInstructions: "Seri numarasını sor."},
	}
	s := &Service{Departments: router}
	conv := db.Conversation{Uuid: uuid.New(), DepartmentID: pgtype.Int8{Int64: 7, Valid: true}}
	opts := core.AgentOptions{StableData: "persona"}
	ok, err := s.departmentAI(context.Background(), 1, conv, &opts)
	if err != nil || !ok || !strings.Contains(opts.StableData, "Seri numarasını sor.") {
		t.Fatalf("department AI = %v %v %q", ok, err, opts.StableData)
	}
	router.ai.AiEnabled = false
	if ok, _ = s.departmentAI(context.Background(), 1, conv, &opts); ok {
		t.Fatal("AI answered in a department that turned it off")
	}
	called := false
	tools, runner := s.departmentTool(context.Background(), 1, conv, func(context.Context, ai.ToolCall) (ai.ToolResult, error) {
		called = true
		return ai.ToolResult{Content: "other"}, nil
	})
	if len(tools) != 1 || tools[0].Name != TransferTool {
		t.Fatalf("tools = %+v", tools)
	}
	if _, err := runner(context.Background(), ai.ToolCall{Name: "lookup_order"}); err != nil || !called {
		t.Fatal("other tools no longer reach their runner")
	}
	input, _ := json.Marshal(map[string]string{"department_uuid": sales.String(), "reason": "fiyat", "context_summary": "Paket soruyor"})
	res, err := runner(context.Background(), ai.ToolCall{Name: TransferTool, Input: input})
	if err != nil || res.IsError || len(router.transfers) != 1 || *router.transfers[0].DepartmentUUID != sales || router.actors[0].Kind != deptmodel.SourceAI || router.transfers[0].ContextSummary != "Paket soruyor" {
		t.Fatalf("transfer = %+v %v %+v", res, err, router.transfers)
	}
}
