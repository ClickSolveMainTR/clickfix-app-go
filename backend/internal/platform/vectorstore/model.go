package vectorstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// SearchModel is the model a query vector for the organization must be built
// with. It is the target model unless the organization's stored vectors are
// still from an older model (re-embedding in progress); then search keeps
// using the old vectors until the switch.
func SearchModel(ctx context.Context, db Querier, src Source, org int64, target string) (string, error) {
	if err := src.valid(); err != nil {
		return "", err
	}
	var model string
	// #nosec G201 -- table name is a validated code constant.
	err := db.QueryRow(ctx, fmt.Sprintf(`SELECT embedding_model FROM %s WHERE organization_id=$1 AND embedding IS NOT NULL
 ORDER BY (embedding_model=$2) DESC LIMIT 1`, src.Table), org, target).Scan(&model)
	if errors.Is(err, pgx.ErrNoRows) {
		return target, nil
	}
	return model, err
}

// Placement says where a freshly built target-model vector goes: into
// embedding (searchable now) or, while the organization still searches an
// older model, into pending_embedding (searchable after the switch).
type Placement struct {
	Embedding        Vector
	PendingEmbedding Vector
	PendingModel     *string
}

// Place builds the placement of vector v for the organization.
func Place(ctx context.Context, db Querier, src Source, org int64, target string, v []float32) (Placement, error) {
	if err := Check(v); err != nil {
		return Placement{}, err
	}
	model, err := SearchModel(ctx, db, src, org, target)
	if err != nil {
		return Placement{}, err
	}
	if model == target {
		return Placement{Embedding: Vector(v)}, nil
	}
	return Placement{PendingEmbedding: Vector(v), PendingModel: &target}, nil
}
