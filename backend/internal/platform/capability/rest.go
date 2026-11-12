package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Endpoint declares a capability that is one /v1/tenant call (CF-211):
// most panel actions are a single route, so the module only names the
// route and which params go where. The route itself checks permissions,
// organization scope, validation and writes its audit record.
type Endpoint struct {
	Method string
	// Path is a /v1/tenant template; {key} is filled from the param of the
	// same name (path-escaped). Path params must be validated (UUID) by the
	// input schema.
	Path string
	// Query lists the params copied to the query string.
	Query []string
	// Body lists the params copied to the JSON body; nil = every param that
	// is not a path or query param (writes only).
	Body []string
	// Rename maps a param key to its wire name (body or query).
	Rename map[string]string
	// Fixed body fields (e.g. {"internal": true}).
	Fixed map[string]any
	// Summary is the result's one line ("" = method + path).
	Summary string
	// Route is where the user sees the result ({key} filled like Path).
	Route string
}

var placeholderRxp = regexp.MustCompile(`\{([a-z_]+)\}`)

// PathParams are the {keys} of a template.
func PathParams(template string) []string {
	out := []string{}
	for _, m := range placeholderRxp.FindAllStringSubmatch(template, -1) {
		out = append(out, m[1])
	}
	return out
}

func fill(template string, p Params) (string, error) {
	var missing string
	out := placeholderRxp.ReplaceAllStringFunc(template, func(m string) string {
		key := m[1 : len(m)-1]
		v := strings.TrimSpace(fmt.Sprint(p[key]))
		if p[key] == nil || v == "" {
			missing = key
			return ""
		}
		return url.PathEscape(v)
	})
	if missing != "" {
		return "", Invalid("%s is required: ask the user for it", missing)
	}
	return out, nil
}

func queryValue(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, strings.TrimSpace(x) != ""
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), true
	case int:
		return strconv.Itoa(x), true
	case int64:
		return strconv.FormatInt(x, 10), true
	case bool:
		return strconv.FormatBool(x), true
	case []any:
		parts := []string{}
		for _, e := range x {
			if s, ok := queryValue(e); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ","), len(parts) > 0
	}
	return "", false
}

func (ep Endpoint) wire(key string) string {
	if w, ok := ep.Rename[key]; ok {
		return w
	}
	return key
}

// Request builds the path (with query) and body of one call.
func (ep Endpoint) Request(p Params) (string, any, error) {
	path, err := fill(ep.Path, p)
	if err != nil {
		return "", nil, err
	}
	skip := map[string]bool{}
	for _, k := range PathParams(ep.Path) {
		skip[k] = true
	}
	q := url.Values{}
	for _, k := range ep.Query {
		skip[k] = true
		if v, ok := queryValue(p[k]); ok {
			q.Set(ep.wire(k), v)
		}
	}
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	if ep.Method == "GET" || (ep.Method == "DELETE" && ep.Body == nil && ep.Fixed == nil) {
		return path, nil, nil
	}
	body := map[string]any{}
	for k, v := range ep.Fixed {
		body[k] = v
	}
	keys := ep.Body
	if keys == nil {
		for k := range p {
			if !skip[k] {
				keys = append(keys, k)
			}
		}
	}
	for _, k := range keys {
		if v, ok := p[k]; ok && v != nil {
			body[ep.wire(k)] = v
		}
	}
	return path, body, nil
}

// Call runs the endpoint and returns its data, compacted for the model.
func (ep Endpoint) Call(ctx context.Context, e Exec, p Params) (Result, error) {
	path, body, err := ep.Request(p)
	if err != nil {
		return Result{}, err
	}
	r, err := e.API.Call(ctx, ep.Method, path, body)
	if err != nil {
		return Result{}, err
	}
	if err := r.Err(); err != nil {
		return Result{}, err
	}
	res := Result{Summary: ep.Summary, Data: json.RawMessage(r.Data)}
	if res.Summary == "" {
		res.Summary = ep.Method + " " + strings.SplitN(path, "?", 2)[0]
	}
	if ep.Route != "" {
		if route, err := fill(ep.Route, p); err == nil {
			res.Route = route
		}
	}
	return res, nil
}

// Do is Call as a Run/Apply function.
func (ep Endpoint) Do() func(ctx context.Context, e Exec, p Params) (Result, error) {
	return ep.Call
}
