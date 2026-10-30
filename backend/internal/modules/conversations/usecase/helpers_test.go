package usecase

import (
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations/model"
	"github.com/google/uuid"
)

func TestToMessageUsesGroupAuthorName(t *testing.T) {
	t.Parallel()
	m := db.Message{ContentAttributes: []byte(`{"author_name":" Ayşe Yılmaz "}`)}
	got := toMessage(m, uuid.New(), model.Sender{Type: model.SenderContact, Name: "#destek"})
	if got.Sender.Name != "Ayşe Yılmaz" {
		t.Fatalf("contact sender = %q", got.Sender.Name)
	}
	agent := toMessage(m, uuid.New(), model.Sender{Type: model.SenderAgent, Name: "Temsilci"})
	if agent.Sender.Name != "Temsilci" {
		t.Fatalf("agent sender overwritten: %q", agent.Sender.Name)
	}
	plain := toMessage(db.Message{}, uuid.New(), model.Sender{Type: model.SenderContact, Name: "Ali"})
	if plain.Sender.Name != "Ali" {
		t.Fatalf("contact name lost: %q", plain.Sender.Name)
	}
}
