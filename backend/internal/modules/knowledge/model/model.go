// Package model defines organization-owned knowledge base inputs and citations.
package model

import (
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrInvalid  = errors.New("knowledge: invalid input")
	ErrNotFound = errors.New("knowledge: not found")
	ErrConflict = errors.New("knowledge: conflict")
)
var slug = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

type CollectionInput struct {
	Name   string     `json:"name"`
	Slug   string     `json:"slug"`
	Kind   string     `json:"kind"`
	Parent *uuid.UUID `json:"parent_uuid"`
}

func (in CollectionInput) Validate() error {
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 200 || len(in.Slug) > 200 || !slug.MatchString(in.Slug) {
		return ErrInvalid
	}
	if in.Kind != "collection" && in.Kind != "category" {
		return ErrInvalid
	}
	return nil
}

type ArticleInput struct {
	Collection *uuid.UUID `json:"collection_uuid"`
	Slug       string     `json:"slug"`
	Locale     string     `json:"locale"`
	Title      string     `json:"title"`
	Body       string     `json:"body"`
	Tags       []string   `json:"tags"`
	Visibility string     `json:"visibility"`
	// Required on update: optimistic locking prevents silently overwriting another editor.
	Version int32 `json:"version"`
}

func (in ArticleInput) Validate() error {
	if len(in.Slug) > 200 || !slug.MatchString(in.Slug) || strings.TrimSpace(in.Title) == "" || len(in.Title) > 300 || len(in.Body) > 200000 || len(in.Tags) > 30 {
		return ErrInvalid
	}
	if in.Locale != "tr" && in.Locale != "en" {
		return ErrInvalid
	}
	if in.Visibility != "public" && in.Visibility != "internal" {
		return ErrInvalid
	}
	for _, t := range in.Tags {
		if len(t) > 100 {
			return ErrInvalid
		}
	}
	return nil
}

type Citation struct {
	ChunkUUID   uuid.UUID `json:"chunk_uuid"`
	ArticleUUID uuid.UUID `json:"article_uuid"`
	Version     int32     `json:"version"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	Slug        string    `json:"slug"`
	Locale      string    `json:"locale"`
	Score       float64   `json:"score"`
}
