package usecase

import (
	"math"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/history/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
)

// Case is an anonymized record ready for the archive and the provider.
type Case struct {
	Ref        string
	Subject    string
	Problem    string
	Resolution string
	Messages   int32
}

// BuildCase anonymizes a record on the server. Everything that may later
// reach a provider (subject, problem, resolution) comes out of here, so this
// is the single place where personal data is removed. ok is false when the
// record has no customer problem or no resolution to learn from.
func BuildCase(org int64, r Record) (Case, bool) {
	anon := ai.NewAnonymizer()
	for _, k := range r.Known {
		anon.Known(k.Kind, k.Value)
	}
	var problem, answers []string
	for _, l := range r.Lines {
		switch l.Author {
		case "customer":
			problem = append(problem, l.Body)
		case "agent":
			answers = append(answers, l.Body)
		}
	}
	resolution := r.Resolution
	if strings.TrimSpace(resolution) == "" && len(answers) > 0 {
		// The last agent replies carry the resolution; greetings come first.
		resolution = strings.Join(answers[max(0, len(answers)-2):], "\n")
	}
	c := Case{
		Ref:        refHash(org, r.Ref),
		Subject:    truncate(anon.Redact(r.Subject), model.MaxSubjectRunes),
		Problem:    truncate(anon.Redact(strings.Join(problem, "\n")), model.MaxCaseTextRunes),
		Resolution: truncate(anon.Redact(resolution), model.MaxCaseTextRunes),
		Messages:   int32(len(r.Lines)), // #nosec G115 -- bounded by model.MaxMessages
	}
	if c.Problem == "" && c.Subject != "" {
		c.Problem = c.Subject
	}
	return c, c.Problem != "" && c.Resolution != ""
}

// EmbedText is what represents a case's problem in vector space.
func (c Case) EmbedText() string { return truncate(c.Subject+"\n"+c.Problem, model.MaxCaseTextRunes) }

func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// grow is the running mean of a cluster after adding v.
func grow(centroid []float32, size int32, v []float32) []float32 {
	out := make([]float32, len(centroid))
	n := float32(size)
	for i := range centroid {
		out[i] = (centroid[i]*n + v[i]) / (n + 1)
	}
	return out
}

type candidate struct {
	id       int64
	centroid []float32
	size     int32
}

// nearest returns the best cluster at or above the threshold, or -1.
func nearest(cs []candidate, v []float32, threshold float64) int {
	best, score := -1, threshold
	for i, c := range cs {
		if s := cosine(c.centroid, v); s >= score {
			best, score = i, s
		}
	}
	return best
}

// evidenceGroup finds the largest set of cases whose resolutions agree: the
// resolution with the most neighbours at or above threshold, and those
// neighbours. Its size is the cluster's evidence that one solution works.
func evidenceGroup(resolutions [][]float32, threshold float64) []int {
	var best []int
	for _, a := range resolutions {
		if len(a) == 0 {
			continue
		}
		group := []int{}
		for j, b := range resolutions {
			if len(b) > 0 && cosine(a, b) >= threshold {
				group = append(group, j)
			}
		}
		if len(group) > len(best) {
			best = group
		}
	}
	return best
}
