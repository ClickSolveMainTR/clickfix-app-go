// Package usecase implements organization-scoped file attachments on top of
// the platform object store.
package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/config"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/repository"
	platstorage "github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Errors returned by the service. The handler maps them to HTTP statuses.
var (
	ErrNotFound        = errors.New("attachments: not found")
	ErrInvalidRequest  = errors.New("attachments: invalid request")
	ErrTooLarge        = errors.New("attachments: file exceeds the size limit")
	ErrQuotaExceeded   = errors.New("attachments: storage quota exceeded")
	ErrUnsupportedType = errors.New("attachments: file type is not allowed")
	ErrInfected        = errors.New("attachments: file failed the security scan")
	ErrScanUnavailable = errors.New("attachments: security scan is unavailable")
)

// API-owned paths returned to clients; object keys never leave the server.
const (
	publicDownloadPath  = "/v1/public/attachments/%s"
	tenantDownloadPath  = "/v1/tenant/attachments/%s/download"
	tenantThumbnailPath = "/v1/tenant/attachments/%s/download?variant=thumb"
)

const (
	headSniffBytes = 512
	minSignedTTL   = 10 * time.Second
	maxSignedTTL   = time.Hour
	// defaultMaxFileBytes applies when no limit is configured.
	defaultMaxFileBytes = 25 << 20
)

// Limits are the size limits that apply to one organization right now.
type Limits struct {
	// MaxFileBytes is the largest single file.
	MaxFileBytes int64
	// MaxTotalBytes is the organization's total live bytes; 0 = unlimited.
	MaxTotalBytes int64
}

// LimitsProvider resolves the limits of an organization. The default is
// StaticLimits; the plan/entitlement module supplies a per-plan provider.
type LimitsProvider interface {
	LimitsFor(ctx context.Context, organizationID int64) (Limits, error)
}

// StaticLimits applies the same limits to every organization.
type StaticLimits Limits

// LimitsFor returns the static limits.
func (l StaticLimits) LimitsFor(context.Context, int64) (Limits, error) {
	return Limits(l), nil
}

// Repository is the persistence the service needs.
type Repository interface {
	InsertWithQuota(ctx context.Context, params db.InsertAttachmentParams, quotaBytes int64) (db.Attachment, error)
	Get(ctx context.Context, orgID int64, id uuid.UUID) (db.Attachment, error)
	List(ctx context.Context, list db.ListAttachmentsParams, count db.CountAttachmentsParams) ([]db.Attachment, int64, error)
	SoftDelete(ctx context.Context, orgID int64, id uuid.UUID) (db.Attachment, error)
	Usage(ctx context.Context, orgID int64) (db.SumAttachmentUsageRow, error)
	UserUUIDs(ctx context.Context, ids []int64) (map[int64]uuid.UUID, error)
}

// ObjectStore is the slice of the platform storage driver the service uses.
type ObjectStore interface {
	Upload(ctx context.Context, file platstorage.File, path string) error
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
	Delete(ctx context.Context, path string) error
}

// Service manages attachments.
type Service struct {
	repo    Repository
	store   ObjectStore
	limits  LimitsProvider
	scanner Scanner
	signer  *tokenSigner
	ttl     time.Duration
	log     *slog.Logger
	now     func() time.Time
}

// New builds the service. secret signs download URLs; cfg supplies the
// default limits (replace them with WithLimits).
func New(repo Repository, store ObjectStore, secret string, cfg config.AttachmentConfig, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	maxFile := cfg.MaxFileBytes
	if maxFile <= 0 {
		maxFile = defaultMaxFileBytes
	}
	ttl := cfg.SignedURLTTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	svc := &Service{
		repo:    repo,
		store:   store,
		limits:  StaticLimits{MaxFileBytes: maxFile, MaxTotalBytes: cfg.OrgQuotaBytes},
		scanner: NoopScanner{},
		ttl:     ttl,
		log:     log,
		now:     time.Now,
	}
	svc.signer = newTokenSigner(secret, func() time.Time { return svc.now() })
	return svc
}

// WithLimits swaps the limits provider (plan based entitlements).
func (s *Service) WithLimits(p LimitsProvider) *Service {
	if p != nil {
		s.limits = p
	}
	return s
}

// WithScanner swaps the virus scanner.
func (s *Service) WithScanner(sc Scanner) *Service {
	if sc != nil {
		s.scanner = sc
	}
	return s
}

// ValidOwnerType reports whether an attachment may be linked to the type.
func ValidOwnerType(t string) bool {
	switch t {
	case model.OwnerConversation, model.OwnerMessage, model.OwnerTicket, "billing_invoice":
		return true
	}
	return false
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidRequest, fmt.Sprintf(format, args...))
}

// Upload validates, scans and stores one file for an organization.
func (s *Service) Upload(ctx context.Context, in model.UploadInput) (model.Attachment, error) {
	if in.OrganizationID <= 0 || in.OrganizationUUID == uuid.Nil {
		return model.Attachment{}, invalid("organization is required")
	}
	if in.Body == nil {
		return model.Attachment{}, invalid("file is required")
	}
	if (in.OwnerType == "") != (in.OwnerID == nil) {
		return model.Attachment{}, invalid("owner_type and owner_id must be set together")
	}
	if in.OwnerType != "" && !ValidOwnerType(in.OwnerType) {
		return model.Attachment{}, invalid("owner_type %q is not supported", in.OwnerType)
	}
	filename := sanitizeFilename(in.Filename)
	if filename == "" {
		return model.Attachment{}, invalid("filename is required")
	}
	limits, err := s.limits.LimitsFor(ctx, in.OrganizationID)
	if err != nil {
		return model.Attachment{}, fmt.Errorf("attachments: limits: %w", err)
	}
	maxFile := limits.MaxFileBytes
	if maxFile <= 0 {
		maxFile = defaultMaxFileBytes
	}

	data, err := io.ReadAll(io.LimitReader(in.Body, maxFile+1))
	if err != nil {
		return model.Attachment{}, fmt.Errorf("attachments: read body: %w", err)
	}
	if int64(len(data)) > maxFile {
		return model.Attachment{}, fmt.Errorf("%w: limit is %d bytes", ErrTooLarge, maxFile)
	}
	if len(data) == 0 {
		return model.Attachment{}, invalid("file is empty")
	}

	head := data[:min(len(data), headSniffBytes)]
	if blockedExtensions[extOf(filename)] || looksExecutable(head) {
		return model.Attachment{}, fmt.Errorf("%w: executable files are not accepted", ErrUnsupportedType)
	}
	contentType := resolveContentType(filename, in.DeclaredType, head)
	if !isAllowedType(contentType) {
		return model.Attachment{}, fmt.Errorf("%w: %s", ErrUnsupportedType, contentType)
	}

	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])

	verdict, err := s.scanner.Scan(ctx, ScanInput{Filename: filename, ContentType: contentType, SHA256: digest, Data: data})
	if err != nil {
		s.log.Error("attachment_scan_failed", "organization_id", in.OrganizationID, "error", err)
		return model.Attachment{}, ErrScanUnavailable
	}
	scanStatus := verdict.Status
	switch scanStatus {
	case model.ScanInfected:
		s.log.Warn("attachment_infected", "organization_id", in.OrganizationID, "sha256", digest, "signature", verdict.Detail)
		return model.Attachment{}, ErrInfected
	case model.ScanClean, model.ScanSkipped:
	default:
		scanStatus = model.ScanSkipped
	}

	id := uuid.New()
	objectKey := platstorage.AttachmentObjectKey(in.OrganizationUUID.String(), id.String())
	if err := s.store.Upload(ctx, platstorage.File{
		Body:        bytes.NewReader(data),
		Size:        int64(len(data)),
		ContentType: contentType,
		Filename:    filename,
	}, objectKey); err != nil {
		return model.Attachment{}, fmt.Errorf("attachments: store object: %w", err)
	}

	var thumbKey pgtype.Text
	if thumb := buildThumbnail(contentType, data); thumb != nil {
		key := platstorage.AttachmentThumbKey(in.OrganizationUUID.String(), id.String())
		if err := s.store.Upload(ctx, platstorage.File{
			Body:        bytes.NewReader(thumb),
			Size:        int64(len(thumb)),
			ContentType: "image/jpeg",
			Filename:    "thumb.jpg",
		}, key); err != nil {
			s.log.Warn("attachment_thumbnail_failed", "attachment", id, "error", err)
		} else {
			thumbKey = pgtype.Text{String: key, Valid: true}
		}
	}

	params := db.InsertAttachmentParams{
		ExpiresAt: attachmentExpiry(in.ExpiresAt), Uuid: id,
		OrganizationID: in.OrganizationID,
		Filename:       filename,
		ContentType:    contentType,
		SizeBytes:      int64(len(data)),
		Sha256:         digest,
		ObjectKey:      objectKey,
		ThumbKey:       thumbKey,
		ScanStatus:     scanStatus,
	}
	if in.UploaderID > 0 {
		params.UploadedBy = pgtype.Int8{Int64: in.UploaderID, Valid: true}
	}
	if in.OwnerType != "" {
		params.OwnerType = pgtype.Text{String: in.OwnerType, Valid: true}
		params.OwnerID = pgtype.UUID{Bytes: *in.OwnerID, Valid: true}
	}
	if scanStatus == model.ScanClean {
		params.ScannedAt = pgtype.Timestamptz{Time: s.now(), Valid: true}
	}

	row, err := s.repo.InsertWithQuota(ctx, params, limits.MaxTotalBytes)
	if err != nil {
		s.deleteObjects(ctx, objectKey, thumbKey.String)
		if errors.Is(err, repository.ErrQuotaExceeded) {
			return model.Attachment{}, ErrQuotaExceeded
		}
		return model.Attachment{}, err
	}
	out, err := s.toModels(ctx, []db.Attachment{row})
	if err != nil {
		return model.Attachment{}, err
	}
	return out[0], nil
}

// Get returns one attachment of an organization.
func (s *Service) Get(ctx context.Context, orgID int64, id uuid.UUID) (model.Attachment, error) {
	row, err := s.repo.Get(ctx, orgID, id)
	if err != nil {
		return model.Attachment{}, mapRepoErr(err)
	}
	out, err := s.toModels(ctx, []db.Attachment{row})
	if err != nil {
		return model.Attachment{}, err
	}
	return out[0], nil
}

// List returns one page of an organization's attachments.
func (s *Service) List(ctx context.Context, f model.ListFilter) ([]model.Attachment, int64, error) {
	if f.OwnerID != nil && f.OwnerType == "" {
		return nil, 0, invalid("owner_type is required with owner_id")
	}
	if f.OwnerType != "" && !ValidOwnerType(f.OwnerType) {
		return nil, 0, invalid("owner_type %q is not supported", f.OwnerType)
	}
	var ownerType pgtype.Text
	var ownerID pgtype.UUID
	var q pgtype.Text
	if f.OwnerType != "" {
		ownerType = pgtype.Text{String: f.OwnerType, Valid: true}
	}
	if f.OwnerID != nil {
		ownerID = pgtype.UUID{Bytes: *f.OwnerID, Valid: true}
	}
	if f.Q != "" {
		q = pgtype.Text{String: f.Q, Valid: true}
	}
	sortField := f.SortField
	if sortField == "" {
		sortField = "created_at"
	}
	rows, total, err := s.repo.List(ctx,
		db.ListAttachmentsParams{
			OrganizationID: f.OrganizationID, OwnerType: ownerType, OwnerID: ownerID, Q: q,
			SortField: sortField, SortDesc: f.SortDesc, RowLimit: f.Limit, RowOffset: f.Offset,
		},
		db.CountAttachmentsParams{OrganizationID: f.OrganizationID, OwnerType: ownerType, OwnerID: ownerID, Q: q},
	)
	if err != nil {
		return nil, 0, err
	}
	items, err := s.toModels(ctx, rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Delete soft-deletes an attachment, frees its quota and removes the bytes.
func (s *Service) Delete(ctx context.Context, orgID int64, id uuid.UUID) (model.Attachment, error) {
	row, err := s.repo.SoftDelete(ctx, orgID, id)
	if err != nil {
		return model.Attachment{}, mapRepoErr(err)
	}
	s.deleteObjects(ctx, row.ObjectKey, row.ThumbKey.String)
	out, err := s.toModels(ctx, []db.Attachment{row})
	if err != nil {
		return model.Attachment{}, err
	}
	return out[0], nil
}

// MaxFileBytes returns the per-file limit that applies to an organization.
func (s *Service) MaxFileBytes(ctx context.Context, orgID int64) (int64, error) {
	l, err := s.limits.LimitsFor(ctx, orgID)
	if err != nil {
		return 0, fmt.Errorf("attachments: limits: %w", err)
	}
	if l.MaxFileBytes <= 0 {
		return defaultMaxFileBytes, nil
	}
	return l.MaxFileBytes, nil
}

// Usage reports quota usage and the active per-file limit.
func (s *Service) Usage(ctx context.Context, orgID int64) (model.Usage, error) {
	limits, err := s.limits.LimitsFor(ctx, orgID)
	if err != nil {
		return model.Usage{}, fmt.Errorf("attachments: limits: %w", err)
	}
	u, err := s.repo.Usage(ctx, orgID)
	if err != nil {
		return model.Usage{}, err
	}
	return model.Usage{
		UsedBytes: u.UsedBytes, FileCount: u.FileCount,
		QuotaBytes: limits.MaxTotalBytes, MaxFileBytes: limits.MaxFileBytes,
	}, nil
}

// Open streams an attachment (or its thumbnail) after the caller has been
// authorized for the organization.
func (s *Service) Open(ctx context.Context, orgID int64, id uuid.UUID, variant string) (model.OpenFile, error) {
	row, err := s.repo.Get(ctx, orgID, id)
	if err != nil {
		return model.OpenFile{}, mapRepoErr(err)
	}
	return s.open(ctx, row, variant)
}

func (s *Service) open(ctx context.Context, row db.Attachment, variant string) (model.OpenFile, error) {
	if row.ScanStatus == model.ScanInfected || row.ExpiresAt.Valid && !s.now().Before(row.ExpiresAt.Time) {
		return model.OpenFile{}, ErrNotFound
	}
	key, contentType := row.ObjectKey, row.ContentType
	switch variant {
	case "", model.VariantOriginal:
	case model.VariantThumbnail:
		if !row.ThumbKey.Valid {
			return model.OpenFile{}, ErrNotFound
		}
		key, contentType = row.ThumbKey.String, "image/jpeg"
	default:
		return model.OpenFile{}, invalid("variant %q is not supported", variant)
	}
	body, size, err := s.store.Download(ctx, key)
	if err != nil {
		s.log.Error("attachment_download_failed", "attachment", row.Uuid, "error", err)
		return model.OpenFile{}, ErrNotFound
	}
	items, err := s.toModels(ctx, []db.Attachment{row})
	if err != nil {
		_ = body.Close()
		return model.OpenFile{}, err
	}
	return model.OpenFile{Attachment: items[0], ContentType: contentType, Size: size, Body: body}, nil
}

// SignedURL issues a short-lived public link for one attachment. The link
// carries the organization and attachment in a signed token, so it needs no
// session (usable in <img src> and for hand-offs) and stops working at expiry.
func (s *Service) SignedURL(ctx context.Context, orgID int64, id uuid.UUID, variant string, ttl time.Duration) (model.SignedURL, error) {
	row, err := s.repo.Get(ctx, orgID, id)
	if err != nil {
		return model.SignedURL{}, mapRepoErr(err)
	}
	if row.ScanStatus == model.ScanInfected || row.ExpiresAt.Valid && !s.now().Before(row.ExpiresAt.Time) {
		return model.SignedURL{}, ErrNotFound
	}
	switch variant {
	case "":
		variant = model.VariantOriginal
	case model.VariantOriginal:
	case model.VariantThumbnail:
		if !row.ThumbKey.Valid {
			return model.SignedURL{}, ErrNotFound
		}
	default:
		return model.SignedURL{}, invalid("variant %q is not supported", variant)
	}
	if ttl <= 0 {
		ttl = s.ttl
	}
	ttl = max(minSignedTTL, min(ttl, maxSignedTTL))
	exp := s.now().Add(ttl)
	token := s.signer.sign(tokenClaims{Org: orgID, Item: id, Variant: variant, Exp: exp.Unix()})
	return model.SignedURL{URL: fmt.Sprintf(publicDownloadPath, token), ExpiresAt: exp.UTC()}, nil
}

// OpenSigned streams the attachment named by a signed URL token.
func (s *Service) OpenSigned(ctx context.Context, token string) (model.OpenFile, error) {
	claims, err := s.signer.verify(token)
	if err != nil {
		return model.OpenFile{}, ErrNotFound
	}
	return s.Open(ctx, claims.Org, claims.Item, claims.Variant)
}

func (s *Service) deleteObjects(ctx context.Context, keys ...string) {
	for _, key := range keys {
		if key == "" {
			continue
		}
		if err := s.store.Delete(ctx, key); err != nil {
			s.log.Warn("attachment_object_delete_failed", "key", key, "error", err)
		}
	}
}

func (s *Service) toModels(ctx context.Context, rows []db.Attachment) ([]model.Attachment, error) {
	seen := map[int64]struct{}{}
	var ids []int64
	for _, r := range rows {
		if r.UploadedBy.Valid {
			if _, ok := seen[r.UploadedBy.Int64]; !ok {
				seen[r.UploadedBy.Int64] = struct{}{}
				ids = append(ids, r.UploadedBy.Int64)
			}
		}
	}
	users, err := s.repo.UserUUIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]model.Attachment, 0, len(rows))
	for _, r := range rows {
		a := model.Attachment{
			UUID:         r.Uuid,
			Filename:     r.Filename,
			ContentType:  r.ContentType,
			SizeBytes:    r.SizeBytes,
			SHA256:       r.Sha256,
			ScanStatus:   r.ScanStatus,
			HasThumbnail: r.ThumbKey.Valid,
			DownloadURL:  fmt.Sprintf(tenantDownloadPath, r.Uuid),
			CreatedAt:    r.CreatedAt.Time,
		}
		if r.ThumbKey.Valid {
			u := fmt.Sprintf(tenantThumbnailPath, r.Uuid)
			a.ThumbnailURL = &u
		}
		if r.OwnerType.Valid {
			t := r.OwnerType.String
			a.OwnerType = &t
		}
		if r.OwnerID.Valid {
			o := uuid.UUID(r.OwnerID.Bytes)
			a.OwnerID = &o
		}
		if r.UploadedBy.Valid {
			if u, ok := users[r.UploadedBy.Int64]; ok {
				a.UploadedBy = &u
			}
		}
		out = append(out, a)
	}
	return out, nil
}

func mapRepoErr(err error) error {
	if errors.Is(err, repository.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

func extOf(name string) string {
	return strings.ToLower(filepath.Ext(name))
}

func attachmentExpiry(v *time.Time) pgtype.Timestamptz {
	if v == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *v, Valid: true}
}
