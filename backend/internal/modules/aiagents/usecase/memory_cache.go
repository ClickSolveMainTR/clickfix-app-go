package usecase

import (
	"context"
	"strings"

	memmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aimemory/model"
)

// MemoryProbe is the side-effect free lookup of the knowledge layer
// (aimemory Service.Probe, CF-216).
type MemoryProbe interface {
	Probe(context.Context, memmodel.Query) (bool, error)
}

// MemoryCache adapts the knowledge layer to the measurement set's
// AnswerCache (CF-225): a case counts as a cache hit when the customer AI
// would answer its question from the answer cache or a knowledge unit,
// without a generation call. Probing neither counts the lookup nor writes
// the cache, so measurement runs do not move the panel's hit rate.
type MemoryCache struct{ Memory MemoryProbe }

// Lookup implements AnswerCache.
func (m MemoryCache) Lookup(ctx context.Context, org int64, department *int64, transcript, locale string) (bool, error) {
	q := memmodel.Query{Organization: org, Audience: memmodel.AudienceCustomer, Locale: locale, Question: lastCustomerTurn(transcript)}
	q.Scope.DepartmentID = department
	return m.Memory.Probe(ctx, q)
}

// lastCustomerTurn is the question of a case transcript: the text after the
// last "Customer:" prefix, or the whole transcript without one.
func lastCustomerTurn(transcript string) string {
	lines := strings.Split(strings.TrimSpace(transcript), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(lines[i]), "Customer:"); ok {
			return strings.TrimSpace(rest)
		}
	}
	return strings.TrimSpace(transcript)
}

// ReadyProbe is a MemoryProbe that can tell whether it is configured.
type ReadyProbe interface {
	MemoryProbe
	Ready() bool
}

// WireMeasurement connects the measurement set to departments and the
// knowledge layer (CF-225). The cache stays nil, and cache hit rate
// unmeasured, when the knowledge layer has no embedding model.
func WireMeasurement(s *Service, departments DepartmentDirectory, memory ReadyProbe) {
	s.DepartmentSets = departments
	if memory != nil && memory.Ready() {
		s.Cache = MemoryCache{Memory: memory}
	}
}
