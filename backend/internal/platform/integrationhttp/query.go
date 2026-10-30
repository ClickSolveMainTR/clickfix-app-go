// Package integrationhttp holds HTTP validation shared by developer integration lists.
package integrationhttp

import (
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/apiquery"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"net/http"
)

// Query accepts bounded pagination and optionally text search. Lists with a
// fixed order reject a sort parameter rather than silently ignoring it.
func Query(w http.ResponseWriter, r *http.Request, search bool) (apiquery.Query, bool) {
	q := apiquery.Parse(r.URL.Query())
	details := []response.Detail{}
	if len(q.Sort) > 0 {
		details = append(details, response.Detail{Field: "sort", Code: "unsupported", Message: "This list has a fixed order"})
	}
	if !search && q.Q != "" {
		details = append(details, response.Detail{Field: "q", Code: "unsupported", Message: "This list does not support text search"})
	}
	for _, name := range []string{"page", "page_size", "per_page", "search"} {
		if r.URL.Query().Has(name) {
			details = append(details, response.Detail{Field: name, Code: "unsupported", Message: "Use limit, offset and q"})
		}
	}
	if len(details) > 0 {
		response.ValidationError(w, r, details)
		return q, false
	}
	return q, true
}
