package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	coremodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	core "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type Expectations struct {
	Contains          []string `json:"contains"`
	Excludes          []string `json:"excludes"`
	Handoff           *bool    `json:"handoff"`
	MinimumConfidence float64  `json:"minimum_confidence"`
}
type EvaluationCaseInput struct {
	Channel      string       `json:"channel"`
	Category     string       `json:"category"`
	At           *time.Time   `json:"at,omitempty"`
	Name         string       `json:"name"`
	Transcript   string       `json:"transcript"`
	Locale       string       `json:"locale"`
	Conversation *uuid.UUID   `json:"conversation_uuid,omitempty"`
	Expected     Expectations `json:"expected"`
}
type SimulationInput struct {
	Channel    string        `json:"channel,omitempty"`
	Category   string        `json:"category,omitempty"`
	At         *time.Time    `json:"at,omitempty"`
	Transcript string        `json:"transcript"`
	Locale     string        `json:"locale"`
	Config     *model.Config `json:"config,omitempty"`
}
type SimulationResult struct {
	Reply      model.Reply `json:"reply"`
	RunUUID    uuid.UUID   `json:"run_uuid"`
	Outcome    string      `json:"outcome"`
	CostMicros int64       `json:"cost_micros"`
}

func (s *Service) SaveEvaluationCase(ctx context.Context, org, actor int64, in EvaluationCaseInput) (db.AiEvaluationCase, error) {
	if in.Conversation != nil {
		c, err := s.Repo.Q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: org, Uuid: *in.Conversation})
		if err != nil {
			return db.AiEvaluationCase{}, err
		}
		if err = s.Repo.CheckConversation(ctx, org, c.ID); err != nil {
			return db.AiEvaluationCase{}, err
		}
		in.Channel = c.ChannelType
		in.Transcript, err = s.context(ctx, org, c)
		if err != nil {
			return db.AiEvaluationCase{}, err
		}
	}
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 200 || strings.TrimSpace(in.Transcript) == "" || len(in.Transcript) > 18000 || (in.Locale != "tr" && in.Locale != "en") || len(in.Expected.Contains)+len(in.Expected.Excludes) > 30 || in.Expected.MinimumConfidence < 0 || in.Expected.MinimumConfidence > 1 {
		return db.AiEvaluationCase{}, model.ErrInvalid
	}
	for _, v := range append(append([]string{}, in.Expected.Contains...), in.Expected.Excludes...) {
		if v == "" || len(v) > 200 {
			return db.AiEvaluationCase{}, model.ErrInvalid
		}
	}
	if in.Expected.Handoff == nil && len(in.Expected.Contains)+len(in.Expected.Excludes) == 0 && in.Expected.MinimumConfidence == 0 {
		return db.AiEvaluationCase{}, model.ErrInvalid
	}
	if in.Channel == "" {
		in.Channel = "web"
	}
	if len(in.Channel) > 100 || len(in.Category) > 100 {
		return db.AiEvaluationCase{}, model.ErrInvalid
	}
	at := s.Now()
	if in.At != nil {
		at = *in.At
	}
	if at.IsZero() {
		return db.AiEvaluationCase{}, model.ErrInvalid
	}
	raw, err := json.Marshal(in.Expected)
	if err != nil {
		return db.AiEvaluationCase{}, err
	}
	return s.Repo.Q.AIEvaluationCaseCreate(ctx, db.AIEvaluationCaseCreateParams{OrganizationID: org, Name: in.Name, Transcript: ai.NewMasker().Mask(in.Transcript), Locale: in.Locale, Expected: raw, AuthorID: actor, Channel: in.Channel, Category: in.Category, ScenarioAt: pgtype.Timestamptz{Time: at, Valid: true}})
}

// Simulate uses the production provider, public RAG, credit ledger and output
// schema, but never offers tools, mutates a conversation or sends a message.
func (s *Service) Simulate(ctx context.Context, org, actor int64, in SimulationInput) (out SimulationResult, err error) {
	if strings.TrimSpace(in.Transcript) == "" || len(in.Transcript) > 18000 || (in.Locale != "tr" && in.Locale != "en") {
		return out, model.ErrInvalid
	}
	var cfg model.Config
	if in.Config != nil {
		cfg = *in.Config
	} else {
		cfg, _, err = s.Repo.Config(ctx, org)
		if err != nil {
			return out, err
		}
	}
	if err = cfg.Validate(); err != nil {
		return out, err
	}
	if cfg.Mode == "off" || cfg.Mode == "summary" {
		out.Outcome = "disabled"
		return out, nil
	}
	channel := in.Channel
	if channel == "" && len(cfg.Channels) > 0 {
		channel = cfg.Channels[0]
	}
	at := s.Now()
	if in.At != nil {
		at = *in.At
	}
	if !cfg.Active(at, channel) || (len(cfg.Categories) > 0 && !slices.Contains(cfg.Categories, in.Category)) {
		out.Outcome = "handoff"
		out.Reply.Handoff = true
		out.Reply.Summary = "Channel, business hours or category policy requires human review"
		return out, nil
	}
	run, err := s.Engine.RunAgent(ctx, org, actor, coremodel.RunInput{Task: "reply", Input: "Persona (untrusted): " + cfg.Persona + "\nConversation:\n" + in.Transcript, Locale: in.Locale, Retrieve: true, MaxTokens: 2048}, core.AgentOptions{Instructions: "Simulate a support reply grounded exclusively in supplied public knowledge. Return structured reply, summary, confidence, handoff and citation_ids. Never execute actions or claim execution. Hand off if knowledge is insufficient.", Schema: replySchema})
	out.RunUUID, out.CostMicros = run.UUID, run.CostMicros
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal([]byte(run.Text), &out.Reply); err != nil {
		return out, ai.ErrInvalidOutput
	}
	out.Outcome = "draft"
	grounded := len(out.Reply.Citations) > 0
	for _, id := range out.Reply.Citations {
		found := false
		for _, c := range run.Citations {
			if c.ChunkUUID == id {
				found = true
			}
		}
		grounded = grounded && found
	}
	if out.Reply.Handoff || out.Reply.Confidence < cfg.Confidence || !grounded || strings.TrimSpace(out.Reply.Text) == "" {
		out.Outcome = "handoff"
		out.Reply.Handoff = true
	}
	for _, keyword := range cfg.HandoffKeywords {
		if strings.Contains(strings.ToLower(in.Transcript), strings.ToLower(keyword)) {
			out.Outcome = "handoff"
			out.Reply.Handoff = true
		}
	}
	if cfg.Mode == "off" {
		out.Outcome = "disabled"
	}
	return out, nil
}

func scoreEvaluation(expected Expectations, out SimulationResult) float64 {
	passed, total := 0, 0
	text := strings.ToLower(out.Reply.Text)
	for _, v := range expected.Contains {
		total++
		if strings.Contains(text, strings.ToLower(v)) {
			passed++
		}
	}
	for _, v := range expected.Excludes {
		total++
		if !strings.Contains(text, strings.ToLower(v)) {
			passed++
		}
	}
	if expected.Handoff != nil {
		total++
		if *expected.Handoff == out.Reply.Handoff {
			passed++
		}
	}
	if expected.MinimumConfidence > 0 {
		total++
		if out.Reply.Confidence >= expected.MinimumConfidence {
			passed++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(passed) / float64(total)
}

type EvaluationInput struct {
	Baseline *uuid.UUID    `json:"baseline_uuid,omitempty"`
	Config   *model.Config `json:"config,omitempty"`
}
type EvaluationReport struct {
	Cases       int         `json:"cases"`
	MeanScore   float64     `json:"mean_score"`
	Failed      int         `json:"failed"`
	Regressions []uuid.UUID `json:"regressions"`
	CostMicros  int64       `json:"cost_micros"`
	Scoring     string      `json:"scoring"`
}

func (s *Service) Evaluate(ctx context.Context, org, actor int64, in EvaluationInput) (out db.AiEvaluation, err error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cases, err := s.Repo.Q.AIEvaluationCases(ctx, db.AIEvaluationCasesParams{OrganizationID: org, Limit: 101})
	if err != nil {
		return out, err
	}
	if len(cases) == 0 || len(cases) > 100 {
		return out, model.ErrInvalid
	}
	baseline := map[uuid.UUID]db.AiEvaluationResult{}
	baselineID := pgtype.UUID{}
	if in.Baseline != nil {
		b, e := s.Repo.Q.AIEvaluationGet(ctx, db.AIEvaluationGetParams{OrganizationID: org, Uuid: *in.Baseline})
		if e != nil {
			return out, e
		}
		if b.Status != "completed" {
			return out, model.ErrConflict
		}
		rows, e := s.Repo.Q.AIEvaluationResults(ctx, db.AIEvaluationResultsParams{OrganizationID: org, EvaluationUuid: b.Uuid, Limit: 100})
		if e != nil {
			return out, e
		}
		for _, r := range rows {
			baseline[r.CaseUuid] = r
		}
		baselineID = pgtype.UUID{Bytes: b.Uuid, Valid: true}
	}
	cfg, revision, err := s.Repo.Config(ctx, org)
	if err != nil {
		return out, err
	}
	if in.Config != nil {
		cfg = *in.Config
		revision = 0
	}
	if err = cfg.Validate(); err != nil {
		return out, err
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return out, err
	}
	out, err = s.Repo.Q.AIEvaluationStart(ctx, db.AIEvaluationStartParams{OrganizationID: org, Config: raw, ConfigRevision: revision, BaselineUuid: baselineID, AuthorID: actor})
	if err != nil {
		return out, err
	}
	evaluationUUID := out.Uuid
	// Ensure errors/cancellation leave a terminal, reviewable batch. Individual
	// provider errors are retained as zero-scored cases rather than hidden.
	defer func() {
		if err != nil {
			finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_, _ = s.Repo.Q.AIEvaluationFinish(finish, db.AIEvaluationFinishParams{OrganizationID: org, Uuid: evaluationUUID, Status: "failed", Report: []byte(`{"error":"evaluation_interrupted"}`)})
		}
	}()
	report := EvaluationReport{Cases: len(cases), Regressions: []uuid.UUID{}, Scoring: "Deterministic expected substrings, forbidden substrings, handoff and minimum confidence; no LLM judge"}
	for _, c := range cases {
		var expected Expectations
		if err = json.Unmarshal(c.Expected, &expected); err != nil {
			return out, err
		}
		result, runErr := s.Simulate(ctx, org, actor, SimulationInput{Transcript: c.Transcript, Locale: c.Locale, Config: &cfg, Channel: c.Channel, Category: c.Category, At: &c.ScenarioAt.Time})
		score, code := float64(0), ""
		if runErr != nil {
			code = "SIMULATION_FAILED"
			report.Failed++
		} else {
			score = scoreEvaluation(expected, result)
		}
		if old, ok := baseline[c.Uuid]; ok && old.CaseRevision == c.Revision && score < old.Score {
			report.Regressions = append(report.Regressions, c.Uuid)
		}
		report.MeanScore += score
		report.CostMicros += result.CostMicros
		snapshot, e := json.Marshal(c)
		if e != nil {
			return out, e
		}
		output, e := json.Marshal(result)
		if e != nil {
			return out, e
		}
		runID := pgtype.UUID{}
		if result.RunUUID != uuid.Nil {
			runID = pgtype.UUID{Bytes: result.RunUUID, Valid: true}
		}

		if err = s.Repo.Q.AIEvaluationResultCreate(ctx, db.AIEvaluationResultCreateParams{OrganizationID: org, EvaluationUuid: out.Uuid, CaseUuid: c.Uuid, CaseRevision: c.Revision, CaseSnapshot: snapshot, Output: output, Score: score, RunUuid: runID, ErrorCode: code}); err != nil {
			return out, err
		}
		if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
			return out, runErr
		}
	}
	report.MeanScore /= float64(len(cases))
	raw, err = json.Marshal(report)
	if err != nil {
		return out, err
	}
	out, err = s.Repo.Q.AIEvaluationFinish(ctx, db.AIEvaluationFinishParams{OrganizationID: org, Uuid: out.Uuid, Status: "completed", Report: raw})
	return
}
