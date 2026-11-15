// Package vectorstore is the shared pgvector layer of the knowledge stack
// (CF-214): one embedding model with a fixed dimension, a scope filter
// (organization → brand → department), cosine similarity with a threshold,
// and background re-embedding when the configured model changes.
//
// Every vector table follows one column convention so the same search,
// placement and re-embedding code serves all of them:
//
//	id BIGINT, organization_id BIGINT, brand_id UUID NULL, department_id UUID NULL,
//	embedding vector(N) NULL, embedding_model TEXT NOT NULL,
//	pending_embedding vector(N) NULL, pending_model TEXT NULL, <text column>
//
// NULL brand_id / department_id means "organization wide".
package vectorstore

import (
	"database/sql/driver"
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
)

// ErrDimension reports a vector whose size is not ai.EmbeddingDimensions.
var ErrDimension = errors.New("vectorstore: vector dimension mismatch")

// Vector is a pgvector value. It is sent in pgvector's text form ("[1,2,3]"),
// so no connection-level type registration is needed. A nil Vector is NULL.
type Vector []float32

func (v Vector) Value() (driver.Value, error) {
	if v == nil {
		return nil, nil
	}
	var b strings.Builder
	b.Grow(len(v) * 12)
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(x), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String(), nil
}

func (v *Vector) Scan(src any) error {
	var text string
	switch s := src.(type) {
	case nil:
		*v = nil
		return nil
	case string:
		text = s
	case []byte:
		text = string(s)
	default:
		return errors.New("vectorstore: unsupported vector source")
	}
	text = strings.TrimSpace(text)
	if len(text) < 2 || text[0] != '[' || text[len(text)-1] != ']' {
		return errors.New("vectorstore: malformed vector")
	}
	body := text[1 : len(text)-1]
	if body == "" {
		*v = Vector{}
		return nil
	}
	parts := strings.Split(body, ",")
	out := make(Vector, len(parts))
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 32)
		if err != nil {
			return err
		}
		out[i] = float32(f)
	}
	*v = out
	return nil
}

// Check accepts only finite, non-zero vectors of the fixed dimension.
func Check(v []float32) error {
	if len(v) != ai.EmbeddingDimensions {
		return ErrDimension
	}
	norm := float64(0)
	for _, x := range v {
		if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
			return ai.ErrInvalidOutput
		}
		norm += float64(x) * float64(x)
	}
	if norm == 0 {
		return ai.ErrInvalidOutput
	}
	return nil
}
