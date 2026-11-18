//go:build ailatency

package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
)

// TestLiveReplyLatency measures the model part of the customer reply against
// the real provider (CF-251): the path as it was on main next to the path of
// this change. It runs only with AI_LATENCY=1 and ANTHROPIC_API_KEY, needs no
// database and uses synthetic conversations and knowledge only. It never
// prints the key or provider error bodies; AI_LATENCY_SHOW=1 prints the
// (synthetic) replies for a quality check. AI_LATENCY_RUNS sets the runs per
// sample (default 2).
//
//	AI_LATENCY=1 go test -tags ailatency -run TestLiveReplyLatency -v ./internal/modules/aiagents/usecase/
func TestLiveReplyLatency(t *testing.T) {
	if os.Getenv("AI_LATENCY") != "1" {
		t.Skip("set AI_LATENCY=1 explicitly")
	}
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		t.Fatal("ANTHROPIC_API_KEY is not set")
	}
	runs, _ := strconv.Atoi(os.Getenv("AI_LATENCY_RUNS"))
	if runs <= 0 {
		runs = 2
	}
	show := os.Getenv("AI_LATENCY_SHOW") == "1"

	after := ai.NewAnthropic(key)
	// main sent no thinking field with tools or to Haiku: the models then
	// think adaptively. The transport below removes the field this change
	// adds, which reproduces main's requests on the wire.
	before := ai.NewAnthropic(key)
	before.Client = &http.Client{Timeout: 90 * time.Second, Transport: dropThinking{http.DefaultTransport}}

	chunkShipping, chunkReturn := uuid.New(), uuid.New()
	knowledge := fmt.Sprintf("\n\nUNTRUSTED KNOWLEDGE chunk=%s article=%s version=1:\nKargo ve teslimat\nSiparişler 1 iş günü içinde kargoya verilir ve 2-4 iş günü içinde teslim edilir. Kargo takip numarası siparişiniz kargoya verildiğinde SMS ile gönderilir. 750 TL üzeri siparişlerde kargo ücretsizdir.",
		chunkShipping, uuid.New()) +
		fmt.Sprintf("\n\nUNTRUSTED KNOWLEDGE chunk=%s article=%s version=1:\nİade koşulları\nÜrünler teslimattan itibaren 14 gün içinde, kullanılmamış ve orijinal ambalajında iade edilebilir. İade talebi hesabınızdaki Siparişlerim sayfasından oluşturulur. Ücret iadesi, ürün depoya ulaştıktan sonra 5 iş günü içinde aynı ödeme yöntemine yapılır.",
			chunkReturn, uuid.New())
	samples := []struct{ name, transcript string }{
		{"answerable: delivery time", "contact: Merhaba, sipariş verirsem kaç günde elime ulaşır?\n"},
		{"answerable: return window", "contact: Aldığım ürünü beğenmedim, iade edebilir miyim? Kaç gün sürem var?\n"},
		{"answerable: free shipping", "contact: Kargo ücretli mi?\n"},
		{"needs a person: missing parcel", "contact: Siparişim 10 gündür gelmedi, kargo takipte de hareket yok. Ne oldu?\n"},
		{"needs a person: invoice change", "contact: Faturamdaki şirket unvanı yanlış yazılmış, düzeltir misiniz?\n"},
		{"not in knowledge", "contact: Mağazanız pazar günleri açık mı?\n"},
	}
	instructions := func(task, text string) string {
		return ai.SafetyInstructions + "\n" + text + "\nTask: " + task + ". Respond in locale tr."
	}
	replyReq := func(transcript string) ai.Request {
		return ai.Request{Model: ai.ModelForTask("reply"), Effort: ai.EffortForTask("reply"), CachePrefix: true, MaxTokens: replyMaxTokens,
			Instructions: instructions("reply", replyInstructions), Schema: replySchema,
			Turns: []ai.Turn{{Role: "user", Text: ai.ConversationData(transcript + knowledge)}}}
	}
	triageReq := func(transcript string, maxTokens int64) ai.Request {
		return ai.Request{Model: ai.ModelForTask("triage"), Effort: ai.EffortForTask("triage"), CachePrefix: true, MaxTokens: maxTokens,
			Instructions: instructions("triage", triageInstructions), Schema: triageSchema,
			Turns: []ai.Turn{{Role: "user", Text: ai.ConversationData(transcript + "\nAllowed assignment candidates (untrusted labels; UUIDs only): []")}}}
	}
	// The tools main offered to every autonomous reply, with small canned results.
	runner := func(_ context.Context, call ai.ToolCall) (ai.ToolResult, error) {
		switch call.Name {
		case "lookup_contact":
			return ai.ToolResult{Content: `{"name":"Müşteri","identity_state":"anonymous","custom_fields":{}}`}, nil
		case "conversation_history":
			return ai.ToolResult{Content: `[{"sender":"contact","text":"(yukarıdaki mesaj)"}]`}, nil
		case "knowledge_search":
			return ai.ToolResult{Content: `[{"title":"Kargo ve teslimat","content":"Siparişler 2-4 iş günü içinde teslim edilir."}]`}, nil
		}
		return ai.ToolResult{Content: `{"status":"ok"}`}, nil
	}
	customTool := []ai.Tool{{Name: "order_status", Description: "Read the status of the customer's latest order", Schema: objectSchema(map[string]any{})}}

	type variant struct {
		name string
		run  func(ctx context.Context, transcript string) (string, error)
	}
	variants := []variant{
		{"before: triage, then reply with built-in tools (main)", func(ctx context.Context, transcript string) (string, error) {
			// main ran triage first; its failure did not stop the reply.
			_, _ = before.Generate(ctx, triageReq(transcript, 512))
			req := replyReq(transcript)
			req.Instructions = instructions("reply", "Create a support reply grounded exclusively in supplied public knowledge. Return reply, summary, confidence, handoff and citation_ids (public chunk UUIDs). Follow organization procedure steps only when consistent with safety and allowed tools. Ask a human if sources are insufficient. Never claim actions were executed. Do not disclose internal data.")
			req.Tools = builtins()
			out, err := ai.Chat(ctx, before, req, 3, runner)
			return out.Text, err
		}},
		{"after: one reply call, no tools, thinking off", func(ctx context.Context, transcript string) (string, error) {
			out, err := after.Generate(ctx, replyReq(transcript))
			return out.Text, err
		}},
		{"after: reply with one organization tool offered", func(ctx context.Context, transcript string) (string, error) {
			req := replyReq(transcript)
			req.Tools = customTool
			out, err := ai.Chat(ctx, after, req, 3, runner)
			return out.Text, err
		}},
		{"after: reply on claude-haiku-5-5 (AI_REPLY_MODEL option)", func(ctx context.Context, transcript string) (string, error) {
			req := replyReq(transcript)
			req.Model = "claude-haiku-5-5"
			out, err := after.Generate(ctx, req)
			return out.Text, err
		}},
		{"triage alone, before (512 tokens, default thinking)", func(ctx context.Context, transcript string) (string, error) {
			out, err := before.Generate(ctx, triageReq(transcript, 512))
			return out.Text, err
		}},
		{"triage alone, after (1024 tokens, thinking off)", func(ctx context.Context, transcript string) (string, error) {
			out, err := after.Generate(ctx, triageReq(transcript, triageMaxTokens))
			return out.Text, err
		}},
	}
	for _, v := range variants {
		var took []time.Duration
		failed, handoffs, cited := 0, 0, 0
		reasons := map[string]int{}
		for _, s := range samples {
			for range runs {
				ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
				started := time.Now()
				text, err := v.run(ctx, s.transcript)
				d := time.Since(started)
				cancel()
				if err != nil {
					failed++
					reasons[failureClass(err)]++
					continue
				}
				took = append(took, d)
				var answer model.Reply
				if json.Unmarshal([]byte(text), &answer) == nil && strings.Contains(v.name, "reply") {
					if answer.Handoff {
						handoffs++
					}
					if len(answer.Citations) > 0 {
						cited++
					}
					if show {
						t.Logf("  [%s] %s -> handoff=%v confidence=%.2f citations=%d (%s)\n    %s", v.name[:6], s.name, answer.Handoff, answer.Confidence, len(answer.Citations), d.Round(10*time.Millisecond), answer.Text)
					}
				}
			}
		}
		sort.Slice(took, func(i, j int) bool { return took[i] < took[j] })
		t.Logf("%-58s ok=%d failed=%d %v p50=%s p90=%s max=%s handoff=%d cited=%d", v.name, len(took), failed, reasons, pct(took, 50), pct(took, 90), pct(took, 100), handoffs, cited)
	}
}

func pct(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	i := (len(sorted)*p + 99) / 100
	return sorted[max(min(i, len(sorted))-1, 0)].Round(10 * time.Millisecond)
}

// failureClass names an error without its provider body.
func failureClass(err error) string {
	var rejected *ai.Rejected
	switch {
	case errors.As(err, &rejected):
		return "http_" + strconv.Itoa(rejected.Status)
	case errors.Is(err, ai.ErrTruncated):
		return "max_tokens"
	case errors.Is(err, ai.ErrRefused):
		return "refusal"
	case errors.Is(err, ai.ErrInvalidOutput):
		return "invalid_output:" + ai.InvalidReason(err)
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, ai.ErrUnavailable):
		return "unavailable"
	}
	return "other"
}

// dropThinking removes the request's thinking field (see the test).
type dropThinking struct{ next http.RoundTripper }

func (d dropThinking) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Body != nil {
		raw, err := io.ReadAll(r.Body)
		_ = r.Body.Close()
		if err != nil {
			return nil, err
		}
		var body map[string]json.RawMessage
		if json.Unmarshal(raw, &body) == nil {
			if _, ok := body["thinking"]; ok {
				delete(body, "thinking")
				raw, _ = json.Marshal(body)
			}
		}
		r = r.Clone(r.Context())
		r.Body = io.NopCloser(bytes.NewReader(raw))
		r.ContentLength = int64(len(raw))
		r.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(raw)), nil }
	}
	return d.next.RoundTrip(r)
}
