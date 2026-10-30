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
		tr: Text{"Kayıp riski", "Şirketin sağlık skoru {{score}} oldu: {{company}}"},
		en: Text{"Churn risk", "Health score of {{company}} dropped to {{score}}."},
	},
	"csat.submitted": {
		tr: Text{"Yeni memnuniyet puanı", "Bir müşteri {{rating}}/5 puan verdi."},
		en: Text{"New satisfaction rating", "A customer rated {{rating}}/5."},
	},
	"bulk.failed": {
		tr: Text{"Toplu işlem başarısız", "Bir toplu işlem tamamlanamadı."},
		en: Text{"Bulk action failed", "A bulk action could not be completed."},
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
