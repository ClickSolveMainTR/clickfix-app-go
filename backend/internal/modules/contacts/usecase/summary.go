package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	coremodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	core "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrAIUnavailable: the service was built without the AI engine.
var ErrAIUnavailable = errors.New("contacts: AI is not available")

// Summarizer is the slice of the AI engine (aicore, over platform/ai) the
// contact summary uses: guarded, budgeted, masked and logged like every
// other product AI run. The provider is never called directly.
type Summarizer interface {
	RunAgent(ctx context.Context, org, actor int64, in coremodel.RunInput, options core.AgentOptions) (core.RunResult, error)
}

// WithAI wires the AI engine for contact summaries.
func (s *Service) WithAI(ai Summarizer) *Service {
	s.ai = ai
	return s
}

var contactSummarySchema = map[string]any{
	"type": "object", "additionalProperties": false,
	"required": []string{"summary", "open_topics", "sentiment"},
	"properties": map[string]any{
		"summary":     map[string]any{"type": "string", "maxLength": 1500},
		"open_topics": map[string]any{"type": "array", "maxItems": 6, "items": map[string]any{"type": "string", "maxLength": 200}},
		"sentiment":   map[string]any{"type": "string", "enum": []string{"positive", "neutral", "negative", "unknown"}},
	},
}

const contactSummaryInstructions = `You brief a support agent about one customer before they reply.
Write "summary": 2-4 short sentences on who the customer is and their support history (recurring problems, how past issues ended, satisfaction).
Write "open_topics": unresolved questions, pending commitments or follow-ups, one short item each; empty when there are none.
Write "sentiment": the customer's recent overall tone.
Use only the data given. Never invent facts, never address the customer, never quote personal data such as e-mail, phone or ids.`

// cachedSummary returns the member's cached summary, nil when none exists.
func (s *Service) cachedSummary(ctx context.Context, orgID, contactID int64, a ProfileAccess) (*ContactAISummary, error) {
	row, err := s.q.GetContactAISummary(ctx, db.GetContactAISummaryParams{OrganizationID: orgID, ContactID: contactID, Audience: a.audience()})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	latest, err := s.q.ContactLatestActivityAt(ctx, db.ContactLatestActivityAtParams{OrganizationID: orgID, ContactID: contactID})
	if err != nil {
		return nil, err
	}
	out := summaryOf(row)
	out.Stale = latest.Valid && (!row.SourceAt.Valid || latest.Time.After(row.SourceAt.Time))
	return &out, nil
}

func summaryOf(row db.ContactAiSummary) ContactAISummary {
	out := ContactAISummary{Summary: row.Summary, OpenTopics: []string{}, Sentiment: row.Sentiment, Locale: row.Locale, GeneratedAt: row.CreatedAt.Time}
	_ = json.Unmarshal(row.OpenTopics, &out.OpenTopics)
	return out
}

// GenerateSummary asks the AI engine for a fresh summary of the contact
// from what the member may see, stores it as the member's cached summary and
// returns it. The engine enforces the AI policy, plan, credits and guard.
func (s *Service) GenerateSummary(ctx context.Context, orgID int64, contactUUID uuid.UUID, a ProfileAccess, locale string) (ContactAISummary, error) {
	if s.ai == nil {
		return ContactAISummary{}, ErrAIUnavailable
	}
	if locale != "en" {
		locale = "tr"
	}
	row, err := s.q.GetContactByUUID(ctx, db.GetContactByUUIDParams{OrganizationID: orgID, Uuid: contactUUID})
	if err != nil {
		return ContactAISummary{}, notFound(err)
	}
	input, err := s.summaryInput(ctx, orgID, row, a)
	if err != nil {
		return ContactAISummary{}, err
	}
	latest, err := s.q.ContactLatestActivityAt(ctx, db.ContactLatestActivityAtParams{OrganizationID: orgID, ContactID: row.ID})
	if err != nil {
		return ContactAISummary{}, err
	}
	res, err := s.ai.RunAgent(ctx, orgID, a.UserID, coremodel.RunInput{Task: "triage", Input: input, Locale: locale, MaxTokens: 1024},
		core.AgentOptions{Purpose: "contact_summary", Instructions: contactSummaryInstructions, Schema: contactSummarySchema})
	if err != nil {
		return ContactAISummary{}, err
	}
	var parsed struct {
		Summary    string   `json:"summary"`
		OpenTopics []string `json:"open_topics"`
		Sentiment  string   `json:"sentiment"`
	}
	if json.Unmarshal([]byte(res.Text), &parsed) != nil || strings.TrimSpace(parsed.Summary) == "" {
		return ContactAISummary{}, ai.ErrInvalidOutput
	}
	topics := make([]string, 0, len(parsed.OpenTopics))
	for _, t := range parsed.OpenTopics {
		if t = strings.TrimSpace(t); t != "" && len(topics) < 6 {
			topics = append(topics, clipRunes(t, 200))
		}
	}
	switch parsed.Sentiment {
	case "positive", "neutral", "negative":
	default:
		parsed.Sentiment = "unknown"
	}
	tb, _ := json.Marshal(topics)
	saved, err := s.q.UpsertContactAISummary(ctx, db.UpsertContactAISummaryParams{
		OrganizationID: orgID, ContactID: row.ID, Audience: a.audience(), Summary: clipRunes(strings.TrimSpace(parsed.Summary), 4000),
		OpenTopics: tb, Sentiment: parsed.Sentiment, Locale: locale, RunUuid: pgtype.UUID{Bytes: res.UUID, Valid: res.UUID != uuid.Nil},
		SourceAt: latest, GeneratedBy: pgtype.Int8{Int64: a.UserID, Valid: a.UserID > 0},
	})
	if err != nil {
		return ContactAISummary{}, err
	}
	return summaryOf(saved), nil
}

// summaryInput is the untrusted data the model reads: the contact, its
// visible conversations (latest public messages) and recent notes. The AI
// engine masks personal data before it leaves.
func (s *Service) summaryInput(ctx context.Context, orgID int64, row db.Contact, a ProfileAccess) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Customer: %s\n", clipRunes(strings.TrimSpace(row.Name), 120))
	if row.JobTitle != "" {
		fmt.Fprintf(&b, "Job title: %s\n", clipRunes(row.JobTitle, 120))
	}
	fmt.Fprintf(&b, "Customer since: %s\n", row.CreatedAt.Time.Format("2006-01-02"))
	if a.Conversations {
		convs, err := s.q.ContactSummarySource(ctx, db.ContactSummarySourceParams{
			OrganizationID: orgID, ContactID: row.ID, SeeAll: a.SeeAll, ViewerID: a.UserID, ViewerTeamIds: a.teams(),
		})
		if err != nil {
			return "", err
		}
		if len(convs) == 0 {
			b.WriteString("No conversations yet.\n")
		}
		for _, c := range convs {
			fmt.Fprintf(&b, "\n## Conversation #%d (%s, %s) opened %s, last message %s\nSubject: %s\n%s\n",
				c.Number, c.ChannelType, c.Status, c.CreatedAt.Time.Format("2006-01-02"), c.LastMessageAt.Time.Format("2006-01-02"),
				clipRunes(c.Subject, 200), clipRunes(c.Excerpt, 2400))
		}
	}
	notes, err := s.q.ListContactTimeline(ctx, db.ListContactTimelineParams{
		OrganizationID: orgID, ContactID: row.ID, ContactUuid: row.Uuid, Kinds: []string{ContactTimelineNote}, RowLimit: 5,
		ViewerTeamIds: []int64{},
	})
	if err != nil {
		return "", err
	}
	if len(notes) > 0 {
		b.WriteString("\n## Agent notes\n")
		for _, n := range notes {
			fmt.Fprintf(&b, "- %s: %s\n", n.OccurredAt.Time.Format(time.DateOnly), clipRunes(n.Body, 500))
		}
	}
	return clipRunes(b.String(), 20000), nil
}

func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
