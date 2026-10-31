package demoseed

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	cfmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customfields/model"
	cfrepo "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customfields/repository"
	cfusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/customfields/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forms/engine"
	formsmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forms/model"
	formsusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/forms/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/oncall"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/quality"
	routingmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/routing/model"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/customfields"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
)

// seedModules fills the support-operations modules (status page, community,
// feedback, custom fields, brands, request forms, quality, routing,
// schedules, on-call and coaching). It runs inside the refresh transaction
// after the appointments step, so every write goes through s.q and is
// replaced (not duplicated) by the next refresh's wipe.
//
// Rows are written with the modules' own sqlc queries; side effects the
// usecases would add (outbox events, notifications, queue drains) are
// deliberately skipped so a refresh never pages or e-mails anyone.
func (s *seeder) seedModules(ctx context.Context) error {
	steps := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"custom fields", s.seedCustomFields},
		{"request forms", s.seedRequestForms},
		{"brands", s.seedBrands},
		{"status page", s.seedStatusPage},
		{"forum", s.seedForum},
		{"feedback", s.seedFeedback},
		{"quality", s.seedQuality},
		{"coaching", s.seedCoaching},
		{"routing", s.seedRouting},
		{"schedules", s.seedSchedules},
		{"on-call", s.seedOncall},
	}
	for _, st := range steps {
		if err := st.fn(ctx); err != nil {
			return fmt.Errorf("%s: %w", st.name, err)
		}
	}
	return nil
}

// istanbul is the demo organization's calendar.
func istanbul() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		return time.FixedZone("Europe/Istanbul", 3*60*60)
	}
	return loc
}

// weekStart is Monday 00:00 (Istanbul) of the week containing t.
func weekStart(t time.Time) time.Time {
	t = t.In(istanbul())
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	offset := (int(day.Weekday()) + 6) % 7
	return day.AddDate(0, 0, -offset)
}

func (s *seeder) uid(key string) uuid.UUID { return s.users[key].Uuid }

// customers returns every seeded contact, company contacts first.
func (s *seeder) customers() []db.Contact {
	out := []db.Contact{}
	for _, c := range s.companies {
		out = append(out, c.contacts...)
	}
	return append(out, s.individuals...)
}

// ---------------------------------------------------------------------------
// Custom fields and request forms

type demoField struct {
	entity, key, label, description string
	kind                            customfields.FieldType
	options                         []customfields.Option
	config                          customfields.Config
}

func intPtr(v int) *int              { return &v }
func floatPtr(v float64) *float64    { return &v }
func uuidPtr(v uuid.UUID) *uuid.UUID { return &v }

var demoFields = []demoField{
	{entity: customfields.EntityConversation, key: "urun_modulu", label: "Ürün modülü", description: "Talebin ilgili olduğu ürün modülü",
		kind: customfields.TypeSelect, options: []customfields.Option{
			{Value: "filo_takip", Label: "Filo Takip", Color: "#2563eb"}, {Value: "bayi_portali", Label: "Bayi Portalı", Color: "#16a34a"},
			{Value: "proje_yonetimi", Label: "Proje Yönetimi", Color: "#7c3aed"}, {Value: "scada_raporlama", Label: "SCADA Raporlama", Color: "#d97706"},
			{Value: "ogrenci_bilgi", Label: "Öğrenci Bilgi Sistemi", Color: "#0f766e"}, {Value: "mobil", Label: "Mobil Uygulama", Color: "#dc2626"},
		}},
	{entity: customfields.EntityConversation, key: "siparis_no", label: "Sipariş / bayi kodu", description: "Bayi portalı taleplerinde sipariş numarası",
		kind: customfields.TypeText, config: customfields.Config{MaxLength: 40, Placeholder: "SP-2026-00000"}},
	{entity: customfields.EntityConversation, key: "etki_seviyesi", label: "Etki seviyesi", description: "Sorundan etkilenen kullanıcı kapsamı",
		kind: customfields.TypeSelect, options: []customfields.Option{
			{Value: "tek_kullanici", Label: "Tek kullanıcı"}, {Value: "ekip", Label: "Bir ekip"}, {Value: "tum_sirket", Label: "Tüm şirket"},
		}},
	{entity: customfields.EntityContact, key: "tercih_edilen_kanal", label: "Tercih edilen kanal",
		kind: customfields.TypeSelect, options: []customfields.Option{
			{Value: "e_posta", Label: "E-posta"}, {Value: "telefon", Label: "Telefon"}, {Value: "whatsapp", Label: "WhatsApp"},
		}},
	{entity: customfields.EntityContact, key: "kvkk_acik_riza", label: "KVKK açık rıza", description: "Pazarlama iletişimi için açık rıza alındı mı?",
		kind: customfields.TypeBoolean},
	{entity: customfields.EntityContact, key: "linkedin_profili", label: "LinkedIn profili", kind: customfields.TypeURL},
	{entity: customfields.EntityCompany, key: "sektor", label: "Sektör",
		kind: customfields.TypeSelect, options: []customfields.Option{
			{Value: "lojistik", Label: "Lojistik"}, {Value: "gida", Label: "Gıda"}, {Value: "enerji", Label: "Enerji"}, {Value: "egitim", Label: "Eğitim"},
			{Value: "saglik", Label: "Sağlık"}, {Value: "turizm", Label: "Turizm"}, {Value: "tekstil", Label: "Tekstil"}, {Value: "mimarlik", Label: "Mimarlık"},
		}},
	{entity: customfields.EntityCompany, key: "lisans_sayisi", label: "Lisans sayısı", kind: customfields.TypeNumber,
		config: customfields.Config{Min: floatPtr(0), Max: floatPtr(100000), Precision: intPtr(0)}},
	{entity: customfields.EntityCompany, key: "yillik_sozlesme_bedeli", label: "Yıllık sözleşme bedeli", kind: customfields.TypeMoney,
		config: customfields.Config{Currency: "TRY", Min: floatPtr(0)}},
	{entity: customfields.EntityCompany, key: "sozlesme_bitis", label: "Sözleşme bitiş tarihi", kind: customfields.TypeDate},
}

// seedCustomFields defines the custom fields through the custom field
// usecase, which validates each definition like the field editor does.
// Field values on the seeded rows stay empty.
func (s *seeder) seedCustomFields(ctx context.Context) error {
	svc := cfusecase.New(cfrepo.New(s.pool, s.q))
	for _, f := range demoFields {
		if _, err := svc.CreateDefinition(ctx, s.org.ID, cfmodel.CreateDefinitionInput{
			EntityType: f.entity, Key: f.key, Label: f.label, Description: f.description,
			Type: f.kind, Options: f.options, Config: f.config,
		}); err != nil {
			return fmt.Errorf("%s.%s: %w", f.entity, f.key, err)
		}
	}
	return nil
}

// seedRequestForms writes two forms through the forms usecase (it resolves
// and validates the custom field references). Neither form is offered on the
// widget, so the widget keeps its own pre-chat settings.
func (s *seeder) seedRequestForms(ctx context.Context) error {
	defs, err := s.q.ListFormCustomFieldDefinitions(ctx, s.org.ID)
	if err != nil {
		return err
	}
	byKey := map[string]uuid.UUID{}
	for _, d := range defs {
		byKey[d.EntityType+"."+d.Key] = d.Uuid
	}
	ref := func(key string) (*uuid.UUID, error) {
		id, ok := byKey[key]
		if !ok {
			return nil, fmt.Errorf("custom field %s is missing", key)
		}
		return &id, nil
	}
	module, err := ref("conversation.urun_modulu")
	if err != nil {
		return err
	}
	order, err := ref("conversation.siparis_no")
	if err != nil {
		return err
	}
	impact, err := ref("conversation.etki_seviyesi")
	if err != nil {
		return err
	}
	channel, err := ref("contact.tercih_edilen_kanal")
	if err != nil {
		return err
	}
	active := true
	forms := []formsmodel.FormInput{
		{
			Name: "Teknik destek talebi", Description: "Yardım merkezi ve e-posta ile açılan teknik destek talepleri.",
			IsActive: &active, Channels: []string{"web", "email"},
			Fields: []engine.Field{
				{Key: engine.KeyName, Source: engine.SourceSystem, Label: "Adınız soyadınız", Required: true},
				{Key: engine.KeyEmail, Source: engine.SourceSystem, Label: "E-posta adresiniz", Required: true},
				{Key: engine.KeySubject, Source: engine.SourceSystem, Label: "Konu", Required: true},
				{Source: engine.SourceConversation, CustomFieldUUID: module, Label: "Hangi modülle ilgili?", Required: true, RequiredToResolve: true},
				{Source: engine.SourceConversation, CustomFieldUUID: order, Label: "Sipariş / bayi kodu", Help: "Bayi portalı taleplerinde sipariş numarasını yazın.",
					Condition: &customfields.Condition{Field: "conversation.urun_modulu", Operator: customfields.CondEquals, Value: "bayi_portali"}},
				{Source: engine.SourceConversation, CustomFieldUUID: impact, Label: "Kaç kişi etkileniyor?"},
				{Key: engine.KeyPriority, Source: engine.SourceSystem, Label: "Öncelik"},
				{Key: engine.KeyMessage, Source: engine.SourceSystem, Label: "Sorunu kısaca anlatın", Required: true},
			},
		},
		{
			Name: "Fatura ve iade talebi", Description: "Temsilcilerin telefonla gelen fatura taleplerini kaydettiği form.",
			IsActive: &active, Channels: []string{"agent"},
			Fields: []engine.Field{
				{Key: engine.KeyName, Source: engine.SourceSystem, Label: "Müşteri adı", Required: true},
				{Key: engine.KeyPhone, Source: engine.SourceSystem, Label: "Telefon"},
				{Key: engine.KeySubject, Source: engine.SourceSystem, Label: "Konu", Required: true},
				{Source: engine.SourceContact, CustomFieldUUID: channel, Label: "Dönüş kanalı tercihi"},
				{Key: engine.KeyMessage, Source: engine.SourceSystem, Label: "Talep detayı", Required: true},
			},
		},
	}
	svc := formsusecase.New(s.pool, s.q)
	for _, f := range forms {
		if _, err := svc.Create(ctx, s.org.ID, f); err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Brands

func (s *seeder) seedBrands(ctx context.Context) error {
	days := map[string][]widget.Period{}
	for _, d := range []string{"mon", "tue", "wed", "thu", "fri"} {
		days[d] = []widget.Period{{Start: "09:00", End: "18:00"}}
	}
	hours := mustJSON(widget.BusinessHours{Enabled: true, Timezone: "Europe/Istanbul", Days: days})
	brands := []struct {
		name, primary, email, sender, path, signature, persona string
		policy                                                 string
	}{
		{s.brand(), "#0f766e", "destek@" + s.domain, s.brand() + " Destek", "anadolu",
			"Saygılarımızla,\n" + s.brand() + " Destek Ekibi",
			"Samimi ama profesyonel bir dille, kısa ve net Türkçe yanıtlar ver. Müşteriye 'siz' diye hitap et.", "Standart SLA"},
		{"Filo360", "#2563eb", "destek@filo360.example.com", "Filo360 Destek", "filo360",
			"İyi çalışmalar,\nFilo360 Destek",
			"Filo takip ürünü Filo360'ın uzman destek asistanısın. Teknik terimleri sade bir dille açıkla.", "Kurumsal SLA"},
	}
	for _, b := range brands {
		if _, err := s.q.CreateBrand(ctx, db.CreateBrandParams{
			OrganizationID: s.org.ID, Name: b.name, PrimaryColor: b.primary, SecondaryColor: "#ffffff",
			SupportEmail: b.email, SenderName: b.sender, ReplyTo: b.email, HelpPath: b.path,
			WidgetDefaults: []byte(`{}`), Signature: b.signature, BusinessHours: hours,
			SlaPolicyID: pgtype.UUID{Bytes: s.policies[b.policy].Uuid, Valid: true}, AiPersona: b.persona,
		}); err != nil {
			return fmt.Errorf("%s: %w", b.name, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Status page

var statusSlugRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,62}$`)

// statusSlug picks the page slug. Status slugs are unique across the
// installation and organizations choose their own, so the organization slug
// is used only while no other organization's page took it.
func (s *seeder) statusSlug(ctx context.Context) (string, error) {
	fallback := fmt.Sprintf("durum-%d", s.org.ID)
	slug := s.org.Slug
	if len(slug) > 63 {
		slug = strings.TrimRight(slug[:63], "-")
	}
	if !statusSlugRE.MatchString(slug) {
		return fallback, nil
	}
	var taken bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM status_pages WHERE slug = $1 AND organization_id <> $2)`, slug, s.org.ID).Scan(&taken)
	if err != nil || !taken {
		return slug, err
	}
	return fallback, nil
}

func (s *seeder) seedStatusPage(ctx context.Context) error {
	slug, err := s.statusSlug(ctx)
	if err != nil {
		return err
	}
	if _, err := s.q.StatusSavePage(ctx, db.StatusSavePageParams{
		OrganizationID: s.org.ID, Slug: slug, Title: s.brand() + " Sistem Durumu", Published: true,
	}); err != nil {
		return err
	}
	components := []struct{ name, group, description string }{
		{"Web uygulaması", "Platform", "Yönetim paneli ve müşteri portalı"},
		{"Mobil uygulama", "Platform", "iOS ve Android saha uygulamaları"},
		{"REST API", "Platform", "Entegrasyon API'si ve toplu uçlar"},
		{"E-posta bildirimleri", "Entegrasyonlar", "Sistem e-postaları ve fatura gönderimi"},
		{"Webhook teslimatı", "Entegrasyonlar", "Sipariş ve durum webhook'ları"},
	}
	ids := map[string]uuid.UUID{}
	for i, c := range components {
		row, err := s.q.StatusCreateComponent(ctx, db.StatusCreateComponentParams{
			OrganizationID: s.org.ID, Name: c.name, GroupName: c.group, Description: c.description, SortOrder: safeconv.ToInt32(i),
		})
		if err != nil {
			return err
		}
		ids[c.name] = row.Uuid
	}
	// Incident timestamps stay behind the database clock: resolving uses now().
	now := s.opts.Now
	if real := time.Now().UTC(); real.Before(now) {
		now = real
	}
	type update struct{ status, body string }
	incidents := []struct {
		title, kind, severity string
		started               time.Duration // before now (negative: in the future)
		length                time.Duration // maintenance window
		components            []string
		updates               []update // the first one opens the incident
		postmortem            string
		conversations         int // linked conversations of scenario 2 (mobile login)
	}{
		{
			title: "Mobil uygulamada oturum açma hatası", kind: "incident", severity: "partial_outage", started: 3 * time.Hour,
			components: []string{"Mobil uygulama"}, conversations: 3,
			updates: []update{
				{"investigating", "Mobil uygulamada \"oturum süresi doldu\" hatası bildirimlerini inceliyoruz."},
				{"identified", "Sorun 4.2.1 sürümündeki token yenileme hatasından kaynaklanıyor. Düzeltme hazırlanıyor."},
				{"monitoring", "4.2.2 sürümü mağazalarda yayında. Girişlerin normale döndüğünü izliyoruz."},
				{"resolved", "Sorun giderildi. 4.2.2 sürümüne güncelleyen tüm kullanıcılar giriş yapabiliyor."},
			},
			postmortem: "Kök neden: 4.2.1 sürümünde yenileme token'ı süresinden önce geçersiz sayılıyordu. " +
				"Önlem: token yenileme akışı için uçtan uca test eklendi ve aşamalı yayın oranı %10'a düşürüldü.",
		},
		{
			title: "E-posta bildirimlerinde gecikme", kind: "incident", severity: "degraded", started: 40 * time.Minute,
			components: []string{"E-posta bildirimleri"},
			updates: []update{
				{"investigating", "Bazı sistem e-postalarının 10-15 dakika gecikmeli ulaştığını tespit ettik."},
				{"monitoring", "E-posta sağlayıcımızdaki kuyruk boşaltıldı; gecikmeler azalıyor, izlemeye devam ediyoruz."},
			},
		},
		{
			title: "Planlı bakım: API veritabanı güncellemesi", kind: "maintenance", severity: "maintenance", started: -3 * 24 * time.Hour, length: 2 * time.Hour,
			components: []string{"REST API", "Webhook teslimatı"},
			updates: []update{
				{"scheduled", "Cumartesi gece 02:00-04:00 arasında API veritabanı sürüm yükseltmesi yapılacaktır. Bu sürede API ve webhook'larda kısa kesintiler yaşanabilir."},
			},
		},
	}
	for _, in := range incidents {
		started := now.Add(-in.started)
		ends := pgtype.Timestamptz{}
		if in.kind == "maintenance" {
			// Start on a whole hour so the window reads well on the page.
			started = started.Truncate(time.Hour)
			ends = ts(started.Add(in.length))
		}
		row, err := s.q.StatusCreateIncident(ctx, db.StatusCreateIncidentParams{
			OrganizationID: s.org.ID, Title: in.title, Kind: in.kind, Severity: in.severity, Status: in.updates[0].status,
			Public: true, StartedAt: ts(started), EndsAt: ends,
		})
		if err != nil {
			return err
		}
		for _, name := range in.components {
			n, err := s.q.StatusLinkComponent(ctx, db.StatusLinkComponentParams{OrganizationID: s.org.ID, IncidentID: row.ID, ComponentUuid: ids[name]})
			if err != nil {
				return err
			}
			if n != 1 {
				return fmt.Errorf("component %s was not linked", name)
			}
		}
		if _, err := s.q.StatusAddUpdate(ctx, db.StatusAddUpdateParams{OrganizationID: s.org.ID, IncidentID: row.ID, Status: in.updates[0].status, Body: in.updates[0].body}); err != nil {
			return err
		}
		for i, u := range in.updates[1:] {
			postmortem := ""
			if i == len(in.updates)-2 {
				postmortem = in.postmortem
			}
			if _, err := s.q.StatusSetIncidentState(ctx, db.StatusSetIncidentStateParams{OrganizationID: s.org.ID, Uuid: row.Uuid, Status: u.status, Postmortem: postmortem}); err != nil {
				return err
			}
			if _, err := s.q.StatusAddUpdate(ctx, db.StatusAddUpdateParams{OrganizationID: s.org.ID, IncidentID: row.ID, Status: u.status, Body: u.body}); err != nil {
				return err
			}
		}
		convs := s.convByScn[2]
		for k := 0; k < len(convs) && k < in.conversations; k++ {
			if _, err := s.q.StatusLinkConversation(ctx, db.StatusLinkConversationParams{OrganizationID: s.org.ID, IncidentID: row.ID, ConversationUuid: convs[k].Uuid}); err != nil {
				return err
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Community forum

func (s *seeder) seedForum(ctx context.Context) error {
	people := s.customers()
	if len(people) < 4 {
		return nil
	}
	categories := []struct{ slug, name string }{
		{"genel-sorular", "Genel Sorular"},
		{"entegrasyonlar", "Entegrasyonlar ve API"},
		{"ipuclari", "İpuçları ve En İyi Uygulamalar"},
	}
	catID := map[string]int64{}
	for _, c := range categories {
		if _, err := s.q.ForumCreateCategory(ctx, db.ForumCreateCategoryParams{OrganizationID: s.org.ID, Slug: c.slug, Name: c.name}); err != nil {
			return err
		}
		row, err := s.q.ForumCategory(ctx, db.ForumCategoryParams{OrganizationID: s.org.ID, Slug: c.slug})
		if err != nil {
			return err
		}
		catID[c.slug] = row.ID
	}
	topics := []struct {
		category, slug, title, body string
		replies                     []string
		accepted                    int // index of the accepted reply, -1 for none
		status                      string
	}{
		{"entegrasyonlar", "siparis-webhooklarini-yeniden-deneme", "Sipariş webhook'larını yeniden denemek mümkün mü?",
			"Sunucumuz bakımdayken gelen webhook'ları sonradan tekrar alabilir miyiz? Panelde bir yeniden gönder seçeneği göremedim.",
			[]string{
				"Bizde de aynı ihtiyaç vardı. Destek ekibine yazınca son 24 saatin olaylarını yeniden kuyruğa aldılar.",
				"Ayarlar > Geliştirici > Webhook kayıtları ekranında her teslimatın yanında \"Yeniden gönder\" düğmesi var; toplu seçim de yapılabiliyor.",
			}, 1, "published"},
		{"entegrasyonlar", "api-limitine-takilmadan-toplu-guncelleme", "API limitine takılmadan toplu stok güncelleme",
			"Gece senkronizasyonunda 429 hatası alıyoruz. Toplu uçları kullanan var mı, deneyimleriniz nasıl?",
			[]string{
				"Bulk uca geçince istek sayımız 40'ta bire düştü, 429 hatası tamamen kayboldu.",
				"Tek seferde en fazla 500 kayıt gönderin; daha büyük paketleri bölmek gerekiyor.",
			}, 0, "published"},
		{"genel-sorular", "yeni-kullanici-davet-etme", "Ekibe yeni kullanıcıyı nasıl davet ederim?",
			"Muhasebe ekibimizden iki kişiyi sadece fatura ekranlarını görecek şekilde eklemek istiyoruz.",
			[]string{"Ayarlar > Ekip > Davet et adımından \"Faturalandırma\" rolünü seçmeniz yeterli; diğer ekranları göremezler."}, 0, "published"},
		{"ipuclari", "excel-raporlarini-otomatik-gondermek", "Excel raporlarını her pazartesi otomatik e-postayla almak",
			"Haftalık sevkiyat raporunu zamanlayıp yöneticilere otomatik gönderen var mı? Paylaşırsanız sevinirim.",
			[]string{"Raporlar ekranında \"Zamanla\" seçeneğiyle pazartesi 08:00 için alıcı listesi tanımlayabilirsiniz."}, -1, "published"},
		{"genel-sorular", "karanlik-mod-ne-zaman", "Mobil uygulamaya karanlık mod gelecek mi?",
			"Saha ekiplerimiz gece vardiyasında çalışıyor, karanlık mod çok işimize yarar.",
			nil, -1, "pending"},
	}
	who := 0
	next := func() db.Contact {
		c := people[who%len(people)]
		who++
		return c
	}
	for _, t := range topics {
		author := next()
		topic, err := s.q.ForumCreatePost(ctx, db.ForumCreatePostParams{
			OrganizationID: s.org.ID, CategoryID: catID[t.category], ContactID: author.ID, Slug: t.slug, Title: t.title, Body: t.body,
		})
		if err != nil {
			return fmt.Errorf("%s: %w", t.slug, err)
		}
		if err := s.q.ForumSubscribe(ctx, db.ForumSubscribeParams{OrganizationID: s.org.ID, TopicID: topic.ID, ContactID: author.ID}); err != nil {
			return err
		}
		if t.status != "published" {
			continue
		}
		if _, err := s.q.ForumModerate(ctx, db.ForumModerateParams{OrganizationID: s.org.ID, Uuid: topic.Uuid, Status: "published"}); err != nil {
			return err
		}
		for i, body := range t.replies {
			reply, err := s.q.ForumCreatePost(ctx, db.ForumCreatePostParams{
				OrganizationID: s.org.ID, CategoryID: topic.CategoryID, TopicID: pgtype.Int8{Int64: topic.ID, Valid: true},
				ContactID: next().ID, Slug: fmt.Sprintf("reply-%s-%d", t.slug, i+1), Body: body,
			})
			if err != nil {
				return err
			}
			if _, err := s.q.ForumModerate(ctx, db.ForumModerateParams{OrganizationID: s.org.ID, Uuid: reply.Uuid, Status: "published"}); err != nil {
				return err
			}
			if i == t.accepted {
				if err := s.q.ForumAccept(ctx, db.ForumAcceptParams{OrganizationID: s.org.ID, ID: topic.ID, AcceptedReplyID: pgtype.Int8{Int64: reply.ID, Valid: true}}); err != nil {
					return err
				}
			}
			if err := s.q.ForumVote(ctx, db.ForumVoteParams{OrganizationID: s.org.ID, PostID: reply.ID, ContactID: author.ID}); err != nil {
				return err
			}
		}
		for k := 0; k < 2+len(t.replies); k++ {
			voter := people[(who+k*3)%len(people)]
			if err := s.q.ForumVote(ctx, db.ForumVoteParams{OrganizationID: s.org.ID, PostID: topic.ID, ContactID: voter.ID}); err != nil {
				return err
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Feedback, changelog and roadmap

func (s *seeder) seedFeedback(ctx context.Context) error {
	people := s.customers()
	if len(people) == 0 {
		return nil
	}
	if err := s.q.FeedbackSaveSettings(ctx, db.FeedbackSaveSettingsParams{OrganizationID: s.org.ID, PublicEnabled: true}); err != nil {
		return err
	}
	author := i8(s.users["zeynep.aydin"].ID)
	requests := []struct {
		title, body, status string
		public              bool
		votes               int
		scenario            int // conversations of this scenario are linked as sources (-1: none)
		comments            []string
	}{
		{"Rapor dışa aktarımında tarih filtresi", "Excel'e aktarılan raporlarda ekranda seçili tarih aralığı ve filtreler dosyanın başlığına da yazılsın.",
			"planned", true, 6, 4, []string{"Ay sonu kapanışında çok işimize yarar.", "Filtrelerin ayrı bir sayfada listelenmesi de güzel olur."}},
		{"Mobil uygulamada karanlık mod", "Gece vardiyasında çalışan saha ekipleri için karanlık tema desteği.",
			"in_progress", true, 5, -1, []string{"Saha ekiplerimiz bunu sabırsızlıkla bekliyor!"}},
		{"Toplu fatura indirme", "Seçilen dönemin tüm faturalarını tek bir ZIP dosyası olarak indirebilmek.",
			"released", true, 4, 0, nil},
		{"WhatsApp şablon mesajlarında değişken", "Onaylı WhatsApp şablonlarında müşteri adı ve sipariş numarası gibi değişkenler kullanılabilsin.",
			"reviewing", true, 3, -1, nil},
		{"Azure AD ile tek oturum açma", "Kurumsal müşterilerimiz kullanıcılarını Azure AD üzerinden yönetmek istiyor.",
			"reviewing", false, 2, -1, nil},
	}
	ids := map[string]int64{}
	voter := 0
	for _, r := range requests {
		row, err := s.q.FeedbackCreate(ctx, db.FeedbackCreateParams{
			OrganizationID: s.org.ID, Title: r.title, Body: r.body, IsPublic: r.public, CreatedBy: author,
		})
		if err != nil {
			return err
		}
		if r.status != row.Status {
			if row, err = s.q.FeedbackStatus(ctx, db.FeedbackStatusParams{OrganizationID: s.org.ID, ID: row.ID, Status: r.status, IsPublic: r.public}); err != nil {
				return err
			}
		}
		ids[r.title] = row.ID
		for k := 0; k < r.votes && k < len(people); k++ {
			c := people[(voter+k)%len(people)]
			if err := s.q.FeedbackVote(ctx, db.FeedbackVoteParams{OrganizationID: s.org.ID, RequestID: row.ID, ContactID: c.ID}); err != nil {
				return err
			}
		}
		voter += 3
		if r.scenario >= 0 {
			convs := s.convByScn[r.scenario]
			for k := 0; k < len(convs) && k < 3; k++ {
				if err := s.q.FeedbackSource(ctx, db.FeedbackSourceParams{OrganizationID: s.org.ID, RequestID: row.ID, ConversationID: convs[k].ID}); err != nil {
					return err
				}
			}
		}
		for k, body := range r.comments {
			c := people[(voter+k)%len(people)]
			id, err := s.q.FeedbackComment(ctx, db.FeedbackCommentParams{OrganizationID: s.org.ID, RequestID: row.ID, ContactID: c.ID, Body: body})
			if err != nil {
				return err
			}
			if _, err := s.q.FeedbackModerate(ctx, db.FeedbackModerateParams{OrganizationID: s.org.ID, Uuid: id, Approved: true}); err != nil {
				return err
			}
		}
	}
	now := s.opts.Now
	if real := time.Now().UTC(); real.Before(now) {
		now = real
	}
	entries := []struct {
		title, body string
		tags        []string
		request     string
		age         time.Duration // 0: draft
	}{
		{"Toplu fatura indirme yayında", "Faturalar ekranında dönem seçip tüm faturaları tek bir ZIP dosyası olarak indirebilirsiniz. Bu özellik sizden gelen taleplerle geliştirildi.",
			[]string{"fatura", "yeni-özellik"}, "Toplu fatura indirme", 6 * 24 * time.Hour},
		{"Mobil uygulama 4.2.2", "Bazı cihazlarda oturumun beklenmedik şekilde kapanmasına yol açan hata giderildi. Güncellemeyi mağazadan yükleyebilirsiniz.",
			[]string{"mobil", "düzeltme"}, "", 20 * 24 * time.Hour},
		{"API için toplu uçlar", "Stok, sipariş ve müşteri kayıtları için tek istekte 500 kayda kadar güncelleme yapan toplu uçlar eklendi. Hız limitleri toplu uçlarda kayıt değil istek başına sayılır.",
			[]string{"api", "entegrasyon"}, "", 35 * 24 * time.Hour},
		{"Karanlık mod (beta)", "Mobil uygulamada karanlık tema beta olarak açılıyor. Ayarlar > Görünüm menüsünden deneyebilirsiniz.",
			[]string{"mobil", "beta"}, "", 0},
	}
	for _, e := range entries {
		published := pgtype.Timestamptz{}
		if e.age > 0 {
			published = ts(now.Add(-e.age))
		}
		request := pgtype.Int8{}
		if e.request != "" {
			request = i8(ids[e.request])
		}
		if _, err := s.q.ChangelogCreate(ctx, db.ChangelogCreateParams{
			OrganizationID: s.org.ID, Title: e.title, Body: e.body, Tags: e.tags, RequestID: request, PublishedAt: published,
		}); err != nil {
			return err
		}
	}
	roadmap := []struct{ title, body, status, issue string }{
		{"Çoklu depo desteği", "Stok modülünde birden fazla depo ve depolar arası transfer.", "planned", "URUN-412"},
		{"Yeni rapor tasarımcısı", "Sürükle-bırak ile özel rapor oluşturma ve paylaşma.", "in_progress", "URUN-388"},
		{"KVKK veri saklama politikaları", "Kayıt türüne göre otomatik anonimleştirme ve silme kuralları.", "released", "URUN-351"},
	}
	for _, r := range roadmap {
		if _, err := s.q.RoadmapCreate(ctx, db.RoadmapCreateParams{
			OrganizationID: s.org.ID, Title: r.title, Body: r.body, Status: r.status,
			SourceIssueUrl: "https://issues." + AgentDomain + "/browse/" + r.issue, IsPublic: true,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Quality and coaching

var demoCriteria = []quality.Criterion{
	{Key: "karsilama", Label: "Karşılama ve empati", Weight: 15},
	{Key: "dogru_cozum", Label: "Doğru ve eksiksiz çözüm", Weight: 40, Critical: true},
	{Key: "iletisim", Label: "Açık ve profesyonel iletişim", Weight: 25},
	{Key: "surec_kvkk", Label: "Süreç ve KVKK uyumu", Weight: 20, Critical: true},
}

var demoRationales = map[string][]string{
	"karsilama":   {"Müşteriye adıyla hitap edildi, sorun anlaşıldığı teyit edildi.", "Karşılama kısa ama nazik.", "Empati cümlesi eksik."},
	"dogru_cozum": {"Kök neden bulundu ve kalıcı çözüm sunuldu.", "Çözüm doğru, ancak alternatif adım paylaşılmadı.", "İlk yanıtta eksik bilgi verildi, ikinci yanıtta düzeltildi."},
	"iletisim":    {"Yanıtlar net, adım adım ve yazım hatasız.", "Teknik terimler sadeleştirilebilirdi.", "Yanıt süresi uzun ama içerik açık."},
	"surec_kvkk":  {"Kimlik doğrulaması yapıldı, kişisel veri paylaşılmadı.", "Etiketleme ve kapanış notu eksiksiz.", "Kapanış notu eksik bırakıldı."},
}

// demoScores builds a plausible score list; quality.Calculate checks it.
func (s *seeder) demoScores(variant int) []quality.Score {
	out := make([]quality.Score, 0, len(demoCriteria))
	for _, c := range demoCriteria {
		value := 70 + s.rnd.IntN(31)
		reasons := demoRationales[c.Key]
		reason := reasons[0]
		if value < 85 {
			reason = reasons[1+variant%2]
		}
		out = append(out, quality.Score{Key: c.Key, Value: value, Rationale: reason})
	}
	return out
}

func numeric(v float64) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	err := n.Scan(fmt.Sprintf("%.2f", v))
	return n, err
}

// seedQuality creates the scorecard and manual reviews on resolved,
// assigned conversations (what the review form writes).
func (s *seeder) seedQuality(ctx context.Context) error {
	card := quality.CardInput{Name: "Destek kalite kartı", Criteria: demoCriteria, Threshold: 75}
	if err := quality.ValidateCard(card); err != nil {
		return err
	}
	criteria := mustJSON(card.Criteria)
	row, err := s.q.QACreateScorecard(ctx, db.QACreateScorecardParams{
		OrganizationID: s.org.ID, Name: card.Name, Criteria: criteria, Threshold: card.Threshold,
	})
	if err != nil {
		return err
	}
	reviewers := []int64{s.users["elif.kara"].ID, s.users["mert.demir"].ID}
	reviews := 0
	for i, c := range s.resolved {
		if reviews >= 14 {
			break
		}
		if c.Status != "resolved" || !c.AssigneeUserID.Valid || i%2 == 1 {
			continue
		}
		scores := s.demoScores(i)
		if reviews == 5 {
			// One review shows a critical failure for the follow-up queue.
			scores[3] = quality.Score{Key: "surec_kvkk", Value: 0, Rationale: "Kimlik doğrulaması yapılmadan hesap bilgisi paylaşıldı."}
		}
		score, critical, err := quality.Calculate(demoCriteria, scores)
		if err != nil {
			return err
		}
		n, err := numeric(score)
		if err != nil {
			return err
		}
		reviewer := reviewers[reviews%len(reviewers)]
		if reviewer == c.AssigneeUserID.Int64 {
			reviewer = reviewers[(reviews+1)%len(reviewers)]
		}
		if _, err := s.q.QAInsertReview(ctx, db.QAInsertReviewParams{
			OrganizationID: s.org.ID, ScorecardID: row.ID, ConversationID: c.ID, AgentID: c.AssigneeUserID,
			ReviewerID: i8(reviewer), Source: "manual", Rubric: criteria, Scores: mustJSON(scores), Score: n,
			CriticalFailure: critical, NeedsReview: critical || score < float64(card.Threshold),
		}); err != nil {
			return err
		}
		reviews++
	}
	return nil
}

// seedCoaching writes a calibration session, coaching plans and
// gamification rules.
func (s *seeder) seedCoaching(ctx context.Context) error {
	cards, err := s.q.QAListScorecards(ctx, s.org.ID)
	if err != nil {
		return err
	}
	var resolved []db.Conversation
	for _, c := range s.resolved {
		if c.Status == "resolved" {
			resolved = append(resolved, c)
		}
	}
	if len(cards) > 0 && len(resolved) >= 2 {
		card := cards[0]
		var criteria []quality.Criterion
		if err := json.Unmarshal(card.Criteria, &criteria); err != nil {
			return err
		}
		sessions := []struct {
			name      string
			conv      db.Conversation
			reviewers []string
			closed    bool
		}{
			{"Ekim kalibrasyonu: API hata talebi", resolved[len(resolved)/3], []string{"elif.kara", "mert.demir", "emre.koc"}, true},
			{"Kasım kalibrasyonu: fatura itirazı", resolved[len(resolved)/2], []string{"elif.kara"}, false},
		}
		for i, sess := range sessions {
			row, err := s.q.CalibrationCreate(ctx, db.CalibrationCreateParams{
				OrganizationID: s.org.ID, ConversationID: sess.conv.ID, ScorecardID: card.ID, Rubric: card.Criteria,
				Name: sess.name, CreatedBy: s.users["elif.kara"].ID,
			})
			if err != nil {
				return err
			}
			for k, key := range sess.reviewers {
				scores := s.demoScores(i + k)
				score, critical, err := quality.Calculate(criteria, scores)
				if err != nil {
					return err
				}
				if _, err := s.q.CalibrationScore(ctx, db.CalibrationScoreParams{
					OrganizationID: s.org.ID, CalibrationID: row.ID, ReviewerID: s.users[key].ID,
					Scores: mustJSON(scores), Score: score, CriticalFailure: critical,
				}); err != nil {
					return err
				}
			}
			if sess.closed && len(sess.reviewers) >= 2 {
				if err := s.q.CalibrationClose(ctx, db.CalibrationCloseParams{OrganizationID: s.org.ID, ID: row.ID}); err != nil {
					return err
				}
			}
		}
	}

	coach := s.users["elif.kara"].ID
	now := s.opts.Now
	plans := []struct {
		agent, goal string
		goalDue     time.Duration
		tasks       []string
		note        string
		done        int // completed tasks
	}{
		{"burak.sahin", "İlk yanıt süresini 30 dakikanın altına indir", 30 * 24 * time.Hour,
			[]string{"Entegrasyon soruları için 5 hazır yanıt hazırla", "API hata kodları bilgi bankası makalesini oku"},
			"Teknik bilgisi çok iyi; yoğun saatlerde yanıtları kısaltıp önce teşekkür mesajı göndermeyi deneyelim.", 1},
		{"zeynep.aydin", "Eğitim taleplerinde CSAT ortalamasını 4,7'ye çıkar", 45 * 24 * time.Hour,
			[]string{"Eğitim sonrası takip e-postası şablonu oluştur"},
			"Müşteri geri bildirimleri çok olumlu; oturum sonrası özet paylaşımı standart hale gelsin.", 0},
		{"deniz.yildiz", "KVKK süreç adımlarını eksiksiz uygula", 21 * 24 * time.Hour,
			[]string{"KVKK silme talebi kontrol listesini tamamla", "Kimlik doğrulama adımlarını bir kalibrasyon oturumunda tekrar et"},
			"Son kalite değerlendirmesinde kimlik doğrulama adımı atlanmıştı; birlikte örnek konuşmaların üzerinden geçtik.", 0},
	}
	for _, p := range plans {
		agent := s.users[p.agent].ID
		goal, err := s.q.CoachingCreate(ctx, db.CoachingCreateParams{
			OrganizationID: s.org.ID, AgentID: agent, CoachID: coach, Kind: "goal", Body: p.goal, DueAt: ts(now.Add(p.goalDue)),
		})
		if err != nil {
			return err
		}
		parent, err := s.q.CoachingParent(ctx, db.CoachingParentParams{OrganizationID: s.org.ID, Uuid: goal.Uuid, AgentID: agent})
		if err != nil {
			return err
		}
		for k, body := range p.tasks {
			task, err := s.q.CoachingCreate(ctx, db.CoachingCreateParams{
				OrganizationID: s.org.ID, AgentID: agent, CoachID: coach, Kind: "task", Body: body,
				DueAt: ts(now.Add(time.Duration(7*(k+1)) * 24 * time.Hour)), ParentID: pgtype.Int8{Int64: parent, Valid: true},
			})
			if err != nil {
				return err
			}
			if k < p.done {
				if _, err := s.q.CoachingComplete(ctx, db.CoachingCompleteParams{OrganizationID: s.org.ID, Uuid: task.Uuid, Completed: true}); err != nil {
					return err
				}
			}
		}
		if _, err := s.q.CoachingCreate(ctx, db.CoachingCreateParams{
			OrganizationID: s.org.ID, AgentID: agent, CoachID: coach, Kind: "note", Body: p.note,
		}); err != nil {
			return err
		}
	}

	rules := []struct {
		name, metric      string
		threshold         float64
		points, minSample int32
	}{
		{"Memnuniyet yıldızı", "csat", 4.5, 50, 5},
		{"Hızlı yanıt", "frt", 900, 30, 10}, // seconds; lower is better
		{"Çözüm ustası", "resolutions", 15, 40, 1},
		{"Kalite şampiyonu", "qa", 85, 60, 2},
	}
	for _, r := range rules {
		if _, err := s.q.GameRuleSave(ctx, db.GameRuleSaveParams{
			OrganizationID: s.org.ID, Name: r.name, Metric: r.metric, Threshold: r.threshold,
			Points: r.points, MinSamples: r.minSample, Enabled: true,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Routing

// seedRouting configures rules and capacity but leaves automatic routing
// off: switched on, the sweeper would assign the demo's unassigned queue as
// soon as an agent comes online. The presenter can turn it on live.
func (s *seeder) seedRouting(ctx context.Context) error {
	if _, err := s.q.UpsertRoutingSettings(ctx, db.UpsertRoutingSettingsParams{
		OrganizationID: s.org.ID, Enabled: false, Strategy: "load_balanced", RespectCapacity: true,
		VipAccountOwner: true, ReassignOnOffline: true, UnansweredMinutes: pgtype.Int4{Int32: 30, Valid: true},
	}); err != nil {
		return err
	}
	rules := []struct {
		name       string
		conditions routingmodel.Conditions
		team       string
		skills     []string
	}{
		{"Fatura ve iade konuları", routingmodel.Conditions{ChannelTypes: []string{}, Priorities: []string{}, TagUUIDs: []uuid.UUID{s.tags["fatura"].Uuid, s.tags["iade"].Uuid}}, "Faturalandırma", []string{}},
		{"Acil talepler teknik desteğe", routingmodel.Conditions{ChannelTypes: []string{}, Priorities: []string{"urgent"}, TagUUIDs: []uuid.UUID{}}, "Teknik Destek", []string{}},
		{"Entegrasyon soruları", routingmodel.Conditions{ChannelTypes: []string{"email"}, Priorities: []string{}, TagUUIDs: []uuid.UUID{s.tags["entegrasyon"].Uuid}}, "Teknik Destek", []string{"api"}},
		{"WhatsApp mobil sorunları", routingmodel.Conditions{ChannelTypes: []string{"whatsapp"}, Priorities: []string{}, TagUUIDs: []uuid.UUID{}}, "", []string{"mobil"}},
		{"Eğitim talepleri", routingmodel.Conditions{ChannelTypes: []string{}, Priorities: []string{}, TagUUIDs: []uuid.UUID{s.tags["eğitim"].Uuid}}, "Müşteri Başarısı", []string{}},
	}
	for i, r := range rules {
		team := pgtype.Int8{}
		if r.team != "" {
			team = i8(s.teams[r.team].ID)
		}
		if _, err := s.q.InsertRoutingRule(ctx, db.InsertRoutingRuleParams{
			OrganizationID: s.org.ID, Name: r.name, Position: safeconv.ToInt32(i + 1), IsActive: true,
			Conditions: mustJSON(r.conditions), TeamID: team, Skills: r.skills,
		}); err != nil {
			return err
		}
	}
	limits := []struct {
		user    string
		channel string
		max     int32
	}{
		{"", "*", 8}, {"", "whatsapp", 5}, {"burak.sahin", "*", 10}, {"elif.kara", "*", 4},
	}
	for _, l := range limits {
		user := pgtype.Int8{}
		if l.user != "" {
			user = i8(s.users[l.user].ID)
		}
		if err := s.q.InsertCapacityLimit(ctx, db.InsertCapacityLimitParams{
			OrganizationID: s.org.ID, UserID: user, ChannelType: l.channel, MaxOpen: l.max,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Schedules

// seedSchedules plans two weeks of weekday shifts and a few leave requests.
// Shifts are informational: enforce_shifts stays off so availability and
// routing are not limited by the demo calendar.
func (s *seeder) seedSchedules(ctx context.Context) error {
	if err := s.q.ScheduleSaveSettings(ctx, db.ScheduleSaveSettingsParams{OrganizationID: s.org.ID, EnforceShifts: false}); err != nil {
		return err
	}
	const tz = "Europe/Istanbul"
	templates := []struct {
		name          string
		start, length int32 // minutes
	}{
		{"Sabah vardiyası", 8 * 60, 8 * 60},
		{"Akşam vardiyası", 12 * 60, 8 * 60},
		{"Hafta sonu nöbeti", 10 * 60, 6 * 60},
	}
	rows := make([]db.ShiftTemplate, 0, len(templates))
	for _, t := range templates {
		row, err := s.q.ScheduleCreateTemplate(ctx, db.ScheduleCreateTemplateParams{
			OrganizationID: s.org.ID, Name: t.name, Timezone: tz, StartMinute: t.start, DurationMinutes: t.length,
		})
		if err != nil {
			return err
		}
		rows = append(rows, row)
	}
	monday := weekStart(s.opts.Now)
	// Deniz is on approved leave next Thursday and Friday.
	leaveStart, leaveEnd := monday.AddDate(0, 0, 10), monday.AddDate(0, 0, 12)
	agents := []string{"mert.demir", "elif.kara", "burak.sahin", "zeynep.aydin", "emre.koc", "deniz.yildiz"}
	for a, key := range agents {
		user := s.users[key].ID
		for d := 0; d < 14; d++ {
			day := monday.AddDate(0, 0, d)
			tmpl := (a + d/7) % 2
			if wd := day.Weekday(); wd == time.Saturday || wd == time.Sunday {
				// One agent per weekend day covers the weekend shift.
				if agents[(d/7*2+int(wd))%len(agents)] != key {
					continue
				}
				tmpl = 2
			}
			if key == "deniz.yildiz" && !day.Before(leaveStart) && day.Before(leaveEnd) {
				continue
			}
			t := rows[tmpl]
			start := day.Add(time.Duration(t.StartMinute) * time.Minute)
			if _, err := s.q.ScheduleCreateShift(ctx, db.ScheduleCreateShiftParams{
				OrganizationID: s.org.ID, UserID: user, TemplateID: i8(t.ID), StartsAt: ts(start),
				EndsAt: ts(start.Add(time.Duration(t.DurationMinutes) * time.Minute)), Timezone: tz,
			}); err != nil {
				return err
			}
		}
	}
	reviewer := i8(s.users["elif.kara"].ID)
	leaves := []struct {
		user, reason, status string
		from, to             time.Time
	}{
		{"deniz.yildiz", "Yıllık izin", "approved", leaveStart, leaveEnd},
		{"zeynep.aydin", "Müşteri eğitimi için şehir dışı görev", "pending", monday.AddDate(0, 0, 14), monday.AddDate(0, 0, 15)},
		{"burak.sahin", "Sağlık kontrolü (yarım gün)", "approved", monday.AddDate(0, 0, 15).Add(9 * time.Hour), monday.AddDate(0, 0, 15).Add(13 * time.Hour)},
	}
	for _, l := range leaves {
		row, err := s.q.ScheduleRequestLeave(ctx, db.ScheduleRequestLeaveParams{
			OrganizationID: s.org.ID, UserID: s.users[l.user].ID, StartsAt: ts(l.from), EndsAt: ts(l.to), Reason: l.reason,
		})
		if err != nil {
			return err
		}
		if l.status == "pending" {
			continue
		}
		if _, err := s.q.ScheduleReviewLeave(ctx, db.ScheduleReviewLeaveParams{
			OrganizationID: s.org.ID, Uuid: row.Uuid, Status: l.status, ReviewedBy: reviewer,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// On-call

// seedOncall writes rotas and escalation policies in the shape the on-call
// usecase validates (members only, ascending step delays, in-app/push
// channels). Uptime monitors are not seeded: they would probe real URLs.
func (s *seeder) seedOncall(ctx context.Context) error {
	monday := weekStart(s.opts.Now)
	rotas := []struct {
		name      string
		layers    []oncall.Layer
		overrides []oncall.Override
	}{
		{"Teknik Destek haftalık nöbet",
			[]oncall.Layer{{StartsAt: monday.Add(9 * time.Hour), RotationMinutes: 7 * 24 * 60,
				Agents: []uuid.UUID{s.uid("mert.demir"), s.uid("burak.sahin"), s.uid("deniz.yildiz")}}},
			[]oncall.Override{{StartsAt: monday.AddDate(0, 0, 3).Add(18 * time.Hour), EndsAt: monday.AddDate(0, 0, 4).Add(9 * time.Hour), Agent: s.uid("elif.kara")}}},
		{"Faturalandırma ay sonu nöbeti",
			[]oncall.Layer{{StartsAt: monday.Add(9 * time.Hour), RotationMinutes: 24 * 60,
				Agents: []uuid.UUID{s.uid("emre.koc"), s.uid("deniz.yildiz")}}},
			[]oncall.Override{}},
	}
	rotaIDs := make([]uuid.UUID, 0, len(rotas))
	for _, r := range rotas {
		row, err := s.q.OncallCreateRota(ctx, db.OncallCreateRotaParams{
			OrganizationID: s.org.ID, Name: r.name, Timezone: "Europe/Istanbul",
			Layers: mustJSON(r.layers), Overrides: mustJSON(r.overrides),
		})
		if err != nil {
			return err
		}
		rotaIDs = append(rotaIDs, row.Uuid)
	}
	push := []string{"inapp", "push"}
	policies := []struct {
		name              string
		steps             []oncall.Step
		urgent, incidents bool
		enabled           bool
	}{
		{"Durum sayfası olayları", []oncall.Step{
			{AfterMinutes: 0, Rota: uuidPtr(rotaIDs[0]), Channels: push},
			{AfterMinutes: 15, Agent: uuidPtr(s.uid("elif.kara")), Channels: push},
			{AfterMinutes: 30, Team: uuidPtr(s.teams["Teknik Destek"].Uuid), Channels: []string{"inapp"}},
		}, false, true, true},
		// Off by default so a new urgent demo conversation does not page anyone.
		{"Acil konuşmalar", []oncall.Step{
			{AfterMinutes: 5, Rota: uuidPtr(rotaIDs[0]), Channels: push},
			{AfterMinutes: 20, Agent: uuidPtr(s.uid("mert.demir")), Channels: []string{"inapp"}},
		}, true, false, false},
	}
	for _, p := range policies {
		if _, err := s.q.OncallCreatePolicy(ctx, db.OncallCreatePolicyParams{
			OrganizationID: s.org.ID, Name: p.name, Steps: mustJSON(p.steps),
			UrgentConversations: p.urgent, Incidents: p.incidents, Enabled: p.enabled,
		}); err != nil {
			return err
		}
	}
	return nil
}
