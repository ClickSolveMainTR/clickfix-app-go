package pushtext

import (
	"strings"
	"testing"
)

func TestForRendersSafeVarsOnly(t *testing.T) {
	got := For("exports.ready", "tr", map[string]string{"resource": "Kullanıcılar", "email": "a@b.c"})
	if got.Title != "Dışa aktarım hazır" || !strings.Contains(got.Body, "Kullanıcılar") || strings.Contains(got.Body, "a@b.c") {
		t.Fatalf("got %+v", got)
	}
	en := For("bulk.completed", "en", map[string]string{"succeeded": "12"})
	if en.Title != "Bulk action finished" || en.Body != "The bulk action on 12 records has finished." {
		t.Fatalf("en %+v", en)
	}
}

func TestForFallsBackWhenVarMissing(t *testing.T) {
	got := For("exports.ready", "tr", nil)
	if strings.Contains(got.Title+got.Body, "{{") || got.Body != "Yeni bir bildiriminiz var." {
		t.Fatalf("missing var leaked: %+v", got)
	}
	unknown := For("nope.kind", "en", nil)
	if unknown.Title != "Notification" || unknown.Body != "You have a new notification." {
		t.Fatalf("unknown kind %+v", unknown)
	}
	if !Known("bulk.failed") || Known("nope") {
		t.Fatal("Known mismatch")
	}
}

// Every in-app template (seeded notification_templates, channel "inapp") gets
// a dedicated push, so no lock screen shows the generic "new notification".
func TestEveryInAppTemplateHasText(t *testing.T) {
	codes := []string{
		"auth.profile_updated", "auth.welcome",
		"billing.canceled", "billing.card_expiring", "billing.invoice_issued", "billing.payment_failed",
		"billing.payment_received", "billing.payment_required", "billing.quota_exhausted", "billing.quota_warning",
		"billing.reconciliation_mismatch", "billing.refund_succeeded", "billing.renewal_reminder", "billing.suspended",
		"billing.transfer_approved", "billing.transfer_pending", "billing.transfer_rejected", "billing.trial_ending",
		"billing.wallet_low", "billing.welcome",
		"bulk.completed", "bulk.failed", "company.health_dropped", "conversation.assigned", "conversation.mentioned",
		"csat.submitted", "exports.ready", "imports.applied", "imports.failed", "message.received",
		"notifications.test", "sla.breached", "sla.warning", "ticket.assigned", "ticket.commented",
	}
	for _, c := range codes {
		if !Known(c) {
			t.Errorf("no push text for %s", c)
		}
	}
}

// A placeholder outside SafeVars is never substituted, so its text would
// always fall back to the generic body.
func TestCatalogUsesOnlySafeVars(t *testing.T) {
	safe := map[string]bool{}
	for _, v := range SafeVars {
		safe[v] = true
	}
	for kind, p := range catalog {
		for _, s := range []string{p.tr.Title, p.tr.Body, p.en.Title, p.en.Body} {
			for _, m := range placeholderRE.FindAllStringSubmatch(s, -1) {
				if !safe[strings.ToLower(m[1])] {
					t.Errorf("%s uses unsafe var %q", kind, m[1])
				}
			}
		}
	}
}
