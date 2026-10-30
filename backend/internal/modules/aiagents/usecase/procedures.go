package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Procedure is a versioned organization preference. Its text is untrusted data
// to the model; publishing cannot grant tool permissions or override guardrails.
type Procedure struct {
	Intent   string   `json:"intent"`
	Locale   string   `json:"locale"`
	Title    string   `json:"title"`
	Steps    []string `json:"steps"`
	Tools    []string `json:"allowed_tools"`
	Revision int32    `json:"revision"`
}

func (p Procedure) validate() error {
	if len(strings.TrimSpace(p.Intent)) == 0 || len(p.Intent) > 100 || len(strings.TrimSpace(p.Title)) == 0 || len(p.Title) > 200 || len(p.Steps) == 0 || len(p.Steps) > 20 || len(p.Tools) > 20 {
		return model.ErrInvalid
	}
	if p.Locale != "tr" && p.Locale != "en" {
		return model.ErrInvalid
	}
	size := 0
	for _, step := range p.Steps {
		if len(strings.TrimSpace(step)) == 0 || len(step) > 1000 {
			return model.ErrInvalid
		}
		size += len(step)
	}
	if size > 3000 {
		return model.ErrInvalid
	}
	for _, tool := range p.Tools {
		if !toolName.MatchString(tool) {
			return model.ErrInvalid
		}
	}
	return nil
}
func (s *Service) SaveProcedure(ctx context.Context, org int64, id uuid.UUID, in Procedure) (db.AiProcedure, error) {
	if err := in.validate(); err != nil {
		return db.AiProcedure{}, err
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return db.AiProcedure{}, err
	}
	if id == uuid.Nil {
		row, err := s.Repo.Q.AIProcedureCreate(ctx, db.AIProcedureCreateParams{OrganizationID: org, Intent: in.Intent, Locale: in.Locale, Title: in.Title, Draft: raw})
		var conflict *pgconn.PgError
		if errors.As(err, &conflict) && conflict.Code == "23505" {
			return row, model.ErrConflict
		}
		return row, err
	}
	row, err := s.Repo.Q.AIProcedureUpdate(ctx, db.AIProcedureUpdateParams{OrganizationID: org, Uuid: id, Title: in.Title, Draft: raw, Revision: in.Revision, Intent: in.Intent, Locale: in.Locale})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, model.ErrConflict
	}
	return row, err
}

func (s *Service) procedure(ctx context.Context, org int64, conversation uuid.UUID, message int64, locale string) (*Procedure, error) {
	jobs, err := s.Repo.Q.AIAgentJobs(ctx, db.AIAgentJobsParams{OrganizationID: org, Uuid: conversation, Limit: 20})
	if err != nil {
		return nil, err
	}
	for _, job := range jobs {
		if job.MessageID != message || job.Task != "triage" || job.Status != "completed" {
			continue
		}
		var triage model.Triage
		if err = json.Unmarshal(job.Result, &triage); err != nil {
			return nil, err
		}
		row, err := s.Repo.Q.AIProcedurePublished(ctx, db.AIProcedurePublishedParams{OrganizationID: org, Intent: triage.Intent, Locale: locale})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		var p Procedure
		if err = json.Unmarshal(row.Published, &p); err != nil {
			return nil, err
		}
		return &p, nil
	}
	return nil, nil
}
