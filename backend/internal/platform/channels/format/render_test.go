package format

import (
	"strings"
	"testing"
)

// sample uses every construct of the source format plus the text that must
// survive untouched.
const sample = "**Merhaba** *Ayşe*, `kod` ve [yardım](https://ex.com/a_b?x=1&y=2).\n" +
	"Hesap: 2*3*4 = 24, dosya_adi.pdf, snake_case_name, 5 * 3 ve tek * işaret <b>&\n" +
	"\n" +
	"Adımlar:\n" +
	"- **Ayarlar**dan gir\n" +
	"- https://ex.com/a_b_c bak\n" +
	"1. bir\n" +
	"2. iki\n" +
	"```\nx := a*b\n```\n" +
	"_son_ [posta](mailto:a@b.co)"

const untouched = "Hesap: 2*3*4 = 24, dosya_adi.pdf, snake_case_name, 5 * 3 ve tek * işaret"

func TestChannels(t *testing.T) {
	cases := []struct {
		name string
		fn   func(string) string
		want string
	}{
		{"plain (sms, instagram, messenger)", Plain, "Merhaba Ayşe, kod ve yardım (https://ex.com/a_b?x=1&y=2).\n" +
			untouched + " <b>&\n\nAdımlar:\n- Ayarlardan gir\n- https://ex.com/a_b_c bak\n1. bir\n2. iki\nx := a*b\nson posta (a@b.co)"},
		{"speech (voice)", Speech, "Merhaba Ayşe, kod ve yardım.\n" +
			untouched + " <b>&\n\nAdımlar:\nAyarlardan gir\nhttps://ex.com/a_b_c bak\n1. bir\n2. iki\nson posta"},
		{"whatsapp", WhatsApp, "*Merhaba* _Ayşe_, `kod` ve yardım (https://ex.com/a_b?x=1&y=2).\n" +
			untouched + " <b>&\n\nAdımlar:\n- Ayarlardan gir\n- https://ex.com/a_b_c bak\n1. bir\n2. iki\n```\nx := a*b\n```\n_son_ posta (a@b.co)"},
		{"slack", Slack, "*Merhaba* _Ayşe_, `kod` ve <https://ex.com/a_b?x=1&amp;y=2|yardım>.\n" +
			untouched + " &lt;b&gt;&amp;\n\nAdımlar:\n• Ayarlardan gir\n• <https://ex.com/a_b_c> bak\n1. bir\n2. iki\n```\nx := a*b\n```\n_son_ <mailto:a@b.co|posta>"},
		{"google chat", GoogleChat, "**Merhaba** *Ayşe*, `kod` ve [yardım](https://ex.com/a_b?x=1&y=2).\n" +
			untouched + " <b>&\n\nAdımlar:\n- **Ayarlar**dan gir\n- https://ex.com/a_b_c bak\n1. bir\n2. iki\n```\nx := a*b\n```\n*son* [posta](mailto:a@b.co)"},
		{"teams", Teams, "**Merhaba** *Ayşe*, `kod` ve [yardım](https://ex.com/a_b?x=1&y=2).  \n" +
			"Hesap: 2\\*3\\*4 = 24, dosya_adi.pdf, snake_case_name, 5 * 3 ve tek * işaret &lt;b>&amp;\n\nAdımlar:\n\n- **Ayarlar**dan gir\n- https://ex.com/a_b_c bak\n\n1. bir\n2. iki\n\n```\nx := a*b\n```\n\n*son* [posta](mailto:a@b.co)"},
		{"email html", HTML, `<p><strong>Merhaba</strong> <em>Ayşe</em>, <code>kod</code> ve <a href="https://ex.com/a_b?x=1&amp;y=2">yardım</a>.<br>` +
			untouched + " &lt;b&gt;&amp;</p>\n<p>Adımlar:</p>\n" +
			`<ul><li><strong>Ayarlar</strong>dan gir</li><li><a href="https://ex.com/a_b_c">https://ex.com/a_b_c</a> bak</li></ul>` + "\n" +
			"<ol><li>bir</li><li>iki</li></ol>\n" +
			`<pre style="white-space:pre-wrap"><code>x := a*b</code></pre>` + "\n" +
			`<p><em>son</em> <a href="mailto:a@b.co">posta</a></p>`},
		{"preview", Strip, "Merhaba Ayşe, kod ve yardım.\n" +
			untouched + " <b>&\n\nAdımlar:\n- Ayarlardan gir\n- https://ex.com/a_b_c bak\n1. bir\n2. iki\nx := a*b\nson posta"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.fn(sample); got != c.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, c.want)
			}
		})
	}
}

// Text without marks must reach every channel as typed (Slack and Teams
// only escape their control characters).
func TestPlainTextIsNotChanged(t *testing.T) {
	texts := []string{
		"2*3*4 = 24",
		"dosya_adi.pdf ve snake_case_name",
		"https://ex.com/yol_alt_cizgi/dosya_adi.pdf?a_b=c_d",
		"tek * yıldız, tek _ alt çizgi ve tek ` işaret",
		"__init__ ve **kapanmayan",
		"Fiyat* (*KDV dahil)",
		"24. madde gereği\n3) üçüncü şık",
		"Merhaba,\n\nSiparişiniz yola çıktı.\nİyi günler",
	}
	for _, text := range texts {
		for name, fn := range map[string]func(string) string{"plain": Plain, "speech": Speech, "whatsapp": WhatsApp, "google chat": GoogleChat, "telegram": func(s string) string {
			r := Telegram(s)
			if len(r.Entities) > 0 {
				t.Errorf("telegram: %q got entities %+v", s, r.Entities)
			}
			return r.Text
		}} {
			want := text
			if name == "whatsapp" || name == "google chat" || name == "telegram" {
				want = strings.ReplaceAll(want, "3) ", "3. ") // ordered markers are normalized
			}
			if got := fn(text); got != want {
				t.Errorf("%s: %q became %q", name, text, got)
			}
		}
		if got := Slack(text); strings.NewReplacer("<", "", ">", "").Replace(got) != strings.ReplaceAll(text, "3) ", "3. ") {
			t.Errorf("slack: %q became %q", text, got)
		}
	}
	if got, want := Teams("2*3*4 ve snake_case ve 5 * 3"), `2\*3\*4 ve snake_case ve 5 * 3`; got != want {
		t.Errorf("teams: got %q want %q", got, want)
	}
}

func TestEdges(t *testing.T) {
	cases := []struct {
		name string
		fn   func(string) string
		in   string
		want string
	}{
		{"slack escapes code and labels", Slack, "`a<b` [x & y](https://ex.com/?q=a|b>c)", "`a&lt;b` <https://ex.com/?q=a%7Cb%3Ec|x &amp; y>"},
		{"slack keeps a bare ampersand link valid", Slack, "bak: https://ex.com/?a=1&b=2", "bak: <https://ex.com/?a=1&amp;b=2>"},
		{"whatsapp nests bold and italic", WhatsApp, "**çok *önemli* not**", "*çok _önemli_ not*"},
		{"whatsapp drops marks it would not format", WhatsApp, "*İptal*dir, **Tamam**.", "İptaldir, *Tamam*."},
		{"whatsapp link with the address as label", WhatsApp, "[https://ex.com](https://ex.com)", "https://ex.com"},
		{"no javascript links", HTML, "[tıkla](javascript:alert(1))", "<p>[tıkla](javascript:alert(1))</p>"},
		{"html escapes text and attributes", HTML, `<script>"x"</script> [a"b](https://ex.com/?q="x")`, `<p>&lt;script&gt;&#34;x&#34;&lt;/script&gt; <a href="https://ex.com/?q=&#34;x&#34;">a&#34;b</a></p>`},
		{"html numbered list keeps its start", HTML, "3. üç\n4. dört", `<ol start="3"><li>üç</li><li>dört</li></ol>`},
		{"teams escapes headings and quotes", Teams, "#1 müşteri\n> alıntı değil", "\\#1 müşteri  \n\\> alıntı değil"},
		{"speech keeps a code-only message", Speech, "```\nABC-123\n```", "ABC-123"},
		{"crlf", Plain, "a\r\nb\rc", "a\nb\nc"},
		// Review fixes.
		{"bold italic on whatsapp", WhatsApp, "***çok önemli*** not", "*_çok önemli_* not"},
		{"bold italic in html", HTML, "***ikisi***", "<p><strong><em>ikisi</em></strong></p>"},
		{"bold italic on teams", Teams, "***ikisi***", "***ikisi***"},
		{"marker runs stay text", WhatsApp, "****x**** ve kelime***y***", "****x**** ve kelime***y***"},
		{"address ends at a no-break space", Slack, "https://ex.com\u00a0devam", "<https://ex.com>\u00a0devam"},
		{"address with a no-break space is not a link", HTML, "[a](https://ex.com/x\u00a0y)", "<p>[a](<a href=\"https://ex.com/x\">https://ex.com/x</a>\u00a0y)</p>"},
		{"whatsapp does not strike literal tildes", WhatsApp, "~iptal~ ve ~5 dk, `~kod~` kalır", "∼iptal∼ ve ~5 dk, `~kod~` kalır"},
		{"slack does not strike literal tildes", Slack, "**a** ~iptal edildi~ <b>", "*a* ∼iptal edildi∼ &lt;b&gt;"},
		{"teams escapes math, strikethrough and plus bullets", Teams, "5$ ve 10$, ~~eski~~\n+ artı", "5\\$ ve 10\\$, \\~\\~eski\\~\\~  \n\\+ artı"},
		{"teams escapes a setext underline", Teams, "Saygılar\n--\nAli\n===", "Saygılar  \n\\--  \nAli  \n\\==="},
		{"teams escapes a rule", Teams, "üst\n\n---\n\nalt", "üst\n\n\\---\n\nalt"},
		{"only a fence: plain", Plain, "```", "```"},
		{"only a fence: whatsapp", WhatsApp, "```\n```", "```\n\n```"},
		{"only a fence: slack", Slack, "```", "```\n\n```"},
		{"only a fence: google chat", GoogleChat, "```", "```\n\n```"},
		{"only a fence: teams", Teams, "```", "```\n\n```"},
		{"only a fence: speech", Speech, "```", "```"},
		{"only a fence: html", HTML, "```", "<p>```</p>"},
		{"blank stays blank", WhatsApp, " \n ", ""},
		{"empty", WhatsApp, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.fn(c.in); got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}

// A retried send reads the stored source again, so the same source must
// always give the same text.
func TestDeterministic(t *testing.T) {
	first := WhatsApp(sample) + Slack(sample) + Teams(sample)
	for range 3 {
		if again := WhatsApp(sample) + Slack(sample) + Teams(sample); again != first {
			t.Fatal("renderer is not deterministic")
		}
	}
}
