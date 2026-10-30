package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
)

const (
	apiTimeout    = 30 * time.Second
	uploadTimeout = 2 * time.Minute
)

// APIError is an unsuccessful Bot API response.
type APIError struct {
	Method      string
	Code        int
	Description string
	// RetryAfter is parameters.retry_after (seconds) of a 429.
	RetryAfter int
	// MigrateToChatID is set when a group was upgraded to a supergroup.
	MigrateToChatID int64
}

func (e *APIError) Error() string {
	return fmt.Sprintf("telegram: %s: %d %s", e.Method, e.Code, e.Description)
}

type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	ErrorCode   int             `json:"error_code"`
	Description string          `json:"description"`
	Parameters  *struct {
		RetryAfter      int   `json:"retry_after"`
		MigrateToChatID int64 `json:"migrate_to_chat_id"`
	} `json:"parameters"`
}

func (a *Adapter) methodURL(token, method string) string {
	return a.baseURL + "/bot" + token + "/" + method
}

// call POSTs params as JSON and decodes result into out (nil = ignore).
func (a *Adapter) call(ctx context.Context, token, method string, params map[string]any, out any) error {
	if params == nil {
		params = map[string]any{}
	}
	body, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("telegram: %s: encode: %w", method, err)
	}
	ctx, cancel := context.WithTimeout(ctx, apiTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.methodURL(token, method), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram: %s: %w", method, redact(err))
	}
	req.Header.Set("Content-Type", "application/json")
	return a.do(req, method, out)
}

// upload sends params plus one file as multipart/form-data.
func (a *Adapter) upload(ctx context.Context, token, method string, params map[string]any, field string, att channels.OutboundAttachment, out any) error {
	rc, err := att.Open(ctx)
	if err != nil {
		return fmt.Errorf("telegram: %s: open attachment: %w", method, err)
	}
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		defer func() { _ = rc.Close() }()
		err := func() error {
			for k, v := range params {
				if err := mw.WriteField(k, paramString(v)); err != nil {
					return err
				}
			}
			h := textproto.MIMEHeader{}
			name := att.Filename
			if name == "" {
				name = "file"
			}
			h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, field, quoteEscaper.Replace(name)))
			ct := att.ContentType
			if ct == "" {
				ct = "application/octet-stream"
			}
			h.Set("Content-Type", ct)
			fw, err := mw.CreatePart(h)
			if err != nil {
				return err
			}
			if _, err := io.Copy(fw, rc); err != nil {
				return err
			}
			return mw.Close()
		}()
		pw.CloseWithError(err)
	}()
	ctx, cancel := context.WithTimeout(ctx, uploadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.methodURL(token, method), pr)
	if err != nil {
		pr.CloseWithError(err)
		return fmt.Errorf("telegram: %s: %w", method, redact(err))
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return a.do(req, method, out)
}

var quoteEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\r", "", "\n", "")

func paramString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case bool:
		return strconv.FormatBool(t)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func (a *Adapter) do(req *http.Request, method string, out any) error {
	resp, err := a.hc.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: %s: %w", method, redact(err))
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("telegram: %s: read: %w", method, redact(err))
	}
	var r apiResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return &APIError{Method: method, Code: resp.StatusCode, Description: http.StatusText(resp.StatusCode)}
	}
	if !r.OK {
		ae := &APIError{Method: method, Code: r.ErrorCode, Description: r.Description}
		if ae.Code == 0 {
			ae.Code = resp.StatusCode
		}
		if r.Parameters != nil {
			ae.RetryAfter = r.Parameters.RetryAfter
			ae.MigrateToChatID = r.Parameters.MigrateToChatID
		}
		return ae
	}
	if out != nil && len(r.Result) > 0 {
		if err := json.Unmarshal(r.Result, out); err != nil {
			return fmt.Errorf("telegram: %s: decode result: %w", method, err)
		}
	}
	return nil
}

// redact drops the request URL (it contains the bot token) from transport
// errors.
func redact(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}
