# Kanal adaptörleri (CF-8)

Yeni bir kanal eklemek = `backend/internal/platform/channels/<tip>` altında bir `channels.Adapter` yazmak ve
`backend/internal/inbox/inbox.go` içindeki `DefaultRegistry`'ye kaydetmek. Gelen kutusu çekirdeği
(`modules/conversations/usecase`) sağlayıcıyı tanımaz.

## Akış

```
sağlayıcı ──POST /v1/webhooks/channels/{type}/{instance-uuid}──▶ channels handler
            (veya POST /v1/webhooks/channels/{type} + Router)        │ ForWebhook: instance + çözülmüş kimlik bilgisi
                                                                     │ adapter.VerifyWebhook (imza, örneğin kendi sırrı)
                                                                     │ adapter.ParseWebhook → WebhookResult
                                                                     ▼
                                              conversations.Receive (tek işlem / mesaj)
     kişi çözümleme (kimlik → e-posta/telefon → yeni) → konuşma (thread / son açık / yeni) → mesaj (external_id ile idempotent)
     → outbox (conversations.created, conversations.reopened, messages.received) → realtime `inbox:{org_uuid}`
     → ekler: InboundAttachment.Open ile indirilip attachments (owner_type=message) olarak saklanır

temsilci ──Post (API)──▶ messages (status=queued) ──asynq app:conversations:send (kuyruk "messaging")──▶ Deliver
     pencere kontrolü (Capabilities.ReplyWindow) → alıcı kimliği → örnek başına hız limiti (Redis) → adapter.Send
     → sent (external_id) | geçici hata: yeniden dene (8 kez, üstel) | kalıcı hata / son deneme: failed
sağlayıcı makbuzu (WebhookResult.Statuses) ──▶ delivered / read / failed (sadece ileri)
```

## Arayüz

| Metot | Ne yapar |
|-------|----------|
| `Type()` | Tip kodu (`whatsapp`, `instagram`, `messenger`, `telegram`, `email`, `widget`, `test`) |
| `Capabilities()` | medya, şablon, yazıyor, tepki, teslim/okundu makbuzu, yanıt penceresi, thread, max metin |
| `Fields()` | Bağlama formu alanları (`Secret` olanlar API'den sadece maskeli döner); `label_key` → `frontend/src/locales/*/channels.json` |
| `Connect(ctx, inst)` | Kimlik bilgisini sağlayıcıda doğrular, webhook yönlendirmesi için sağlayıcı hesap id'sini döner (`channel_instances.external_id`, tüm organizasyonlarda tekil) |
| `VerifyWebhook(r, body, inst)` | İmza / paylaşılan sır; hata → 401 |
| `ParseWebhook(ctx, r, body, inst)` | Normalize mesajlar + makbuzlar |
| `Send(ctx, inst, msg)` | Gönderir, sağlayıcı mesaj id'sini döner. Kalıcı hata için `channels.Permanent(code, err)` |

Opsiyonel: `Challenger` (Meta `hub.challenge` GET), `Router` (tek uygulama webhook'u → hesap id'leri), `Typer`.

## Kurallar

- `InboundMessage.ExternalID` örnek içinde tekil olmalı (Telegram gibi sohbet başına id'lerde `chatID:messageID`).
- `Sender.InstanceScoped`: sayfa/bot kapsamlı id'ler `true` (IGSID, PSID, Telegram chat id, widget ziyaretçi id);
  telefon ve e-posta `false` → aynı kişi farklı inbox'larda tanınır.
- E-posta gibi thread'li kanallar `ThreadID` (kök Message-ID) ve `Subject` doldurur; konuşma thread ile eşleşir.
- Kimlik bilgileri `channels.Vault` ile `channel_instances.credentials_enc`'de mühürlüdür ve organizasyon + örnek
  uuid'sine bağlıdır; başka satıra kopyalanan şifreli metin açılmaz.
- `test` kanalı üretim dışında kayıtlıdır: HMAC imzalı JSON webhook alır, gönderdiklerini bellekte tutar
  (`#fail` geçici, `#reject` kalıcı hata üretir). Uçtan uca test: `backend/internal/inbox/inbox_test.go`.
- Örnek ayarı `send_rate_per_minute` (varsayılan 120) giden hız limitidir.

## Kanallar (CF-56..CF-61)

| Tip | Paket | Gelen | Giden | Bağlama |
|-----|-------|-------|-------|---------|
| `email` | `channels/email` | Resend Receiving: `<alias>@EMAIL_INBOUND_DOMAIN` → `POST /v1/webhooks/channels/email` (Svix imzası, `Router` alıcı alias'ı ile) → ham MIME indirilir | Resend (`from_address` alan adı DNS sihirbazıyla doğrulanır) veya kendi SMTP'si (`provider=smtp`) | Ayarlar: `inbound_alias`; DNS: `/v1/tenant/channels/email/domains` |
| `whatsapp` | `channels/whatsapp` | Meta uygulama webhook'u `/v1/webhooks/channels/whatsapp` (`X-Hub-Signature-256`, `phone_number_id` ile yönlendirme) | Cloud API: metin, medya, şablon, buton/liste | Embedded Signup `/v1/tenant/channels/whatsapp/signup`; şablonlar `/{uuid}/whatsapp/templates` |
| `instagram` | `channels/instagram` | DM, story yanıtı/bahsetme, gönderi yorumları (yorum = ayrı konuşma, `comment:<id>` thread) | DM, herkese açık yorum yanıtı veya `reply_mode=dm` ile özel yanıt; `human_agent` etiketi | Instagram Login OAuth `/v1/tenant/channels/oauth/instagram/*` |
| `messenger` | `channels/messenger` | Sayfa mesajları, postback, teslim makbuzu | RESPONSE; pencere dışı hatada bir kez HUMAN_AGENT (7 gün) | Facebook Login → sayfa seçimi (sayfa token'ları mühürlü, tarayıcıya gitmez) |
| `telegram` | `channels/telegram` | Bot webhook'u (örnek uuid'li yol, `X-Telegram-Bot-Api-Secret-Token` = bot token'ından türetilen HMAC); grupta yalnızca bahsetme/yanıt | Metin (4096'da bölünür), medya, satır içi butonlar | Bot token'ı; `Connect` setWebhook yapar, silme deleteWebhook |

- **Meta kanalları** `META_CHANNELS_ENABLED=true` ve Meta uygulaması (`META_APP_ID`, `META_APP_SECRET`,
  `META_WEBHOOK_VERIFY_TOKEN`) olmadan kayıt edilmez (uygulama incelemesi bekleniyor). Testler kayıtlı fixture'larla
  (`testdata/`) sahte Graph sunucusuna karşı çalışır.
- **Paylaşılan webhook**: `GET /v1/webhooks/channels/{type}` Meta `hub.challenge`'ı uygulama verify token'ı ile yanıtlar
  (`SharedChallenger`).
- **Ücretli mesaj ölçümü**: WhatsApp `sent` durumundaki `pricing.billable=true` → `billing.usage_recorded`
  (`whatsapp.templates`, olay anahtarı `whatsapp:<wamid>`, tekrar gelen makbuz ikinci kez sayılmaz).
- **Sağlık** (`channel_instances.health_*`, `last_webhook_at`): `HealthChecker` (yoksa `Connect`) her
  `CHANNEL_HEALTH_INTERVAL`'da; Instagram uzun ömürlü token'ı bitişe 10 gün kala yenilenir. Liste görünümündeki `health`:
  durum (ok/warning/error/paused/unknown), son 24 saatin gönderim/hata sayısı, token bitişi, "yeniden bağlan" uyarısı.
  `POST /{uuid}/check`, `POST /{uuid}/reconnect`.
- **Mutabakat** (garanti-kulucka `meta-reconcile.ts`): `Reconciler` kanalları (Instagram, Messenger) her
  `META_RECONCILE_INTERVAL`'da son `META_RECONCILE_WINDOW` içindeki mesajları Graph'tan okur; webhook'un kaçırdıkları
  dış id ile tekilleşerek saklanır.
- **Kanal sayısı** plan limiti `channels` (entitlement) ile sınırlıdır: `403 PLAN_LIMIT_EXCEEDED`.
- **E-posta döngü koruması**: `Auto-Submitted`, `Precedence: bulk/list`, liste başlıkları ve kendi
  `X-ClickFix-Instance` başlığımız taşıyan iletiler alınmaz; gönderilenler `X-Auto-Response-Suppress` taşır. Bizim
  Message-ID'miz zincir kökünü taşır (`<cf.{mesaj}.{base64url(kök)}@alan>`), References'ı düşüren istemciler de doğru
  konuşmaya bağlanır. RFC 3464 bounce'ları ve Resend `email.bounced/complained` olayları mesajı `failed` yapar.
- IMAP yoklaması yok: yönlendirme her sağlayıcıda çalışır ve posta kutusu parolası saklamayı gerektirmez.
