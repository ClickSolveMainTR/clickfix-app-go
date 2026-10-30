package content

import (
	"strings"
	"time"
)

// Vars are the facts a producer knows about an event. Keys are shared by
// all builders; a builder leaves out a detail row whose fact is empty.
//
//	actor        who did it ("Ayşe Yılmaz")
//	customer     contact / invitee name
//	channel      channel type (whatsapp, email, widget…)
//	number       conversation number
//	subject      conversation subject
//	preview      message / note text (any format; shortened here)
//	url          app path of the subject ("/inbox?c=…")
//	timezone     organization time zone (IANA)
//	due          RFC 3339 deadline (SLA)
//	metric       SLA metric (first_response, next_response, resolution)
//	assignable   "1" when the conversation has no assignee
//	start, end   RFC 3339 appointment times; previous: the old start
//	type         appointment type name
//	location     appointment place / meeting kind
//	video_url    join link
//	rating, comment, company, score, previous_score, threshold, ticket,
//	balance, resource, count, title
type Vars map[string]string

type builder func(lang string, v Vars, now time.Time) Message

var builders = map[string]builder{
	"conversation.assigned":     conversationAssigned,
	"conversation.mentioned":    conversationMentioned,
	"message.received":          messageReceived,
	"conversation.reminder_due": reminderDue,
	"sla.warning":               slaWarning,
	"sla.breached":              slaBreached,
	"csat.submitted":            csatSubmitted,
	"company.health_dropped":    companyHealthDropped,
	"ticket.assigned":           ticketAssigned,
	"calls.missed":              callMissed,
	"oncall.page":               oncallPage,
	"teamchat.mentioned":        teamchatMentioned,
	"video.balance_low":         videoBalanceLow,
	"reports.ready":             reportReady,
	"exports.ready":             exportReady,
	"imports.applied":           importApplied,
	"bulk.completed":            bulkCompleted,
	"bulk.failed":               bulkFailed,
	"appointment.booked":        appointmentHost("booked"),
	"appointment.rescheduled":   appointmentHost("rescheduled"),
	"appointment.confirmed":     appointmentHost("confirmed"),
	"appointment.cancelled":     appointmentHost("cancelled"),
	"appointment.reminder":      appointmentHost("reminder"),
	"appointment.no_show":       appointmentHost("no_show"),
}

// Has reports whether event has a builder.
func Has(event string) bool {
	_, ok := builders[event]
	return ok
}

// Events lists the events with a builder.
func Events() []string {
	out := make([]string, 0, len(builders))
	for k := range builders {
		out = append(out, k)
	}
	return out
}

// Build renders event in lang (tr | en) from v. ok is false for an event
// without a builder (the caller keeps its own text).
func Build(event, lang string, v Vars, now time.Time) (Message, bool) {
	b, ok := builders[event]
	if !ok {
		return Message{}, false
	}
	lang = Lang(lang)
	if v == nil {
		v = Vars{}
	}
	m := b(lang, v, now)
	m.Event, m.Lang = event, lang
	if m.Category == "" {
		m.Category = CategoryGeneric
	}
	return m, true
}

// --- conversations ---------------------------------------------------------

// ChannelLabel names a channel type for people ("whatsapp" → "WhatsApp").
func ChannelLabel(kind, lang string) string {
	switch strings.ToLower(kind) {
	case "whatsapp":
		return "WhatsApp"
	case "instagram":
		return "Instagram"
	case "messenger":
		return "Messenger"
	case "telegram":
		return "Telegram"
	case "slack":
		return "Slack"
	case "teams":
		return "Microsoft Teams"
	case "google_chat":
		return "Google Chat"
	case "sms":
		return "SMS"
	case "email":
		return t(lang, "E-posta", "Email")
	case "widget":
		return t(lang, "Web sohbet", "Web chat")
	case "voice":
		return t(lang, "Telefon", "Phone")
	case "api":
		return "API"
	case "":
		return ""
	}
	return kind
}

func convRef(v Vars) string {
	if v["number"] == "" {
		return ""
	}
	return "#" + v["number"]
}

// conversationDetails are the rows every conversation notification shows.
func conversationDetails(lang string, v Vars, now time.Time) []Detail {
	var d []Detail
	add := func(label, value string) {
		if strings.TrimSpace(value) != "" {
			d = append(d, Detail{Label: label, Value: value})
		}
	}
	add(t(lang, "Müşteri", "Customer"), v["customer"])
	add(t(lang, "Kanal", "Channel"), ChannelLabel(v["channel"], lang))
	add(t(lang, "Konuşma", "Conversation"), convRef(v))
	add(t(lang, "Konu", "Subject"), Preview(v["subject"], 120))
	if due, ok := parseTime(v["due"]); ok {
		left := due.Sub(now)
		if left > 0 {
			add(t(lang, "SLA kalan süre", "SLA time left"), Duration(left, lang))
		} else {
			add(t(lang, "SLA", "SLA"), t(lang, "Süre doldu (", "Overdue (")+Duration(-left, lang)+")")
		}
	}
	return d
}

func openConversation(lang string, v Vars) []Action {
	if v["url"] == "" {
		return nil
	}
	return []Action{{ID: ActionOpen, Label: t(lang, "Konuşmayı aç", "Open conversation"), URL: v["url"]}}
}

func conversationCategory(v Vars) string {
	if v["assignable"] == "1" {
		return CategoryConversationAssign
	}
	return CategoryConversation
}

// pushRef is the lock-screen reference of a conversation: "WhatsApp · #42".
func pushRef(lang string, v Vars) string {
	parts := nonEmpty([]string{ChannelLabel(v["channel"], lang), convRef(v)})
	return strings.Join(parts, " · ")
}

func withSuffix(s, suffix string) string {
	if suffix == "" {
		return s
	}
	return s + " · " + suffix
}

func conversationAssigned(lang string, v Vars, now time.Time) Message {
	who := v["customer"]
	var lead string
	switch {
	case v["actor"] != "" && who != "":
		lead = t(lang, v["actor"]+", "+who+" ile olan "+convRef(v)+" numaralı konuşmayı size atadı.",
			v["actor"]+" assigned you the conversation "+convRef(v)+" with "+who+".")
	case v["actor"] != "":
		lead = t(lang, v["actor"]+", "+convRef(v)+" numaralı konuşmayı size atadı.",
			v["actor"]+" assigned you conversation "+convRef(v)+".")
	default:
		lead = t(lang, convRef(v)+" numaralı konuşma size atandı.", "Conversation "+convRef(v)+" was assigned to you.")
	}
	return Message{
		Title:      withSuffix(t(lang, "Konuşma size atandı", "Conversation assigned to you"), strings.Join(nonEmpty([]string{convRef(v), who}), " · ")),
		Lead:       lead,
		Quote:      Preview(v["preview"], 200),
		QuoteLabel: t(lang, "Son mesaj", "Last message"),
		Paragraphs: []string{t(lang, "Müşteriye yanıt vermek için konuşmayı açın.", "Open the conversation to reply to the customer.")},
		Details:    conversationDetails(lang, v, now),
		Actions:    openConversation(lang, v),
		Push:       Push{Title: t(lang, "Konuşma size atandı", "Conversation assigned"), Body: pushRef(lang, v)},
		Category:   conversationCategory(v),
	}
}

func conversationMentioned(lang string, v Vars, now time.Time) Message {
	actor := v["actor"]
	if actor == "" {
		actor = t(lang, "Bir ekip arkadaşınız", "A teammate")
	}
	lead := t(lang, actor+", "+convRef(v)+" numaralı konuşmadaki bir notta sizden bahsetti.",
		actor+" mentioned you in a note on conversation "+convRef(v)+".")
	if v["customer"] != "" {
		lead = t(lang, actor+", "+v["customer"]+" ile olan "+convRef(v)+" numaralı konuşmadaki bir notta sizden bahsetti.",
			actor+" mentioned you in a note on conversation "+convRef(v)+" with "+v["customer"]+".")
	}
	return Message{
		Title:      withSuffix(t(lang, actor+" sizden bahsetti", actor+" mentioned you"), convRef(v)),
		Lead:       lead,
		Quote:      Preview(v["preview"], 280),
		QuoteLabel: t(lang, "Not", "Note"),
		Details:    conversationDetails(lang, v, now),
		Actions:    openConversation(lang, v),
		Push:       Push{Title: t(lang, "Sizden bahsedildi", "You were mentioned"), Body: pushRef(lang, v)},
		Category:   conversationCategory(v),
	}
}

func messageReceived(lang string, v Vars, now time.Time) Message {
	who := v["customer"]
	if who == "" {
		who = t(lang, "Müşteri", "The customer")
	}
	return Message{
		Title: withSuffix(t(lang, who+" yeni mesaj gönderdi", "New message from "+who), convRef(v)),
		Lead: t(lang, who+", "+convRef(v)+" numaralı konuşmaya yeni bir mesaj yazdı.",
			who+" wrote a new message in conversation "+convRef(v)+"."),
		Quote:      Preview(v["preview"], 200),
		QuoteLabel: t(lang, "Mesaj", "Message"),
		Details:    conversationDetails(lang, v, now),
		Actions:    replyAction(lang, v),
		Push:       Push{Title: t(lang, "Yeni mesaj", "New message"), Body: pushRef(lang, v)},
		Category:   conversationCategory(v),
	}
}

func reminderDue(lang string, v Vars, now time.Time) Message {
	return Message{
		Title: withSuffix(t(lang, "Hatırlatıcı", "Reminder"), convRef(v)),
		Lead: t(lang, convRef(v)+" numaralı konuşma için kurduğunuz hatırlatıcının zamanı geldi.",
			"The reminder you set on conversation "+convRef(v)+" is due."),
		Quote:      Preview(v["preview"], 280),
		QuoteLabel: t(lang, "Hatırlatıcı notu", "Reminder note"),
		Details:    conversationDetails(lang, v, now),
		Actions:    openConversation(lang, v),
		Push:       Push{Title: t(lang, "Hatırlatıcı", "Reminder"), Body: pushRef(lang, v)},
		Category:   conversationCategory(v),
	}
}

func replyAction(lang string, v Vars) []Action {
	if v["url"] == "" {
		return nil
	}
	return []Action{{ID: ActionOpen, Label: t(lang, "Yanıtla", "Reply"), URL: v["url"]}}
}

func metricLabel(metric, lang string) string {
	switch metric {
	case "first_response":
		return t(lang, "İlk yanıt", "First response")
	case "next_response":
		return t(lang, "Sonraki yanıt", "Next response")
	case "resolution":
		return t(lang, "Çözüm", "Resolution")
	}
	return metric
}

func slaWarning(lang string, v Vars, now time.Time) Message {
	left := ""
	if due, ok := parseTime(v["due"]); ok {
		left = Duration(due.Sub(now), lang)
	}
	metric := metricLabel(v["metric"], lang)
	d := conversationDetails(lang, v, now)
	if metric != "" {
		d = append(d, Detail{Label: t(lang, "Hedef", "Target"), Value: metric})
	}
	if due, ok := parseTime(v["due"]); ok {
		d = append(d, Detail{Label: t(lang, "Son tarih", "Due"), Value: When(due, v["timezone"], lang)})
	}
	title := t(lang, "SLA süresi yaklaşıyor", "SLA due soon")
	lead := t(lang, convRef(v)+" numaralı konuşmanın "+strings.ToLower(metric)+" hedefi dolmak üzere.",
		"The "+strings.ToLower(metric)+" target of conversation "+convRef(v)+" is about to be missed.")
	body := pushRef(lang, v)
	if left != "" {
		title = withSuffix(title, convRef(v)+" · "+t(lang, left+" kaldı", left+" left"))
		lead = t(lang, convRef(v)+" numaralı konuşmanın "+strings.ToLower(metric)+" hedefine "+left+" kaldı.",
			left+" left on the "+strings.ToLower(metric)+" target of conversation "+convRef(v)+".")
		body = withSuffix(body, t(lang, left+" kaldı", left+" left"))
	}
	return Message{
		Title: title, Lead: lead, Details: d,
		Paragraphs: []string{t(lang, "Hedefi kaçırmamak için müşteriye şimdi yanıt verin.", "Reply to the customer now to stay within the target.")},
		Actions:    openConversation(lang, v),
		Push:       Push{Title: t(lang, "SLA süresi yaklaşıyor", "SLA due soon"), Body: body},
		Category:   conversationCategory(v),
	}
}

func slaBreached(lang string, v Vars, now time.Time) Message {
	metric := metricLabel(v["metric"], lang)
	d := conversationDetails(lang, v, now)
	if metric != "" {
		d = append(d, Detail{Label: t(lang, "Hedef", "Target"), Value: metric})
	}
	if due, ok := parseTime(v["due"]); ok {
		d = append(d, Detail{Label: t(lang, "Son tarih", "Due"), Value: When(due, v["timezone"], lang)})
	}
	return Message{
		Title: withSuffix(t(lang, "SLA ihlal edildi", "SLA breached"), convRef(v)),
		Lead: t(lang, convRef(v)+" numaralı konuşmanın "+strings.ToLower(metric)+" hedefi aşıldı.",
			"The "+strings.ToLower(metric)+" target of conversation "+convRef(v)+" was missed."),
		Paragraphs: []string{t(lang, "Konuşmayı hemen ele alın veya bir ekip arkadaşınıza atayın.", "Pick the conversation up now or assign it to a teammate.")},
		Details:    d,
		Actions:    openConversation(lang, v),
		Push:       Push{Title: t(lang, "SLA ihlal edildi", "SLA breached"), Body: pushRef(lang, v)},
		Category:   conversationCategory(v),
	}
}

func csatSubmitted(lang string, v Vars, now time.Time) Message {
	who := v["customer"]
	if who == "" {
		who = t(lang, "Bir müşteri", "A customer")
	}
	d := []Detail{{Label: t(lang, "Puan", "Rating"), Value: v["rating"] + "/5"}}
	d = append(d, conversationDetails(lang, v, now)...)
	return Message{
		Title: withSuffix(t(lang, "Yeni memnuniyet puanı: "+v["rating"]+"/5", "New satisfaction rating: "+v["rating"]+"/5"), convRef(v)),
		Lead: t(lang, who+", "+convRef(v)+" numaralı konuşmaya "+v["rating"]+"/5 puan verdi.",
			who+" rated conversation "+convRef(v)+" "+v["rating"]+"/5."),
		Quote:      Preview(v["comment"], 280),
		QuoteLabel: t(lang, "Yorum", "Comment"),
		Details:    d,
		Actions:    openConversation(lang, v),
		Push:       Push{Title: t(lang, "Yeni memnuniyet puanı", "New satisfaction rating"), Body: t(lang, "Bir müşteri "+v["rating"]+"/5 puan verdi.", "A customer rated "+v["rating"]+"/5.")},
		Category:   CategoryGeneric,
	}
}

func companyHealthDropped(lang string, v Vars, _ time.Time) Message {
	d := []Detail{
		{Label: t(lang, "Şirket", "Company"), Value: v["company"]},
		{Label: t(lang, "Sağlık skoru", "Health score"), Value: v["score"]},
	}
	if v["previous_score"] != "" {
		d = append(d, Detail{Label: t(lang, "Önceki skor", "Previous score"), Value: v["previous_score"]})
	}
	if v["threshold"] != "" {
		d = append(d, Detail{Label: t(lang, "Risk eşiği", "Risk threshold"), Value: v["threshold"]})
	}
	var actions []Action
	if v["url"] != "" {
		actions = []Action{{ID: ActionOpen, Label: t(lang, "Şirketi aç", "Open company"), URL: v["url"]}}
	}
	return Message{
		Title: withSuffix(t(lang, "Kayıp riski", "Churn risk"), v["company"]),
		Lead: t(lang, "Sorumlu olduğunuz "+v["company"]+" şirketinin sağlık skoru "+v["score"]+" puana düştü.",
			"The health score of "+v["company"]+", an account you own, dropped to "+v["score"]+"."),
		Paragraphs: []string{t(lang, "Son konuşmaları ve kullanım verilerini inceleyip müşteriyle iletişime geçin.", "Review recent conversations and usage, then reach out to the customer.")},
		Details:    d, Actions: actions,
		Push: Push{Title: t(lang, "Kayıp riski", "Churn risk"), Body: t(lang, "Sorumlu olduğunuz bir şirketin sağlık skoru düştü.", "The health score of an account you own dropped.")},
	}
}

func ticketAssigned(lang string, v Vars, _ time.Time) Message {
	lead := t(lang, v["ticket"]+" numaralı ürün talebi size atandı.", "Product request "+v["ticket"]+" was assigned to you.")
	if v["actor"] != "" {
		lead = t(lang, v["actor"]+", "+v["ticket"]+" numaralı ürün talebini size atadı.", v["actor"]+" assigned you product request "+v["ticket"]+".")
	}
	var actions []Action
	if v["url"] != "" {
		actions = []Action{{ID: ActionOpen, Label: t(lang, "Talebi aç", "Open request"), URL: v["url"]}}
	}
	return Message{
		Title:   withSuffix(t(lang, "Ürün talebi size atandı", "Product request assigned to you"), v["ticket"]),
		Lead:    lead,
		Actions: actions,
		Push:    Push{Title: t(lang, "Ürün talebi size atandı", "Product request assigned"), Body: v["ticket"]},
	}
}

// --- operations ------------------------------------------------------------

func callMissed(lang string, v Vars, now time.Time) Message {
	d := []Detail{}
	if v["channel_name"] != "" {
		d = append(d, Detail{Label: t(lang, "Hat", "Line"), Value: v["channel_name"]})
	}
	if at, ok := parseTime(v["at"]); ok {
		d = append(d, Detail{Label: t(lang, "Zaman", "Time"), Value: When(at, v["timezone"], lang)})
	}
	var actions []Action
	if v["url"] != "" {
		actions = []Action{{ID: ActionOpen, Label: t(lang, "Aramaları aç", "Open calls"), URL: v["url"]}}
	}
	return Message{
		Title:   t(lang, "Cevapsız arama", "Missed call"),
		Lead:    t(lang, "Ses kanalınızda cevapsız bir arama var. Müşteriyi geri arayın.", "You have a missed call on your voice channel. Call the customer back."),
		Details: d, Actions: actions,
		Push: Push{Title: t(lang, "Cevapsız arama", "Missed call"), Body: t(lang, "Ses kanalınızda cevapsız bir arama var.", "You have a missed call on your voice channel.")},
	}
}

func oncallPage(lang string, v Vars, _ time.Time) Message {
	d := []Detail{}
	if v["title"] != "" {
		d = append(d, Detail{Label: t(lang, "Uyarı", "Alert"), Value: Preview(v["title"], 120)})
	}
	return Message{
		Title:      t(lang, "ClickFix nöbet uyarısı", "ClickFix on-call alert"),
		Lead:       t(lang, "Açık bir nöbet uyarısı var ve sizin müdahalenizi bekliyor.", "An open on-call alert is waiting for you."),
		Paragraphs: []string{t(lang, "Uyarıyı üstlenmek için nöbet sayfasını açın.", "Open the on-call page to acknowledge it.")},
		Details:    d,
		Actions:    []Action{{ID: ActionOpen, Label: t(lang, "Uyarıyı aç", "Open alert"), URL: "/oncall"}},
		Push:       Push{Title: t(lang, "Nöbet uyarısı", "On-call alert"), Body: t(lang, "Açık bir nöbet uyarısı var.", "An on-call alert is open.")},
	}
}

func teamchatMentioned(lang string, v Vars, _ time.Time) Message {
	actor := v["actor"]
	if actor == "" {
		actor = t(lang, "Bir ekip arkadaşınız", "A teammate")
	}
	return Message{
		Title:      t(lang, "Ekip sohbetinde sizden bahsedildi", "You were mentioned in team chat"),
		Lead:       t(lang, actor+" ekip sohbetinde sizden bahsetti.", actor+" mentioned you in team chat."),
		Quote:      Preview(v["preview"], 200),
		QuoteLabel: t(lang, "Mesaj", "Message"),
		Push:       Push{Title: t(lang, "Sizden bahsedildi", "You were mentioned"), Body: t(lang, "Ekip sohbetinde yeni bir mesaj var.", "There is a new team chat message.")},
	}
}

func videoBalanceLow(lang string, v Vars, _ time.Time) Message {
	return Message{
		Title: t(lang, "Görüntülü görüşme dakikanız azalıyor", "Your video minutes are running low"),
		Lead:  t(lang, "Kalan görüntülü görüşme dakikanız: "+v["balance"]+".", "Video minutes left: "+v["balance"]+"."),
		Paragraphs: []string{t(lang, "Bakiye bittiğinde yeni oda açılamaz ve süren görüşmeler sonlandırılır. Kesinti olmaması için dakika paketi ekleyin.",
			"When the balance runs out no new room can be opened and running calls are ended. Add minutes to avoid an interruption.")},
		Details: []Detail{{Label: t(lang, "Kalan dakika", "Minutes left"), Value: v["balance"]}},
		Actions: []Action{{ID: ActionOpen, Label: t(lang, "Dakika ekle", "Add minutes"), URL: "/billing"}},
		Push:    Push{Title: t(lang, "Görüntülü görüşme dakikası azalıyor", "Video minutes running low"), Body: t(lang, "Kalan dakika: "+v["balance"], "Minutes left: "+v["balance"])},
	}
}

func reportReady(lang string, v Vars, _ time.Time) Message {
	title := strings.TrimSpace(v["title"])
	lead := t(lang, "Zamanlanmış raporunuz hazır.", "Your scheduled report is ready.")
	if title != "" {
		lead = t(lang, "\""+title+"\" raporunuz hazır.", "Your report \""+title+"\" is ready.")
	}
	return Message{
		Title:   withSuffix(t(lang, "Rapor hazır", "Report ready"), title),
		Lead:    lead,
		Actions: linkAction(t(lang, "Raporu görüntüle", "View report"), v["url"]),
		Push:    Push{Title: t(lang, "Rapor hazır", "Report ready"), Body: t(lang, "Zamanlanmış raporunuz hazır.", "Your scheduled report is ready.")},
	}
}

// --- background jobs (resource names arrive already localized) ---------------

func exportReady(lang string, v Vars, _ time.Time) Message {
	d := []Detail{{Label: t(lang, "Kayıt türü", "Records"), Value: v["resource"]}}
	if v["format"] != "" {
		d = append(d, Detail{Label: t(lang, "Biçim", "Format"), Value: v["format"]})
	}
	return Message{
		Title:   withSuffix(t(lang, "Dışa aktarım hazır", "Export ready"), v["resource"]),
		Lead:    t(lang, v["resource"]+" dışa aktarımınız indirilmeye hazır.", "Your "+v["resource"]+" export is ready to download."),
		Details: d,
		Actions: linkAction(t(lang, "Dışa aktarımı aç", "Open export"), v["url"]),
		Push:    Push{Title: t(lang, "Dışa aktarım hazır", "Export ready"), Body: t(lang, v["resource"]+" dışa aktarımınız hazır.", "Your "+v["resource"]+" export is ready.")},
	}
}

func importApplied(lang string, v Vars, _ time.Time) Message {
	d := []Detail{{Label: t(lang, "Kayıt türü", "Records"), Value: v["resource"]}}
	for _, k := range []struct{ key, tr, en string }{{"created", "Eklenen", "Created"}, {"updated", "Güncellenen", "Updated"}, {"failed", "Hatalı", "Failed"}} {
		if v[k.key] != "" {
			d = append(d, Detail{Label: t(lang, k.tr, k.en), Value: v[k.key]})
		}
	}
	return Message{
		Title:   withSuffix(t(lang, "İçe aktarım tamamlandı", "Import finished"), v["resource"]),
		Lead:    t(lang, v["resource"]+" içe aktarımı tamamlandı.", "The "+v["resource"]+" import has finished."),
		Details: d,
		Actions: linkAction(t(lang, "Sonucu görüntüle", "View result"), v["url"]),
		Push:    Push{Title: t(lang, "İçe aktarım tamamlandı", "Import finished"), Body: t(lang, v["resource"]+" içe aktarımı tamamlandı.", "The "+v["resource"]+" import has finished.")},
	}
}

func bulkCompleted(lang string, v Vars, _ time.Time) Message {
	d := []Detail{{Label: t(lang, "Kayıt türü", "Records"), Value: v["resource"]}, {Label: t(lang, "Başarılı", "Succeeded"), Value: v["succeeded"]}}
	if v["failed"] != "" && v["failed"] != "0" {
		d = append(d, Detail{Label: t(lang, "Başarısız", "Failed"), Value: v["failed"]})
	}
	return Message{
		Title:   t(lang, "Toplu işlem tamamlandı", "Bulk action finished"),
		Lead:    t(lang, v["resource"]+" kayıtları üzerindeki toplu işlem tamamlandı.", "The bulk action on "+v["resource"]+" records has finished."),
		Details: d,
		Push:    Push{Title: t(lang, "Toplu işlem tamamlandı", "Bulk action finished"), Body: t(lang, v["succeeded"]+" kayıt üzerinde toplu işlem tamamlandı.", "The bulk action on "+v["succeeded"]+" records has finished.")},
	}
}

func bulkFailed(lang string, v Vars, _ time.Time) Message {
	return Message{
		Title:      t(lang, "Toplu işlem başarısız", "Bulk action failed"),
		Lead:       t(lang, v["resource"]+" kayıtları üzerindeki toplu işlem tamamlanamadı.", "The bulk action on "+v["resource"]+" records could not be completed."),
		Quote:      Preview(v["error"], 200),
		QuoteLabel: t(lang, "Hata", "Error"),
		Paragraphs: []string{t(lang, "Kayıtlar değişmedi veya geri alındı; işlemi yeniden başlatabilirsiniz.", "Records were left unchanged or rolled back; you can start the action again.")},
		Push:       Push{Title: t(lang, "Toplu işlem başarısız", "Bulk action failed"), Body: t(lang, "Bir toplu işlem tamamlanamadı.", "A bulk action could not be completed.")},
	}
}

// --- appointments (the host's side; invitee e-mails use the organization's templates) ---

func appointmentHost(event string) builder {
	return func(lang string, v Vars, _ time.Time) Message {
		who := v["customer"]
		if who == "" {
			who = t(lang, "Bir müşteri", "A customer")
		}
		start, hasStart := parseTime(v["start"])
		when := ""
		clock := ""
		if hasStart {
			when = When(start, v["timezone"], lang)
			clock = Clock(start, v["timezone"], lang)
		}
		var title, lead, pushTitle string
		switch event {
		case "booked":
			title, pushTitle = t(lang, "Yeni randevu: "+who, "New appointment: "+who), t(lang, "Yeni randevu", "New appointment")
			lead = t(lang, who+" sizinle "+when+" için "+v["type"]+" randevusu aldı.", who+" booked a "+v["type"]+" with you for "+when+".")
		case "rescheduled":
			title, pushTitle = t(lang, "Randevu yeniden planlandı: "+who, "Appointment rescheduled: "+who), t(lang, "Randevu yeniden planlandı", "Appointment rescheduled")
			lead = t(lang, who+" ile olan "+v["type"]+" randevusu "+when+" olarak yeniden planlandı.", "Your "+v["type"]+" with "+who+" was moved to "+when+".")
		case "confirmed":
			title, pushTitle = t(lang, "Randevu onaylandı: "+who, "Appointment confirmed: "+who), t(lang, "Randevu onaylandı", "Appointment confirmed")
			lead = t(lang, who+" ile "+when+" tarihli "+v["type"]+" randevusu onaylandı.", "Your "+v["type"]+" with "+who+" on "+when+" is confirmed.")
		case "cancelled":
			title, pushTitle = t(lang, "Randevu iptal edildi: "+who, "Appointment canceled: "+who), t(lang, "Randevu iptal edildi", "Appointment canceled")
			lead = t(lang, who+" ile "+when+" tarihli "+v["type"]+" randevusu iptal edildi.", "Your "+v["type"]+" with "+who+" on "+when+" was canceled.")
		case "reminder":
			title, pushTitle = t(lang, "Yaklaşan randevu: "+who, "Upcoming appointment: "+who), t(lang, "Yaklaşan randevu", "Upcoming appointment")
			lead = t(lang, who+" ile "+v["type"]+" randevunuz "+when+" tarihinde başlıyor.", "Your "+v["type"]+" with "+who+" starts "+when+".")
		case "no_show":
			title, pushTitle = t(lang, "Randevuya katılım olmadı: "+who, "No-show: "+who), t(lang, "Randevuya katılım olmadı", "Appointment no-show")
			lead = t(lang, who+", "+when+" tarihli "+v["type"]+" randevusuna katılmadı.", who+" did not attend the "+v["type"]+" on "+when+".")
		default:
			title, pushTitle = t(lang, "Randevu güncellendi: "+who, "Appointment updated: "+who), t(lang, "Randevu güncellendi", "Appointment updated")
			lead = t(lang, who+" ile "+when+" tarihli "+v["type"]+" randevusu güncellendi.", "Your "+v["type"]+" with "+who+" on "+when+" was updated.")
		}
		var d []Detail
		add := func(label, value string) {
			if strings.TrimSpace(value) != "" {
				d = append(d, Detail{Label: label, Value: value})
			}
		}
		add(t(lang, "Müşteri", "Customer"), v["customer"])
		add(t(lang, "Tür", "Type"), v["type"])
		add(t(lang, "Tarih", "Date"), when)
		if end, ok := parseTime(v["end"]); ok && hasStart {
			add(t(lang, "Süre", "Duration"), Duration(end.Sub(start), lang))
		}
		if prev, ok := parseTime(v["previous"]); ok && event == "rescheduled" {
			add(t(lang, "Önceki zaman", "Previous time"), When(prev, v["timezone"], lang))
		}
		add(t(lang, "Yer", "Where"), v["location"])
		var actions []Action
		if v["url"] != "" {
			actions = append(actions, Action{ID: ActionOpen, Label: t(lang, "Randevuyu aç", "Open appointment"), URL: v["url"]})
		}
		if v["video_url"] != "" && event != "cancelled" && event != "no_show" {
			actions = append(actions, Action{ID: ActionJoin, Label: t(lang, "Görüşmeye katıl", "Join the call"), URL: v["video_url"]})
		}
		body := strings.Join(nonEmpty([]string{clock, v["type"]}), " · ")
		return Message{
			Title: title, Lead: lead, Details: d, Actions: actions,
			Push: Push{Title: pushTitle, Body: body},
		}
	}
}

// linkAction is an "open" action, or none without a URL.
func linkAction(label, url string) []Action {
	if url == "" {
		return nil
	}
	return []Action{{ID: ActionOpen, Label: label, URL: url}}
}

func parseTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	at, err := time.Parse(time.RFC3339, s)
	return at, err == nil
}
