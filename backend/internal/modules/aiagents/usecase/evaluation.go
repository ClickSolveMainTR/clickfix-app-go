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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Expectations struct {
	Contains          []string `json:"contains"`
	Excludes          []string `json:"excludes"`
	Handoff           *bool    `json:"handoff"`
	MinimumConfidence float64  `json:"minimum_confidence"`
}
type EvaluationCaseInput struct {
	Channel  string     `json:"channel"`
	Category string     `json:"category"`
	At       *time.Time `json:"at,omitempty"`
	Name     string     `json:"name"`
	// Transcript is the conversation the agent answers; Question is the
	// short form ("Customer: <question>") of a department question set.
	Transcript   string       `json:"transcript"`
	Question     string       `json:"question,omitempty"`
	Locale       string       `json:"locale"`
	Conversation *uuid.UUID   `json:"conversation_uuid,omitempty"`
	Expected     Expectations `json:"expected"`
	// CF-220: department set, expected answer and expected sources.
	Department       *uuid.UUID  `json:"department_uuid,omitempty"`
	ExpectedAnswer   string      `json:"expected_answer"`
	ExpectedArticles []uuid.UUID `json:"expected_article_uuids"`
	// Active (default true) keeps the case in runs.
	Active *bool `json:"active,omitempty"`
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
	// Model, LatencyMs and CitedArticles feed the evaluation metrics.
	Model         string      `json:"model"`
	LatencyMs     int64       `json:"latency_ms"`
	CitedArticles []uuid.UUID `json:"cited_article_uuids"`
}

// evaluationCaseFields is a validated case ready for the database.
type evaluationCaseFields struct {
	in         EvaluationCaseInput
	expected   []byte
	department *int64
	at         time.Time
	active     bool
}

func (s *Service) prepareEvaluationCase(ctx context.Context, org int64, in EvaluationCaseInput) (f evaluationCaseFields, err error) {
	if strings.TrimSpace(in.Transcript) == "" && strings.TrimSpace(in.Question) != "" {
		in.Transcript = "Customer: " + strings.TrimSpace(in.Question)
	}
	if in.Conversation != nil {
		c, err := s.Repo.Q.GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: org, Uuid: *in.Conversation})
		if err != nil {
			return f, err
		}
		if err = s.Repo.CheckConversation(ctx, org, c.ID); err != nil {
			return f, err
		}
		in.Channel = c.ChannelType
		in.Transcript, err = s.context(ctx, org, c)
		if err != nil {
			return f, err
		}
	}
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 200 || strings.TrimSpace(in.Transcript) == "" || len(in.Transcript) > 18000 || (in.Locale != "tr" && in.Locale != "en") || len(in.Expected.Contains)+len(in.Expected.Excludes) > 30 || in.Expected.MinimumConfidence < 0 || in.Expected.MinimumConfidence > 1 {
		return f, model.ErrInvalid
	}
	for _, v := range append(append([]string{}, in.Expected.Contains...), in.Expected.Excludes...) {
		if v == "" || len(v) > 200 {
			return f, model.ErrInvalid
		}
	}
	in.ExpectedAnswer = strings.TrimSpace(in.ExpectedAnswer)
	if len(in.ExpectedAnswer) > 6000 || len(in.ExpectedArticles) > 20 {
		return f, model.ErrInvalid
	}
	for _, id := range in.ExpectedArticles {
		if id == uuid.Nil {
			return f, model.ErrInvalid
		}
	}
	if in.ExpectedArticles == nil {
		in.ExpectedArticles = []uuid.UUID{}
	}
	if in.Expected.Handoff == nil && len(in.Expected.Contains)+len(in.Expected.Excludes) == 0 && in.Expected.MinimumConfidence == 0 && in.ExpectedAnswer == "" && len(in.ExpectedArticles) == 0 {
		return f, model.ErrInvalid
	}
	if in.Channel == "" {
		in.Channel = "web"
	}
	if len(in.Channel) > 100 || len(in.Category) > 100 {
		return f, model.ErrInvalid
	}
	at := s.Now()
	if in.At != nil {
		at = *in.At
	}
	if at.IsZero() {
		return f, model.ErrInvalid
	}
	if f.department, err = s.ResolveDepartment(ctx, org, in.Department); err != nil {
		return f, err
	}
	if f.expected, err = json.Marshal(in.Expected); err != nil {
		return f, err
	}
	in.Transcript = ai.NewMasker().Mask(in.Transcript)
	in.ExpectedAnswer = ai.NewMasker().Mask(in.ExpectedAnswer)
	f.in, f.at, f.active = in, at, in.Active == nil || *in.Active
	return f, nil
}

// SaveEvaluationCase adds a question to the organization's (or a
// department's) set.
func (s *Service) SaveEvaluationCase(ctx context.Context, org, actor int64, in EvaluationCaseInput) (db.AiEvaluationCase, error) {
	f, err := s.prepareEvaluationCase(ctx, org, in)
	if err != nil {
		return db.AiEvaluationCase{}, err
	}
	row, err := s.Repo.Q.AIEvaluationCaseCreate(ctx, db.AIEvaluationCaseCreateParams{OrganizationID: org, Name: f.in.Name, Transcript: f.in.Transcript, Locale: f.in.Locale, Expected: f.expected, AuthorID: actor, Channel: f.in.Channel, Category: f.in.Category, ScenarioAt: pgtype.Timestamptz{Time: f.at, Valid: true}, DepartmentID: optionalDepartment(f.department), ExpectedAnswer: f.in.ExpectedAnswer, ExpectedArticleUuids: f.in.ExpectedArticles})
	if err != nil || f.active {
		return row, err
	}
	return s.UpdateEvaluationCase(ctx, org, row.Uuid, in)
}

// UpdateEvaluationCase replaces a case and bumps its revision, so earlier
// results of the case are no longer compared with new ones.
func (s *Service) UpdateEvaluationCase(ctx context.Context, org int64, id uuid.UUID, in EvaluationCaseInput) (db.AiEvaluationCase, error) {
	f, err := s.prepareEvaluationCase(ctx, org, in)
	if err != nil {
		return db.AiEvaluationCase{}, err
	}
	return s.Repo.Q.AIEvaluationCaseUpdate(ctx, db.AIEvaluationCaseUpdateParams{OrganizationID: org, Uuid: id, Name: f.in.Name, Transcript: f.in.Transcript, Locale: f.in.Locale, Expected: f.expected, Channel: f.in.Channel, Category: f.in.Category, DepartmentID: optionalDepartment(f.department), ExpectedAnswer: f.in.ExpectedAnswer, ExpectedArticleUuids: f.in.ExpectedArticles, Active: f.active})
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
	open := true
	if s.Hours != nil {
		if st, err := s.Hours.HoursState(ctx, org, at); err == nil {
			open = st.Open
		}
	}
	if !cfg.Active(channel, open) || (len(cfg.Categories) > 0 && !slices.Contains(cfg.Categories, in.Category)) {
		out.Outcome = "handoff"
		out.Reply.Handoff = true
		out.Reply.Summary = "Channel, business hours or category policy requires human review"
		return out, nil
	}
	started := time.Now()
	run, err := s.Engine.RunAgent(ctx, org, actor, coremodel.RunInput{Task: "reply", Input: "Persona (untrusted): " + cfg.Persona + "\nConversation:\n" + in.Transcript, Locale: in.Locale, Retrieve: true, MaxTokens: 2048}, core.AgentOptions{Instructions: "Simulate a support reply grounded exclusively in supplied public knowledge. Return structured reply, summary, confidence, handoff and citation_ids. Never execute actions or claim execution. Hand off if knowledge is insufficient.", Schema: replySchema})
	out.RunUUID, out.CostMicros, out.Model, out.LatencyMs = run.UUID, run.CostMicros, run.Model, time.Since(started).Milliseconds()
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal([]byte(run.Text), &out.Reply); err != nil {
		return out, ai.ErrInvalidOutput
	}
	out.Outcome = "draft"
	grounded := len(out.Reply.Citations) > 0
	out.CitedArticles = []uuid.UUID{}
	for _, id := range out.Reply.Citations {
		found := false
		for _, c := range run.Citations {
			if c.ChunkUUID == id {
				found = true
				if !slices.Contains(out.CitedArticles, c.ArticleUUID) {
					out.CitedArticles = append(out.CitedArticles, c.ArticleUUID)
				}
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
	// Department runs that department's set only; nil runs every case.
	Department *uuid.UUID `json:"department_uuid,omitempty"`
}
type EvaluationReport struct {
	Cases       int         `json:"cases"`
	MeanScore   float64     `json:"mean_score"`
	Failed      int         `json:"failed"`
	Regressions []uuid.UUID `json:"regressions"`
	CostMicros  int64       `json:"cost_micros"`
	Scoring     string      `json:"scoring"`
	// CF-220: before/after metrics, per department.
	Metrics     EvaluationMetrics   `json:"metrics"`
	Before      *EvaluationMetrics  `json:"before"`
	Departments []DepartmentMetrics `json:"departments"`
}

const evaluationScoring = "Deterministic, no LLM judge: expected and forbidden substrings, handoff and minimum confidence; the expected answer counts as correct when the reply holds at least 60% of its content words; a source hit is a citation of one of the expected knowledge articles; cache hit asks the answer cache whether it would answer the question."

// Evaluate runs the set by hand (trigger "manual").
func (s *Service) Evaluate(ctx context.Context, org, actor int64, in EvaluationInput) (db.AiEvaluation, error) {
	fp, err := s.CurrentFingerprint(ctx, org)
	if err != nil {
		return db.AiEvaluation{}, err
	}
	return s.evaluate(ctx, org, actor, in, "manual", fp)
}

func metricsOf(e db.AiEvaluation) *EvaluationMetrics {
	var r EvaluationReport
	if json.Unmarshal(e.Report, &r) == nil && r.Metrics.Cases > 0 {
		return &r.Metrics
	}
	val := func(v pgtype.Float8) *float64 {
		if !v.Valid {
			return nil
		}
		x := v.Float64
		return &x
	}
	return &EvaluationMetrics{AnswerAccuracy: val(e.AnswerAccuracy), SourceHitRate: val(e.SourceHitRate), CacheHitRate: val(e.CacheHitRate), AvgLatencyMs: val(e.AvgLatencyMs), CostPerCaseMicros: val(e.CostPerCaseMicros)}
}

func (s *Service) evaluate(ctx context.Context, org, actor int64, in EvaluationInput, trigger string, fp Fingerprint) (out db.AiEvaluation, err error) {
	// CF-228: evaluations are batch work and use only the batch share of the
	// daily AI limits.
	ctx, cancel := context.WithTimeout(ai.WithBatch(ctx), 2*time.Minute)
	defer cancel()
	department, err := s.ResolveDepartment(ctx, org, in.Department)
	if err != nil {
		return out, err
	}
	cases, err := s.Repo.Q.AIEvaluationRunCases(ctx, db.AIEvaluationRunCasesParams{OrganizationID: org, DepartmentID: optionalDepartment(department)})
	if err != nil {
		return out, err
	}
	if len(cases) == 0 || len(cases) > 100 {
		return out, model.ErrInvalid
	}
	baseline := map[uuid.UUID]db.AiEvaluationResult{}
	baselineID := pgtype.UUID{}
	var before *EvaluationMetrics
	var b db.AiEvaluation
	if in.Baseline != nil {
		b, err = s.Repo.Q.AIEvaluationGet(ctx, db.AIEvaluationGetParams{OrganizationID: org, Uuid: *in.Baseline})
		if err != nil {
			return out, err
		}
		if b.Status != "completed" {
			return out, model.ErrConflict
		}
	} else {
		// Without an explicit baseline the previous completed run of the
		// same scope is the "before".
		b, err = s.Repo.Q.AIEvaluationLatestCompleted(ctx, db.AIEvaluationLatestCompletedParams{OrganizationID: org, DepartmentID: optionalDepartment(department)})
		if errors.Is(err, pgx.ErrNoRows) {
			err = nil
		}
		if err != nil {
			return out, err
		}
	}
	if b.Uuid != uuid.Nil {
		rows, e := s.Repo.Q.AIEvaluationResults(ctx, db.AIEvaluationResultsParams{OrganizationID: org, EvaluationUuid: b.Uuid, Limit: 100})
		if e != nil {
			return out, e
		}
		for _, r := range rows {
			baseline[r.CaseUuid] = r
		}
		baselineID = pgtype.UUID{Bytes: b.Uuid, Valid: true}
		before = metricsOf(b)
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
	fpRaw, err := json.Marshal(fp)
	if err != nil {
		return out, err
	}
	out, err = s.Repo.Q.AIEvaluationStart(ctx, db.AIEvaluationStartParams{OrganizationID: org, Config: raw, ConfigRevision: revision, BaselineUuid: baselineID, AuthorID: actor, TriggerKind: trigger, DepartmentID: optionalDepartment(department), Fingerprint: fpRaw})
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
	report := EvaluationReport{Cases: len(cases), Regressions: []uuid.UUID{}, Scoring: evaluationScoring, Before: before, Departments: []DepartmentMetrics{}}
	total := metricAccumulator{}
	byDepartment := map[int64]*metricAccumulator{}
	orgWide := metricAccumulator{}
	runModel := ""
	for _, c := range cases {
		var expected Expectations
		if err = json.Unmarshal(c.Expected, &expected); err != nil {
			return out, err
		}
		var caseDepartment *int64
		if c.DepartmentID.Valid {
			caseDepartment = &c.DepartmentID.Int64
		}
		var cacheHit *bool
		if s.Cache != nil {
			if hit, e := s.Cache.Lookup(ctx, org, caseDepartment, c.Transcript, c.Locale); e == nil {
				cacheHit = &hit
			}
		}
		result, runErr := s.Simulate(ctx, org, actor, SimulationInput{Transcript: c.Transcript, Locale: c.Locale, Config: &cfg, Channel: c.Channel, Category: c.Category, At: &c.ScenarioAt.Time})
		outcome := caseOutcome{CostMicros: result.CostMicros, Failed: true}
		code := ""
		if runErr != nil {
			code = "SIMULATION_FAILED"
			report.Failed++
		} else {
			outcome = measureCase(expected, c.ExpectedAnswer, c.ExpectedArticleUuids, result)
			if runModel == "" {
				runModel = result.Model
			}
		}
		outcome.CacheHit = cacheHit
		total.add(outcome)
		acc := &orgWide
		if caseDepartment != nil {
			if byDepartment[*caseDepartment] == nil {
				byDepartment[*caseDepartment] = &metricAccumulator{}
			}
			acc = byDepartment[*caseDepartment]
		}
		acc.add(outcome)
		if old, ok := baseline[c.Uuid]; ok && old.CaseRevision == c.Revision && outcome.Score < old.Score {
			report.Regressions = append(report.Regressions, c.Uuid)
		}
		report.MeanScore += outcome.Score
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
		if err = s.Repo.Q.AIEvaluationResultCreate(ctx, db.AIEvaluationResultCreateParams{OrganizationID: org, EvaluationUuid: out.Uuid, CaseUuid: c.Uuid, CaseRevision: c.Revision, CaseSnapshot: snapshot, Output: output, Score: outcome.Score, RunUuid: runID, ErrorCode: code,
			DepartmentID: c.DepartmentID, AnswerCorrect: outcome.AnswerCorrect, SourceHit: optionalBool(outcome.SourceHit), CacheHit: optionalBool(outcome.CacheHit), LatencyMs: outcome.LatencyMs, CostMicros: outcome.CostMicros}); err != nil {
			return out, err
		}
		if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
			return out, runErr
		}
	}
	report.MeanScore /= float64(len(cases))
	report.Metrics = total.metrics()
	if orgWide.cases > 0 {
		report.Departments = append(report.Departments, DepartmentMetrics{EvaluationMetrics: orgWide.metrics()})
	}
	if len(byDepartment) > 0 {
		ids := make([]int64, 0, len(byDepartment))
		for id := range byDepartment {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		uuids, e := s.DepartmentUUIDs(ctx, org, ids)
		if e != nil {
			return out, e
		}
		for _, id := range ids {
			m := DepartmentMetrics{EvaluationMetrics: byDepartment[id].metrics()}
			if u, ok := uuids[id]; ok {
				m.DepartmentUUID = &u
			}
			report.Departments = append(report.Departments, m)
		}
	}
	raw, err = json.Marshal(report)
	if err != nil {
		return out, err
	}
	m := report.Metrics
	out, err = s.Repo.Q.AIEvaluationFinish(ctx, db.AIEvaluationFinishParams{OrganizationID: org, Uuid: out.Uuid, Status: "completed", Report: raw, Model: runModel,
		AnswerAccuracy: optionalFloat(m.AnswerAccuracy), SourceHitRate: optionalFloat(m.SourceHitRate), CacheHitRate: optionalFloat(m.CacheHitRate), AvgLatencyMs: optionalFloat(m.AvgLatencyMs), CostPerCaseMicros: optionalFloat(m.CostPerCaseMicros)})
	return
}
