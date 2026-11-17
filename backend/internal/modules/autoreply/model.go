// Package autoreply keeps the customer from waiting in silence (CF-198):
// every new conversation gets an acknowledgement with the estimated reply
// time inside working hours, an out-of-hours message outside them, and the
// customer is told when the AI agent hands the conversation to a person.
// Each text has one source, this module; the working hours it follows are
// the SLA module's schedule.
package autoreply

import (
	"fmt"
	"strings"
	"time"
)

// Kinds of automatic replies, stored in the message attributes (auto_reply).
const (
	KindAck     = "ack"
	KindOffline = "offline"
	KindHandoff = "handoff"
)

// Locales the texts are kept in.
var Locales = []string{"tr", "en"}

// MaxTextLength bounds one text.
const MaxTextLength = 1000

// Localized is a text per locale.
type Localized map[string]string

// Get returns the text for locale, falling back to the other locales.
func (l Localized) Get(locale string) string {
	if v := strings.TrimSpace(l[locale]); v != "" {
		return v
	}
	for _, loc := range Locales {
		if v := strings.TrimSpace(l[loc]); v != "" {
			return v
		}
	}
	return ""
}

// Reply is one automatic reply: on/off and its text per locale. The
// acknowledgement text may hold {eta}, replaced with the estimated reply
// time (or removed when none is known).
type Reply struct {
	Enabled bool      `json:"enabled"`
	Text    Localized `json:"text"`
}

// Settings are the organization's automatic replies.
type Settings struct {
	Ack     Reply `json:"ack"`
	Offline Reply `json:"offline"`
	Handoff Reply `json:"handoff"`
}

// Defaults are the texts of an organization that never saved its own.
func Defaults() Settings {
	return Settings{
		Ack: Reply{Enabled: true, Text: Localized{
			"tr": "Mesajınızı aldık, en kısa sürede dönüş yapacağız. {eta}",
			"en": "We have received your message and will get back to you shortly. {eta}",
		}},
		Offline: Reply{Enabled: true, Text: Localized{
			"tr": "Şu anda çalışma saatlerimizin dışındayız. Mesajınızı aldık; çalışma saatlerimiz başladığında size döneceğiz.",
			"en": "We are currently outside our working hours. We have your message and will reply when we are back.",
		}},
		Handoff: Reply{Enabled: true, Text: Localized{
			"tr": "Sorunuzu ekibimize ilettim; bir temsilci en kısa sürede size dönecek. Bu sırada sipariş ya da talep numarası ve tarih gibi ayrıntıları paylaşırsanız daha hızlı yardımcı olabiliriz.",
			"en": "I have passed your question to our team; a representative will get back to you shortly. Meanwhile, sharing details such as your order or request number and the date will help us assist you faster.",
		}},
	}
}

// Normalize fills missing texts from the defaults and trims the rest; it
// reports a text that is too long.
func Normalize(in Settings) (Settings, error) {
	def := Defaults()
	fix := func(name string, r *Reply, d Reply) error {
		if r.Text == nil {
			r.Text = Localized{}
		}
		for _, loc := range Locales {
			v := strings.TrimSpace(r.Text[loc])
			if len([]rune(v)) > MaxTextLength {
				return fmt.Errorf("%w: %s.text.%s is longer than %d characters", ErrInvalid, name, loc, MaxTextLength)
			}
			if v == "" {
				v = d.Text[loc]
			}
			r.Text[loc] = v
		}
		for k := range r.Text {
			if !contains(Locales, k) {
				delete(r.Text, k)
			}
		}
		return nil
	}
	if err := fix("ack", &in.Ack, def.Ack); err != nil {
		return in, err
	}
	if err := fix("offline", &in.Offline, def.Offline); err != nil {
		return in, err
	}
	if err := fix("handoff", &in.Handoff, def.Handoff); err != nil {
		return in, err
	}
	return in, nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// Render returns the text of one kind in locale; eta (nil: unknown) fills
// {eta} of the acknowledgement. "" means the kind is off.
func (s Settings) Render(kind, locale string, eta *time.Duration) string {
	var r Reply
	switch kind {
	case KindAck:
		r = s.Ack
	case KindOffline:
		r = s.Offline
	case KindHandoff:
		r = s.Handoff
	default:
		return ""
	}
	if !r.Enabled {
		return ""
	}
	text := r.Text.Get(locale)
	if kind == KindAck {
		text = strings.ReplaceAll(text, "{eta}", ETAText(locale, eta))
	}
	return strings.Join(strings.Fields(text), " ")
}

// ETAText words an estimated reply time ("Tahmini yanıt süresi: 2 saat.");
// "" when unknown.
func ETAText(locale string, eta *time.Duration) string {
	if eta == nil || *eta <= 0 {
		return ""
	}
	d := *eta
	en := strings.HasPrefix(strings.ToLower(locale), "en")
	var amount string
	switch {
	case d < time.Hour:
		m := int(d.Round(time.Minute) / time.Minute)
		if m < 1 {
			m = 1
		}
		if en {
			amount = fmt.Sprintf("%d minutes", m)
		} else {
			amount = fmt.Sprintf("%d dakika", m)
		}
	case d < 24*time.Hour:
		h := int(d.Round(30*time.Minute) / time.Hour)
		half := d.Round(30*time.Minute)%time.Hour != 0
		if en {
			amount = fmt.Sprintf("%d hours", h)
			if half {
				amount = fmt.Sprintf("%d.5 hours", h)
			}
		} else {
			amount = fmt.Sprintf("%d saat", h)
			if half {
				amount = fmt.Sprintf("%d buçuk saat", h)
			}
		}
	default:
		days := int((d + 12*time.Hour) / (24 * time.Hour))
		if en {
			amount = fmt.Sprintf("%d working days", days)
		} else {
			amount = fmt.Sprintf("%d iş günü", days)
		}
	}
	if en {
		return "Estimated reply time: " + amount + "."
	}
	return "Tahmini yanıt süresi: " + amount + "."
}
