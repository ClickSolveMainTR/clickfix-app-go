// Package repository owns AI conversation workflow persistence.
package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	billingrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Store struct{ *billingrepo.Repository }

func (s *Store) Config(ctx context.Context, org int64) (model.Config, int32, error) {
	row, err := s.Q.AIAgentConfig(ctx, org)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.DefaultConfig(), 0, nil
	}
	if err != nil {
		return model.Config{}, 0, err
	}
	var out model.Config
	if err = json.Unmarshal(row.Config, &out); err != nil {
		return out, 0, err
	}
	return out, row.Revision, nil
}
func (s *Store) SaveConfig(ctx context.Context, org, actor int64, in model.Config) (int32, error) {
	if err := in.Validate(); err != nil {
		return 0, err
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return 0, err
	}
	row, err := s.Q.AIAgentSaveConfig(ctx, db.AIAgentSaveConfigParams{OrganizationID: org, Config: raw, UpdatedBy: pgtype.Int8{Int64: actor, Valid: actor > 0}})
	return row.Revision, err
}
