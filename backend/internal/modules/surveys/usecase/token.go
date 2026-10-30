package usecase

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/surveys/model"
	"github.com/google/uuid"
	"strings"
	"time"
)

type claims struct {
	Org     int64     `json:"o"`
	ID      uuid.UUID `json:"i"`
	Expires int64     `json:"e"`
}

func (s *Service) token(org int64, id uuid.UUID, expiry time.Time) string {
	body, _ := json.Marshal(claims{org, id, expiry.Unix()})
	enc := base64.RawURLEncoding.EncodeToString(body)
	return enc + "." + s.mac(enc)
}
func (s *Service) mac(enc string) string {
	h := hmac.New(sha256.New, []byte(s.secret))
	_, _ = h.Write([]byte("clickfix/survey-response/v1:" + enc))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
func (s *Service) verify(token string) (claims, error) {
	var c claims
	enc, sig, ok := strings.Cut(token, ".")
	if !ok || len(token) > 1024 || !hmac.Equal([]byte(sig), []byte(s.mac(enc))) {
		return c, model.ErrInvalid
	}
	b, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil || json.Unmarshal(b, &c) != nil || c.Org <= 0 || c.ID == uuid.Nil {
		return c, model.ErrInvalid
	}
	if time.Now().Unix() >= c.Expires {
		return c, model.ErrExpired
	}
	return c, nil
}
