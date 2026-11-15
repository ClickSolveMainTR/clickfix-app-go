package classify

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	aimodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/model"
	aiusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aicore/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
)

type fixed struct {
	name string
	p    Prediction
	err  error
	hits int
}

func (f *fixed) Name() string { return f.name }
func (f *fixed) Predict(context.Context, Input, []Candidate) (Prediction, error) {
	f.hits++
	return f.p, f.err
}

var cands = []Candidate{{ID: 1, Name: "Fatura", Slug: "fatura"}, {ID: 2, ParentID: 1, Name: "İade", Slug: "iade"}, {ID: 3, Name: "Teknik", Slug: "teknik"}}

func TestChainStopsAtFirstDecision(t *testing.T) {
	emb := &fixed{name: "e", p: Prediction{CategoryID: 2, Confidence: 0.9, Source: SourceEmbedding, Decided: true}}
	aiStage := &fixed{name: "a", p: Prediction{CategoryID: 3, Confidence: 0.99, Source: SourceAI, Decided: true}}
	c := &Chain{Stages: []Stage{emb, aiStage}}
	res, err := c.Classify(context.Background(), Input{Text: "para iadesi"}, cands)
	if err != nil || res.CategoryID != 2 || res.Source != SourceEmbedding || res.NeedsReview || aiStage.hits != 0 {
		t.Fatalf("res = %+v err=%v ai hits=%d", res, err, aiStage.hits)
	}
}

func TestChainFallsThroughToAIAndHuman(t *testing.T) {
	emb := &fixed{name: "e", p: Prediction{CategoryID: 2, Confidence: 0.7, Source: SourceEmbedding}}
	aiStage := &fixed{name: "a", p: Prediction{CategoryID: 3, Confidence: 0.85, Source: SourceAI, Decided: true}}
	res, _ := (&Chain{Stages: []Stage{emb, aiStage}}).Classify(context.Background(), Input{Text: "x"}, cands)
	if res.CategoryID != 3 || res.Source != SourceAI || res.NeedsReview {
		t.Fatalf("ai decision = %+v", res)
	}
	// Nobody decides: best guess kept, an agent reviews.
	aiStage.p = Prediction{CategoryID: 3, Confidence: 0.4, Source: SourceAI}
	res, _ = (&Chain{Stages: []Stage{emb, aiStage}}).Classify(context.Background(), Input{Text: "x"}, cands)
	if res.CategoryID != 2 || !res.NeedsReview || res.Decided {
		t.Fatalf("review = %+v", res)
	}
	// Stage errors are not fatal; unknown categories are ignored.
	var seen []string
	broken := &fixed{name: "down", err: ai.ErrUnavailable}
	rogue := &fixed{name: "rogue", p: Prediction{CategoryID: 99, Confidence: 1, Decided: true}}
	c := &Chain{Stages: []Stage{broken, rogue}, OnStageError: func(s string, _ error) { seen = append(seen, s) }}
	res, err := c.Classify(context.Background(), Input{Text: "x"}, cands)
	if err != nil || res.CategoryID != 0 || !res.NeedsReview || len(seen) != 1 {
		t.Fatalf("errors = %+v %v %v", res, err, seen)
	}
}

func TestChainReviewThresholdAndPrepend(t *testing.T) {
	low := &fixed{name: "m", p: Prediction{CategoryID: 1, Confidence: 0.5, Source: SourceModel, Decided: true}}
	c := &Chain{Stages: []Stage{&fixed{name: "e"}}}
	c.Prepend(low)
	if c.Stages[0] != low {
		t.Fatal("prepend")
	}
	res, _ := c.Classify(context.Background(), Input{Text: "x"}, cands)
	if res.CategoryID != 1 || !res.NeedsReview {
		t.Fatalf("decided below review bar = %+v", res)
	}
	if _, err := c.Classify(context.Background(), Input{Text: "x"}, nil); !errors.Is(err, ErrNoCandidates) {
		t.Fatalf("no candidates = %v", err)
	}
	if res, _ := c.Classify(context.Background(), Input{Text: "  "}, cands); !res.NeedsReview || low.hits != 1 {
		t.Fatalf("empty text = %+v", res)
	}
}

func TestPaths(t *testing.T) {
	got := Paths(cands)
	if got[1].Path != "Fatura / İade" || got[2].Path != "Teknik" {
		t.Fatalf("paths = %+v", got)
	}
}

func TestEmbeddingDecide(t *testing.T) {
	s := &EmbeddingStage{}
	if p := s.decide([]Score{{1, 0.9}, {2, 0.7}}); !p.Decided || p.CategoryID != 1 {
		t.Fatalf("clear winner = %+v", p)
	}
	if p := s.decide([]Score{{1, 0.9}, {2, 0.88}}); p.Decided {
		t.Fatalf("near tie decided = %+v", p)
	}
	if p := s.decide([]Score{{1, 0.6}}); p.Decided {
		t.Fatalf("low score decided = %+v", p)
	}
}

type memStore struct {
	missing []Example
	saved   int
	scores  []Score
}

func (m *memStore) MissingExamples(context.Context, int64, int64, string) ([]Example, error) {
	out := m.missing
	m.missing = nil
	return out, nil
}
func (m *memStore) SaveExamples(_ context.Context, _ int64, _ string, ex []Example, v [][]float32) error {
	m.saved += len(ex)
	return nil
}
func (m *memStore) Similar(context.Context, int64, int64, string, []float32) ([]Score, error) {
	return m.scores, nil
}

type fakeEmbedder struct{ calls [][]string }

func (f *fakeEmbedder) Model() string { return "test-embed" }
func (f *fakeEmbedder) Embed(_ context.Context, in []string) (ai.Embeddings, error) {
	f.calls = append(f.calls, in)
	v := make([][]float32, len(in))
	for i := range v {
		v[i] = []float32{1, 0}
	}
	return ai.Embeddings{Vectors: v}, nil
}

func TestEmbeddingStageEmbedsMissingExamplesFirst(t *testing.T) {
	store := &memStore{missing: []Example{{1, "Fatura"}, {2, "İade talebi"}}, scores: []Score{{2, 0.93}, {1, 0.5}}}
	emb := &fakeEmbedder{}
	p, err := (&EmbeddingStage{Embedder: emb, Store: store}).Predict(context.Background(), Input{OrganizationID: 1, Text: "iade istiyorum"}, cands)
	if err != nil || !p.Decided || p.CategoryID != 2 || store.saved != 2 || len(emb.calls) != 2 {
		t.Fatalf("p=%+v err=%v saved=%d calls=%v", p, err, store.saved, emb.calls)
	}
	if _, err := (&EmbeddingStage{}).Predict(context.Background(), Input{}, cands); !errors.Is(err, ai.ErrNotConfigured) {
		t.Fatalf("unconfigured = %v", err)
	}
}

type fakeRunner struct {
	text string
	in   aimodel.RunInput
	opt  aiusecase.AgentOptions
}

func (f *fakeRunner) RunAgent(_ context.Context, _, _ int64, in aimodel.RunInput, opt aiusecase.AgentOptions) (aiusecase.RunResult, error) {
	f.in, f.opt = in, opt
	var r aiusecase.RunResult
	r.Text = f.text
	return r, nil
}

func TestAIStage(t *testing.T) {
	out, _ := json.Marshal(map[string]any{"category": "iade", "confidence": 0.8})
	r := &fakeRunner{text: string(out)}
	p, err := (&AIStage{AI: r}).Predict(context.Background(), Input{OrganizationID: 1, Text: "iade"}, Paths(cands))
	if err != nil || p.CategoryID != 2 || !p.Decided || p.Source != SourceAI {
		t.Fatalf("p = %+v %v", p, err)
	}
	if r.in.Task != "triage" || r.opt.Schema == nil || !strings.Contains(r.in.Input, "iade: Fatura / İade") {
		t.Fatalf("request = %+v", r.in)
	}
	r.text = `{"category":"yok","confidence":0.99}`
	if p, err := (&AIStage{AI: r}).Predict(context.Background(), Input{Text: "x"}, cands); err != nil || p.CategoryID != 0 {
		t.Fatalf("unknown slug = %+v %v", p, err)
	}
	r.text = `not json`
	if _, err := (&AIStage{AI: r}).Predict(context.Background(), Input{Text: "x"}, cands); !errors.Is(err, ai.ErrInvalidOutput) {
		t.Fatalf("bad output = %v", err)
	}
}
