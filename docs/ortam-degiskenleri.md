# Ortam değişkenleri envanteri

Dokploy'a girilen ortam değişkenlerinin tamamı bu sayfada. Değer veya sır içermez. Kurulum sırasında rastgele
üretilen sırlar ve sabit değerler [deploy-dokploy.md](./deploy-dokploy.md) "Ortam değişkenleri" bölümündedir.
Bu sayfa, hesap sahibinin dış servislerden alıp gireceği değerleri tek listede toplar (CF-136).

Kurallar:

- Dokploy'da tek bir env listesi tüm servislere gider. Bir değişkenin backend'e ulaşması için `compose.prod.yml`
  içindeki `x-backend-environment` bloğunda da adının yazılı olması gerekir. CI bunu
  `scripts/check-compose-env.sh` ile denetler: backend kodunun okuduğu her değişken bu blokta olmalıdır.
- Boş bırakılan isteğe bağlı değişken, o özelliği kapatır ya da koddaki varsayılanı kullandırır.
- "Sır" işaretli değerler sohbete, issue'ya veya repoya yazılmaz. Sadece Dokploy'a ya da GitHub secrets'a girilir.

## Dış servislerden alınacaklar

| Değişken | Zorunlu | Sır | Nereden |
|---|---|---|---|
| **E-posta (Resend)** | | | |
| `SMTP_PASSWORD` | Evet | Evet | resend.com → API Keys (giden sistem e-postaları, `SMTP_USERNAME=resend`) |
| `RESEND_API_KEY` | E-posta kanalı için | Evet | resend.com → API Keys |
| `RESEND_WEBHOOK_SECRET` | E-posta kanalı için | Evet | resend.com → Webhooks → endpoint → Signing secret |
| `EMAIL_INBOUND_DOMAIN` | E-posta kanalı için | Hayır | `in.getclickfix.com`. MX kaydı Resend'i gösterir (Resend → Domains) |
| **Ödeme: PayTR** | | | |
| `PAYMENTS_PAYTR_ENABLED` | PayTR kullanılacaksa | Hayır | `true` |
| `PAYMENTS_PAYTR_MODE` | Hayır | Hayır | `sandbox` (varsayılan) veya `live` |
| `PAYMENTS_PAYTR_MERCHANT_ID` | Açıksa evet | Hayır | PayTR Mağaza Paneli → Bilgi → API Entegrasyon Bilgileri |
| `PAYMENTS_PAYTR_MERCHANT_KEY` | Açıksa evet | Evet | Aynı ekran |
| `PAYMENTS_PAYTR_MERCHANT_SALT` | Açıksa evet | Evet | Aynı ekran |
| `PAYMENTS_PAYTR_ACCOUNT_UUID` | Hayır | Hayır | Boş bırakın. Mağaza değişirse geçmişi korumak için eski UUID girilir |
| `PAYMENTS_PAYTR_WEBHOOK_CIDRS` | Hayır | Hayır | PayTR bildirim IP aralıkları (virgülle). Boş (varsayılan) = her kaynak kabul edilir, imza (HMAC) yine zorunludur. Ayrıntı [odeme.md](./odeme.md) |
| **Ödeme: iyzico** | | | |
| `PAYMENTS_IYZICO_ENABLED` | iyzico kullanılacaksa | Hayır | `true` |
| `PAYMENTS_IYZICO_MODE` | Hayır | Hayır | `sandbox` (varsayılan) veya `live` |
| `PAYMENTS_IYZICO_API_KEY` | Açıksa evet | Evet | merchant.iyzipay.com (sandbox: sandbox-merchant.iyzipay.com) → Ayarlar → Firma Ayarları |
| `PAYMENTS_IYZICO_SECRET_KEY` | Açıksa evet | Evet | Aynı ekran |
| `PAYMENTS_IYZICO_ACCOUNT_UUID` | Hayır | Hayır | Boş bırakın (PayTR ile aynı kural) |
| `PAYMENTS_IYZICO_STATEMENTS_ENABLED` | Hayır | Hayır | Hesap özeti içe aktarma; boş = iyzico açıksa açık |
| `PAYMENTS_IYZICO_WEBHOOK_CIDRS` | Hayır | Hayır | iyzico bildirim IP aralıkları. PayTR ile aynı kural |
| `PAYMENTS_TRUSTED_PROXY_CIDRS` | Hayır | Hayır | Proxy aralıkları; bu aralıktan gelen isteğin gerçek kaynağı `X-Forwarded-For`'dan okunur. Boş = özel ağlar ve loopback (Traefik/BFF). Daraltmak için `docker network inspect dokploy-network` → Subnet |
| **Ödeme: ortak** | | | |
| `PUBLIC_API_URL` | Kartla ödeme için evet | Hayır | `https://api.getclickfix.com`. iyzico ödeme sonrası tarayıcıyı buraya döndürür (`/v1/public/billing/payments/iyzico/return/...`); webhook adresleri de bu alan adındadır |
| **Faturalama** | | | |
| `BILLING_SELLER_JSON` | Evet | Hayır | Şirketin fatura kimliği (unvan, VKN, vergi dairesi, adres). Tek satır JSON, dışına tırnak konmaz; biçim `.env.example`'da. Boşsa ilk faturası olmayan her organizasyonda kredi satın alma, deneme ödemesi ve yenileme `409 BILLING_SELLER_NOT_CONFIGURED` ile durur (CF-154'te görülen 503'ün nedeni). Geçersiz JSON API'nin açılmasını engeller |
| `BILLING_EINVOICE_PROVIDER` | e-Fatura için | Hayır | Entegratör kodu. Entegratör hesabı açılınca (CF-31/CF-38); boş = kapalı |
| `BILLING_REFERRAL_CREDITS` | Hayır | Hayır | Referans kodu başına AI kredisi; boş = kapalı |
| **Yapay zekâ** | | | |
| `ANTHROPIC_API_KEY` | En az biri | Evet | console.anthropic.com → API Keys |
| `OPENAI_API_KEY` | En az biri | Evet | platform.openai.com → API keys |
| `OPENAI_BASE_URL` | Hayır | Hayır | Boş = `https://api.openai.com/v1` |
| `AI_EMBEDDING_MODEL` | Hayır | Hayır | Boş = `text-embedding-3-small`. 1536 boyutlu olmalı; değiştirmek tüm vektörleri arka planda yeniden üretir (CF-214, [bilgi-katmani.md](./design/bilgi-katmani.md)) |
| **Meta (WhatsApp, Instagram, Messenger)**: ayrıntı [meta-kurulum.md](./meta-kurulum.md) | | | |
| `META_APP_ID`, `META_INSTAGRAM_APP_ID`, `META_WHATSAPP_CONFIG_ID` | Meta için | Hayır | meta-kurulum.md "Kimlikler" tablosu |
| `META_APP_SECRET`, `META_INSTAGRAM_APP_SECRET` | Meta için | Evet | developers.facebook.com → ClickFix → App settings → Basic / Instagram API setup |
| `META_WEBHOOK_VERIFY_TOKEN` | Meta için | Evet | Kendiniz üretin (`openssl rand -hex 32`); aynısı Meta webhook formlarına yazılır |
| `META_CHANNELS_ENABLED` | Meta için | Hayır | Webhook'lar doğrulanınca `true` |
| **Slack**: ayrıntı [slack-teams-kurulum.md](./slack-teams-kurulum.md) | | | |
| `SLACK_CHANNEL_CLIENT_ID` | Slack için | Hayır | api.slack.com → Apps → ClickFix → Basic Information |
| `SLACK_CHANNEL_CLIENT_SECRET` | Slack için | Evet | Aynı ekran |
| `SLACK_CHANNEL_SIGNING_SECRET` | Slack için | Evet | Aynı ekran |
| `SLACK_CHANNEL_REDIRECT_URL` | Slack için | Hayır | slack-teams-kurulum.md'deki OAuth redirect |
| **Telegram** | | | |
| `TELEGRAM_CHANNEL_ENABLED` | Hayır | Hayır | `true`. Bot token'ı her organizasyon panelden girer (BotFather) |
| **Mobil giriş ve push**: ayrıntı `backend/docs/auth.md` | | | |
| `EXPO_ACCESS_TOKEN` | Mobil push için | Evet | expo.dev → Account settings → Access tokens |
| `APNS_KEY_ID`, `APNS_TEAM_ID` | Live Activity push için | Hayır | developer.apple.com → Keys → APNs anahtarı (Key ID) ve Membership (Team ID). Ayrıntı [mobile/widgets.md](./mobile/widgets.md); boş = Live Activity push'u kapalı |
| `APNS_PRIVATE_KEY` | Live Activity push için | Evet | Aynı anahtarın .p8 içeriği, base64 (`base64 -w0 AuthKey_XXXX.p8`) |
| `APNS_TOPIC` | Hayır | Hayır | iOS bundle id; varsayılan `com.getclickfix.app` |
| `AUTH_GOOGLE_NATIVE_CLIENT_IDS` | Google ile mobil giriş için | Hayır | Google Cloud Console → Credentials → iOS ve Android OAuth client id'leri (virgülle) |
| `AUTH_APPLE_NATIVE_CLIENT_IDS` | Apple ile giriş için | Hayır | Bundle id (`com.getclickfix.app`) ve Services ID |
| `AUTH_APPLE_TEAM_ID`, `AUTH_APPLE_KEY_ID` | Hayır | Hayır | developer.apple.com → Keys. Tercihen .p8 dosyası Platform → Entegrasyonlar → Apple ekranından yüklenir |
| `AUTH_APPLE_PRIVATE_KEY` | Hayır | Evet | Aynı .p8 (yukarıdaki panel yolu tercih edilir) |
| `AUTH_REVIEW_ACCOUNTS` | Mağaza incelemesi için | Evet | `eposta:kod` çiftleri; App Store/Play inceleme hesabı |
| **Hata izleme** | | | |
| `BACKEND_SENTRY_DSN`, `FRONTEND_SENTRY_DSN` | Hayır | Hayır | Sentry (veya uyumlu) → Project → Client Keys (DSN); boş = kapalı |
| `SENTRY_TRACES_SAMPLE_RATE` | Hayır | Hayır | `0` kapalı, örn. `0.1` |
| `SENTRY_RELEASE` | Hayır | Hayır | Boş bırakılabilir |
| **Görüntülü görüşme (LiveKit)**: ayrıntı [video-rooms.md](./video-rooms.md) | | | |
| `LIVEKIT_URL` | Video için | Hayır | `wss://video.getclickfix.com` (Dokploy'da `livekit` servisine verilen domain, port 7880) |
| `LIVEKIT_API_KEY` | Video için | Hayır | Kendiniz üretin: `API` + `openssl rand -hex 6`. Aynı değer LiveKit ve egress'e gider |
| `LIVEKIT_API_SECRET` | Video için | Evet | Kendiniz üretin: `openssl rand -base64 36` (en az 32 karakter) |
| `LIVEKIT_TURN_DOMAIN` | Hayır | Hayır | `turn.getclickfix.com`; boş = TURN kapalı (katı güvenlik duvarlı ağlar bağlanamayabilir) |
| `LIVEKIT_NODE_IP` | Hayır | Hayır | Boş = genel IP STUN ile bulunur. NAT arkasındaki sunucuda sabit genel IP (`134.199.188.121`) |
| `LIVEKIT_API_URL` | Girilmez | Hayır | `compose.prod.yml` kurar: `http://livekit.<ağ>:7880` (API → LiveKit iç ağ). Yerelde `.env.example` |
| `VIDEO_EGRESS_S3_ENDPOINT` | Girilmez | Hayır | `compose.prod.yml` kurar: `http://seaweedfs.<ağ>:8333` (egress → platform kayıt deposu). Yerelde `.env.example` |
| `LIVEKIT_RTC_TCP_PORT`, `LIVEKIT_RTC_PORT_START`, `LIVEKIT_RTC_PORT_END`, `LIVEKIT_TURN_RELAY_START`, `LIVEKIT_TURN_RELAY_END`, `LIVEKIT_USE_EXTERNAL_IP`, `LIVEKIT_EGRESS_MEM_LIMIT` | Hayır | Hayır | Varsayılanlar `7881`, `50000`-`50100`, `50200`-`50300`, `true`, `2g`. Değişirse güvenlik duvarı da güncellenir |
| **Güvenlik** | | | |
| `SSO_TRUSTED_PROXY_CIDRS` | IP kısıtlaması kullanılacaksa | Hayır | Dokploy/Traefik ağının CIDR'ı. Boş = `X-Forwarded-For` yok sayılır ve IP kısıtlaması doğrudan TCP adresine göre çalışır |
| `AUTH_QR_TRUST_GEO_HEADERS` | Hayır | Hayır | Sadece Cloudflare arkasında ve konum başlıkları açıkken `true` |

## Ayar (isteğe bağlı, boş = kod varsayılanı)

`JWT_ACCESS_TTL` (15m), `JWT_REFRESH_TTL` (168h), `DB_MAX_CONNS` (20), `HTTP_READ_TIMEOUT` (10s),
`HTTP_WRITE_TIMEOUT` (30s), `HTTP_IDLE_TIMEOUT` (60s), `STORAGE_MAX_BYTES`, `BULK_SYNC_MAX` (50),
`BULK_ROLLBACK_HOURS` (24), `CENTRIFUGO_ENABLED` (true), `CENTRIFUGO_TOKEN_TTL` (1h),
`ATTACHMENT_MAX_FILE_BYTES`, `ATTACHMENT_ORG_QUOTA_BYTES`, `ATTACHMENT_SIGNED_URL_TTL`,
`TENANCY_CACHE_TTL`, `TENANCY_INVITE_TTL`, `ONBOARDING_AI_CREDITS` (40; yeni organizasyonun akıllı kurulum için tek seferlik AI kredisi, [onboarding-kurulum.md](./onboarding-kurulum.md)), `META_RECONCILE_INTERVAL`, `META_RECONCILE_WINDOW`,
`CHANNEL_HEALTH_INTERVAL`.

`LIVEKIT_API_URL` ve `VIDEO_EGRESS_S3_ENDPOINT` Dokploy'a girilmez: `compose.prod.yml` bunları iç ağ adresleriyle
(`http://livekit.<ağ>:7880`, `http://seaweedfs.<ağ>:8333`) kendisi kurar.

## Env değil: organizasyon başına panelden girilenler

Aşağıdakiler Dokploy'a girilmez. Her müşteri organizasyonu kendi bilgilerini panelde **Kanallar → Kanal ekle**
ekranından girer (Teams, SMS, ses ve API kanalı formları CF-119 arayüz PR'ında). Bilgiler veritabanında `APP_ENCRYPTION_KEY` ile şifreli saklanır.

| Kanal | Girilen bilgiler |
|---|---|
| Microsoft Teams | Bot uygulama id'si, uygulama şifresi, tenant id (Azure Bot kaydı; slack-teams-kurulum.md) |
| Google Chat | Google Cloud proje numarası ve hizmet hesabı anahtarı (JSON) ([google-chat-kurulum.md](./google-chat-kurulum.md)) |
| SMS (Netgsm, İleti Merkezi, Twilio) | Sağlayıcı, API kullanıcı/anahtarları, gönderici başlığı, İYS marka kodu |
| Ses (Twilio Voice, Netgsm santral) | Hesap bilgileri ve numara |
| API kanalı | İmzalama anahtarı (bir kez gösterilir) ve yanıtların gideceği webhook adresi |
| E-posta (kendi SMTP/IMAP sunucusu) | SMTP ve IMAP sunucu, port, kullanıcı, şifre |
| Telegram | BotFather bot token'ı |
| WhatsApp, Instagram, Messenger, Slack | Kullanıcı Meta/Slack girişiyle bağlar; token'lar otomatik saklanır |
| Mühendislik köprüsü (GitHub, Jira, Linear) | Ayarlar → Entegrasyonlar ekranından erişim bilgileri |
| Görüntülü görüşme kayıt deposu (CF-139) | Ayarlar → Görüntülü görüşme: kendi S3 bucket'ı (https endpoint, bölge, bucket, erişim anahtarları). Boşsa kayıtlar platform deposuna gider ve saklama süresi (varsayılan 24 saat) sonunda silinir |

## Yeni değişken eklerken

1. Kodda okuyun (`backend/internal/config` ya da ilgili modül).
2. `.env.example`'a açıklamalı satır ekleyin.
3. `compose.prod.yml` `x-backend-environment` bloğuna `NAME: ${NAME:-}` ekleyin (zorunluysa `:?mesaj`).
4. Dış servisten geliyorsa bu sayfadaki tabloya ekleyin.

CI'daki `scripts/check-compose-env.sh` 3. adımın atlanmasını yakalar. Yalnızca yerelde kullanılan değişkenler
betikteki `allow` listesindedir.

### Worker kuyruk ayarları (CF-130)

| Değişken | Varsayılan | Açıklama |
|---|---|---|
| `QUEUE_QUEUES` | boş | Virgüllü `kuyruk[:ağırlık]` listesi (`default:6,critical:10`, `sync:1`). Boşken mevcut ağırlıklar + `sync:1`; üretim ana worker'ında varsayılan liste `sync` dışındadır. Verilen liste varsayılanların tamamını değiştirir. |
| `QUEUE_SCHEDULER_ENABLED` | `true` | Periyodik görevleri kaydetme/başlatma. Yalnız bir worker instance'ında açık olmalı; sync ve ek tüketici replikalarında `false`. In-process worker için de geçerlidir. |
| `QUEUE_SYNC_CONCURRENCY` | `4` | Compose `worker-sync` servisinin `QUEUE_CONCURRENCY` değeri; ana worker eşzamanlılığından bağımsızdır. |
| `QUEUE_TASK_CHECK_INTERVAL` | `200ms` | Boştaki worker'ın Redis'e yeni iş sorma aralığı; bir işin en kötü alınma gecikmesidir (CF-251). |
| `QUEUE_AI_CONCURRENCY` | `16` | Compose `ai-reply` servisinin `QUEUE_CONCURRENCY` değeri: replika başına eşzamanlı müşteri cevabı (CF-251). |
| `QUEUE_AI_REPLICAS` | `1` | `ai-reply` replika sayısı. Servis durum tutmaz; yük artınca yükseltin. |
| `QUEUE_AI_DB_MAX_CONNS` | `24` | `ai-reply` veritabanı havuzu (replika başına). Her iş çalışırken bir bağlantı tutar; `QUEUE_AI_CONCURRENCY` değerinin üstünde kalmalı. Replika ya da eşzamanlılık artarken Postgres `max_connections` (varsayılan 100) bütçesini aşmamalı. |
| `QUEUE_AI_TASK_CHECK_INTERVAL` | `100ms` | `ai-reply` servisinin Redis yoklama aralığı. |
| `AI_REPLY_TIMEOUT` | `20s` | Tek müşteri cevabı üretiminin süre sınırı. Aşılırsa müşteriye devir mesajı gider (CF-251). |

Ağırlıklar pozitif tamsayı olmalı; boş öğe, yinelenen kuyruk, geçersiz ağırlık
ve geçersiz scheduler boolean değeri başlangıçta hatayla reddedilir.
`QUEUE_CONCURRENCY` ana worker'da mevcut varsayılan 10 olarak kalır.
`QUEUE_ENABLED=false` worker tüketimini ve scheduler'ı kapatır.

### Kayıt transkripsiyonu (CF-122)

| Değişken | Açıklama |
|---|---|
| `AI_TRANSCRIBE_MODEL` | OpenAI uyumlu ses modeli. Boşsa kapalı; `OPENAI_API_KEY` ve `OPENAI_BASE_URL` kullanılır. |
| `AI_TRANSCRIBE_PRICE_MICROS` | Başlanan ses dakikası başına USD mikro birimi. Pozitif fiyat yoksa kapalı. Üst sınır 1.000.000.000. |
| `AI_TRANSCRIBE_SUMMARY` | `true` ise kayıt sonrası `reply` rotasıyla özet ve aksiyonlar; varsayılan `false`. Reply fiyatları ve organizasyon AI izni gerekir. |

Bir AI kredisi / başlanan dakika ayrılır; mevcut `ai.tokens` kotasında ses saniyesi sayılır.
Maliyet sağlayıcının bildirdiği metin tokenlarıyla değil kayıt süresiyle ölçülür.
İşlem 25 MiB ve 24 saat sınırındadır; büyük video kayıtları `failed` olur, parçalara bölünmez.
Whisper uyumlu modeller `verbose_json`, `gpt-*` modeller `json`, `*diarize*` modeller `diarized_json` kullanır.
Sağlayıcının verdiği segment zamanları ve konuşmacı etiketleri saklanır; yoksa üretilmez.
Metin ve konuşmacı alanlarına mevcut PII maskesi uygulanır. Asynq `sync` ve `maintenance` kuyrukları gerekir.
Belirsiz sonuçta rezervasyon korunur ve çağrı tekrarlanmaz; kesilen işler 15 dakika sonra başarısız olur.
Dökümler ve özetler kayıt silmesi/saklama süresi ve KVKK silmesine tabidir; timeline yalnızca döküm UUID referansı taşır.
Özet mevcut `reply` giriş sınırına (24.000 bayt) tabidir; daha uzun döküm korunur, özet `failed` olur ve özet sağlayıcısı çağrılmaz. Geçmiş kayıtlara otomatik toplu transkripsiyon uygulanmaz; yeni tamamlanan kayıtlar işlenir.
