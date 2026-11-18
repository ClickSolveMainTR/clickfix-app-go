package usecase

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/eventdispatch"
)

// CF-251: the customer reply is latency bound (first meaningful answer p50
// under 5 s, p90 under 10 s). This file holds what keeps it there: the reply
// no longer waits for triage unless it reads it, one model call answers from
// context prepared up front, generation has a deadline, and every reply logs
// its end-to-end time.

// DefaultReplyTimeout bounds one customer reply generation. Past it the
// customer gets the handoff notice instead of waiting for a slow provider.
const DefaultReplyTimeout = 20 * time.Second

func (s *Service) replyTimeout() time.Duration {
	if s.ReplyTimeout > 0 {
		return s.ReplyTimeout
	}
	return DefaultReplyTimeout
}

// replyNeedsTriage reports whether the reply reads the triage of the same
// message: a category allowlist, a published procedure or a connected
// system playbook (both matched by the triage intent). Only then triage
// stays in front of the reply; otherwise the customer is answered first and
// triage, which serves the agents, runs after it. An unknown answer keeps
// the safe order.
func (s *Service) replyNeedsTriage(ctx context.Context, org int64, cfg model.Config) bool {
	if len(cfg.Categories) > 0 {
		return true
	}
	const page = 200
	procedures, err := s.Repo.Q.AIProcedures(ctx, db.AIProceduresParams{OrganizationID: org, Limit: page})
	if err != nil || len(procedures) == page {
		// Unknown, or more procedures than one page shows: keep triage first.
		return true
	}
	for _, p := range procedures {
		if len(p.Published) > 0 && string(p.Published) != "null" {
			return true
		}
	}
	if s.Systems != nil {
		intents, err := s.Systems.Intents(ctx, org)
		if err != nil || len(intents) > 0 {
			return true
		}
	}
	return false
}

// replyTools are the tools an autonomous reply may call. What the model used
// to fetch with a tool turn of about two seconds each is in its context
// already: the transcript (conversation_history), the retrieved public
// knowledge (knowledge_search) and the customer's fields (lookup_contact).
// Approval-only tools cannot run without a person and are left out. That
// leaves the organization's own HTTP tools.
func (s *ToolService) replyTools(ctx context.Context, org int64) ([]ai.Tool, error) {
	offered, err := s.Offered(ctx, org)
	if err != nil {
		return nil, err
	}
	prepared := map[string]bool{"lookup_contact": true, "conversation_history": true, "knowledge_search": true}
	out := make([]ai.Tool, 0, len(offered))
	for _, tool := range offered {
		if tool.RequiresApproval || prepared[tool.Name] {
			continue
		}
		out = append(out, tool)
	}
	return out, nil
}

// piiPlaceholder starts a masked value. A text that still carries one is
// never sent to a customer.
const piiPlaceholder = "[PII_"

// maxHandoffMessage bounds the model's handoff message to the customer.
const maxHandoffMessage = 1200

var handoffLink = regexp.MustCompile(`(?i)(https?://|www\.)`)

// actionClaim matches a first-person or passive claim that something was
// done, is being done or will be done for the customer (refund, cancel,
// update, create, send ...), in Turkish and English. The handoff message is
// not grounded in sources, so such a claim sends the generic notice instead.
// It errs on the side of the notice: a question like "ödeme yapıldı mı?"
// matches too.
var actionClaim = regexp.MustCompile(`(^|[^\p{L}])(` +
	`(iade|iptal|teslim|tahsil|kayıt|rezerve|aktive|onay|talep|transfer|havale)\s+(ettim|ettik|edildi|edilmiştir|ediyorum|ediyoruz|edeceğim|edeceğiz|edilecek|edilecektir)` +
	`|güncelledi[mk]|güncellendi|oluşturdu[mk]|oluşturuldu|gönderdi[mk]|gönderildi|tamamladı[mk]|tamamlandı|onayladı[mk]|onaylandı` +
	`|başlattı[mk]|başlatıldı|değiştirdi[mk]|değiştirildi|sıfırladı[mk]|sıfırlandı|tanımladı[mk]|tanımlandı|düzeltti[mk]|düzeltildi` +
	`|çözdü[mk]|çözüldü|kaldırdı[mk]|kaldırıldı|ekledi[mk]|eklendi|yaptı[mk]|yapıldı|yapılmıştır|açtı[mk]|kapattı[mk]` +
	`|yapacağım|yapacağız|göndereceğim|göndereceğiz|arayacağım|arayacağız|güncelleyeceğim|güncelleyeceğiz` +
	`|(i|we)(\s+have|'ve|\s+had|\s+just)?\s+(already\s+)?(refunded|cancell?ed|updated|changed|created|opened|issued|processed|sent|reset|approved|completed|fixed|resolved|removed|added|activated|booked|scheduled)` +
	`|(has|have|had)\s+been\s+(refunded|cancell?ed|updated|changed|created|opened|issued|processed|sent|reset|approved|completed|fixed|resolved|removed|added|activated|booked|scheduled|shipped)` +
	`|(i|we)('ll|\s+will)\s+(refund|cancel|update|change|create|open|issue|process|send|reset|approve|complete|fix|resolve|remove|add|activate|book|schedule|call)` +
	`)($|[^\p{L}])`)

// safeHandoffMessage decides whether the model's own handoff message may go
// to the customer in place of the generic notice (CF-251). It holds no fact
// the application can check against a source, so it must be short, carry no
// masked value, pass the output guard, link nowhere and claim no action.
// The reply instructions ask for exactly that; this is the backstop.
func safeHandoffMessage(text string) bool {
	text = strings.TrimSpace(text)
	if n := len([]rune(text)); n < 15 || n > maxHandoffMessage {
		return false
	}
	if strings.Contains(text, piiPlaceholder) || ai.Guard(text) != nil || handoffLink.MatchString(text) {
		return false
	}
	return !actionClaim.MatchString(strings.ToLower(text))
}

// replyTrace times one customer reply from the stored inbound message to
// the posted answer (or handoff notice).
type replyTrace struct {
	// received is when the customer's message was stored.
	received time.Time
	// started is when this consumer began handling it.
	started time.Time
	// triageFirst: triage ran in front of the reply (the reply reads it).
	triageFirst bool
	triage      time.Duration
	// prepare is the handler work in front of the model call (minus
	// triage), model the generation, deliver the post of the answer.
	modelStarted time.Time
	model        time.Duration
	// replied is when the customer-facing message was posted.
	replied time.Time
	tools   int
	memory  string
}

func newReplyTrace(msg db.Message) *replyTrace {
	t := &replyTrace{started: time.Now(), received: msg.CreatedAt.Time}
	if !msg.CreatedAt.Valid {
		t.received = t.started
	}
	return t
}

func ms(d time.Duration) int64 { return max(d, 0).Milliseconds() }

// log writes the structured end-to-end measurement of one reply job:
// message received -> handler started (outbox + queue) -> model call ->
// customer message posted. received_to_reply_ms is the number the product
// promises; it is 0 when nothing was posted to the customer.
func (t *replyTrace) log(ctx context.Context, org int64, conv db.Conversation, msg db.Message, run uuid.UUID, status, outcome string) {
	if t == nil {
		return
	}
	now := time.Now()
	queueWait := int64(0)
	if d, ok := eventdispatch.DeliveryOf(ctx); ok && !d.EnqueuedAt.IsZero() {
		queueWait = ms(d.StartedAt.Sub(d.EnqueuedAt))
	}
	prepare, toReply := int64(0), int64(0)
	if !t.modelStarted.IsZero() {
		prepare = ms(t.modelStarted.Sub(t.started) - t.triage)
	}
	if !t.replied.IsZero() {
		toReply = ms(t.replied.Sub(t.received))
	}
	slog.InfoContext(ctx, "ai_reply_latency",
		"organization_id", org, "conversation_id", conv.ID, "message_uuid", msg.Uuid, "run_uuid", run,
		"channel", conv.ChannelType, "status", status, "outcome", outcome,
		"received_to_start_ms", ms(t.started.Sub(t.received)), "queue_wait_ms", queueWait,
		"triage_first", t.triageFirst, "triage_ms", ms(t.triage), "prepare_ms", prepare,
		"model_ms", ms(t.model), "handler_ms", ms(now.Sub(t.started)),
		"received_to_reply_ms", toReply, "tools", t.tools, "memory", t.memory)
}
