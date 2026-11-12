package capability

import (
	"sort"
	"strings"
	"unicode"
)

// Intent-based tool loading (CF-211): instead of sending every capability
// on every turn, the assistant sends the page's own capabilities, those of
// the modules the user's words point at, and two small meta tools
// (find_tools / use_tool) that reach the rest. Keywords are word stems in
// Turkish and English, matched on folded text (lowercase, no diacritics),
// so "kanalları" and "Kanal" both hit "kanal".

// ModuleKeywords maps a capability module to the stems that select it.
var ModuleKeywords = map[string][]string{
	"navigation":    {"git", "ac", "goster", "sayfa", "nerede", "open", "go to", "show me", "where"},
	"channels":      {"kanal", "rest api", "e-posta kanal", "email channel", "whatsapp", "instagram", "telegram", "channel", "inbox bagla", "baglanti test"},
	"automation":    {"is akis", "akis", "workflow", "tetikleyici", "trigger", "otomasyon", "kural", "automation", "rule", "her yeni", "whenever"},
	"setup":         {"kurulum", "baslangic", "nereden basla", "eksik", "setup", "onboarding", "get started", "missing"},
	"sla":           {"sla", "calisma saat", "mesai", "business hour", "yanit sure", "cozum sure", "response time", "hedef sure"},
	"teams":         {"takim", "ekip", "team"},
	"members":       {"uye", "kullanici", "temsilci", "davet", "rol", "member", "user", "agent", "invite", "role", "seat"},
	"ai":            {"yapay zeka", "ai ", "ai'", "asistan ajan", "bot", "agent ai", "otomatik yanit", "artificial", "llm"},
	"widget":        {"widget", "sohbet balon", "canli destek", "chat bubble", "kurulum kod", "install code", "embed"},
	"webhooks":      {"webhook", "olay bildirim", "event delivery", "teslimat"},
	"api_keys":      {"api anahtar", "api key", "token", "anahtar"},
	"integrations":  {"uygulama", "marketplace", "pazar yeri", "entegrasyon", "integration", "sistem bagla", "crm", "erp", "concept", "kavram", "playbook"},
	"conversations": {"konusma", "mesaj", "talep", "bilet", "ticket", "conversation", "message", "yanitla", "cevap", "reply", "ata", "assign", "bekleyen", "acik", "kapat", "cozuldu", "resolve", "pending", "gelen kutu", "inbox", "not ekle", "note", "etiket", "tag", "oncelik", "priority", "hatirlat", "remind", "makro uygula"},
	"contacts":      {"kisi", "musteri", "contact", "customer", "telefon", "e-posta adres", "email address"},
	"companies":     {"firma", "sirket", "company", "account", "hesap sagl", "health"},
	"macros":        {"makro", "hazir yanit", "kalip yanit", "canned", "macro", "saved repl", "template repl"},
	"knowledge":     {"bilgi bankas", "makale", "yardim merkez", "dokuman", "knowledge", "article", "help center", "faq", "sss", "koleksiyon", "collection"},
	"tags":          {"etiket", "tag", "label"},
	"reports":       {"rapor", "analiz", "istatistik", "metrik", "performans", "csat", "memnuniyet", "anket", "survey", "nps", "report", "analytics", "stat", "metric", "dashboard", "kac tane", "how many", "bu hafta", "this week", "trend"},
	"appointments":  {"randevu", "toplanti", "gorusme saat", "takvim", "appointment", "meeting", "booking", "slot", "calendar"},
	"organization":  {"organizasyon", "isletme", "sirket ayar", "saat dilimi", "dil ayar", "logo", "organization", "workspace", "timezone", "settings", "ayar"},
	"billing":       {"fatura", "abonelik", "plan", "kullanim", "kredi", "cuzdan", "odeme", "billing", "invoice", "subscription", "usage", "credit", "wallet", "payment", "price", "fiyat"},
	"audit":         {"denetim", "kim yapti", "kim degistir", "log", "gecmis islem", "audit", "who changed", "activity"},
	"custom_fields": {"ozel alan", "custom field", "alan ekle", "form alan"},
	"status":        {"durum sayfa", "olay", "kesinti", "incident", "status page", "outage"},
}

// Fold lowercases with Turkish rules and strips diacritics.
func Fold(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case 'İ', 'I', 'ı':
			r = 'i'
		case 'Ş', 'ş':
			r = 's'
		case 'Ç', 'ç':
			r = 'c'
		case 'Ğ', 'ğ':
			r = 'g'
		case 'Ü', 'ü', 'Û', 'û':
			r = 'u'
		case 'Ö', 'ö':
			r = 'o'
		case 'Â', 'â':
			r = 'a'
		default:
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

// matchStem reports whether a stem occurs at a word start in folded text.
func matchStem(text, stem string) bool {
	for i := 0; ; {
		j := strings.Index(text[i:], stem)
		if j < 0 {
			return false
		}
		at := i + j
		if at == 0 || !unicode.IsLetter(rune(text[at-1])) {
			return true
		}
		i = at + 1
		if i >= len(text) {
			return false
		}
	}
}

// IntentModules are the modules the text points at, best match first.
func IntentModules(text string) []string {
	t := " " + Fold(text) + " "
	type hit struct {
		module string
		n      int
	}
	hits := []hit{}
	for m, stems := range ModuleKeywords {
		n := 0
		for _, s := range stems {
			if matchStem(t, s) {
				n++
			}
		}
		if n > 0 {
			hits = append(hits, hit{m, n})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].n != hits[j].n {
			return hits[i].n > hits[j].n
		}
		return hits[i].module < hits[j].module
	})
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.module)
	}
	return out
}

// Selection is the capabilities loaded as tools in one turn and the rest
// the caller may still reach through find_tools / use_tool.
type Selection struct {
	Loaded []*Capability
	Other  []*Capability
}

// Select picks the tools of one turn: the page's capabilities, those of
// the modules in intent (in order), and Always, up to max; every other
// capability the caller may use is in Other. With max <= 0 or when all
// fit, everything is loaded (small surfaces such as the widget).
func (r *Registry) Select(f Filter, intent []string, always ...string) Selection {
	all := r.Available(Filter{Surface: f.Surface, Route: f.Route, Has: f.Has})
	if f.Max <= 0 || len(all) <= f.Max {
		return Selection{Loaded: all}
	}
	rank := map[*Capability]int{}
	for _, c := range all {
		switch {
		case containsStr(always, c.Name):
			rank[c] = 0
		case c.Relevant(f.Route):
			rank[c] = 1
		default:
			rank[c] = -1
			for i, m := range intent {
				if moduleOf(c) == m {
					rank[c] = 2 + i
					break
				}
			}
		}
	}
	picked := []*Capability{}
	for _, c := range all {
		if rank[c] >= 0 {
			picked = append(picked, c)
		}
	}
	sort.SliceStable(picked, func(i, j int) bool { return rank[picked[i]] < rank[picked[j]] })
	if len(picked) > f.Max {
		picked = picked[:f.Max]
	}
	in := map[*Capability]bool{}
	for _, c := range picked {
		in[c] = true
	}
	// Stable order (by name) keeps the tool list identical between turns
	// on the same page, so the provider's prompt cache can hit.
	sort.SliceStable(picked, func(i, j int) bool { return picked[i].Name < picked[j].Name })
	sel := Selection{Loaded: picked}
	for _, c := range all {
		if !in[c] {
			sel.Other = append(sel.Other, c)
		}
	}
	return sel
}

// moduleOf is the intent module of a capability.
func moduleOf(c *Capability) string { return c.Module }

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Search ranks the capabilities matching a free-text query (find_tools):
// the query's intent modules first, then words found in the name or
// description.
func Search(caps []*Capability, query string, limit int) []*Capability {
	q := Fold(query)
	mods := IntentModules(query)
	words := strings.FieldsFunc(q, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' })
	type scored struct {
		c *Capability
		s int
	}
	list := []scored{}
	for _, c := range caps {
		s := 0
		for i, m := range mods {
			if moduleOf(c) == m {
				s += 10 - min(i, 5)
			}
		}
		hay := Fold(c.Name + " " + c.Description)
		for _, w := range words {
			if len(w) >= 3 && strings.Contains(hay, w) {
				s += 2
			}
		}
		if s > 0 {
			list = append(list, scored{c, s})
		}
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].s != list[j].s {
			return list[i].s > list[j].s
		}
		return list[i].c.Name < list[j].c.Name
	})
	out := []*Capability{}
	for _, x := range list {
		if limit > 0 && len(out) >= limit {
			break
		}
		out = append(out, x.c)
	}
	return out
}

// Signature is a short param list for find_tools ("uuid*, limit:int").
func (c *Capability) Signature() string {
	props, _ := c.ToolSchema()["properties"].(map[string]any)
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	req := c.required()
	parts := []string{}
	for _, k := range keys {
		p, _ := props[k].(map[string]any)
		s := k
		if containsStr(req, k) {
			s += "*"
		}
		switch t, _ := p["type"].(string); {
		case p["enum"] != nil:
			s += ":" + enumText(p["enum"])
		case t != "" && t != "string":
			s += ":" + t
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ", ")
}

func enumText(v any) string {
	switch x := v.(type) {
	case []string:
		return strings.Join(x, "|")
	case []any:
		parts := []string{}
		for _, e := range x {
			if s, ok := e.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "|")
	}
	return "enum"
}
