// Package retention removes diagnostic binaries and context after the capture TTL.
package retention

import (
	"context"
	"fmt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
)

const Task = "app:widget:diagnostics-purge"

type ObjectStore interface {
	Delete(context.Context, string) error
}
type Service struct {
	Q     *db.Queries
	Store ObjectStore
}

func (s Service) Purge(ctx context.Context) error {
	rows, err := s.Q.ListExpiredDiagnosticAttachments(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := s.Store.Delete(ctx, row.ObjectKey); err != nil {
			return err
		}
		if row.ThumbKey.Valid {
			if err := s.Store.Delete(ctx, row.ThumbKey.String); err != nil {
				return err
			}
		}
		if _, err := s.Q.SoftDeleteAttachment(ctx, db.SoftDeleteAttachmentParams{OrganizationID: row.OrganizationID, Uuid: row.Uuid}); err != nil {
			return err
		}
	}
	if err := s.Q.PurgeExpiredWidgetDiagnosticContext(ctx); err != nil {
		return err
	}
	chunks, err := s.Q.ExpiredWidgetReplayChunks(ctx)
	if err != nil {
		return err
	}
	for _, ch := range chunks {
		driver, ok := s.Store.(storage.Driver)
		if !ok {
			return fmt.Errorf("replay retention requires version-aware storage")
		}
		if err := storage.PurgeObject(ctx, driver, ch.ObjectKey); err != nil {
			return err
		}
		if err := s.Q.DeleteWidgetReplayChunk(ctx, db.DeleteWidgetReplayChunkParams{OrganizationID: ch.OrganizationID, Uuid: ch.Uuid}); err != nil {
			return err
		}
	}
	return s.Q.PurgeWidgetSessions(ctx)
}
