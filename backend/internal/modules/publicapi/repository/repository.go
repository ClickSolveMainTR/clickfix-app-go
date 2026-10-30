package repository

import "github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"

// Store contains generated, organization-scoped integration queries.
type Store struct{ *db.Queries }

func New(q *db.Queries) *Store { return &Store{q} }
