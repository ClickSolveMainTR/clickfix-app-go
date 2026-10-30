package usecase

import (
	"context"
	"fmt"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/diagnostics"
	"github.com/google/uuid"
)

// Diagnostics applies the same org and team visibility as conversation detail.
func (s *Service) Diagnostics(ctx context.Context, org int64, id uuid.UUID) ([]diagnostics.Record, error) {
	conv, err := s.store.Q().GetConversationByUUID(ctx, db.GetConversationByUUIDParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return nil, notFound(err)
	}
	if err := s.mustSee(ctx, org, conv.ID); err != nil {
		return nil, err
	}
	rows, err := s.store.Q().ListWidgetDiagnostics(ctx, db.ListWidgetDiagnosticsParams{OrganizationID: org, ConversationID: conv.ID})
	if err != nil {
		return nil, err
	}
	out := make([]diagnostics.Record, 0, len(rows))
	for _, row := range rows {
		v := diagnostics.Model(row)
		if v.AttachmentID != nil {
			link := fmt.Sprintf(attachmentDownloadPath, *v.AttachmentID)
			v.DownloadURL = &link
		}
		out = append(out, v)
	}
	return out, nil
}
