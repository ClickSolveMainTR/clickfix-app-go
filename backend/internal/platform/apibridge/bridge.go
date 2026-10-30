// Package apibridge routes integrations through the same authenticated API as the panel.
// Ported from takwim-app-go internal/modules/mcp/bridge.go (staff path).
package apibridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/google/uuid"
	"io"
	"net/http"
)

const MaxResultBytes = 256 << 10

type Bridge struct {
	Handler http.Handler
	Tokens  *jwt.Manager
}
type Principal struct {
	UserUUID         uuid.UUID
	OrganizationUUID uuid.UUID
}
type Result struct {
	Status    int
	Body      []byte
	Truncated bool
}

func (b *Bridge) Do(ctx context.Context, p Principal, method, path string, body []byte) (Result, error) {
	if b == nil || b.Handler == nil || b.Tokens == nil {
		return Result{}, fmt.Errorf("API bridge is unavailable")
	}
	tok, _, err := b.Tokens.IssueAccess(jwt.AccessInput{UserID: p.UserUUID})
	if err != nil {
		return Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, method, path, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("X-Organization", p.OrganizationUUID.String())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ClickFix-Integration/1")
	w := &capture{header: http.Header{}, status: 200}
	b.Handler.ServeHTTP(w, req)
	return Result{Status: w.status, Body: w.buf.Bytes(), Truncated: w.truncated}, nil
}
func (b *Bridge) JSON(ctx context.Context, p Principal, method, path string, body any) (Result, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return Result{}, err
	}
	return b.Do(ctx, p, method, path, raw)
}

type capture struct {
	header    http.Header
	status    int
	wrote     bool
	buf       bytes.Buffer
	truncated bool
}

func (c *capture) Header() http.Header { return c.header }
func (c *capture) WriteHeader(s int) {
	if !c.wrote {
		c.status = s
		c.wrote = true
	}
}
func (c *capture) Write(p []byte) (int, error) {
	c.wrote = true
	n := min(len(p), max(0, MaxResultBytes-c.buf.Len()))
	c.buf.Write(p[:n])
	c.truncated = c.truncated || n < len(p)
	return len(p), nil
}

var _ io.Writer = (*capture)(nil)
