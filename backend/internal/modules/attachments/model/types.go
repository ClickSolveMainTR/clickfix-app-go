// Package model holds the attachment DTOs shared by the handler and usecase.
package model

import (
	"io"
	"time"

	"github.com/google/uuid"
)

// Scan statuses stored on an attachment.
const (
	ScanPending  = "pending"
	ScanClean    = "clean"
	ScanInfected = "infected"
	ScanSkipped  = "skipped"
	ScanError    = "error"
)

// Variants of a downloadable attachment.
const (
	VariantOriginal  = "original"
	VariantThumbnail = "thumb"
)

// Owner types an attachment may be linked to. New owner modules (messages,
// notes, ...) register their type here so ownerless typos are rejected.
const (
	OwnerConversation = "conversation"
	OwnerMessage      = "message"
	OwnerTicket       = "ticket"
)

// Attachment is the API representation of a stored file. URLs are API-owned
// paths; the browser never sees an object key or a bucket URL.
type Attachment struct {
	UUID         uuid.UUID  `json:"uuid"`
	Filename     string     `json:"filename"`
	ContentType  string     `json:"content_type"`
	SizeBytes    int64      `json:"size_bytes"`
	SHA256       string     `json:"sha256"`
	OwnerType    *string    `json:"owner_type"`
	OwnerID      *uuid.UUID `json:"owner_id"`
	UploadedBy   *uuid.UUID `json:"uploaded_by"`
	ScanStatus   string     `json:"scan_status"`
	HasThumbnail bool       `json:"has_thumbnail"`
	DownloadURL  string     `json:"download_url"`
	ThumbnailURL *string    `json:"thumbnail_url"`
	CreatedAt    time.Time  `json:"created_at"`
}

// UploadInput describes one file to store for an organization.
type UploadInput struct {
	ExpiresAt        *time.Time
	OrganizationID   int64
	OrganizationUUID uuid.UUID
	UploaderID       int64
	Filename         string
	DeclaredType     string
	OwnerType        string
	OwnerID          *uuid.UUID
	Body             io.Reader
}

// ListFilter narrows an attachment listing. OwnerType and OwnerID are
// optional but OwnerID requires OwnerType.
type ListFilter struct {
	OrganizationID int64
	OwnerType      string
	OwnerID        *uuid.UUID
	Q              string
	SortField      string
	SortDesc       bool
	Limit          int32
	Offset         int32
}

// Usage reports how much of the organization quota is used.
type Usage struct {
	UsedBytes    int64 `json:"used_bytes"`
	FileCount    int64 `json:"file_count"`
	QuotaBytes   int64 `json:"quota_bytes"`
	MaxFileBytes int64 `json:"max_file_bytes"`
}

// SignedURL is a short-lived public link to one attachment.
type SignedURL struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

// OpenFile is an attachment body ready to stream.
type OpenFile struct {
	Attachment  Attachment
	ContentType string
	Size        int64
	Body        io.ReadCloser
}
