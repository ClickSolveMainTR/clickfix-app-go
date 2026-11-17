package realtime_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/remotesupport"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/realtime"
	"github.com/google/uuid"
)

// Centrifugo rejects publishes and subscriptions to a namespace its config
// does not declare ("unknown channel"), silently killing that live feed.
// Every channel the backend publishes on must have its namespace in both
// deploy configs.
func TestCentrifugoConfigsDeclareBackendNamespaces(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	channels := []string{
		realtime.InboxChannel(id),
		realtime.OrgChannel(id),
		realtime.QRLoginChannel("abc"),
		"user:" + id.String(),
		remotesupport.Channel(id),
	}
	for _, name := range []string{"config.json", "config.prod.json"} {
		path := filepath.Join("..", "..", "..", "deploy", "centrifugo", name)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var cfg struct {
			Namespaces []struct {
				Name string `json:"name"`
			} `json:"namespaces"`
		}
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		declared := map[string]bool{}
		for _, ns := range cfg.Namespaces {
			declared[ns.Name] = true
		}
		for _, ch := range channels {
			ns, _, _ := strings.Cut(ch, ":")
			if !declared[ns] {
				t.Errorf("%s: namespace %q (channel %s) is not declared", name, ns, ch)
			}
		}
	}
}
