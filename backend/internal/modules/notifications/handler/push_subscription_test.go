package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestUpsertPushSubscriptionRejectsInternalEndpoints: the API POSTs to the
// stored endpoint on every push, so internal targets are refused (CF-153).
// Rejection happens before the service is touched (svc is nil).
func TestUpsertPushSubscriptionRejectsInternalEndpoints(t *testing.T) {
	h := New(nil)
	for _, endpoint := range []string{
		"http://fcm.googleapis.com/fcm/send/x",
		"https://169.254.169.254/latest/meta-data/",
		"https://127.0.0.1:8080/v1/internal/auth/users",
		"https://backend:8080/v1/x",
		"https://metadata.google.internal/computeMetadata/v1/",
		"https://[::1]/",
		"https://user:pw@push.example.com/",
		"file:///etc/passwd",
	} {
		w := httptest.NewRecorder()
		body := `{"endpoint":"` + endpoint + `","key_p256dh":"k","key_auth":"a"}`
		h.UpsertPushSubscription(w, pushRequest(http.MethodPost, "/v1/notifications/push-subscriptions", body, 7))
		if w.Code != http.StatusUnprocessableEntity && w.Code != http.StatusBadRequest {
			t.Fatalf("%s: status %d, want a validation error", endpoint, w.Code)
		}
		if !strings.Contains(w.Body.String(), "endpoint") {
			t.Fatalf("%s: body %s, want an endpoint field error", endpoint, w.Body.String())
		}
	}
}
