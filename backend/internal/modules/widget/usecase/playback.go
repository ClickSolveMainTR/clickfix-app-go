package usecase

import (
	"context"
	"io"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type ManifestChunk struct {
	Sequence int32  `json:"sequence"`
	URL      string `json:"url"`
}
type Manifest struct {
	Session   Session         `json:"session"`
	Chunks    []ManifestChunk `json:"chunks"`
	ExpiresAt time.Time       `json:"expires_at"`
}

func (s *Service) Manifest(ctx context.Context, row db.WidgetSession) (Manifest, error) {
	out := Manifest{Session: sessionModel(row), Chunks: []ManifestChunk{}, ExpiresAt: s.now().Add(2 * time.Minute)}
	if row.Kind != "replay" || row.Status == "revoked" || row.Status == "declined" || !row.ExpiresAt.Time.After(s.now()) || len(s.replaySecret) < 24 {
		return out, ErrNotFound
	}
	if row.ExpiresAt.Time.Before(out.ExpiresAt) {
		out.ExpiresAt = row.ExpiresAt.Time
	}
	chunks, err := s.q.ListWidgetReplayChunks(ctx, db.ListWidgetReplayChunksParams{OrganizationID: row.OrganizationID, SessionID: row.ID})
	if err != nil {
		return out, err
	}
	for _, ch := range chunks {
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"org": row.OrganizationID, "sub": ch.Uuid.String(), "aud": "widget-replay", "exp": out.ExpiresAt.Unix()}).SignedString(s.replaySecret)
		if err != nil {
			return out, err
		}
		out.Chunks = append(out.Chunks, ManifestChunk{Sequence: ch.Sequence, URL: "/api/v1/public/widget-replays/" + token})
	}
	if err = s.auditSession(ctx, s.q, row, "playback"); err != nil {
		return out, err
	}
	return out, nil
}
func (s *Service) ReplayDownload(ctx context.Context, token string) (io.ReadCloser, int64, error) {
	var claims struct {
		Org int64 `json:"org"`
		jwt.RegisteredClaims
	}
	_, err := jwt.ParseWithClaims(token, &claims, func(_ *jwt.Token) (any, error) { return s.replaySecret, nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithAudience("widget-replay"), jwt.WithExpirationRequired())
	if err != nil || len(s.replaySecret) < 24 {
		return nil, 0, ErrNotFound
	}
	id, err := uuid.Parse(claims.Subject)
	if err != nil {
		return nil, 0, ErrNotFound
	}
	ch, err := s.q.GetWidgetReplayChunk(ctx, db.GetWidgetReplayChunkParams{OrganizationID: claims.Org, Uuid: id})
	if err != nil || s.replayStore == nil {
		return nil, 0, ErrNotFound
	}
	return s.replayStore.Download(ctx, ch.ObjectKey)
}
