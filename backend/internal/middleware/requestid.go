package middleware

import (
	"context"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/reqid"
	"net/http"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/logging"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

// RequestIDHeader is the correlation id HTTP header.
const RequestIDHeader = "X-Request-ID"

func init() {
	response.RequestIDFunc = GetRequestID
	logging.RequestIDFromContext = GetRequestID
}

// RequestID ensures every request has a correlation id in context and response headers.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(RequestIDHeader)
		if !reqid.Valid(id) {
			id = uuid.NewString()
		}
		w.Header().Set(RequestIDHeader, id)
		ctx := reqid.With(r.Context(), id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID extracts the request id from context.
func GetRequestID(ctx context.Context) string {
	return reqid.From(ctx)
}
