package usecase

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/supportops"
	"github.com/google/uuid"
)

func (s *Service) signature(org int64, delivery, recipient uuid.UUID, expires int64) string {
	mac := hmac.New(sha256.New, []byte(s.Secret))
	_, _ = mac.Write([]byte("report-download:v1:" + strconv.FormatInt(org, 10) + ":" + delivery.String() + ":" + recipient.String() + ":" + strconv.FormatInt(expires, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Service) DownloadURL(org int64, delivery, recipient uuid.UUID, expires time.Time) string {
	q := url.Values{"org": {strconv.FormatInt(org, 10)}, "recipient": {recipient.String()}, "expires": {strconv.FormatInt(expires.Unix(), 10)}, "signature": {s.signature(org, delivery, recipient, expires.Unix())}}
	return strings.TrimRight(s.BaseURL, "/") + "/api/v1/public/storage/reports/" + delivery.String() + "?" + q.Encode()
}

func (s *Service) Download(ctx context.Context, org int64, delivery, recipient uuid.UUID, expires int64, signature string) (db.ReportsDownloadRow, error) {
	if s.Secret == "" || org < 1 || delivery == uuid.Nil || recipient == uuid.Nil || expires < time.Now().Unix() || expires > time.Now().Add(8*24*time.Hour).Unix() || !hmac.Equal([]byte(signature), []byte(s.signature(org, delivery, recipient, expires))) {
		return db.ReportsDownloadRow{}, supportops.ErrDenied
	}
	if _, err := s.permitted(ctx, org, recipient); err != nil {
		return db.ReportsDownloadRow{}, supportops.ErrDenied
	}
	return s.Repo.Q.ReportsDownload(ctx, db.ReportsDownloadParams{OrganizationID: org, Uuid: delivery})
}
