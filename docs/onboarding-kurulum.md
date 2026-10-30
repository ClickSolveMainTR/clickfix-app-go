# Akıllı kurulum ve hazır şablonlar (CF-140)

Yeni bir organizasyon açıldığında sahibi `/getting-started` ekranına gelir. İki yol vardır:

1. **Yapay zekâ ile hazırla:** şirketin web sitesi (isteğe bağlı) ve sohbet biçimindeki kısa bir anket
   (sektör, ekip büyüklüğü, kanallar, destek saatleri, diller, sık talepler, ton) okunur; AI çekirdeği
   yapılandırılmış çıktı (JSON şema) ile bir **SetupPlan** üretir.
2. **Hazır şablon seç:** sekiz kullanım senaryosu şablonu (SaaS B2B destek, e-ticaret, ajans, kurumsal BT
   helpdesk, finans/fintech, eğitim, sağlık randevulu destek, saha servis/setup oturumları). Aynı SetupPlan
   şemasını kullanır.

Plan her zaman önce **önizlenir** (yeni / zaten var / kullanılamaz, onay kutularıyla), sonra tek bir arka plan
işinde uygulanır. İlerleme Centrifugo'dan canlı akar, ekran her öğeyi sırayla gösterir; sonunda ekip daveti,
ilk kanal (widget planın karşılama ve renkleriyle tek tıkla) ve gelen kutusu gelir. CF-13 kurulum listesi
(`/setup`) yerinde kalır; üstündeki kart akıllı kuruluma, uygulandıktan sonra 24 saat geri almaya götürür.

## SetupPlan (sürüm 1)

Kaynak: `backend/internal/modules/onboarding/plan.go`, OpenAPI `OnboardingSetupPlan`.

| Alan | Yazıldığı yer |
|------|----------------|
| `branding.primary_color` | `organizations.settings.branding` (yardım merkezi, e-posta, panel); web sitesi boşsa şirket profiline yazılır |
| `business_hours` | `business_hours` (organizasyonun varsayılanı yoksa varsayılan olur) |
| `teams` | `teams` (anahtarlarına yönlendirme kuralları bağlanır) |
| `tags`, `statuses` | `tags`, `conversation_statuses` |
| `sla_policies` | `sla_policies` + `sla_policy_targets` (yeni mesai saatlerine bağlı) |
| `custom_fields` | `custom_field_definitions` (konuşma / kişi) |
| `canned_replies`, `macros` | `canned_responses`, `macros` (organizasyon kapsamı) |
| `forms` | `request_forms` (sistem alanları ve planın özel alanları `custom:<key>`) |
| `knowledge` | Bilgi bankası koleksiyonları ve yayımlanmış başlangıç makaleleri (`onboarding` etiketiyle) |
| `widget` | Var olan ilk widget kanalının karşılama ve rengi; widget yoksa son adımda bu ayarlarla bağlanır |
| `ai_agent` | `ai_agent_configs.persona` (mod değişmez, otomasyonu sahibi açar), ton ve yasaklar `business_context_sections`, prosedürler taslak `ai_procedures` |
| `routing_rules`, `views` | `routing_rules`, `conversation_views` |
| `survey` | CSAT `survey_templates` (çözümde gönderilir) |
| `appointment_types` | Randevu modülü (CF-138), `AppointmentsAdapter` üzerinden: plan özelliği (`module.appointments`) ve organizasyon anahtarı açıksa randevu türü oluşturulur (tek süre, görüntülü, kurulumu yapan kişi sunucu); değilse öğe "kullanılamaz" görünür |
| `notifications` | Sahibin bildirim tercihleri (yalnızca hiç ayarlamamışsa) |

`Normalize` her öğeyi hedef tablonun kısıtlarına göre temizler (renk, slug, kısa yol, alan türü, öncelik,
uzunluk ve adet sınırları); bozuk bir öğe atılır, planın geri kalanı uygulanır.

## Şablonlar

`backend/internal/modules/onboarding/templates/*.json`, binary'ye gömülür. Her dosya: `key`, `version`, ikon,
vurgu rengi, TR/EN ad, açıklama ve öne çıkanlar, `industries` (AI kullanılamadığında en yakın şablonu seçmek
için) ve `plan`. İçerik değişince `version` artırılır; çalıştırma hangi sürümü uyguladığını saklar. Şablon içeriği Türkçedir
ve Türkçe (`tr`) olarak yazılır (makaleler, prosedürler); İngilizce içerik isteyen müşteri yapay zekâ yolunu
kullanır, AI planı arayüz dilinde üretir. Kart başlıkları ve açıklamaları TR/EN'dir.
`TestTemplatesLoad` ve `TestSetupApplyAndUndoAgainstPostgres` her şablonun gerçek şemaya hatasız yazıldığını ve
geri alındığını doğrular.

## Kaynak etiketi ve geri alma

Her uygulama `onboarding_setup_runs` satırıdır; oluşturulan veya değiştirilen her öğe `onboarding_setup_items`
defterine `source = 'onboarding'`, varlık türü ve kimliğiyle (ayarlar için önceki değeriyle) yazılır. Aynı adla
zaten var olan varlıklar **atlanır, asla değiştirilmez**; aynı plan ikinci kez uygulansa da kopya oluşmaz.

"Geri al" uygulamadan sonraki 24 saat içinde açıktır: oluşturulanlar ters sırayla silinir, değiştirilen
ayarlar (marka rengi, widget, AI kişiliği) eski hâline döner. Kullanılmaya başlanmış varlıklar (davet
gönderilmiş anket, yayımlanmış prosedür, makale eklenmiş koleksiyon) korunur ve nedeni öğede yazar. İş ve
bağlam bölümleri değişmez sürüm geçmişi tuttuğu için silinmez, pasifleştirilir.

Uygulama API sürecinde ayrık bir iş olarak çalışır (birkaç düzine ekleme, saniyeler); durumu satırdadır.
Yarıda kalan (15 dakika ilerlemeyen) bir çalışma `failed / interrupted` olarak kapanır, yeniden uygulandığında
oluşmuş öğeler "zaten var" diye atlanır. Bir organizasyonda aynı anda tek çalışma uygulanabilir.

## Yapay zekâ, krediler ve izinler

- Keşif çağrısı her AI çalıştırması gibi ölçülür: `ai_policies`, `ai.tokens` hakkı ve `ai.credits` cüzdanı
  (`aicore.RunAgent`). AI'yı hiç yapılandırmamış organizasyonda keşif varsayılan AI politikasını açar.
- **Başlangıç kredisi:** self-serve açılan her organizasyona bir kez `ONBOARDING_AI_CREDITS` (varsayılan 40,
  ~40 bin token, 30 gün geçerli) `ai_credits` yüklenir (`billing.WalletCommand`, anahtar
  `onboarding-allowance:<org uuid>`). Bu, iki-üç keşif çağrısına yeter. `0` kapatır.
- AI yoksa (sağlayıcı yapılandırılmamış, planda AI yok, kredi bitmiş, günlük sınır) en yakın şablon
  yanıtlarla kişiselleştirilir ve önizleme nedenini söyler (`discovery.notice`).
- Günde en fazla 5 keşif (web sitesi okuması dahil).
- Tüm uç noktalar `tenant.onboarding.manage` ister (sahip ve yönetici; migration `000200`).

## Web sitesi okuma (SSRF)

Yalnızca herkese açık HTTPS, 443 portu. Adres kaydedilmeden ve her yönlendirmede (en fazla 3) doğrulanır;
webhook taşıyıcısı DNS çözümlemesinden **sonra** özel/yerel adresleri (loopback, RFC1918, link-local, CGNAT,
ULA, metadata) bağlantı anında reddeder. Sayfa başına 512 KiB, toplam en fazla 4 sayfa (ana sayfa + aynı
alan adındaki hakkımızda/hizmetler/destek/iletişim bağlantıları), 15 saniye. Script, stil ve gezinme metni
atılır; başlık, açıklama, `theme-color` (marka rengi), başlıklar ve görünür metin AI'ya **güvenilmeyen veri**
olarak gider. Site okunamazsa keşif yanıtlarla devam eder.

## Mobil

Karar: **tam akış web'de.** Mobil uygulama organizasyon açmaz; organizasyonu olmayan kullanıcı web panelinde
`/setup?yeni=1` açar, panel organizasyon açılınca `/getting-started`'a geçer. Keşif sohbeti, galeri, uzun önizleme
listesi ve davetler geniş ekran ister; panel oturumu organizasyonu orada açar.

Mobilde kısa sürüm: Ayarlar'daki **Akıllı kurulum** kartı (sahip / yönetici) son çalıştırmanın durumunu
gösterir, uygulanırken ilerleme çubuğunu 3 saniyede bir yeniler ve ilgili ekranı uygulama içi tarayıcıda
açar (`/getting-started?calisma=<uuid>`).

## Uç noktalar

| Yöntem | Yol | Ne yapar |
|--------|-----|----------|
| GET | `/v1/tenant/onboarding/setup/templates` | Galeri |
| POST | `/v1/tenant/onboarding/setup/templates/{key}` | Şablondan taslak çalıştırma + önizleme |
| POST | `/v1/tenant/onboarding/setup/discover` | Web sitesi + anket → AI planı (veya en yakın şablon) |
| GET | `/v1/tenant/onboarding/setup/latest` | Son çalıştırma |
| GET | `/v1/tenant/onboarding/setup/runs/{uuid}` | Çalıştırma (taslakta önizleme, sonra defter durumları) |
| POST | `/v1/tenant/onboarding/setup/runs/{uuid}/apply` | Seçilen öğeleri uygula (202) |
| POST | `/v1/tenant/onboarding/setup/runs/{uuid}/undo` | 24 saat içinde geri al |

İlerleme olayı: `org:{uuid}` kanalında `onboarding.setup.progress`
(`run_uuid`, `status`, `done`, `failed`, `total`, `item`).
