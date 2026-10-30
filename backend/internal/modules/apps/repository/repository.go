package repository

import "github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"

type Store struct{ *db.Queries }

func New(q *db.Queries) *Store { return &Store{Queries: q} }
