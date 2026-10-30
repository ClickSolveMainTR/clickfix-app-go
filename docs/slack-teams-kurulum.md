# Slack Connect ve Microsoft Teams kurulumu

CF-119 (Slack) ve CF-120 (Teams) için sağlayıcı tarafında yapılan kurulumun kaydı. Sırlar (Slack Client Secret,
Signing Secret, bot token'ları, Teams client secret) bu dosyada **yoktur**; Slack sırları sadece Dokploy ortam
değişkenlerinde, Teams sırları ise ClickFix panelinde kanalın şifreli kimlik bilgilerinde tutulur.

Kod: `backend/internal/platform/channels/{slack,teams}` (PR #99, `cf-119-channels`), tasarım notu
`docs/design/channels-g4.md`. Alan adları `docs/meta-kurulum.md` ile aynı: panel `https://getclickfix.com`,
API `https://api.getclickfix.com`.

Son güncelleme: 29 Eki 2026.

## Slack (CF-119)

Tek bir ClickFix Slack uygulaması vardır; her müşteri organizasyonu kendi workspace'ine OAuth ile kurar. Bot
token'ı organizasyon kasasında şifreli saklanır. Uygulamanın tek bir Events URL'si vardır, backend olayı
`team_id` ile doğru kanala yönlendirir.

### Kimlikler

| Ne | Değer / nerede | Ortam değişkeni |
|----|----------------|-----------------|
| Slack workspace (test) | ClickFix, Team ID `T0CCXPXRMJ6` | — |
| Slack uygulaması | ClickFix, App ID `A0CCRQ1J8KV` | — |
| Client ID | `12439813871618.12433817620675` (Basic Information → App Credentials) | `SLACK_CHANNEL_CLIENT_ID` |
| Client Secret | Aynı ekran → Client Secret → Show (sır) | `SLACK_CHANNEL_CLIENT_SECRET` |
| Signing Secret | Aynı ekran → Signing Secret → Show (sır) | `SLACK_CHANNEL_SIGNING_SECRET` |
| OAuth redirect | `https://getclickfix.com/channels/connect/slack` | `SLACK_CHANNEL_REDIRECT_URL` |

Dört değişkenin hepsi dolu değilse Slack OAuth kapalıdır (`OAuthEnabled`).

### Uygulama ayarları

Koddan gelen değerler (`slack.go` → `LoginURL`, `ParseWebhook`):

- **Bot token scope'ları**: `chat:write`, `channels:history`, `groups:history`, `reactions:read`,
  `channels:read`, `groups:read`, `users:read`. `channels:read` ve `groups:read` iki iş görür: panelde botun eklendiği
  kanalları listelemek (`GET /v1/tenant/channels/{uuid}/slack/conversations`) ve gelen mesajda sohbet adını
  `#kanal` olarak almak (`conversations.info`). `users:read` mesajı yazanın adını alır (`users.info`, mesajda
  `author_name`). Adlar 24 saat önbellekte tutulur (Redis varsa süreçler arasında paylaşılır; anahtar organizasyon + workspace kapsamlı). Okunamazsa mesaj yine alınır, ad boş kalır ve aynı kimlik 5 dakika sorulmaz. Başka scope istenmez;
  OAuth isteği tam bu listeyi gönderir.
- **Redirect URL**: `https://getclickfix.com/channels/connect/slack` (OAuth & Permissions → Redirect URLs).
- **Event Subscriptions**: Request URL `https://api.getclickfix.com/v1/webhooks/channels/slack`
  (tüm workspace'ler için tek adres). Bot olayları: `message.channels`, `message.groups`, `reaction_added`.
- **App Home / Bot**: bot kullanıcısı açık, görünen ad ClickFix.

Manifest (Basic Information → App Manifest ya da "Create New App → From a manifest"):

```yaml
display_information:
  name: ClickFix
  description: Slack Connect kanallarındaki müşteri mesajlarını ClickFix gelen kutusuna taşır.
  background_color: "#0f766e"
features:
  bot_user:
    display_name: ClickFix
    always_online: true
oauth_config:
  redirect_urls:
    - https://getclickfix.com/channels/connect/slack
  scopes:
    bot:
      - chat:write
      - channels:history
      - groups:history
      - reactions:read
      - channels:read
      - groups:read
      - users:read
settings:
  event_subscriptions:
    request_url: https://api.getclickfix.com/v1/webhooks/channels/slack
    bot_events:
      - message.channels
      - message.groups
      - reaction_added
  org_deploy_enabled: false
  socket_mode_enabled: false
  token_rotation_enabled: false
```

Token rotation kapalı kalmalı: backend refresh token'ı saklamıyor ve yenilemiyor.

### Çalışma şekli (kısa)

- Bağlantı: panel `GET /v1/tenant/channels/oauth/slack/start` ile Slack onay ekranına gider, Slack
  redirect'e `code` ve `state` döner, panel bunları `POST /v1/tenant/channels/oauth/slack/complete`'e gönderir.
- Kanal seçimi: kanalın `settings.channel_ids` listesi boşsa hiçbir mesaj alınmaz. Bot, mesajı alınacak her
  Slack Connect kanalına `/invite @ClickFix` ile eklenmelidir.
- `:ticket:` tepkisi aynı konuşmaya "🎫 Destek talebi" olarak düşer. Yanıtlar her zaman ilgili thread'e gider.

## Microsoft Teams (CF-120)

Teams kanalı organizasyon başınadır ve **single-tenant**'tır: her müşteri kendi Entra tenant'ında bir app
registration + Azure Bot açar, ClickFix'te `teams` kanalı oluştururken `app_id`, `app_password`, `tenant_id`
girer. Sunucu tarafında Teams için ortam değişkeni yoktur. Aşağıdaki kurulum ClickFix'in kendi test
tenant'ındaki örnek kurulumdur ve müşteri yönergesine de temel olur.

### Kimlikler

| Ne | Değer / nerede | ClickFix kanal alanı |
|----|----------------|----------------------|
| Entra tenant | Directory (tenant) ID `<doldurulacak>` | `tenant_id` |
| App registration | ClickFix Bot, Application (client) ID `<doldurulacak>` | `app_id` |
| Client secret | Entra → App registrations → ClickFix Bot → Certificates & secrets → New client secret → **Value** (sadece bir kez görünür; sır) | `app_password` |
| Azure Bot kaynağı | `<doldurulacak>` (F0, ücretsiz katman) | — |

### Ayarlar

- **App registration**: Supported account types = *Accounts in this organizational directory only*
  (single-tenant). Redirect URI gerekmez. Backend token'ı
  `https://login.microsoftonline.com/{tenant_id}/oauth2/v2.0/token` adresinden, scope
  `https://api.botframework.com/.default` ile alır.
- **Azure Bot**: Type of App = *Single Tenant*, mevcut app registration kullanılır (aynı App ID ve tenant).
  Pricing tier **F0** (Teams gibi standart kanallar ücretsiz).
- **Messaging endpoint**: `https://api.getclickfix.com/v1/webhooks/channels/teams/{kanal-uuid}`.
  `{kanal-uuid}` ClickFix'te `teams` kanalı oluşturulunca belli olur (kanal detayındaki `webhook_path`).
  Bu yüzden endpoint kanal oluşturulduktan sonra girilir.
- **Kanallar**: Azure Bot → Channels → Microsoft Teams eklenir (Commercial cloud).
- **Teams uygulama paketi**: botu Teams'e kurmak için manifest gerekir (aşağıda). Teams Developer Portal
  (dev.teams.microsoft.com) → Apps → New app → App features → Bot → mevcut bot ID ile oluşturulabilir, ya da
  manifest + iki ikon zip'lenip Teams → Apps → Manage your apps → Upload a custom app ile yüklenir
  (tenant'ta özel uygulama yükleme açık olmalı).

```json
{
  "$schema": "https://developer.microsoft.com/json-schemas/teams/v1.17/MicrosoftTeams.schema.json",
  "manifestVersion": "1.17",
  "version": "1.0.0",
  "id": "<app_id>",
  "developer": {
    "name": "ClickFix",
    "websiteUrl": "https://getclickfix.com",
    "privacyUrl": "https://getclickfix.com/legal/privacy",
    "termsOfUseUrl": "https://getclickfix.com/legal/terms"
  },
  "name": { "short": "ClickFix", "full": "ClickFix Destek" },
  "description": {
    "short": "Teams mesajlarını ClickFix gelen kutusuna taşır.",
    "full": "Teams'te bota yazılan mesajlar ve kanal yanıtları ClickFix gelen kutusuna düşer; temsilci yanıtları aynı sohbete döner."
  },
  "icons": { "color": "color.png", "outline": "outline.png" },
  "accentColor": "#0F766E",
  "bots": [
    { "botId": "<app_id>", "scopes": ["personal", "team", "groupChat"], "supportsFiles": false, "isNotificationOnly": false }
  ],
  "validDomains": ["api.getclickfix.com"]
}
```

Bu dilimde sadece metin desteklenir (kart, dosya, düzenleme ve tepki yok), manifest buna göre dosya desteğini
kapatır.

## Yapılanlar

- **Slack (28 Eki)**: ClickFix workspace'inde (`T0CCXPXRMJ6`) "ClickFix" uygulaması oluşturuldu (`A0CCRQ1J8KV`).
  App Manifest ekranından yukarıdaki manifest kaydedildi; görünen ad, açıklama, bot kullanıcısı, redirect URL ve
  dört bot scope'u ayarlı. Token rotation, Socket Mode ve org deploy kapalı.
- **Slack (29 Eki)**: Dokploy'a `SLACK_CHANNEL_*` girildi ve API yayına alındı. Event Subscriptions açıldı. Request URL
  `https://api.getclickfix.com/v1/webhooks/channels/slack` Slack'te **Verified** oldu; bu, Signing Secret'ın
  doğru girildiğini gösteriyor. Bot olayları eklendi: `message.channels`, `message.groups`, `reaction_added`.
  App Manifest'e `channels:read`, `groups:read` ve (sohbet/yazar adları için) `users:read` eklendi; uygulamada şu an
  `incoming-webhook` scope'u da var.
  Kod bu scope'u istemiyor, kaldırılabilir.
- **Teams**: henüz başlanmadı (Microsoft 365 tenant ve Azure aboneliği bekleniyor).

## Kalanlar

1. **Slack'i ClickFix panelinden bağla**: Slack'in "Install App" düğmesiyle yapılan kurulum ClickFix'e token
   vermez. Bağlantı ClickFix → Kanallar → Slack bağla akışıyla yapılmalı
   (`/channels/connect/slack` geri dönüşü). Ardından kanal seçiminde mesajları alınacak kanallar işaretlenir ve
   bot o kanallara `/invite @ClickFix` ile eklenir. Uçtan uca test: seçili kanala yazılan mesajın gelen kutusuna
   düşmesi ve yanıtın aynı thread'e dönmesi.
2. **Scope değişikliği sonrası yeniden bağlantı**: `users:read` 29 Eki'de eklendi. Daha önce bağlanmış workspace'lerin
   ClickFix → Kanallar → Slack bağla akışını bir kez daha onaylaması gerekiyor; o zamana kadar yazar adı boş
   kalır, sohbet adı yine gelir.
3. **Slack public distribution**: başka workspace'lerin kurabilmesi için Manage Distribution → Activate
   Public Distribution (HTTPS redirect şart). Slack Marketplace onayı ayrı bir süreçtir, iddia edilmez.
4. **Teams messaging endpoint**: ClickFix'te test organizasyonunda `teams` kanalı oluşturulunca webhook UUID'si
   Azure Bot → Configuration → Messaging endpoint'e girilir; Teams'ten bota yazılıp gelen kutusunda görülür.
5. **Slack uygulama ikonu**: Basic Information → Display Information → App icon (512–2000 px kare PNG);
   logo kitindeki yeşil tile kullanılabilir.
6. **Teams uygulama paketi**: ikonlar (192×192 renkli, 32×32 şeffaf kontur) eklenip yüklenir. Mağaza / Partner
   Center yayını hesap sahibine aittir.
