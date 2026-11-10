package usecase

import (
	"net/netip"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
)

func TestToolSSRFAndSchemaBoundaries(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.1.2.3", "192.168.1.1", "169.254.169.254", "100.64.0.1", "0.0.0.0", "::1", "::ffff:127.0.0.1", "fc00::1", "fe80::1", "64:ff9b::a00:1"} {
		if publicToolIP(netip.MustParseAddr(ip)) {
			t.Errorf("allowed private/reserved %s", ip)
		}
	}
	for _, url := range []string{"http://example.com", "https://user:password@example.com", "https://127.0.0.1", "https://example.com:8443/x", "https://example.com/?secret=x", "https://example.com/#frag"} {
		if _, err := toolURL(url); err == nil {
			t.Errorf("allowed %s", url)
		}
	}
	if _, err := toolURL("https://orders.example.com/lookup"); err != nil {
		t.Fatal(err)
	}
	if ai.ValidateSchema(map[string]any{"$ref": "https://127.0.0.1/private"}) == nil {
		t.Fatal("network schema reference allowed")
	}
	if ai.ValidateJSON(objectSchema(map[string]any{"status": enumSchema("open", "closed")}), []byte(`{"status":"open","organization_id":99}`)) == nil {
		t.Fatal("extra tool authority accepted")
	}
}
func TestHoursAndChannelAllowlist(t *testing.T) {
	cfg := model.DefaultConfig()
	cfg.Channels = []string{"whatsapp"}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	// Around the clock by default; the organization's hours (CF-198) only
	// restrict the agent when business_hours_only is on.
	if !cfg.Active("whatsapp", false) || cfg.Active("email", true) {
		t.Fatal("channel restriction bypassed")
	}
	cfg.BusinessHoursOnly = true
	if !cfg.Active("whatsapp", true) || cfg.Active("whatsapp", false) {
		t.Fatal("hours restriction bypassed")
	}
}
