package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	attachmentmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/model"
	attachmentuc "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/usecase"
	convrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget/diagnostics"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/events"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/outbox"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const DiagnosticRetention = 30 * 24 * time.Hour
const MaxScreenshotBytes = 5 << 20
const MaxRecordingBytes = 10 << 20

var ErrDiagnosticQuota = errors.New("widget: diagnostic quota exceeded")

type CaptureFiles interface {
	Upload(context.Context, attachmentmodel.UploadInput) (attachmentmodel.Attachment, error)
	Delete(context.Context, int64, uuid.UUID) (attachmentmodel.Attachment, error)
}
type DiagnosticInput struct {
	Kind      string
	Context   diagnostics.Context
	Filename  string
	Body      io.Reader
	Consent   bool
	PIIMasked bool
}
type Diagnostic = diagnostics.Record

func (s *Service) Capture(ctx context.Context, w Widget, claims widget.Claims, id uuid.UUID, in DiagnosticInput) (Diagnostic, error) {
	if !in.Consent {
		return Diagnostic{}, fmt.Errorf("%w: explicit visitor consent required", ErrInvalid)
	}
	if !w.Settings.FileUpload {
		return Diagnostic{}, ErrUploadDisabled
	}
	conv, err := s.conversation(ctx, w, claims, id)
	if err != nil {
		return Diagnostic{}, err
	}
	if conv.Status == "closed" {
		return Diagnostic{}, ErrConversationClosed
	}
	clean, err := in.Context.Mask()
	if err != nil {
		return Diagnostic{}, fmt.Errorf("%w: technical context", ErrInvalid)
	}
	raw, err := json.Marshal(clean)
	if err != nil {
		return Diagnostic{}, err
	}
	var data []byte
	duration := 0
	filename := ""
	declared := ""
	switch in.Kind {
	case "technical_context":
		if in.Body != nil {
			return Diagnostic{}, ErrInvalid
		}
	case "screenshot", "recording":
		if !in.PIIMasked || in.Body == nil {
			return Diagnostic{}, fmt.Errorf("%w: masked capture required", ErrInvalid)
		}
		capBytes := int64(MaxScreenshotBytes)
		if in.Kind == "recording" {
			capBytes = MaxRecordingBytes
		}
		data, err = io.ReadAll(io.LimitReader(in.Body, capBytes+1))
		if err != nil {
			return Diagnostic{}, ErrInvalid
		}
		if int64(len(data)) > capBytes {
			return Diagnostic{}, ErrFileTooLarge
		}
		if in.Kind == "screenshot" {
			cfg, format, e := image.DecodeConfig(bytes.NewReader(data))
			if e != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 10000 || cfg.Height > 10000 || int64(cfg.Width)*int64(cfg.Height) > 40000000 || (format != "png" && format != "jpeg") {
				return Diagnostic{}, fmt.Errorf("%w: screenshot must be PNG or JPEG, at most 40 megapixels", ErrInvalid)
			}
			filename = "screenshot." + format
			declared = "image/" + format
		} else {
			duration, err = diagnostics.WebMDuration(data)
			if err != nil {
				return Diagnostic{}, fmt.Errorf("%w: recording must be WebM video up to 60 seconds", ErrInvalid)
			}
			filename = "recording.webm"
			declared = "video/webm"
		}
	default:
		return Diagnostic{}, ErrInvalid
	}
	var row db.WidgetDiagnostic
	var uploaded *uuid.UUID
	files, _ := s.files.(CaptureFiles)
	if len(data) > 0 && files == nil {
		return Diagnostic{}, ErrUploadDisabled
	}
	expires := s.now().Add(DiagnosticRetention)
	err = s.store.InTx(ctx, func(ctx context.Context, tx *convrepo.Tx) error {
		// Serialize the daily organization quota, then the per-conversation quota.
		if e := tx.Q.LockWidgetDiagnosticQuota(ctx, w.Instance.OrganizationID); e != nil {
			return e
		}
		locked, e := tx.Q.GetConversationByID(ctx, db.GetConversationByIDParams{OrganizationID: w.Instance.OrganizationID, ID: conv.ID})
		if e != nil {
			return e
		}
		if locked.Status == "closed" {
			return ErrConversationClosed
		}
		used, e := tx.Q.CountWidgetDiagnostics(ctx, db.CountWidgetDiagnosticsParams{OrganizationID: conv.OrganizationID, ConversationID: conv.ID})
		if e != nil {
			return e
		}
		daily, e := tx.Q.CountOrganizationDiagnosticsToday(ctx, conv.OrganizationID)
		if e != nil {
			return e
		}
		if used >= 100 || daily >= 1000 {
			return ErrDiagnosticQuota
		}
		var file pgtype.UUID
		if len(data) > 0 {
			org, e := tx.Q.GetOrganizationByID(ctx, conv.OrganizationID)
			if e != nil {
				return e
			}
			f, e := files.Upload(ctx, attachmentmodel.UploadInput{OrganizationID: conv.OrganizationID, OrganizationUUID: org.Uuid, OwnerType: attachmentmodel.OwnerConversation, OwnerID: &conv.Uuid, Filename: filename, DeclaredType: declared, ExpiresAt: &expires, Body: bytes.NewReader(data)})
			if e != nil {
				return captureFileError(e)
			}
			uploaded = &f.UUID
			file = pgtype.UUID{Bytes: f.UUID, Valid: true}
			if e := tx.Q.SetDiagnosticAttachmentExpiry(ctx, db.SetDiagnosticAttachmentExpiryParams{OrganizationID: conv.OrganizationID, Uuid: f.UUID, ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true}}); e != nil {
				return e
			}
		}
		// Attachment privacy guards lock the conversation on a separate connection.
		// Take our write lock after that upload commits to avoid a self-deadlock.
		locked, e = tx.Q.LockConversation(ctx, db.LockConversationParams{OrganizationID: conv.OrganizationID, ID: conv.ID})
		if e != nil {
			return e
		}
		if locked.Status == "closed" || locked.AnonymizedAt.Valid {
			return ErrConversationClosed
		}
		row, e = tx.Q.InsertWidgetDiagnostic(ctx, db.InsertWidgetDiagnosticParams{OrganizationID: conv.OrganizationID, ConversationID: conv.ID, Kind: in.Kind, AttachmentID: file, DurationMs: safeconv.ToInt32(duration), Context: raw, ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true}})
		if e != nil {
			return e
		}
		return outbox.EnqueueQueries(ctx, tx.Q, events.New("widget.diagnostic_created").WithOrganization(conv.OrganizationID).WithEntity("conversation", &conv.ID, &conv.Uuid).WithPayload(map[string]any{"diagnostic_uuid": row.Uuid, "kind": in.Kind}))
	})
	if err != nil {
		if uploaded != nil {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			if _, e := files.Delete(cleanup, conv.OrganizationID, *uploaded); e != nil {
				s.log.Warn("widget_capture_cleanup_failed", "error", e)
			}
		}
		return Diagnostic{}, err
	}
	s.notifyAgents(ctx, conv.OrganizationID, map[string]any{"type": "conversation.diagnostic_added", "conversation_uuid": conv.Uuid, "diagnostic_uuid": row.Uuid})
	return diagnostics.Model(row), nil
}
func captureFileError(err error) error {
	switch {
	case errors.Is(err, attachmentuc.ErrTooLarge):
		return ErrFileTooLarge
	case errors.Is(err, attachmentuc.ErrQuotaExceeded):
		return ErrDiagnosticQuota
	case errors.Is(err, attachmentuc.ErrUnsupportedType), errors.Is(err, attachmentuc.ErrInvalidRequest):
		return fmt.Errorf("%w: capture file", ErrInvalid)
	}
	return err
}
