// Package pushtext holds the lock-screen-safe texts of mobile push
// notifications. In-app titles/bodies may carry personal data; a push is
// visible on a locked screen, so it only ever uses the fixed texts below plus
// a small whitelist of harmless variables. Add a catalog entry per
// notification kind (template code) that should get a dedicated push text.
package pushtext

import (
	"regexp"
	"strings"
)

// SafeVars are the only template variables a push text may contain. They are
// copied into the in-app notification payload under "push_vars".
var SafeVars = []string{"resource", "succeeded", "number", "rating"}

// Text is one localized push.
type Text struct {
	Title string
	Body  string
}

type pair struct{ tr, en Text }

var catalog = map[string]pair{
	"exports.ready": {
		tr: Text{"Dışa aktarım hazır", "{{resource}} dışa aktarımınız indirilmeye hazır."},
		en: Text{"Export ready", "Your {{resource}} export is ready to download."},
	},
	"imports.applied": {
		tr: Text{"İçe aktarım tamamlandı", "{{resource}} içe aktarımı tamamlandı."},
		en: Text{"Import finished", "The {{resource}} import has finished."},
	},
	"bulk.completed": {
		tr: Text{"Toplu işlem tamamlandı", "{{succeeded}} kayıt üzerinde toplu işlem tamamlandı."},
		en: Text{"Bulk action finished", "The bulk action on {{succeeded}} records has finished."},
	},
	"conversation.assigned": {
		tr: Text{"Konuşma size atandı", "#{{number}} numaralı konuşma size atandı."},
		en: Text{"Conversation assigned", "Conversation #{{number}} was assigned to you."},
	},
	"message.received": {
		tr: Text{"Yeni mesaj", "#{{number}} numaralı konuşmada yeni mesaj var."},
		en: Text{"New message", "There is a new message in conversation #{{number}}."},
	},
	"conversation.mentioned": {
		tr: Text{"Sizden bahsedildi", "#{{number}} numaralı konuşmada sizden bahsedildi."},
		en: Text{"You were mentioned", "You were mentioned in conversation #{{number}}."},
	},
	"ticket.assigned": {
		tr: Text{"Ürün talebi size atandı", "Bir ürün talebi size atandı."},
		en: Text{"Product request assigned", "A product request was assigned to you."},
	},
	"ticket.commented": {
		tr: Text{"Ürün talebine yorum eklendi", "Takip ettiğiniz bir ürün talebine yorum eklendi."},
		en: Text{"New product request comment", "A product request you follow has a new comment."},
	},
	"sla.warning": {
		tr: Text{"SLA süresi yaklaşıyor", "Bir konuşmanın SLA süresi dolmak üzere."},
		en: Text{"SLA due soon", "An SLA target is about to be missed."},
	},
	"sla.breached": {
		tr: Text{"SLA ihlal edildi", "Bir SLA hedefi aşıldı."},
		en: Text{"SLA breached", "An SLA target was missed."},
	},
	"company.health_dropped": {
		tr: Text{"Kayıp riski", "Bir şirketin sağlık skoru düştü."},
		en: Text{"Churn risk", "A company's health score dropped."},
	},
	"csat.submitted": {
		tr: Text{"Yeni memnuniyet puanı", "Bir müşteri {{rating}}/5 puan verdi."},
		en: Text{"New satisfaction rating", "A customer rated {{rating}}/5."},
	},
	"bulk.failed": {
		tr: Text{"Toplu işlem başarısız", "Bir toplu işlem tamamlanamadı."},
		en: Text{"Bulk action failed", "A bulk action could not be completed."},
	},
	"imports.failed": {
		tr: Text{"İçe aktarım başarısız", "{{resource}} içe aktarımı tamamlanamadı."},
		en: Text{"Import failed", "The {{resource}} import could not be completed."},
	},
	"notifications.test": {
		tr: Text{"Test bildirimi", "Bildirimler bu cihazda çalışıyor."},
		en: Text{"Test notification", "Notifications work on this device."},
	},
	"auth.welcome": {
		tr: Text{"ClickFix’e hoş geldiniz", "Hesabınız hazır."},
		en: Text{"Welcome to ClickFix", "Your account is ready."},
	},
	"auth.profile_updated": {
		tr: Text{"Profil güncellendi", "Hesap bilgileriniz değiştirildi."},
		en: Text{"Profile updated", "Your account details were changed."},
	},
	"billing.welcome": {
		tr: Text{"Aboneliğiniz başladı", "ClickFix aboneliğiniz etkinleştirildi."},
		en: Text{"Subscription started", "Your ClickFix subscription is active."},
	},
	"billing.payment_received": {
		tr: Text{"Ödeme alındı", "Ödemeniz başarıyla alındı."},
		en: Text{"Payment received", "Your payment was received."},
	},
	"billing.payment_failed": {
		tr: Text{"Ödeme başarısız", "Son ödeme alınamadı. Ödeme yönteminizi kontrol edin."},
		en: Text{"Payment failed", "The last payment failed. Check your payment method."},
	},
	"billing.payment_required": {
		tr: Text{"Ödeme gerekiyor", "Hizmetin sürmesi için ödeme gerekiyor."},
		en: Text{"Payment required", "A payment is needed to keep the service running."},
	},
	"billing.invoice_issued": {
		tr: Text{"Yeni fatura", "Yeni bir faturanız düzenlendi."},
		en: Text{"New invoice", "A new invoice was issued."},
	},
	"billing.refund_succeeded": {
		tr: Text{"İade tamamlandı", "İadeniz işlendi."},
		en: Text{"Refund completed", "Your refund was processed."},
	},
	"billing.transfer_pending": {
		tr: Text{"Havale onay bekliyor", "Havale bildiriminiz alındı, onay bekliyor."},
		en: Text{"Transfer pending", "Your bank transfer was received and awaits approval."},
	},
	"billing.transfer_approved": {
		tr: Text{"Havale onaylandı", "Havale ödemeniz onaylandı."},
		en: Text{"Transfer approved", "Your bank transfer was approved."},
	},
	"billing.transfer_rejected": {
		tr: Text{"Havale reddedildi", "Havale bildiriminiz onaylanmadı."},
		en: Text{"Transfer rejected", "Your bank transfer was not approved."},
	},
	"billing.reconciliation_mismatch": {
		tr: Text{"Ödeme uyuşmazlığı", "Bir ödeme kaydında uyuşmazlık bulundu."},
		en: Text{"Payment mismatch", "A payment record does not match."},
	},
	"billing.card_expiring": {
		tr: Text{"Kartınızın süresi doluyor", "Kayıtlı kartınızın süresi yakında doluyor."},
		en: Text{"Card expiring", "Your saved card expires soon."},
	},
	"billing.renewal_reminder": {
		tr: Text{"Yenileme yaklaşıyor", "Aboneliğiniz yakında yenilenecek."},
		en: Text{"Renewal coming up", "Your subscription renews soon."},
	},
	"billing.trial_ending": {
		tr: Text{"Deneme süresi bitiyor", "Deneme süreniz yakında sona eriyor."},
		en: Text{"Trial ending", "Your trial ends soon."},
	},
	"billing.quota_warning": {
		tr: Text{"Kota dolmak üzere", "Kullanım kotanız dolmak üzere."},
		en: Text{"Quota almost used", "You are close to your usage quota."},
	},
	"billing.quota_exhausted": {
		tr: Text{"Kota doldu", "Kullanım kotanız doldu."},
		en: Text{"Quota used up", "You have used up your usage quota."},
	},
	"billing.wallet_low": {
		tr: Text{"Bakiye azaldı", "Cüzdan bakiyeniz azaldı."},
		en: Text{"Low balance", "Your wallet balance is low."},
	},
	"billing.suspended": {
		tr: Text{"Hizmet askıya alındı", "Ödeme alınamadığı için hizmet askıya alındı."},
		en: Text{"Service suspended", "The service was suspended because payment failed."},
	},
	"billing.canceled": {
		tr: Text{"Abonelik iptal edildi", "Aboneliğiniz iptal edildi."},
		en: Text{"Subscription canceled", "Your subscription was canceled."},
	},
}

var fallback = pair{
	tr: Text{"Bildirim", "Yeni bir bildiriminiz var."},
	en: Text{"Notification", "You have a new notification."},
}

var placeholderRE = regexp.MustCompile(`\{\{\s*\.?([a-zA-Z0-9_]+)\s*\}\}`)

// For renders the push for a notification kind (e.g. "exports.ready") in
// locale ("tr" | "en"; anything else → "tr"). Unknown kinds get a generic
// text. Only SafeVars are substituted; a text whose variables are missing
// falls back to the kind-less generic body so no "{{…}}" or empty gap leaks.
func For(kind, locale string, vars map[string]string) Text {
	p, ok := catalog[kind]
	if !ok {
		p = fallback
	}
	t := p.tr
	fb := fallback.tr
	if strings.EqualFold(strings.TrimSpace(locale), "en") {
		t = p.en
		fb = fallback.en
	}
	safe := map[string]string{}
	for _, k := range SafeVars {
		if v := strings.TrimSpace(vars[k]); v != "" {
			safe[k] = v
		}
	}
	title := render(t.Title, safe)
	body := render(t.Body, safe)
	if title == "" {
		title = fb.Title
	}
	if body == "" {
		body = fb.Body
	}
	return Text{Title: title, Body: body}
}

// render substitutes vars; returns "" when any placeholder has no value.
func render(tpl string, vars map[string]string) string {
	missing := false
	out := placeholderRE.ReplaceAllStringFunc(tpl, func(m string) string {
		v := vars[strings.ToLower(placeholderRE.FindStringSubmatch(m)[1])]
		if v == "" {
			missing = true
		}
		return v
	})
	if missing {
		return ""
	}
	return strings.TrimSpace(out)
}

// Known reports whether kind has a dedicated text.
func Known(kind string) bool {
	_, ok := catalog[kind]
	return ok
}
