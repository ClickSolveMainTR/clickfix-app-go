// Package repository reuses the shared transaction and outbox implementation.
package repository

import billing "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/billing/repository"

type Repository = billing.Repository
