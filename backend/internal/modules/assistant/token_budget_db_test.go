package assistant_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"

	appsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/apps"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/assistant"
	automationmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/automation"
	channelsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels"
	conversationsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/conversations"
	systemsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/integrations/systems"
	publicapimodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/publicapi"
	slamodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sla"
	teamsmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/teams"
	webhooksmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/webhooks"
	widgetmodule "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/widget"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/ai"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/capability"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/rbac"
)

// requestTokens estimates the input tokens of one provider request: the
// instructions, the tool definitions and the transcript, ≈ 4 bytes per
// token (the same estimate for every scenario, before and after CF-211).
func requestTokens(req ai.Request) (instr, tools, turns int) {
	type wireTool struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Schema      map[string]any `json:"input_schema"`
	}
	ws := []wireTool{}
	for _, t := range req.Tools {
		ws = append(ws, wireTool{t.Name, t.Description, t.Schema})
	}
	rawTools, _ := json.Marshal(ws)
	n := 0
	for _, t := range req.Turns {
		n += len(t.Text)
		for _, c := range t.Calls {
			n += len(c.Input) + len(c.Name)
		}
	}
	return (len(req.Instructions) + 3) / 4, (len(rawTools) + 3) / 4, (n + 3) / 4
}

// TestTokenBudget measures the input of one reply's first model call on a
// production-like registry (CF-211). It always passes; the table is in the
// test log and, with TOKEN_REPORT=<file>, written to that file.
func TestTokenBudget(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	reg := capability.NewRegistry().
		Register(assistant.CoreCapabilities(f.q)...).
		Register(automationmodule.Capabilities()...).
		Register(channelsmodule.Capabilities()...).
		Register(automationmodule.RuleCapabilities()...).
		Register(slamodule.Capabilities()...).
		Register(slamodule.WriteCapabilities()...).
		Register(automationmodule.RuleWriteCapabilities()...).
		Register(assistant.TeamAndAICapabilities(f.q)...).
		Register(teamsmodule.Capabilities()...).
		Register(widgetmodule.Capabilities("https://app.clickfix.example")...).
		Register(webhooksmodule.WriteCapabilities()...).
		Register(publicapimodule.Capabilities("https://api.clickfix.example")...).
		Register(appsmodule.Capabilities()...).
		Register(webhooksmodule.Capabilities()...).
		Register(systemsmodule.Capabilities()...).
		Register(systemsmodule.SetupCapabilities()...).
		Register(conversationsmodule.Capabilities()...)
	for _, extra := range budgetExtraCapabilities(f) {
		reg.Register(extra...)
	}
	f.svc.Caps = reg
	// An owner: every permission any capability asks for.
	perms := map[string]bool{rbac.PermTenantAssistantUse: true}
	for _, cp := range reg.All() {
		if cp.Permission != "" {
			perms[cp.Permission] = true
		}
	}
	scope := f.scope
	scope.MemberRole = "owner"
	scope.Permissions = nil
	for p := range perms {
		scope.Permissions = append(scope.Permissions, p)
	}
	c := assistant.Caller{Scope: scope, UserID: f.user, Locale: "tr"}
	longReply := strings.Repeat("Kanal kurulumu için önce Kanallar sayfasını açın, ardından REST API türünü seçip webhook adresini girin. ", 8)

	type scenario struct {
		name, route, prompt string
		history             int
	}
	scenarios := []scenario{
		{"Pano, ilk mesaj", "/dashboard", "Merhaba, neler yapabilirsin?", 0},
		{"Kanallar, REST API kurulumu", "/channels", "REST API kanalını sistemimize bağlamak istiyorum", 0},
		{"Gelen kutusu, konuşma arama", "/conversations", "Bekleyen iade konuşmalarını listele", 0},
		{"İş akışları, 10 tur geçmiş", "/automation/workflows", "Yeni mesajda VIP etiketi varsa ekibe ata", 10},
		{"Ayarlar, 20 mesaj geçmiş + rapor", "/settings", "Bu haftanın çözüm süresi raporu nasıl?", 10},
	}
	rows := []string{"| Senaryo | Araç sayısı | Sistem istemi | Araç tanımları | Geçmiş + bağlam | Toplam |", "|---|---:|---:|---:|---:|---:|"}
	for _, sc := range scenarios {
		pc := assistant.Context{Surface: "web", Route: sc.route}
		th, err := f.svc.CreateThread(ctx, f.scope.ID, f.user, pc)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < sc.history; i++ {
			reply, _ := json.Marshal(map[string]any{"reply": longReply, "suggestions": []string{"Devam"}})
			f.prov.steps = []ai.Result{{Text: string(reply)}}
			if _, err := f.svc.Send(ctx, c, th.UUID, fmt.Sprintf("Önceki soru %d: kanalı nasıl test ederim, webhook teslimatı neden başarısız?", i), pc); err != nil {
				t.Fatal(err)
			}
		}
		f.prov.mu.Lock()
		f.prov.reqs, f.prov.steps = nil, nil
		f.prov.mu.Unlock()
		if _, err := f.svc.Send(ctx, c, th.UUID, sc.prompt, pc); err != nil {
			t.Fatal(err)
		}
		f.prov.mu.Lock()
		req := f.prov.reqs[0]
		f.prov.mu.Unlock()
		in, tl, tu := requestTokens(req)
		rows = append(rows, fmt.Sprintf("| %s | %d | %d | %d | %d | %d |", sc.name, len(req.Tools), in, tl, tu, in+tl+tu))
	}
	all := 0
	for _, cp := range reg.All() {
		if cp.OnSurface(capability.SurfaceMCP) {
			all++
		}
	}
	rows = append(rows, fmt.Sprintf("\nKayıtlı yetenek: %d (MCP yüzeyinde: %d)", len(reg.All()), all))
	out := strings.Join(rows, "\n")
	t.Log("\n" + out)
	if path := os.Getenv("TOKEN_REPORT"); path != "" {
		_ = os.WriteFile(path, []byte(out+"\n"), 0o644)
	}
	_ = uuid.Nil
}
