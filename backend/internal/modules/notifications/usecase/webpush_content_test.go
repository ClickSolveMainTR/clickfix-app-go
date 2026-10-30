package usecase

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/google/uuid"
)

// TestWebPushIsLockScreenSafe: the browser push uses the same safe text as
// the mobile push, never the in-app title/body (CF-157), and offers the
// open / mark-read actions.
func TestWebPushIsLockScreenSafe(t *testing.T) {
	row := exportRow(t)
	title, body, data := WebPushContent(row, "en")
	if title != "Export ready" || body != "Your Users export is ready to download." {
		t.Fatalf("text %q / %q", title, body)
	}
	for _, leak := range []string{"Ahmet", "0532", "1.250"} {
		if strings.Contains(title+body, leak) {
			t.Fatalf("leaks %q", leak)
		}
	}
	acts, _ := data["actions"].([]map[string]string)
	if len(acts) != 2 || acts[0]["action"] != "open" || acts[1]["action"] != "read" || acts[1]["title"] != "Mark read" {
		t.Fatalf("actions %v", acts)
	}
	if data["action_url"] != "/platform/exports/0b7c" || data["tag"] != "notification:"+row.Uuid.String() || data["category"] != "generic" {
		t.Fatalf("data %v", data)
	}
}

func TestPushUsesEnrichedText(t *testing.T) {
	conv := uuid.New()
	row := conversationRow(conv)
	payload, _ := json.Marshal(map[string]any{
		"kind": "message.received", "category": "conversation_assign",
		"push": map[string]string{"title": "Yeni mesaj", "body": "WhatsApp · #12"},
	})
	row.Payload = payload
	msg := BuildScopedPushMessage(row, "tr", db.PushDevice{Token: "ExponentPushToken[a]"}, PushScope{OrganizationSlug: "acme"})
	if msg.Title != "Yeni mesaj" || msg.Body != "WhatsApp · #12" || msg.CategoryID != PushCategoryConversationAssign {
		t.Fatalf("expo %+v", msg)
	}
	title, body, data := WebPushContent(row, "tr")
	acts, _ := data["actions"].([]map[string]string)
	if title != "Yeni mesaj" || body != "WhatsApp · #12" || acts[0]["title"] != "Aç" || acts[1]["title"] != "Okundu" {
		t.Fatalf("web %q %q %v", title, body, acts)
	}
	// Assigning needs the organization: without it only the generic buttons.
	if m := BuildPushMessage(row, "tr", db.PushDevice{}); m.CategoryID != PushCategoryGeneric {
		t.Fatalf("no org category %q", m.CategoryID)
	}
}
