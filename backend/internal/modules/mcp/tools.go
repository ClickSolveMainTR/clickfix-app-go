package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/mcpauth/model"
)

// Outcomes of a tools/call (activity payload "result").
const (
	OutcomeOK         = "ok"
	OutcomeError      = "error"
	OutcomeNotAllowed = "not_allowed"
	OutcomeInvalid    = "invalid_arguments"
)

// Tool error codes the model sees.
const (
	CodeToolNotAllowed   = "TOOL_NOT_ALLOWED"
	CodeInvalidArguments = "INVALID_ARGUMENTS"
	CodeToolFailed       = "TOOL_FAILED"
)

var instructions = map[string]string{model.LayerBusiness: "ClickFix support tools for the organization approved by the user. Current API permissions apply. Confirm with the user before changing data."}

// build is the SDK server of one request: exactly the tools the principal
// may use now.
func (s *Server) build(sess *session) *mcpsdk.Server {
	layer := sess.principal.Layer
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "clickfix-" + layer, Title: "ClickFix", Version: s.cfg.Version},
		&mcpsdk.ServerOptions{
			Instructions: instructions[layer],
			// The list depends on the user's permissions and modules; it is
			// read again at the start of every client session.
			Capabilities: &mcpsdk.ServerCapabilities{Tools: &mcpsdk.ToolCapabilities{ListChanged: false}},
			Logger:       s.sdkLog,
		})
	for _, t := range sess.tools {
		srv.AddTool(&mcpsdk.Tool{
			Name: t.Name, Title: t.Title, Description: t.Description, InputSchema: t.Input,
			Annotations: annotations(t),
		}, s.handler(sess, t))
	}
	srv.AddReceivingMiddleware(s.gate(sess))
	return srv
}

func annotations(t *Tool) *mcpsdk.ToolAnnotations {
	closed, destructive := false, t.Destructive
	return &mcpsdk.ToolAnnotations{
		Title: t.Title, ReadOnlyHint: !t.Write, DestructiveHint: &destructive, OpenWorldHint: &closed,
	}
}

// gate answers a tools/call of a tool the principal may not use (the
// client's list is old, or it invents a name) and records it.
func (s *Server) gate(sess *session) mcpsdk.Middleware {
	return func(next mcpsdk.MethodHandler) mcpsdk.MethodHandler {
		return func(ctx context.Context, method string, req mcpsdk.Request) (mcpsdk.Result, error) {
			call, ok := req.(*mcpsdk.CallToolRequest)
			if method != "tools/call" || !ok || call.Params == nil {
				return next(ctx, method, req)
			}
			for _, t := range sess.tools {
				if t.Name == call.Params.Name {
					return next(ctx, method, req)
				}
			}
			start := s.now()
			msg := "the tool " + call.Params.Name + " is not available to this user. Do not call it again; " +
				"answer with the tools you have."
			s.record(ctx, sess, call.Params.Name, call.Params.Arguments, start, OutcomeNotAllowed, CodeToolNotAllowed)
			return errorResult(CodeToolNotAllowed, msg), nil
		}
	}
}

// handler runs one tool: the catalog checks the principal again, the
// arguments are validated, and the /v1 route runs through the bridge.
func (s *Server) handler(sess *session, t *Tool) mcpsdk.ToolHandler {
	return func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		start := s.now()
		raw := json.RawMessage(req.Params.Arguments)
		res, outcome, code := s.run(ctx, sess, t, raw)
		s.record(ctx, sess, t.Name, raw, start, outcome, code)
		return res, nil
	}
}

func (s *Server) run(ctx context.Context, sess *session, t *Tool, raw json.RawMessage) (*mcpsdk.CallToolResult, string, string) {
	if !s.cfg.Catalog.Allowed(sess.principal, t) {
		return errorResult(CodeToolNotAllowed, "the tool "+t.Name+" is not available to this user."), OutcomeNotAllowed, CodeToolNotAllowed
	}
	args := Args{}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return errorResult(CodeInvalidArguments, "arguments must be a JSON object"), OutcomeInvalid, CodeInvalidArguments
		}
	}
	if err := t.Validate(args); err != nil {
		return errorResult(CodeInvalidArguments, err.Error()), OutcomeInvalid, CodeInvalidArguments
	}
	call, err := t.Request(args)
	var ae *ArgumentError
	if errors.As(err, &ae) {
		return errorResult(CodeInvalidArguments, ae.Message), OutcomeInvalid, CodeInvalidArguments
	}
	if err != nil {
		s.cfg.Log.ErrorContext(ctx, "mcp_tool_failed", "tool", t.Name, "error", err)
		return errorResult(CodeToolFailed, "the tool failed because of an internal error"), OutcomeError, CodeToolFailed
	}
	resp, err := s.cfg.Bridge.Do(ctx, sess.principal, call, sess.req)
	if err != nil {
		s.cfg.Log.ErrorContext(ctx, "mcp_tool_failed", "tool", t.Name, "error", err)
		return errorResult(CodeToolFailed, "the tool failed because of an internal error"), OutcomeError, CodeToolFailed
	}
	out := resp.Read()
	if !out.OK {
		outcome := OutcomeError
		if out.Code == "FORBIDDEN" {
			outcome = OutcomeNotAllowed
		}
		return errorResult(out.Code, out.Message), outcome, out.Code
	}
	return okResult(t.Name, out.Data), OutcomeOK, ""
}

func errorResult(code, msg string) *mcpsdk.CallToolResult {
	return &mcpsdk.CallToolResult{
		IsError:           true,
		Content:           []mcpsdk.Content{&mcpsdk.TextContent{Text: code + ": " + msg}},
		StructuredContent: map[string]any{"error": map[string]string{"code": code, "message": msg}},
	}
}

// okResult is a short text line plus the JSON; an object also goes out as
// structured content (lists are wrapped as {"items": …}).
func okResult(name string, data json.RawMessage) *mcpsdk.CallToolResult {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(data)}}}
	}
	obj, ok := v.(map[string]any)
	if !ok {
		obj = map[string]any{"items": v}
		data, _ = json.Marshal(obj)
	}
	return &mcpsdk.CallToolResult{
		Content:           []mcpsdk.Content{&mcpsdk.TextContent{Text: summary(name, obj)}, &mcpsdk.TextContent{Text: string(data)}},
		StructuredContent: obj,
	}
}

// summary is the short text line of a JSON result.
func summary(name string, obj map[string]any) string {
	items, isList := obj["items"].([]any)
	if total, ok := obj["total"].(float64); ok && isList {
		return fmt.Sprintf("%s: %d of %d items.", name, len(items), int64(total))
	}
	if isList {
		return fmt.Sprintf("%s: %d items.", name, len(items))
	}
	return name + ": done."
}
