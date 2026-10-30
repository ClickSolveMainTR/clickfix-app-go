# Web widget ve widget SDK (CF-55)

Müşterinin sitesine tek satırla eklenen canlı destek balonu. Widget, `widget` tipinde bir kanal örneğidir
(`channel_instances`); bir organizasyon birden çok widget açabilir (site, marka başına). Örneğin uuid'si
**herkese açık widget id**'sidir.

```html
<script src="https://<app>/widget.js" data-widget="<public id>" async></script>
```

## Parçalar

| Parça | Yer |
|-------|-----|
| Kanal adaptörü (`channels.Adapter` + `Typer`), ayarların normalize edilmesi, ziyaretçi jetonu, `user_hash` | `backend/internal/platform/channels/widget` |
| Ziyaretçi API'si `/v1/widget/{public_id}/*` | `backend/internal/modules/widget` |
| CSAT tablosu `conversation_ratings` | `backend/migrations/000047_widget_ratings` |
| Next aktarıcısı `https://<app>/api/v1/widget/*` → Go `/v1/widget/*` | `frontend/src/app/api/v1/widget/[...path]/route.ts` |
| SDK (TypeScript, bağımlılıksız, Shadow DOM) | `frontend/widget-sdk/src/{widget.ts,runtime,shared}` → `pnpm widget:build` (esbuild) → `frontend/public/widget.js` |
| SDK genel tipleri | `frontend/public/widget.d.ts` (`https://<app>/widget.d.ts`) |
| Yönetim ekranı (editör) | `frontend/src/features/widget`, rota `/widget` (yetki: `tenant.channels.read` / `.manage`, yayın `tenant.widgets.publish`) |
| Editör API'si (taslak, yayın, sürüm, kurulum) | `backend/internal/modules/widget/{usecase,handler}/config.go`, `/v1/tenant/widgets/{uuid}/*` |
| Ayar şeması v2, doğrulama, hedefleme, CSS temizleme | `backend/internal/platform/channels/widget/{settings,normalize,targeting,css,attachments}.go`; TS ikizi `frontend/widget-sdk/src/shared/*` |
| Taslak, sürüm ve kurulum tabloları | `backend/migrations/000209_widget_config` |
| SDK başvurusu | [../widget-sdk.md](../widget-sdk.md) |

## Akış

```
ziyaretçi ─POST bootstrap─▶ ayarlar + çevrimiçi durumu + ekip + konuşmalar + ziyaretçi jetonu
          ─POST conversations (ilk mesaj, ön sohbet adı/e-postası)─▶ InboundMessage
                 (Sender.ExternalID = "visitor:<id>" veya "user:<id>", InstanceScoped, ThreadID = "widget:<uuid>")
                 ─▶ conversations.Receive (diğer kanallarla aynı hat: kişi → konuşma → mesaj → outbox → inbox:{org})
temsilci ─Post─▶ kuyruk ─▶ Deliver ─▶ widget.Send: ziyaretçinin kanalına yayın `widget:<instance>.<hash>`
                                       (sent; harici id "widget:<message uuid>")
ziyaretçi ─GET messages?after=─▶ sadece kendi mesajları + teslim edilmiş yanıtlar (özel notlar asla)
          ─POST read─▶ ApplyStatus(read)  ·  ─POST typing─▶ inbox:{org} `contact.typing`
          ─POST rating─▶ conversation_ratings (çözülmüş/kapalı konuşma, bir kez)
```

## Kararlar

| Konu | Karar | Neden |
|------|-------|-------|
| Ziyaretçi kimliği | Sunucunun ürettiği anonim id + HMAC imzalı ziyaretçi jetonu (12 saat). Süresi geçmiş ama doğru imzalı jeton 90 gün içinde bootstrap ile aynı ziyaretçiye yenilenir | Laravel `anonymous_id` (docs/widget/02, 04) ile aynı anlam; tarayıcı yalnız önbellek, kaynak backend. Jeton widget + ziyaretçiye bağlı; başka widget'ta geçmez |
| Kimlik doğrulama | `user_hash = hex(HMAC-SHA256(identity_secret, user_id))`; gizli anahtar kanal kimlik bilgisidir (Vault'ta mühürlü). Doğrulanınca anonim kişi kullanıcının kişisiyle **birleştirilir** (yeni kişi açılmaz) | docs/widget/05; ortak cihazda başka kullanıcıya ait kişi birleştirilmez |
| Doğrulanmamış e-posta | Kişi eşleştirmesinde kullanılmaz; yalnız ziyaretçinin kendi kişisine, başka kişide yoksa yazılır | Başkasının e-postasını yazarak geçmişine erişilemesin |
| Konuşma ayrımı | Her widget konuşması kendi thread id'si | Ziyaretçi birden çok konuşma açabilir; mevcut pipeline değişmeden |
| Kapalı konuşma | Mesaj `409 WIDGET_CONVERSATION_CLOSED`; çözülmüş konuşmaya mesaj onu yeniden açar | Pipeline kuralı + docs/widget/07 |
| CSAT | Yalnız `resolved`/`closed`; ikinci gönderim mevcut puanı `existing: true` ile döner | docs/widget/07 (200 + existing tercih edildi) |
| Origin | Widget başına izinli alan adı listesi (`example.com`, `*.example.com`); boş = her site. Uygulamanın kendi origin'i hep izinli (önizleme). Next aktarıcısı tarayıcının Origin'ini `X-Widget-Origin` ile iletir | Laravel embed sertleştirmesi (docs/widget/01); listeyi tarayıcı kodu değil API uygular |
| CORS | Yalnız `/api/v1/widget/*` (Next) ve `/v1/widget/*` (Go): her origin'e açık, kimlik bilgisi yok | Oturum çerezleri asla kullanılmaz; erişim ziyaretçi jetonuyla |
| Gerçek zamanlı | Ziyaretçi başına özel Centrifugo kanalı (`widget` ad alanı), anonim bağlantı + yalnız o kanala abonelik jetonu; SDK JSON protokolünü kendisi konuşur. Bağlantı yoksa yoklama (sohbet açıkken 4 sn) | docs/widget/04; SDK'ya bağımlılık eklememek için `centrifuge` paketi gömülmedi |
| Gönderim | `Send` yayın hatasında başarısız olmaz: mesaj zaten kayıtlı, ziyaretçi yoklamayla alır | Gerçek zamanlı kesinti yanıt kaybettirmesin |
| SDK paketleme | Tek TypeScript dosyası, `tsc` ile `public/widget.js` (build ve dev betiklerinde); esbuild eklenmedi | Yeni bağımlılık yok; çıktı `.gitignore`'da |
| Ayarlar | `channel_instances.settings` JSON'u; `widget.Normalize` tek yerde varsayılanları uygular (TS aynası `widget-sdk/src/shared/schema.ts`, Go `testdata/defaults.json` ile eşitliği test edilir) | docs/widget/03 tek şema/normalize |

## Hız sınırları (Redis)

İstek: widget + IP başına 300/dk · bootstrap: IP başına 60/dk · identify: IP başına 20/dk · mesaj: ziyaretçi başına
30/dk · yazıyor: ziyaretçi başına 60/dk. Dosya: en fazla 10 MB (organizasyonun ek limiti daha düşükse o).

## İşletim notları

- Centrifugo'nun `allowed_origins` listesi müşterinin sitesinden açılan websocket'i reddeder. Widget o durumda
  yoklamayla çalışır. Gerçek zamanlı widget için `CENTRIFUGO_ALLOWED_ORIGINS` değerine `*` (bağlantılar zaten
  JWT ile yetkili, çerez yok) veya müşteri alan adları eklenmeli.
- Ekip avatarları: `users` tablosunda avatar alanı yok; widget baş harfleri gösterir (`avatar_url: null`).
- Temsilci "yazıyor" sinyali `widget.Adapter.SendTyping` ile hazır; gelen kutusu ekranı (CF-43) çağıracak.

## CF-150: editör ve SDK yenilemesi

Ekran görüntüleri: [widget-screens](widget-screens).

### Ayar şeması v2

v1 anahtarları (renk, konum, karşılama, ön sohbet, çalışma saatleri…) aynı yerinde kalır; yeni gruplar:
`appearance` (tema, renkler, yazı tipi, köşe, launcher, logo, desen, z-index, özel CSS, marka ibaresi),
`texts` (dile göre metinler), `reply_time`, `home.cards`, `quick_replies`, `csat_options`, `offline`,
`away_mode`, `channels` (WhatsApp/Instagram/Telegram/Messenger/e-posta/telefon bağlantıları, geri arama,
video talebi), `ai`, `attachments`, `features`, `triggers`, `targeting`, `privacy`, `rate_limit`, `mobile`,
`attribute_mapping`, `prechat.{phone,fields,consent}`. `schema_version: 2`. Depolanmış v1 JSON'u v2
varsayılanlarıyla okunur, yani mevcut widget'lar değişmeden çalışır.

`Normalize` gevşek okur (geçersiz değer → varsayılan). `Validate` aynı yoldan geçip reddedilen her değeri
JSON yoluyla (`appearance.launcher.size`, `triggers.2.message`) raporlar. Taslak sorunlarıyla kaydedilir,
yayın temiz şema ister (422 `WIDGET_SETTINGS_INVALID`, `details[].field` = yol).

### Taslak, yayın, sürüm

| Konu | Karar | Neden |
|------|-------|-------|
| Taslak | `widget_drafts` (widget başına bir satır); editör ~1,2 sn sonra otomatik kaydeder, geçersiz değerler dahil | Yazılan hiçbir şey kaybolmasın; ziyaretçiler yalnız yayınlananı görür |
| Yayın | Tek işlemde `channel_instances.settings` + `widget_versions` satırı + taslak silme; `base_version` eşleşmezse 409 | İki editörün birbirini ezmesi engellenir; ziyaretçi API'si değişmeden ayarları okur |
| Geri alma | Eski sürüm yeni sürüm olarak yeniden yayınlanır (`rollback_of`) | Geçmiş değişmez, her canlı durum bir sürümdür |
| Yetki | Taslak `tenant.channels.manage`, yayın/geri alma ayrıca `tenant.widgets.publish` (owner/admin) | Yayın canlı siteyi etkiler; editör rolü taslak hazırlayabilir |
| Çoğaltma | Yayınlanmış ayar + marka kopyalanır, kimlik anahtarı asla | Gizli anahtar paylaşılmaz; kanal limiti `channels.Create` ile uygulanır |
| Kurulum kontrolü | Bootstrap sayfa origin'ini `widget_installs`'a yazar (en fazla 5 dk'da bir, sorgu dizesiz yol) | SSRF'siz ve gerçek ziyaretlere dayalı; uygulamanın kendi origin'i (önizleme) sayılmaz |
| Önizleme | Editör iframe'i (`/widget-preview.html`) gerçek `widget.js`'i önizleme modunda çalıştırır; sahte API | Önizleme ile canlı widget aynı kodu çalıştırır |

### Ziyaretçi API'si eklemeleri

- `POST bootstrap` isteğe bağlı gövde (`page_url`, `referrer`, `language`, `sdk_version`); yanıtta
  `widget.reply` (beklenen yanıt süresi: son 14 günün ortanca ilk yanıtı veya sabit değer, mesai dışında
  bir sonraki açılış) ve `context` (`visible`, `hidden_reason`, `device`, `country`, `returning`,
  `eligible_triggers`).
- `PATCH attributes`: eşlenmiş öznitelikler kişi özel alanlarına (`customfields.Normalize` ile doğrulanarak).
- Mesaj gövdesinde `prechat`, `consent`, `intent` (`human`, `callback`, `video`), `attributes`, `phone`.
- Ek türü ve boyutu, mesaj hız sınırı widget ayarından gelir.
- AI asistan: widget `ai.enabled=false` iken veya ziyaretçi "temsilciye bağlan" dediğinde (ya da devir
  kelimesi yazdığında) mesaj `ai_disabled` / `handoff_requested` ile işaretlenir; `aiagents` bu mesajlara
  yanıt vermez, konuşmayı devreder.
- Ülke: `qrlogin.Locator` (güvenilir kenar başlığı `CF-IPCountry` veya GeoLite2). Next aktarıcısı
  `User-Agent` ve `CF-IPCountry`'yi iletir; başlık yalnız `AUTH_QR_TRUST_GEO_HEADERS` açıkken kullanılır.

## Sonraya kalanlar

Bilinen sorun (olay) banner'ı, web push (docs/widget/09), satır içi (inline) yerleşim modu ve makalelerin
widget içinde okunması bu kapsamda yok.
