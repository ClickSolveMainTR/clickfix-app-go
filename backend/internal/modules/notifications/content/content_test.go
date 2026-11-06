package content

import (
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 2, 7, 0, 0, 0, time.UTC) // 10:00 in Istanbul

// facts carries every key a builder reads, with personal data that must not
// reach a lock screen.
func facts() Vars {
	return Vars{
		"actor": "Ali Veli", "customer": "Deniz Kaya", "channel": "whatsapp", "number": "42",
		"subject": "Kargo gecikti", "preview": "<p>Siparişim <b>hâlâ</b> gelmedi &amp; iade istiyorum</p>",
		"url": "/inbox?c=6f1d2c1e-9b0a-4f53-9a4e-2f8c7d0e5b11", "timezone": "Europe/Istanbul",
		"due": now.Add(25 * time.Minute).Format(time.RFC3339), "metric": "first_response",
		"rating": "2", "comment": "Çok yavaş", "company": "Acme A.Ş.", "score": "38", "previous_score": "71", "threshold": "50",
		"ticket": "#7", "balance": "12", "title": "Haftalık SLA raporu", "resource": "Kişiler", "format": "CSV",
		"succeeded": "120", "failed": "3", "created": "10", "updated": "4", "error": "zaman aşımı",
		"start": now.Add(24 * time.Hour).Format(time.RFC3339), "end": now.Add(24*time.Hour + 45*time.Minute).Format(time.RFC3339),
		"previous": now.Add(48 * time.Hour).Format(time.RFC3339), "type": "Ürün demosu", "location": "Görüntülü görüşme",
		"video_url": "/video/6f1d2c1e-9b0a-4f53-9a4e-2f8c7d0e5b11", "channel_name": "Merkez hat",
	}
}

// TestEveryEventInBothLanguages: each event answers who / what / what next
// in tr and en, the push stays free of personal data and no placeholder or
// HTML leaks into any channel.
func TestEveryEventInBothLanguages(t *testing.T) {
	for _, event := range Events() {
		var titles []string
		for _, lang := range []string{"tr", "en"} {
			m, ok := Build(event, lang, facts(), now)
			if !ok {
				t.Fatalf("%s: no builder", event)
			}
			if m.Event != event || m.Lang != lang || m.Category == "" {
				t.Fatalf("%s/%s: meta %+v", event, lang, m)
			}
			if strings.TrimSpace(m.Title) == "" || strings.TrimSpace(m.Lead) == "" {
				t.Fatalf("%s/%s: empty title or lead", event, lang)
			}
			if m.Push.Title == "" || m.Push.Body == "" {
				t.Fatalf("%s/%s: empty push %+v", event, lang, m.Push)
			}
			for _, secret := range []string{"Deniz", "Siparişim", "Çok yavaş", "Ali Veli", "Kargo"} {
				if strings.Contains(m.Push.Title+m.Push.Body, secret) {
					t.Fatalf("%s/%s: push leaks %q: %+v", event, lang, secret, m.Push)
				}
			}
			mail := m.EmailText("https://app.example.test")
			for _, s := range []string{m.Summary(), mail, m.Title} {
				if strings.Contains(s, "{{") || strings.Contains(s, "<b>") || strings.Contains(s, "&amp;") {
					t.Fatalf("%s/%s: raw markup in %q", event, lang, s)
				}
			}
			if a, ok := m.Primary(); ok {
				if strings.HasPrefix(a.URL, "/") && !strings.Contains(mail, a.Label+": https://app.example.test"+a.URL) {
					t.Fatalf("%s/%s: e-mail lacks the absolute primary link:\n%s", event, lang, mail)
				}
				if strings.Contains(a.URL, "/v1/") {
					t.Fatalf("%s/%s: e-mail action must open a page, not call the API: %s", event, lang, a.URL)
				}
			}
			titles = append(titles, m.Title)
		}
		if titles[0] == titles[1] && !strings.Contains(event, "oncall") {
			t.Fatalf("%s: tr and en titles are the same: %q", event, titles[0])
		}
	}
}

func TestConversationAssignedAnswersEverything(t *testing.T) {
	m, _ := Build("conversation.assigned", "tr", facts(), now)
	if m.Title != "Konuşma size atandı · #42 · Deniz Kaya" {
		t.Fatalf("title %q", m.Title)
	}
	mail := m.EmailText("https://getclickfix.com/")
	for _, want := range []string{
		"Ali Veli, Deniz Kaya ile olan #42 numaralı konuşmayı size atadı.",
		"Son mesaj:\n“Siparişim hâlâ gelmedi & iade istiyorum”",
		"Müşteri: Deniz Kaya\nKanal: WhatsApp\nKonuşma: #42\nKonu: Kargo gecikti\nSLA kalan süre: 25 dk",
		"Konuşmayı aç: https://getclickfix.com/inbox?c=6f1d2c1e-9b0a-4f53-9a4e-2f8c7d0e5b11",
	} {
		if !strings.Contains(mail, want) {
			t.Errorf("missing %q in:\n%s", want, mail)
		}
	}
	if m.Push.Title != "Konuşma size atandı" || m.Push.Body != "WhatsApp · #42" || m.Category != CategoryConversation {
		t.Fatalf("push %+v %s", m.Push, m.Category)
	}
	v := facts()
	v["assignable"] = "1"
	if m, _ := Build("message.received", "en", v, now); m.Category != CategoryConversationAssign {
		t.Fatalf("unassigned conversation category %q", m.Category)
	}
}

func TestSLAShowsTimeLeftAndOverdue(t *testing.T) {
	m, _ := Build("sla.warning", "en", facts(), now)
	if !strings.Contains(m.Title, "25 min left") || !strings.Contains(m.Push.Body, "25 min left") {
		t.Fatalf("warning %q / %q", m.Title, m.Push.Body)
	}
	if !strings.Contains(m.EmailText(""), "Due: Monday, November 2, 2026, 10:25 AM (Europe/Istanbul)") {
		t.Fatalf("due not in the organization's zone:\n%s", m.EmailText(""))
	}
	v := facts()
	v["due"] = now.Add(-90 * time.Minute).Format(time.RFC3339)
	b, _ := Build("sla.breached", "tr", v, now)
	if !strings.Contains(b.EmailText(""), "SLA: Süre doldu (1 sa 30 dk)") || !strings.Contains(b.Lead, "ilk yanıt hedefi aşıldı") {
		t.Fatalf("breach:\n%s", b.EmailText(""))
	}
}

func TestAppointmentInOrganizationTimeZone(t *testing.T) {
	m, _ := Build("appointment.rescheduled", "tr", facts(), now)
	mail := m.EmailText("https://getclickfix.com")
	for _, want := range []string{
		"Tarih: 3 Kasım 2026 Salı, 10:00 (Europe/Istanbul)", "Süre: 45 dk",
		"Önceki zaman: 4 Kasım 2026 Çarşamba, 10:00 (Europe/Istanbul)", "Yer: Görüntülü görüşme",
		"Randevuyu aç: https://getclickfix.com/inbox?c=", "Görüşmeye katıl: https://getclickfix.com/video/",
	} {
		if !strings.Contains(mail, want) {
			t.Errorf("missing %q in:\n%s", want, mail)
		}
	}
	if m.Push.Body != "10:00 · Ürün demosu" {
		t.Fatalf("push %q", m.Push.Body)
	}
	c, _ := Build("appointment.cancelled", "en", facts(), now)
	for _, a := range c.Actions {
		if a.ID == ActionJoin {
			t.Fatal("a canceled appointment offers no join link")
		}
	}
}

func TestPreview(t *testing.T) {
	cases := map[string]string{
		"<p>Merhaba&nbsp;<b>dünya</b></p>\n\n<script>alert(1)</script>": "Merhaba dünya",
		"  çok   boşluklu\tmetin ":                                      "çok boşluklu metin",
		"**Kargonuz** `yolda`, snake_case":                              "Kargonuz yolda, snake_case",
		"":                                                              "",
	}
	for in, want := range cases {
		if got := Preview(in, 100); got != want {
			t.Errorf("Preview(%q) = %q, want %q", in, got, want)
		}
	}
	long := strings.Repeat("kelime ", 50)
	if got := Preview(long, 30); !strings.HasSuffix(got, "…") || len([]rune(got)) > 31 || strings.HasSuffix(got, " …") {
		t.Fatalf("cut %q", got)
	}
}

func TestDuration(t *testing.T) {
	cases := []struct {
		d  time.Duration
		tr string
		en string
	}{
		{25 * time.Minute, "25 dk", "25 min"},
		{2*time.Hour + 5*time.Minute, "2 sa 5 dk", "2 h 5 min"},
		{3 * time.Hour, "3 sa", "3 h"},
		{26 * time.Hour, "1 gün 2 sa", "1 day 2 h"},
		{72 * time.Hour, "3 gün", "3 days"},
		{-time.Minute, "0 dk", "0 min"},
	}
	for _, c := range cases {
		if got := Duration(c.d, "tr"); got != c.tr {
			t.Errorf("tr %v = %q", c.d, got)
		}
		if got := Duration(c.d, "en"); got != c.en {
			t.Errorf("en %v = %q", c.d, got)
		}
	}
}

func TestAbsoluteOnlyKeepsSafeLinks(t *testing.T) {
	cases := map[string]string{
		"/inbox?c=1":          "https://getclickfix.com/inbox?c=1",
		"https://x.test/a":    "https://x.test/a",
		"//evil.test/a":       "",
		"javascript:alert(1)": "",
		"":                    "",
	}
	for in, want := range cases {
		if got := Absolute("https://getclickfix.com/", in); got != want {
			t.Errorf("Absolute(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUnknownEventHasNoBuilder(t *testing.T) {
	if _, ok := Build("status.changed", "tr", nil, now); ok || Has("status.changed") {
		t.Fatal("events without a builder keep their own text")
	}
}

func TestEmailBodyLeavesPrimaryToTheLayout(t *testing.T) {
	m, _ := Build("appointment.booked", "en", facts(), now)
	body := m.EmailBody("https://getclickfix.com")
	if strings.Contains(body, "Open appointment:") || !strings.Contains(body, "Join the call: https://getclickfix.com/video/") {
		t.Fatalf("body:\n%s", body)
	}
}
