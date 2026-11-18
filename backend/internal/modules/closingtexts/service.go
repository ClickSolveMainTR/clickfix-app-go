// Package closingtexts manages the closing texts agents append to their
// replies (CF-250): an organization-wide or brand list with Turkish and
// English bodies, one default per organization and brand, and whether
// autonomous AI replies get the default too. Appending happens in the inbox
// core (conversations/usecase/closing.go).
package closingtexts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	convusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Limits of a closing text.
const (
	MaxName = 120
	MaxBody = 2000
	MaxList = 100
)

// Input creates or replaces a closing text.
type Input struct {
	Name      string     `json:"name"`
	BodyTr    string     `json:"body_tr"`
	BodyEn    string     `json:"body_en"`
	IsDefault bool       `json:"is_default"`
	Position  int32      `json:"position"`
	BrandUUID *uuid.UUID `json:"brand_uuid"`
}

// ClosingText is the API shape of a closing text.
type ClosingText struct {
	UUID      uuid.UUID  `json:"uuid"`
	Name      string     `json:"name"`
	BodyTr    string     `json:"body_tr"`
	BodyEn    string     `json:"body_en"`
	IsDefault bool       `json:"is_default"`
	Position  int32      `json:"position"`
	BrandUUID *uuid.UUID `json:"brand_uuid"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// Settings are the organization's closing text settings.
type Settings struct {
	ApplyToAI bool `json:"apply_to_ai"`
}

// List is the settings screen's payload.
type List struct {
	Items     []ClosingText `json:"items"`
	Settings  Settings      `json:"settings"`
	Variables []string      `json:"variables"`
}

// Service manages closing texts.
type Service struct {
	Pool *pgxpool.Pool
	Q    *db.Queries
}

func (in *Input) normalize() error {
	in.Name = strings.TrimSpace(in.Name)
	in.BodyTr = strings.TrimSpace(in.BodyTr)
	in.BodyEn = strings.TrimSpace(in.BodyEn)
	switch {
	case in.Name == "" || utf8.RuneCountInString(in.Name) > MaxName:
		return fmt.Errorf("%w: name must be 1-%d characters", supportops.ErrInvalid, MaxName)
	case in.BodyTr == "" || utf8.RuneCountInString(in.BodyTr) > MaxBody:
		return fmt.Errorf("%w: body_tr must be 1-%d characters", supportops.ErrInvalid, MaxBody)
	case utf8.RuneCountInString(in.BodyEn) > MaxBody:
		return fmt.Errorf("%w: body_en is too long", supportops.ErrInvalid)
	case in.Position < 0 || in.Position > 10000:
		return fmt.Errorf("%w: position must be 0-10000", supportops.ErrInvalid)
	case in.BrandUUID != nil && *in.BrandUUID == uuid.Nil:
		in.BrandUUID = nil
	}
	return nil
}

// List returns the organization's closing texts and settings.
func (s *Service) List(ctx context.Context, org int64) (List, error) {
	rows, err := s.Q.ListClosingTexts(ctx, org)
	if err != nil {
		return List{}, err
	}
	out := List{Items: make([]ClosingText, 0, len(rows)), Variables: convusecase.ClosingVariables}
	for _, r := range rows {
		out.Items = append(out.Items, of(r))
	}
	set, err := s.Q.GetClosingTextSettings(ctx, org)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return List{}, err
	}
	out.Settings.ApplyToAI = set.ApplyToAi
	return out, nil
}

// Get returns one closing text.
func (s *Service) Get(ctx context.Context, org int64, id uuid.UUID) (ClosingText, error) {
	r, err := s.Q.GetClosingText(ctx, db.GetClosingTextParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return ClosingText{}, err
	}
	return of(r), nil
}

// Save creates (id == uuid.Nil) or replaces a closing text. Marking it the
// default drops the previous default of the same organization/brand.
func (s *Service) Save(ctx context.Context, org, actor int64, id uuid.UUID, in Input) (ClosingText, error) {
	if err := in.normalize(); err != nil {
		return ClosingText{}, err
	}
	var brand pgtype.UUID
	if in.BrandUUID != nil {
		if _, err := s.Q.GetBrand(ctx, db.GetBrandParams{OrganizationID: org, Uuid: *in.BrandUUID}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ClosingText{}, fmt.Errorf("%w: unknown brand", supportops.ErrInvalid)
			}
			return ClosingText{}, err
		}
		brand = pgtype.UUID{Bytes: *in.BrandUUID, Valid: true}
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ClosingText{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.Q.WithTx(tx)
	var row db.ClosingText
	if id == uuid.Nil {
		n, err := q.ListClosingTexts(ctx, org)
		if err != nil {
			return ClosingText{}, err
		}
		if len(n) >= MaxList {
			return ClosingText{}, fmt.Errorf("%w: at most %d closing texts", supportops.ErrConflict, MaxList)
		}
		row, err = q.InsertClosingText(ctx, db.InsertClosingTextParams{
			OrganizationID: org, BrandID: brand, Name: in.Name, BodyTr: in.BodyTr, BodyEn: in.BodyEn,
			IsDefault: false, Position: in.Position, CreatedBy: pgtype.Int8{Int64: actor, Valid: actor > 0},
		})
		if err != nil {
			return ClosingText{}, err
		}
	} else {
		cur, err := q.GetClosingText(ctx, db.GetClosingTextParams{OrganizationID: org, Uuid: id})
		if err != nil {
			return ClosingText{}, err
		}
		row, err = q.UpdateClosingText(ctx, db.UpdateClosingTextParams{
			BrandID: brand, Name: in.Name, BodyTr: in.BodyTr, BodyEn: in.BodyEn, IsDefault: false,
			Position: in.Position, OrganizationID: org, ID: cur.ID,
		})
		if err != nil {
			return ClosingText{}, err
		}
	}
	if in.IsDefault {
		if err := q.ClearClosingTextDefault(ctx, db.ClearClosingTextDefaultParams{OrganizationID: org, KeepID: row.ID, BrandID: brand}); err != nil {
			return ClosingText{}, err
		}
		row, err = q.UpdateClosingText(ctx, db.UpdateClosingTextParams{
			BrandID: brand, Name: row.Name, BodyTr: row.BodyTr, BodyEn: row.BodyEn, IsDefault: true,
			Position: row.Position, OrganizationID: org, ID: row.ID,
		})
		if err != nil {
			return ClosingText{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ClosingText{}, err
	}
	return of(row), nil
}

// Delete removes a closing text. Agents who last picked it fall back to the
// default.
func (s *Service) Delete(ctx context.Context, org int64, id uuid.UUID) error {
	cur, err := s.Q.GetClosingText(ctx, db.GetClosingTextParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return err
	}
	_, err = s.Q.DeleteClosingText(ctx, db.DeleteClosingTextParams{OrganizationID: org, ID: cur.ID})
	return err
}

// SaveSettings stores the organization's settings.
func (s *Service) SaveSettings(ctx context.Context, org int64, in Settings) (Settings, error) {
	row, err := s.Q.UpsertClosingTextSettings(ctx, db.UpsertClosingTextSettingsParams{OrganizationID: org, ApplyToAi: in.ApplyToAI})
	if err != nil {
		return Settings{}, err
	}
	return Settings{ApplyToAI: row.ApplyToAi}, nil
}

func of(r db.ClosingText) ClosingText {
	out := ClosingText{UUID: r.Uuid, Name: r.Name, BodyTr: r.BodyTr, BodyEn: r.BodyEn, IsDefault: r.IsDefault,
		Position: r.Position, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
	if r.BrandID.Valid {
		b := uuid.UUID(r.BrandID.Bytes)
		out.BrandUUID = &b
	}
	return out
}
