package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/history/model"
	"github.com/jackc/pgx/v5/pgtype"
)

// Known is a personal value taken from a structured field of the record; the
// anonymizer removes it from free text too.
type Known struct {
	Kind  string
	Value string
}

// Line is one message of a record, in order.
type Line struct {
	Author string // customer, agent or system
	Body   string
}

// Record is one resolved case as a source delivers it, before anonymization.
// It never leaves the server in this form.
type Record struct {
	Ref            string // stable id in the source system; stored only hashed
	Subject        string
	Category       string
	Locale         string
	Resolution     string // optional explicit resolution summary
	Lines          []Line
	Known          []Known
	ConversationID int64
	BrandID        pgtype.UUID
	OpenedAt       *time.Time
	ClosedAt       *time.Time
}

// Source yields the records of one run. The uploaded file and our own closed
// conversations are sources today; a connector (Zendesk, Freshdesk, ...)
// implements the same interface and is registered by name in
// Service.Connectors, with history_imports.connector naming it.
type Source interface {
	Records(ctx context.Context, run db.HistoryImport) ([]Record, error)
}

// Blobs is the part of storage.Driver the pipeline uses.
type Blobs interface {
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
	Delete(ctx context.Context, path string) error
}

// FileSource reads the uploaded export of a run.
type FileSource struct{ Blobs Blobs }

func (s FileSource) Records(ctx context.Context, run db.HistoryImport) ([]Record, error) {
	if s.Blobs == nil || run.RawKey == "" {
		return nil, fmt.Errorf("%w: the uploaded file is no longer available", model.ErrUnavailable)
	}
	rc, _, err := s.Blobs.Download(ctx, run.RawKey)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(io.LimitReader(rc, model.MaxUploadBytes+1))
	if err != nil {
		return nil, err
	}
	b, err := ParseBundle(run.FileName, data)
	if err != nil {
		return nil, err
	}
	return BundleRecords(b, run.BrandID), nil
}

// BundleRecords turns a parsed export into records: solved tickets only,
// with their messages in order and the contact's fields as known values.
func BundleRecords(b model.Bundle, brand pgtype.UUID) []Record {
	contacts := make(map[string]model.Contact, len(b.Contacts))
	for _, c := range b.Contacts {
		contacts[strings.TrimSpace(c.ID)] = c
	}
	agents := make(map[string]string, len(b.Agents))
	var agentNames []Known
	for _, a := range b.Agents {
		agents[strings.TrimSpace(a.ID)] = a.Name
		agentNames = append(agentNames, Known{"NAME", a.Name})
	}
	lines := map[string][]Line{}
	for _, m := range b.Messages {
		id := strings.TrimSpace(m.TicketID)
		body := truncate(m.Body, model.MaxMessageRunes)
		if strings.TrimSpace(body) == "" {
			continue
		}
		author := m.Author
		if author == "" {
			author = "customer"
		}
		lines[id] = append(lines[id], Line{Author: author, Body: body})
	}
	out := make([]Record, 0, len(b.Tickets))
	for _, t := range b.Tickets {
		if !model.Solved(t.Status) {
			continue
		}
		id := strings.TrimSpace(t.ID)
		r := Record{Ref: "file:" + id, Subject: truncate(t.Subject, model.MaxSubjectRunes), Category: slug(t.Category), Locale: locale(t.Locale, b.Locale),
			Resolution: truncate(t.Resolution, model.MaxMessageRunes), Lines: lines[id], BrandID: brand, OpenedAt: t.CreatedAt, ClosedAt: t.ClosedAt}
		r.Known = append(r.Known, agentNames...)
		if c, ok := contacts[strings.TrimSpace(t.ContactID)]; ok {
			r.Known = append(r.Known, Known{"NAME", c.Name}, Known{"EMAIL", c.Email}, Known{"PHONE", c.Phone},
				Known{"COMPANY", c.Company}, Known{"TAXNO", c.TaxNumber}, Known{"ADDRESS", c.Address})
		}
		out = append(out, r)
	}
	return out
}

// ConversationSource reads our own conversations closed inside the run's
// window: the nightly run. Private notes and non-text messages are skipped.
type ConversationSource struct{ Q *db.Queries }

func (s ConversationSource) Records(ctx context.Context, run db.HistoryImport) ([]Record, error) {
	rows, err := s.Q.HistoryClosedConversations(ctx, db.HistoryClosedConversationsParams{OrganizationID: run.OrganizationID, Limit: model.MaxTickets, Since: run.WindowStart, Until: run.WindowEnd})
	if err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(rows))
	for _, c := range rows {
		msgs, err := s.Q.HistoryConversationMessages(ctx, db.HistoryConversationMessagesParams{OrganizationID: run.OrganizationID, ConversationID: c.ID})
		if err != nil {
			return nil, err
		}
		r := Record{Ref: "conversation:" + c.Uuid.String(), Subject: truncate(c.Subject, model.MaxSubjectRunes), Locale: "tr", ConversationID: c.ID, BrandID: c.BrandID,
			Known: []Known{{"NAME", c.ContactName}, {"EMAIL", c.ContactEmail}, {"PHONE", c.ContactPhone}, {"COMPANY", c.CompanyName}}}
		if c.CreatedAt.Valid {
			t := c.CreatedAt.Time
			r.OpenedAt = &t
		}
		if c.EndedAt.Valid {
			t := c.EndedAt.Time
			r.ClosedAt = &t
		}
		for _, m := range msgs {
			author := "customer"
			switch m.SenderType {
			case "user", "agent":
				author = "agent"
			case "bot", "system":
				author = "system"
			}
			r.Lines = append(r.Lines, Line{Author: author, Body: truncate(m.Content, model.MaxMessageRunes)})
		}
		out = append(out, r)
	}
	return out, nil
}

// refHash keeps re-uploads idempotent without storing the source id.
func refHash(org int64, ref string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s", org, ref)))
	return hex.EncodeToString(sum[:])
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func locale(values ...string) string {
	for _, v := range values {
		if v == "tr" || v == "en" {
			return v
		}
	}
	return "tr"
}

// slug normalizes a source category to a taxonomy-like key.
func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	repl := strings.NewReplacer("ç", "c", "ğ", "g", "ı", "i", "ö", "o", "ş", "s", "ü", "u", "İ", "i")
	s = repl.Replace(s)
	var b strings.Builder
	dash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return truncate(strings.TrimSuffix(b.String(), "-"), 100)
}
