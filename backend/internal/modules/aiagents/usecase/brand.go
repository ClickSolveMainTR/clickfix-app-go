package usecase

import (
	"context"
	"log/slog"
	"unicode/utf8"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/vectorstore"
	"github.com/google/uuid"
)

// conversationScope is the knowledge scope of a conversation: its brand and
// its department plus organization-wide knowledge.
func conversationScope(org int64, conv db.Conversation) vectorstore.Scope {
	return vectorstore.Conversation(org, conv.BrandID, conv.DepartmentID)
}

// brandPersona is the ai_persona of the conversation's brand ("" without a
// brand). A lookup failure only drops the persona; the reply still runs.
func (s *Service) brandPersona(ctx context.Context, org int64, conv db.Conversation) string {
	if !conv.BrandID.Valid {
		return ""
	}
	brand, err := s.Repo.Q.GetBrand(ctx, db.GetBrandParams{OrganizationID: org, Uuid: uuid.UUID(conv.BrandID.Bytes)})
	if err != nil {
		slog.WarnContext(ctx, "ai_brand_persona_unavailable", "organization_id", org, "error", err)
		return ""
	}
	persona := brand.AiPersona
	// RunAgent rejects longer personas; trim on a rune boundary instead.
	for len(persona) > 4000 {
		_, size := utf8.DecodeLastRuneInString(persona)
		persona = persona[:len(persona)-size]
	}
	return persona
}
