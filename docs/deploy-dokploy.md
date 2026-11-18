# Dokploy kurulumu (canlı ortam)

ClickFix üretim yığınının Dokploy'da nasıl kurulduğunu anlatır. Bu dosyada sır yoktur. Değerler sadece Dokploy'daki
compose ortam değişkenlerinde durur. Genel kurallar için `docs/DEVELOPMENT.md` ("Yayınlama") ve
`compose.prod.yml` dosyalarına bakın.

## Nerede

| Ne | Değer |
| --- | --- |
| Panel | `https://dokploy.getclickfix.com` (Dokploy v0.30) |
| Sunucu | Dokploy'un kendi sunucusu (uzak sunucu yok). DigitalOcean sabit IP `134.199.188.121`, droplet IP `164.90.185.156` |
| Proje / ortam | `ClickFix` / `production` |
| Servis | Compose `ClickFix`, Dokploy appName `clickfix-qjlddr` |
| Kaynak | GitHub sağlayıcısı "Dokploy Getclickfix V2", repo `ClickSolveMainTR/clickfix-app-go`, dal `main`, dosya `./compose.prod.yml` |
| Tetik | `tag` (otomatik deploy açık). CI'daki `release-tag` işi her yeşil `main` çalıştırmasında `v0.N` etiketi koyar ve Dokploy bu etiketle yeniden kurar |
| İmajlar | CI'daki `publish-images` işi her yeşil `main` çalıştırmasında `ghcr.io/clicksolvemaintr/clickfix-backend` ve `clickfix-frontend` imajlarını `:main` ve `:sha-<kısa>` etiketleriyle GHCR'a gönderir. Sunucuda derleme yoktur; Dokploy imajları çeker (`pull_policy: always`). GHCR kaydı Dokploy → Registry'de `ghcr.io`, `read:packages` yetkili token ile tanımlıdır |

Aynı sunucuda başka yığınlar da çalışıyor (Ledgerbot, YMG, Easytrade WhatsApp). Çakışma olmaması için tüm konteyner,
volume ve ağ adları `NAME_PREFIX=clickfix` ile başlar. Özel ağ adı `clickfix-app`'tir. İç adresler her zaman
`<servis>.clickfix-app` biçiminde nitelenir.

## Servisler

| Servis | İmaj | Ağ | Dışarı açık mı |
| --- | --- | --- | --- |
| `postgres` | `pgvector/pgvector:0.8.7-pg18-trixie` | app | hayır |
| `redis` | `redis:8-alpine` (AOF açık) | app | hayır |
| `seaweedfs` | `chrislusf/seaweedfs:4.47` (tek düğüm, S3 `:8333`) | app | hayır |
| `meilisearch` | `getmeili/meilisearch:v1.11` | app | hayır |
| `centrifugo` | `centrifugo/centrifugo:v5` | app + dokploy-network | `rt.getclickfix.com` → `:8000` |
| `migrate` | backend imajı, `APP_ROLE=migrate` | app | hayır (tek seferlik, backend ve worker bunu bekler) |
| `backend` | backend imajı, `APP_ROLE=server` | app (+ dokploy-network, domain için Dokploy ekler) | `api.getclickfix.com` → `:8080` |
| `worker` | backend imajı, `APP_ROLE=worker`; müşteri işleri ve tek scheduler | app | hayır |
| `worker-sync` | aynı backend imajı ve worker binary; yalnız `sync` kuyruğu | app | hayır |
| `ai-reply` | aynı backend imajı ve worker binary; yalnız `ai` kuyruğu (müşteriye AI cevabı, CF-251). `QUEUE_AI_REPLICAS` ile ölçeklenir | app | hayır |
| `frontend` | frontend imajı (Next.js BFF) | app + dokploy-network | `getclickfix.com`, `www`, `widget` → `:3000` |
| `livekit` | `livekit/livekit-server:v1.13.11` + `deploy/livekit/` yapılandırması (CF-139) | app + dokploy-network | `video.getclickfix.com` → `:7880` (wss). Medya portları host'ta yayınlanır, aşağıda |
| `livekit-egress` | `livekit/egress:v1.15.0` (kayıtlar) | app | hayır |

Migration'lar her deploy'da `migrate` servisinde çalışır. Backend, worker, worker-sync ve ai-reply ancak migration başarıyla bitince açılır.

## Domain'ler

Hepsi Dokploy'da compose domain'i olarak tanımlı. HTTPS açık, sertifika Let's Encrypt.

| Host | Servis | Port |
| --- | --- | --- |
| `getclickfix.com` | frontend | 3000 |
| `www.getclickfix.com` | frontend | 3000 |
| `widget.getclickfix.com` | frontend | 3000 |
| `api.getclickfix.com` | backend | 8080 |
| `rt.getclickfix.com` | centrifugo | 8000 |
| `video.getclickfix.com` | livekit | 7880 |

`api.getclickfix.com` Meta, Slack ve Teams webhook'ları ile `PUBLIC_API_URL` için gerekli
(`docs/meta-kurulum.md`, `docs/slack-teams-kurulum.md`). Panel ise `/api` üzerinden aynı origin'den gider.

## DNS

Alan adı DigitalOcean DNS'te (`ns1-3.digitalocean.com`). Tüm uygulama kayıtları sabit IP'ye gider.

| Tür | Ad | Değer |
| --- | --- | --- |
| A | `@` | `134.199.188.121` |
| A | `dokploy` | `134.199.188.121` |
| A | `www` | `134.199.188.121` |
| A | `api` | `134.199.188.121` |
| A | `widget` | `134.199.188.121` |
| A | `rt` | `134.199.188.121` |
| A | `video` | `134.199.188.121` (LiveKit sinyal, wss) |
| A | `turn` | `134.199.188.121` (LiveKit TURN; `LIVEKIT_TURN_DOMAIN` doluysa) |
| MX | `in` | Resend'in verdiği inbound MX (`EMAIL_INBOUND_DOMAIN=in.getclickfix.com`) |
| TXT / MX | `resend._domainkey`, `send` | Resend'in verdiği DKIM / SPF / bounce kayıtları |
| TXT | `_dmarc` | `v=DMARC1; p=none; rua=mailto:destek@getclickfix.com` |
| MX | `@` | `destek@` kutusunu barındıran posta sağlayıcısı |

Bir A kaydı sonradan eklenirse Let's Encrypt sertifikası için compose'u bir kez yeniden deploy edin.

## Ortam değişkenleri

Dokploy → ClickFix → Environment. Dokploy bunları compose'un yanına `.env` olarak yazar. `compose.prod.yml`
konteynerlere sadece kendi listelediği değişkenleri geçirir. Yeni bir değişken eklendiğinde compose dosyasına da
eklenmelidir.

### Kurulumda üretilenler (sır, rastgele)

`DB_PASSWORD`, `REDIS_PASSWORD`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `SEAWEEDFS_JWT_WRITE_KEY`,
`SEAWEEDFS_JWT_READ_KEY`, `CENTRIFUGO_API_KEY`, `CENTRIFUGO_TOKEN_HMAC_SECRET`, `JWT_ACCESS_SECRET`,
`JWT_REFRESH_SECRET`, `AUTH_ADAPTER_SECRET`, `AUTH_SECRET`, `APP_ENCRYPTION_KEY` (32 bayt, base64),
`MEILI_MASTER_KEY`, `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY` (P-256, base64url).

`DB_PASSWORD`, `S3_*` ve `MEILI_MASTER_KEY` ilk açılışta volume'lara yazılır. Sonradan değiştirmek veriyi
kilitler: önce servisin kendisinde değiştirin. `APP_ENCRYPTION_KEY` değişirse veritabanındaki şifreli kimlik
bilgileri (GitHub App, Apple .p8, kanal sırları) okunamaz.

### Sabit değerler

| Değişken | Değer |
| --- | --- |
| `NAME_PREFIX` | `clickfix` |
| `BACKEND_IMAGE` / `FRONTEND_IMAGE` | `ghcr.io/clicksolvemaintr/clickfix-backend:main` / `ghcr.io/clicksolvemaintr/clickfix-frontend:main`. Geri dönmek için `:sha-<kısa>` etiketine sabitlenir |
| `DB_USER` / `DB_NAME` / `DB_SSLMODE` | `clickfix` / `clickfix` / `disable` |
| `STORAGE_DRIVER` / `S3_BUCKET` / `S3_REGION` | `s3` / `clickfix` / `us-east-1` |
| `CENTRIFUGO_WS_URL` | `wss://rt.getclickfix.com/connection/websocket` |
| `CENTRIFUGO_ALLOWED_ORIGINS` | `https://getclickfix.com https://www.getclickfix.com https://widget.getclickfix.com` (boşlukla) |
| `CORS_ALLOWED_ORIGINS` | aynı üç origin (virgülle) |
| `AUTH_URL` / `PUBLIC_FRONTEND_URL` | `https://getclickfix.com` |
| `PUBLIC_API_URL` | `https://api.getclickfix.com` |
| `AUTH_TRUST_HOST` / `AUTH_WEBAUTHN_RP_ID` | `true` / `getclickfix.com` |
| `MEILI_INDEX_PREFIX` | `clickfix` |
| `VAPID_SUBJECT` | `mailto:destek@getclickfix.com` |
| `MAIL_DRIVER` / `SMTP_HOST` / `SMTP_PORT` / `SMTP_USERNAME` | `smtp` / `smtp.resend.com` / `587` / `resend` |
| `MAIL_FROM` / `MAIL_FROM_NAME` | `noreply@getclickfix.com` / `ClickFix` |
| `SMTP_TLS_MODE` / `SMTP_FALLBACK_PORT` | `starttls` / `0`. Sunucu 587'yi engelliyorsa (DigitalOcean 25/465/587'yi kapatır) `SMTP_PORT=2587` kullanın; yedek olarak `SMTP_FALLBACK_PORT=2465` (Resend implicit TLS) verilirse birincil port erişilemediğinde ileti bir kez implicit TLS ile o porttan denenir. Doğrudan SMTPS için `SMTP_TLS_MODE=implicit` + `SMTP_PORT=465` |
| `SMTP_DIAL_TIMEOUT` / `SMTP_SEND_TIMEOUT` | `10s` / `30s` (bağlantı ve ileti başına toplam süre; engelli port işçiyi dakikalarca bekletmez) |
| `TENANCY_ENABLED` | `true` (self-serve sign-up is the platform setting Giriş ve kayıt → Kayıt: açık / davetle / kapalı) |
| `QUEUE_ENABLED` / `QUEUE_CONCURRENCY` | `true` / `10` |
| `LOG_LEVEL` / `LOG_FORMAT` | `info` / `json` |
| `EMAIL_INBOUND_DOMAIN` / `RESEND_BASE_URL` | `in.getclickfix.com` / `https://api.resend.com` |
| `META_CHANNELS_ENABLED` / `META_GRAPH_VERSION` | `false` / `v21.0` |
| `TELEGRAM_CHANNEL_ENABLED` | `true` |
| `BILLING_INVOICE_SERIES` | `CFX` |
| `AUTH_QR_TRUST_GEO_HEADERS` | `false` |
| `SENTRY_ENVIRONMENT` / `SENTRY_TRACES_SAMPLE_RATE` | `production` / `0` |

### Hesap sahibinin dolduracakları (boş bırakıldı)

Tam liste, kaynak ekranları ve zorunluluk bilgisiyle: [ortam-degiskenleri.md](./ortam-degiskenleri.md).

| Değişken | Nereden |
| --- | --- |
| `SMTP_PASSWORD` | Resend API anahtarı (giden e-posta) |
| `RESEND_API_KEY`, `RESEND_WEBHOOK_SECRET` | Resend → API Keys / Webhooks (e-posta kanalı) |
| `META_APP_ID`, `META_APP_SECRET`, `META_WEBHOOK_VERIFY_TOKEN`, `META_WHATSAPP_CONFIG_ID`, `META_INSTAGRAM_APP_ID`, `META_INSTAGRAM_APP_SECRET` | `docs/meta-kurulum.md`. Webhook'lar doğrulanınca `META_CHANNELS_ENABLED=true` |
| `BILLING_SELLER_JSON` | Şirketin fatura kimliği. Tek satır JSON, dışına tırnak konmaz (örnek biçim `.env.example`'da) |
| `BILLING_EINVOICE_PROVIDER` | e-Arşiv/e-Fatura entegratörü (CF-16) |
| `BILLING_REFERRAL_CREDITS`, `PAYMENTS_IYZICO_STATEMENTS_ENABLED`, `PAYMENTS_PAYTR_WEBHOOK_CIDRS`, `PAYMENTS_IYZICO_WEBHOOK_CIDRS` | İsteğe bağlı |
| `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `OPENAI_BASE_URL`, `AI_EMBEDDING_MODEL` | AI sağlayıcıları |
| `AUTH_REVIEW_ACCOUNTS`, `AUTH_APPLE_NATIVE_CLIENT_IDS`, `AUTH_GOOGLE_NATIVE_CLIENT_IDS`, `AUTH_APPLE_TEAM_ID`, `AUTH_APPLE_KEY_ID`, `AUTH_APPLE_PRIVATE_KEY`, `EXPO_ACCESS_TOKEN` | Mobil giriş ve push (`backend/docs/auth.md`) |
| `BACKEND_SENTRY_DSN`, `FRONTEND_SENTRY_DSN` | Hata izleme (boş = kapalı) |
| `LIVEKIT_URL`, `LIVEKIT_API_KEY`, `LIVEKIT_API_SECRET`, `LIVEKIT_TURN_DOMAIN` | Görüntülü görüşme (CF-139). Anahtarlar kurulumda üretilir; ayrıntı [video-rooms.md](./video-rooms.md) |

PayTR / iyzico mağaza anahtarları ortam değişkenidir (`PAYMENTS_*`); API'den girilemez. Kurulum, webhook adresleri
ve canlıya geçiş: [odeme.md](./odeme.md).

`.env.example`'daki `DB_HOST`, `REDIS_ADDR`, `S3_ENDPOINT`, `MEILI_HOST`, `CENTRIFUGO_API_URL`, `API_URL`,
`*_PORT`, `MAILHOG_*`, `ADMINER_PORT`, `QUEUE_WORKER_INPROCESS`, `*_VOLUME_NAME`, `LOCAL_NETWORK_NAME` gibi
değişkenler sadece yerel geliştirme içindir. Üretimde `compose.prod.yml` bunları kendisi kurar.

## Deploy akışı

1. PR `main`'e birleşir, `ci` yeşil biter.
2. `publish-images` işi backend ve frontend imajlarını GHCR'a `:main` ve `:sha-<kısa>` olarak gönderir.
3. `release-tag` işi (imajlar gönderildikten sonra) `v0.N` etiketini koyar.
4. GitHub etiket olayı Dokploy'a gelir, Dokploy `main` dalını çekip `compose.prod.yml`'i yeniden kurar:
   imajlar GHCR'dan çekilir, `migrate` çalışır, ardından backend, worker, worker-sync ve frontend yenilenir.
5. Elle deploy: Dokploy → ClickFix → Deploy (veya Redeploy).
6. Geri dönüş: `BACKEND_IMAGE` / `FRONTEND_IMAGE` değerini önceki `:sha-<kısa>` etiketine çevirip Deploy.

Kontroller:

- `https://api.getclickfix.com/readyz` → 200 (Postgres, Redis ve diğer bağımlılıklar hazır)
- `https://getclickfix.com/health` → 200
- `wss://rt.getclickfix.com/connection/websocket` bağlantısı (panelde giriş yapınca gerçek zamanlı bildirimler)
- Dokploy → Logs → `migrate` konteyneri hatasız biter

## Süper admin

İlk kurulumdan sonra bir kez, Dokploy → ClickFix → Docker → `clickfix-backend` → Terminal:

```sh
/app/create-super-admin -email <e-posta> -password '<şifre>' -name <ad> -surname <soyad>
```

Bu komut imaja CF-135 düzeltme PR'ıyla girdi; daha eski bir imajda yoktur. Şifre en az 8 karakter olmalı ve büyük
harf, küçük harf ve rakam içermeli. E-posta ve şifreyi hesap sahibi girer. Şifre bu dosyaya, sohbete veya loga
yazılmaz. Meilisearch dizinlerini yeniden kurmak için aynı terminalde `/app/search-reindex` çalıştırılır.

## Yedekler

Hedef: Dokploy destination "DigitalOcean" (Spaces, bucket `clickfix-server-backups`).

| Ne | Yöntem | Zaman (UTC) | Saklama | Önek |
| --- | --- | --- | --- | --- |
| Postgres (`clickfix` veritabanı) | Dokploy compose veritabanı yedeği (`pg_dump`) | her gün 02:00 | son 14 | `clickfix/postgres` |
| SeaweedFS (`clickfix-prod-seaweedfs` volume) | Dokploy volume yedeği, servis durdurulmadan | her gün 02:30 | son 7 | `clickfix/seaweedfs` |

Redis (kuyruk ve önbellek, AOF ile kalıcı) ve Meilisearch (veritabanından yeniden kurulabilir) yedeklenmez.
Meilisearch kaybolursa backend konteynerinde `/app/search-reindex` ile dizinler yeniden kurulur.
Felaket kurtarma ve yedek geri yükleme denemesi CF-129'da.

## Geri dönüş

- **Kod**: Dokploy → ClickFix → Deployments listesinden önceki başarılı deploy'u yeniden çalıştırın veya
  `main`'i istenen etikete döndüren bir PR birleştirin. Migration'lar yalnız ileri gider; geri alınması gereken bir
  şema değişikliği yeni bir migration ile düzeltilir.
- **Veri**: Postgres için Dokploy → Backups → ilgili dosya → Restore. SeaweedFS için volume yedeğini aynı adlı volume'a
  geri yükleyin (önce `seaweedfs`, `backend`, `worker` ve `worker-sync` durdurulur).
- **Ortam değişkeni**: Dokploy env geçmişi tutmaz. Değiştirmeden önce mevcut değeri güvenli bir yere alın.

## Görüntülü görüşme (LiveKit, CF-139)

Kurulum, portlar, TURN ve şifreleme ayrıntısı: [video-rooms.md](./video-rooms.md). Kısaca:

1. Anahtarları üretin: `LIVEKIT_API_KEY=API$(openssl rand -hex 6)`, `LIVEKIT_API_SECRET=$(openssl rand -base64 36)`
   (en az 32 karakter) ve `LIVEKIT_URL=wss://video.getclickfix.com` girin.
2. Dokploy → ClickFix → Domains: `video.getclickfix.com` → servis `livekit`, port `7880`, HTTPS açık.
3. Sunucu güvenlik duvarında (DigitalOcean Cloud Firewall + `ufw`) açın: `7881/tcp`, `50000-50100/udp`,
   `3478/udp`, `50200-50300/udp`. Bu portlar Traefik'ten geçmez; doğrudan konteynere gider.
4. İsteğe bağlı TURN/TLS: `LIVEKIT_TURN_DOMAIN=turn.getclickfix.com` ve Traefik'te `turn.getclickfix.com` için
   TLS sonlandıran bir TCP router (`HostSNI`) → `livekit:5349` (video-rooms.md, "TURN").
5. Platform paneli → Görüntülü görüşme: modülü açın, limitleri ve planlara özel kuralları girin; Faturalama →
   kredi paketleri ekranında `video_minutes` birimiyle dakika paketleri tanımlayın.

Anahtarlar boşken `livekit` ve `livekit-egress` konteynerleri boşta bekler (deploy'u bozmaz) ve panelde modül
"yapılandırılmamış" görünür. `livekit-egress` kayıt sırasında yaklaşık 2 CPU ve 2-4 GB bellek kullanır
(`LIVEKIT_EGRESS_MEM_LIMIT`, varsayılan `2g`; 8 GB sunucuda daha yükseği diğer servisleri bellek dışına iter).

## Bilinen notlar

- `backend` servisi domain aldığı için Dokploy onu `dokploy-network`'e de bağlar. Bu ağda `backend` adı başka bir
  yığının nitelenmemiş `backend` aramasına cevap verebilir. Sunucudaki diğer yığınlar nitelenmiş adlar kullandığı
  için şu an sorun yok. Yeni bir yığın eklenirken buna dikkat edin.

### Müşteriye AI cevabı worker'ı (CF-251)

`ai-reply` yalnız `ai` kuyruğunu tüketir: gelen müşteri mesajını cevaplayan
`ai.conversation` tüketicisi ve onun gönderdiği mesajların teslimi. Başka hiçbir
arka plan işi bu serviste çalışmaz; dışa aktarma, senkronizasyon, webhook ya da
uzun bir backfill ana `worker`'ı doldursa bile cevap beklemez. Ana `worker` da
`ai` kuyruğunu yedek olarak dinler (`ai-reply` kapalıysa cevaplar durmaz).

- Eşzamanlılık: `QUEUE_AI_CONCURRENCY` (varsayılan 32). Yük artınca önce
  `QUEUE_AI_REPLICAS` yükseltilir; servis durum tutmaz ve `container_name` taşımaz.
- Her iş çalışırken bir veritabanı bağlantısı tutar: `QUEUE_AI_DB_MAX_CONNS`
  eşzamanlılığın üstünde kalmalı ve Postgres `max_connections` bütçesine sığmalı.
- Organizasyon başına eşzamanlı AI çağrısı ayrıca `ai.concurrency` plan hakkıyla
  sınırlıdır (varsayılan 3). Sınır doluyken cevap 5 sn'ye kadar boş yer bekler;
  yoğun müşterilerde bu hakkı yükseltin.
- Ölçüm: her cevap işi `ai_reply_latency` log satırı yazar
  (`received_to_reply_ms` = mesajın kaydı → cevabın gönderimi; `queue_wait_ms`,
  `triage_ms`, `prepare_ms`, `model_ms`, `outcome`). Hedef p50 < 5 sn, p90 < 10 sn.

### Sağlayıcı polling worker'ı

`worker-sync`, IMAP tick/poll, Meta mesaj uzlaştırma, kanal sağlık/token yenileme,
bilgi kaynağı tarama, Expo alındıları, ödeme/iyzico ekstre uzlaştırması ve
GitHub/Jira/Linear senkronizasyonunu tüketir. Ana `worker`, mesaj yanıtları,
e-posta/push, webhook teslimi, engineering müşteri takip mesajları, AI işleri,
yenileme tahsilatı ve mevcut yerel bakım işlerini tüketmeye devam eder.

Üretimde ana worker'ın varsayılan `QUEUE_QUEUES` listesi `sync` içermez;
`worker-sync` daima `QUEUE_QUEUES=sync` ve `QUEUE_SCHEDULER_ENABLED=false` kullanır.
Scheduler ana worker'da tüm kuyruklar için görev üretir. Scheduler etkin olan
servisten **yalnız bir instance** çalıştırın; ek ana worker replikalarında da
`QUEUE_SCHEDULER_ENABLED=false` ayarlayın. Backend'in in-process worker'ı üretimde kapalıdır.

Önce `QUEUE_SYNC_CONCURRENCY` (varsayılan 4), gerekirse `worker-sync` replika
sayısını artırın: `docker compose -f compose.prod.yml up -d --scale worker-sync=2`.
Servis sabit `container_name` kullanmaz; her replikanın bellek sınırı 512 MiB,
shutdown süresi 35 saniyedir. Scheduler'ı bu replikalarda etkinleştirmeyin.
`clickfix_asynq_queue_depth{queue="sync"}` backlog'u gösterir;
`ClickFixSyncQueueBacklog` 10 dakika boyunca 100 üstünde bekleyen/yeniden denenen
poll için uyarır. Sağlayıcı hatalarını incelemeden sınırsız ölçeklemeyin.

Geçişte eski kuyruklara yazılmış polling görevleri taşınmaz; eski kuyruklarını
tüketen ana worker bunları bir kez tamamlar. Yeni kayıtlar `sync` kuyruğuna gider.
Yerelde boş `QUEUE_QUEUES`, mevcut kuyrukları ve `sync` kuyruğunu birlikte tüketir;
`QUEUE_WORKER_INPROCESS=true` aynı periyodik kayıtları başlatır.
