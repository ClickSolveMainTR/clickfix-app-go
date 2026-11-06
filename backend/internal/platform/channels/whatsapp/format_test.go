package whatsapp_test

import (
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
)

// CF-177: text, captions and interactive bodies go out in WhatsApp's own
// marks; the 4096 limit counts the text that is sent.
func TestSendTranslatesFormat(t *testing.T) {
	g := newGraph(t)
	sendOK(g)
	src := "**Siparişiniz** *yolda*: [takip](https://kargo.example/t_1), 2*3*4"
	want := "*Siparişiniz* _yolda_: takip (https://kargo.example/t_1), 2*3*4"
	msgs := []channels.OutboundMessage{
		{To: to(), Text: src},
		{To: to(), Text: src, Attachments: []channels.OutboundAttachment{{ContentType: "image/jpeg", URL: "https://api.clickfix.app/f/2"}}},
		{To: to(), Text: src, Attributes: map[string]any{"buttons": []any{map[string]any{"id": "yes", "title": "Evet"}}}},
		// Twice the same message: a retry is translated from the source again.
		{To: to(), Text: src},
	}
	for _, m := range msgs {
		if _, err := g.adapter().Send(t.Context(), instance(), m); err != nil {
			t.Fatal(err)
		}
	}
	got := []any{
		g.bodies[0]["text"].(map[string]any)["body"],
		g.bodies[1]["image"].(map[string]any)["caption"],
		g.bodies[2]["interactive"].(map[string]any)["body"].(map[string]any)["text"],
		g.bodies[3]["text"].(map[string]any)["body"],
	}
	for i, v := range got {
		if v != want {
			t.Errorf("message %d: %q", i, v)
		}
	}

	// 4098 characters in the source, 4096 once "**" became "*".
	if _, err := g.adapter().Send(t.Context(), instance(), channels.OutboundMessage{To: to(), Text: "**" + strings.Repeat("a", 4094) + "**"}); err != nil {
		t.Fatalf("text within the limit after translation was refused: %v", err)
	}
	if _, err := g.adapter().Send(t.Context(), instance(), channels.OutboundMessage{To: to(), Text: strings.Repeat("a", 4097)}); !channels.IsPermanent(err) {
		t.Fatalf("too long text: %v", err)
	}
}
