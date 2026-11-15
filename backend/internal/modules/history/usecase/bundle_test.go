package usecase

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/history/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/history/sample"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestDocsSampleMatchesEmbeddedSample(t *testing.T) {
	docs, err := os.ReadFile("../../../../../docs/examples/gecmis-ornek.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(docs, sample.File) {
		t.Fatal("docs/examples/gecmis-ornek.json and the embedded sample differ; copy one over the other")
	}
}

func TestParseSampleJSON(t *testing.T) {
	b, err := ParseBundle("gecmis-ornek.json", sample.File)
	if err != nil {
		t.Fatal(err)
	}
	records := BundleRecords(b, pgtype.UUID{})
	// t-1005 is open and is not history yet.
	if len(b.Tickets) != 5 || len(records) != 4 {
		t.Fatalf("tickets=%d records=%d", len(b.Tickets), len(records))
	}
	if records[0].Category != "fatura" || records[3].Category != "hesap" {
		t.Fatalf("categories %q %q", records[0].Category, records[3].Category)
	}
}

func TestParseZipParts(t *testing.T) {
	var b model.Bundle
	if err := json.Unmarshal(sample.File, &b); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, v := range map[string]any{"export/contacts.json": b.Contacts, "export/tickets.json": b.Tickets, "export/messages.json": b.Messages, "export/agents.json": b.Agents} {
		w, _ := zw.Create(name)
		_ = json.NewEncoder(w).Encode(v)
	}
	_ = zw.Close()
	got, err := ParseBundle("export.zip", buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tickets) != len(b.Tickets) || len(got.Messages) != len(b.Messages) || len(got.Contacts) != len(b.Contacts) {
		t.Fatalf("zip parts lost data: %+v", got)
	}
}

func TestParseRejectsBadFiles(t *testing.T) {
	for name, data := range map[string]string{
		"wrong schema":    `{"schema":"other/v9","tickets":[{"id":"1"}]}`,
		"no tickets":      `{"tickets":[]}`,
		"duplicate id":    `{"tickets":[{"id":"1"},{"id":"1"}]}`,
		"unknown ticket":  `{"tickets":[{"id":"1"}],"messages":[{"ticket_id":"2","body":"x"}]}`,
		"bad author":      `{"tickets":[{"id":"1"}],"messages":[{"ticket_id":"1","author":"bot","body":"x"}]}`,
		"not json at all": `hello`,
	} {
		if _, err := ParseBundle("x.json", []byte(data)); !errors.Is(err, model.ErrInvalid) {
			t.Errorf("%s: got %v", name, err)
		}
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("readme.txt")
	_, _ = w.Write([]byte("nothing here"))
	_ = zw.Close()
	if _, err := ParseBundle("x.zip", buf.Bytes()); !errors.Is(err, model.ErrInvalid) {
		t.Errorf("zip without parts: %v", err)
	}
}

// personal returns every personal value of the sample: what must never be
// stored in the archive or reach a provider.
func personal(t *testing.T) []string {
	t.Helper()
	var b model.Bundle
	if err := json.Unmarshal(sample.File, &b); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, a := range b.Agents {
		out = append(out, strings.Fields(a.Name)...)
	}
	for _, c := range b.Contacts {
		out = append(out, strings.Fields(c.Name)...)
		out = append(out, c.Email, c.Phone, c.Company, c.TaxNumber, c.Address)
	}
	out = append(out, "555 000", "0555", "Deneme Yazılım", "Örnek Kırtasiye", "Deneme Mah", "Örnek Sok")
	kept := out[:0]
	for _, v := range out {
		if len([]rune(v)) >= 3 {
			kept = append(kept, v)
		}
	}
	return kept
}

func TestBuildCaseRemovesPersonalData(t *testing.T) {
	b, err := ParseBundle("gecmis-ornek.json", sample.File)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range BundleRecords(b, pgtype.UUID{}) {
		c, ok := BuildCase(1, r)
		if !ok {
			t.Fatalf("%s skipped", r.Ref)
		}
		text := c.Subject + "\n" + c.Problem + "\n" + c.Resolution
		for _, v := range personal(t) {
			if strings.Contains(strings.ToLower(text), strings.ToLower(v)) {
				t.Errorf("%s leaks %q: %s", r.Ref, v, text)
			}
		}
		if strings.Contains(c.Ref, "t-100") || len(c.Ref) != 64 {
			t.Errorf("ref is not a hash: %q", c.Ref)
		}
	}
}

func TestEvidenceGroupAndNearest(t *testing.T) {
	a, b, c := []float32{1, 0, 0}, []float32{0.98, 0.1, 0}, []float32{0, 1, 0}
	if g := evidenceGroup([][]float32{a, b, c, nil}, 0.9); len(g) != 2 {
		t.Fatalf("group %v", g)
	}
	cs := []candidate{{id: 1, centroid: c}, {id: 2, centroid: a}}
	if i := nearest(cs, b, 0.9); i != 1 {
		t.Fatalf("nearest %d", i)
	}
	if i := nearest(cs, []float32{0, 0, 1}, 0.9); i != -1 {
		t.Fatalf("below threshold matched %d", i)
	}
	if g := grow([]float32{1, 0}, 1, []float32{0, 1}); g[0] != 0.5 || g[1] != 0.5 {
		t.Fatalf("grow %v", g)
	}
}
