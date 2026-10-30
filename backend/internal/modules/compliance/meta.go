package compliance

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/compliance/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/jackc/pgx/v5"
)

// metaSubject authenticates the original encoded payload, never re-encoded JSON.
func metaSubject(signed, secret string) (string, error) {
	invalid := errors.New("invalid signed_request")
	sig, payload, ok := strings.Cut(signed, ".")
	if !ok || secret == "" || strings.Contains(payload, ".") {
		return "", invalid
	}
	decode := func(s string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "=")) }
	signature, err := decode(sig)
	if err != nil {
		return "", invalid
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return "", invalid
	}
	raw, err := decode(payload)
	if err != nil {
		return "", invalid
	}
	var in struct {
		Algorithm string `json:"algorithm"`
		UserID    string `json:"user_id"`
	}
	if json.Unmarshal(raw, &in) != nil || in.Algorithm != "HMAC-SHA256" || len(in.UserID) == 0 || len(in.UserID) > 320 {
		return "", invalid
	}
	for _, r := range in.UserID {
		if r < '0' || r > '9' {
			return "", invalid
		}
	}
	return in.UserID, nil
}

// RegisterMetaRoutes is public: Meta's app-specific HMAC supplies authentication.
// These routes remain available even while messaging is gated for App Review.
func RegisterMetaRoutes(mux *http.ServeMux, s *usecase.Service, metaSecret, instagramSecret string) {
	for _, app := range []string{"facebook", "instagram"} {
		secret := metaSecret
		if app == "instagram" {
			secret = instagramSecret
		}
		for _, action := range []string{"deauthorize", "data-deletion"} {
			mux.HandleFunc("POST /v1/webhooks/meta/"+app+"/"+action, func(w http.ResponseWriter, r *http.Request) {
				if secret == "" {
					response.Error(w, r, http.StatusServiceUnavailable, "META_NOT_CONFIGURED", "Meta callback is not configured")
					return
				}
				r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
				if err := r.ParseForm(); err != nil {
					response.BadRequest(w, r, "INVALID_SIGNED_REQUEST", "Invalid signed_request")
					return
				}
				subject, err := metaSubject(r.PostForm.Get("signed_request"), secret)
				if err != nil {
					response.BadRequest(w, r, "INVALID_SIGNED_REQUEST", "Invalid signed_request")
					return
				}
				erase := action == "data-deletion"
				code, err := s.MetaCallback(r.Context(), app, subject, erase)
				if err != nil {
					response.Error(w, r, http.StatusServiceUnavailable, "META_CALLBACK_UNAVAILABLE", "Please retry the callback")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				// Meta requires this exact top-level shape, outside the API envelope.
				if erase {
					_ = json.NewEncoder(w).Encode(map[string]string{"url": "https://getclickfix.com/account-deletion?code=" + code, "confirmation_code": code})
				} else {
					_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
				}
			})
		}
	}
	mux.HandleFunc("GET /v1/public/meta/data-deletion/{code}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		code := r.PathValue("code")
		if len(code) != 32 {
			response.NotFound(w, r, "Deletion request not found")
			return
		}
		status, err := s.Repo.Q.MetaDeletionStatus(r.Context(), code)
		if errors.Is(err, pgx.ErrNoRows) {
			response.NotFound(w, r, "Deletion request not found")
			return
		}
		if err != nil {
			response.Error(w, r, http.StatusServiceUnavailable, "META_CALLBACK_UNAVAILABLE", "Please retry")
			return
		}
		response.JSON(w, r, http.StatusOK, map[string]string{"confirmation_code": status.Code, "status": status.Status})
	})
}
