package usecase

import (
	"context"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/attachments/model"
)

// ScanInput is what a Scanner inspects before an upload is stored.
type ScanInput struct {
	Filename    string
	ContentType string
	SHA256      string
	Data        []byte
}

// ScanResult is the verdict of a Scanner.
type ScanResult struct {
	// Status is model.ScanClean, ScanInfected or ScanSkipped.
	Status string
	// Detail names the signature for an infected file (stored, never shown
	// to the uploader beyond a generic message).
	Detail string
}

// Scanner is the virus-scan hook point. An implementation (ClamAV over
// clamd, a cloud scanner) is injected with Service.WithScanner. A returned
// error fails the upload closed: nothing is stored when the verdict is
// unknown.
type Scanner interface {
	Scan(ctx context.Context, in ScanInput) (ScanResult, error)
}

// NoopScanner accepts every file and records that no scan happened. It is the
// default until a real scanner is wired (CF-11 leaves ClamAV as a follow-up).
type NoopScanner struct{}

// Scan reports the file as skipped.
func (NoopScanner) Scan(context.Context, ScanInput) (ScanResult, error) {
	return ScanResult{Status: model.ScanSkipped}, nil
}
