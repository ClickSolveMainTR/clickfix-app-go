package usecase

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/history/model"
)

// ParseBundle reads an export in the documented schema: a JSON bundle, or a
// zip holding either history.json (a bundle) or contacts.json, tickets.json
// and messages.json (arrays, agents.json optional). Unknown fields are
// ignored so exports can carry more than we read.
func ParseBundle(name string, data []byte) (model.Bundle, error) {
	if len(data) == 0 || len(data) > model.MaxUploadBytes {
		return model.Bundle{}, fmt.Errorf("%w: file must be 1 byte to 50 MB", model.ErrInvalid)
	}
	var b model.Bundle
	switch {
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		if err := parseZip(data, &b); err != nil {
			return model.Bundle{}, err
		}
	case strings.HasSuffix(strings.ToLower(name), ".json") || bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")):
		if err := json.Unmarshal(data, &b); err != nil {
			return model.Bundle{}, fmt.Errorf("%w: not valid JSON: %v", model.ErrInvalid, err)
		}
	default:
		return model.Bundle{}, fmt.Errorf("%w: upload a .json or .zip file", model.ErrInvalid)
	}
	return b, validateBundle(b)
}

func parseZip(data []byte, b *model.Bundle) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("%w: not a valid zip", model.ErrInvalid)
	}
	var budget int64 = model.MaxUnpackedBytes
	read := func(f *zip.File) ([]byte, error) {
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("%w: cannot open %s", model.ErrInvalid, f.Name)
		}
		defer func() { _ = rc.Close() }()
		// The declared size is not trusted: read at most the remaining budget.
		out, err := io.ReadAll(io.LimitReader(rc, budget+1))
		if err != nil {
			return nil, fmt.Errorf("%w: cannot read %s", model.ErrInvalid, f.Name)
		}
		budget -= int64(len(out))
		if budget < 0 {
			return nil, fmt.Errorf("%w: unpacked files exceed 200 MB", model.ErrInvalid)
		}
		return out, nil
	}
	parts := map[string]any{"contacts.json": &b.Contacts, "tickets.json": &b.Tickets, "messages.json": &b.Messages, "agents.json": &b.Agents}
	seen := false
	for _, f := range zr.File {
		base := strings.ToLower(path.Base(f.Name))
		if f.FileInfo().IsDir() || strings.HasPrefix(base, ".") || strings.HasPrefix(f.Name, "__MACOSX/") {
			continue
		}
		var dst any
		if base == "history.json" {
			dst = b
		} else if p, ok := parts[base]; ok {
			dst = p
		} else {
			continue
		}
		raw, err := read(f)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(raw, dst); err != nil {
			return fmt.Errorf("%w: %s is not valid JSON: %v", model.ErrInvalid, base, err)
		}
		seen = true
	}
	if !seen {
		return fmt.Errorf("%w: the zip needs history.json or tickets.json and messages.json", model.ErrInvalid)
	}
	return nil
}

func validateBundle(b model.Bundle) error {
	if b.Schema != "" && b.Schema != model.SchemaVersion {
		return fmt.Errorf("%w: unsupported schema %q (expected %s)", model.ErrInvalid, b.Schema, model.SchemaVersion)
	}
	if len(b.Tickets) == 0 {
		return fmt.Errorf("%w: the file has no tickets", model.ErrInvalid)
	}
	if len(b.Tickets) > model.MaxTickets || len(b.Messages) > model.MaxMessages {
		return fmt.Errorf("%w: at most %d tickets and %d messages per file", model.ErrInvalid, model.MaxTickets, model.MaxMessages)
	}
	ids := make(map[string]bool, len(b.Tickets))
	for i, t := range b.Tickets {
		id := strings.TrimSpace(t.ID)
		if id == "" || len(id) > 200 {
			return fmt.Errorf("%w: tickets[%d].id is required (max 200)", model.ErrInvalid, i)
		}
		if ids[id] {
			return fmt.Errorf("%w: ticket id %q appears twice", model.ErrInvalid, id)
		}
		ids[id] = true
	}
	for i, m := range b.Messages {
		if !ids[strings.TrimSpace(m.TicketID)] {
			return fmt.Errorf("%w: messages[%d] refers to an unknown ticket", model.ErrInvalid, i)
		}
		switch m.Author {
		case "customer", "agent", "system", "":
		default:
			return fmt.Errorf("%w: messages[%d].author must be customer, agent or system", model.ErrInvalid, i)
		}
	}
	return nil
}
