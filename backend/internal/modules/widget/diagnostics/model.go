package diagnostics

import (
	"encoding/json"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/google/uuid"
	"time"
)

type Record struct {
	UUID         uuid.UUID       `json:"uuid"`
	Kind         string          `json:"kind"`
	AttachmentID *uuid.UUID      `json:"attachment_id"`
	DownloadURL  *string         `json:"download_url"`
	DurationMS   int32           `json:"duration_ms"`
	Context      json.RawMessage `json:"context"`
	ExpiresAt    time.Time       `json:"expires_at"`
	CreatedAt    time.Time       `json:"created_at"`
}

func Model(row db.WidgetDiagnostic) Record {
	var file *uuid.UUID
	if row.AttachmentID.Valid {
		v := uuid.UUID(row.AttachmentID.Bytes)
		file = &v
	}
	return Record{UUID: row.Uuid, Kind: row.Kind, AttachmentID: file, DurationMS: row.DurationMs, Context: row.Context, ExpiresAt: row.ExpiresAt.Time, CreatedAt: row.CreatedAt.Time}
}
