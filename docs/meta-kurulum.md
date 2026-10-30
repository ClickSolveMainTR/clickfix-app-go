# Meta (WhatsApp / Messenger / Instagram) kurulumu

CF-2 için Meta tarafında yapılan kurulumun kaydı. Sırlar (App Secret, Instagram App Secret, verify token,
erişim token'ları) bu dosyada **yoktur**; sadece Dokploy ortam değişkenlerinde tutulur.

Son güncelleme: 28 Eki 2026.

## Kimlikler

| Ne | Değer | Ortam değişkeni |
|----|-------|-----------------|
| İşletme portfolyosu | ClickFix, `1359252679521888` (doğrulanmamış) | — |
| Meta uygulaması | ClickFix, App ID `1117894644223620` (Business, geliştirme modunda) | `META_APP_ID` |
| App Secret | Uygulama → App settings → Temel → App secret → Show | `META_APP_SECRET` |
| Instagram uygulaması | ClickFix-IG, Instagram app ID `865817129922912` | `META_INSTAGRAM_APP_ID` |
| Instagram App Secret | Kullanım durumları → Instagram API → API setup with Instagram login → Show | `META_INSTAGRAM_APP_SECRET` |
| WhatsApp Embedded Signup config | `942776138660676` (şablon: WhatsApp Embedded Signup, sistem kullanıcısı token'ı, 60 gün) | `META_WHATSAPP_CONFIG_ID` |
| Webhook verify token | Rastgele üretildi; Dokploy'a girilecek, Meta webhook formlarına aynısı yazılacak | `META_WEBHOOK_VERIFY_TOKEN` |
| Graph sürümü | `v21.0` (`.env.example` varsayılanı) | `META_GRAPH_VERSION` |
| Kanallar açık | Webhook'lar doğrulanınca `true` | `META_CHANNELS_ENABLED` |

## Yapılanlar

- **Temel ayarlar**: görünen ad ClickFix, iletişim e-postası, ikon (logo kitindeki yeşil tile), kategori Araçlar ve
  Verimlilik, App domain `getclickfix.com`, gizlilik `https://getclickfix.com/legal/privacy`, koşullar
  `https://getclickfix.com/legal/terms`, Website platformu `https://getclickfix.com/`.
- **Kullanım durumları**: Messenger from Meta, Instagram API, Connect on WhatsApp. Facebook Login for Business
  otomatik eklendi.
- **Messenger izinleri** (test için hazır): `pages_messaging`, `pages_manage_metadata`, `pages_show_list`,
  `pages_read_engagement`, Human Agent, Business Asset User Profile Access, `business_management`.
- **Instagram izinleri** (test için hazır): `instagram_business_basic`, `instagram_business_manage_messages`,
  `instagram_business_manage_comments` (+ Human Agent, Business Asset User Profile Access).
- **WhatsApp izinleri** (test için hazır): `whatsapp_business_management`, `whatsapp_business_messaging`,
  `business_management`.
- **Facebook Login for Business**: geçerli OAuth redirect `https://getclickfix.com/channels/connect/messenger`,
  JavaScript SDK ile giriş açık, izinli alan `https://getclickfix.com/`.
- **Instagram business login**: redirect `https://getclickfix.com/channels/connect/instagram`.

Redirect adresleri koddan geliyor: `backend/internal/inbox/inbox.go` →
`PUBLIC_FRONTEND_URL + /channels/connect/{messenger|instagram}`.

## Kalanlar

1. **Veri silme URL'si**: `https://getclickfix.com/account-deletion` (talimat sayfası, TR/EN). Meta adresi kaydederken
   doğruluyor; sayfa yayına girince Temel ayarlar → User data deletion alanına eklenmeli. Meta'nın `signed_request`
   veri silme callback'i ayrı bir backend işi.
2. **Webhook'lar**: Dokploy'da backend, worker ve worker-sync için `META_*` değişkenleri ve
   `META_CHANNELS_ENABLED=true` girilip yeniden deploy edilir; `GET /v1/webhooks/channels/whatsapp` 404 yerine
   doğrulama yanıtı (yanlış token için 403) dönünce aynı verify token ile:
   - WhatsApp: `https://api.getclickfix.com/v1/webhooks/channels/whatsapp`, alan: `messages`
     (Connect on WhatsApp → Configuration).
   - Messenger: `https://api.getclickfix.com/v1/webhooks/channels/messenger`, alanlar: `messages`,
     `messaging_postbacks`, `message_deliveries`, `message_reads`, `message_echoes`.
   - Instagram: `https://api.getclickfix.com/v1/webhooks/channels/instagram`, alanlar: `messages`,
     `messaging_postbacks`, `messaging_seen`, `message_reactions`, `comments`, `mentions`.
   Sayfa/hesap abonelikleri (`subscribed_apps`) bağlantı sırasında backend tarafından yapılır.
3. **İşletme doğrulaması** (ClickFix portfolyosu, `1359252679521888`): resmi belgeler gerektirir, kullanıcı yapar.
   Advanced Access ve Tech Provider için şart.
4. **Tech Provider**: Connect on WhatsApp → Become a Partner → Become Tech Provider (doğrulamadan sonra).
   Tech Provider olunca süresi dolmayan token'lı yeni bir Embedded Signup yapılandırması oluşturulup
   `META_WHATSAPP_CONFIG_ID` güncellenmeli. Şu anki yapılandırmanın token'ı 60 günde bitiyor ve backend
   bu token'ı yenilemiyor.
5. **App Review** (kullanıcının onayı olmadan gönderilmez); aşağıdaki liste hazır.
6. Uygulamayı **Live** moda almak (gizlilik URL'si erişilebilir olmalı; Instagram webhook'ları sadece
   yayındaki uygulamaya gelir).

## App Review: istenecek izinler

İngilizce form metinleri ve video senaryoları: [meta-review/](./meta-review/README.md). Business Asset User Profile
Access kullanım durumuna eklendi ama kod kullanmıyor (gönderen adı/resmi okunmuyor); review'da **istenmemeli**.

| İzin | Gerekçe (tek satır) |
|------|---------------------|
| `pages_messaging` | İşletmenin Facebook Sayfası'na gelen Messenger mesajlarını ClickFix gelen kutusunda yanıtlamak. |
| `pages_manage_metadata` | Sayfayı uygulamanın webhook'una abone etmek (`subscribed_apps`). |
| `pages_show_list` | Bağlantı sırasında kullanıcının yönettiği Sayfaları listeleyip seçtirmek. |
| `pages_read_engagement` | Sayfa adını/kimliğini okumak; kaçan webhook'ları Sayfa konuşmalarından tamamlamak. |
| Human Agent | 24 saat penceresi dışında temsilcinin 7 gün içinde yanıt verebilmesi (`HUMAN_AGENT` etiketi). |
| `instagram_business_basic` | Bağlanan Instagram profesyonel hesabının kimliğini ve kullanıcı adını okumak. |
| `instagram_business_manage_messages` | Instagram DM'lerini almak ve yanıtlamak. |
| `instagram_business_manage_comments` | Gönderi yorumlarını ve bahsetmeleri konuşma olarak alıp yanıtlamak. |
| `whatsapp_business_management` | Embedded Signup ile bağlanan WABA'yı, numarayı ve şablonları yönetmek, webhook aboneliği. |
| `whatsapp_business_messaging` | Müşteriyle WhatsApp üzerinden mesajlaşmak (metin, medya, şablon). |
| `business_management` | Embedded Signup sırasında müşterinin işletme varlıklarına erişim (WABA paylaşımı). |

### Gereken ekran kayıtları

Her biri İngilizce arayüz veya altyazıyla, giriş adımından başlayarak:

1. **Messenger**: ClickFix'e giriş → Kanallar → Messenger bağla → Facebook ile giriş ve izin ekranı → Sayfa seçimi →
   test kullanıcısının Sayfa'ya yazdığı mesajın gelen kutusuna düşmesi → temsilcinin yanıtı ve Messenger'da görünmesi
   → 24 saat dışı yanıt (backend otomatik HUMAN_AGENT etiketiyle yeniden gönderir).
2. **Instagram**: Instagram ile giriş ve izin ekranı → DM'in gelen kutusuna düşmesi ve yanıt → bir gönderi yorumunun
   konuşma olarak gelmesi ve herkese açık yanıt.
3. **WhatsApp**: Embedded Signup (FB.login penceresi, WABA ve numara seçimi) → kanalın bağlanması → gelen WhatsApp
   mesajı ve yanıtı → şablon listesi ve yeni şablonun onaya gönderilmesi.

Panelde henüz olmayanlar (videoda gösterilemez): Instagram yorumuna özel (DM) yanıt seçimi, temsilcinin Human Agent'ı
elle seçmesi, konuşmadan şablon mesaj gönderme. Review'dan önce eklenirse senaryolara bir adım eklenir.

## Veri silme ve deauthorize callback'leri (CF-2 / CF-127)

Meta App Dashboard'a aşağıdaki **POST** URL'lerini kaydedin (kanallar beta kapalıyken de erişilebilir):

| Uygulama | Data Deletion Request URL | Deauthorize Callback URL |
| --- | --- | --- |
| Facebook / Messenger / WhatsApp | `https://getclickfix.com/api/v1/webhooks/meta/facebook/data-deletion` | `https://getclickfix.com/api/v1/webhooks/meta/facebook/deauthorize` |
| Instagram Login uygulaması | `https://getclickfix.com/api/v1/webhooks/meta/instagram/data-deletion` | `https://getclickfix.com/api/v1/webhooks/meta/instagram/deauthorize` |

BFF `/api/v1/*` yolunu Go `/v1/*` yoluna iletir. `application/x-www-form-urlencoded` içindeki `signed_request`, orijinal base64url payload üzerinde HMAC-SHA256 ile doğrulanır; Facebook için `META_APP_SECRET`, Instagram için **yalnızca** `META_INSTAGRAM_APP_SECRET` kullanılır. Yanlış/eksik imza 400, yapılandırılmamış secret veya geçici veritabanı hatası 503 döndürür. Ham payload, token ve sağlayıcı kullanıcı kimliği audit'e yazılmaz.

Deauthorize, doğrulanmış kullanıcıya `external_id` veya OAuth sırasında sunucunun yazdığı `settings.meta_user_id` ile bağlı instance'ları kapatır ve şifreli tokenlarını siler. Messenger OAuth'ta Graph `/me` uygulama kapsamlı kimliği, WhatsApp Embedded Signup'ta `debug_token` ile doğrulanan kullanıcı kimliği, Instagram'da hesabın kimliği saklanır. Eski, bu bağı bulunmayan Messenger bağlantıları bir kez yeniden bağlanmalıdır. Manuel bağlantılar için yalnızca instance dış kimliği eşleşmesi kullanılabilir.

Veri silme callback'i ayrıca aynı uygulamanın page-scoped / IG-scoped `contact_identities.external_id` eşleşmelerini tüm organizasyonlarda bulur. Her biri için CF-127 `data_subject_requests` silme kaydı oluşturur; konuşma, ek, AI kaydı ve kişi verisi mevcut compliance erasure işleminden geçer. İş `maintenance` kuyruğundadır; Redis hatasında kalıcı kayıtlar günlük recovery sweep ile işlenir. Bu silme, hesap silmedeki 7 günlük beklemeye tabi değildir.

Yanıt Meta'nın istediği üst seviye JSON'dur (API envelope kullanılmaz):

```json
{"url":"https://getclickfix.com/account-deletion?code=<code>","confirmation_code":"<code>"}
```

Bilinmeyen kişiler de aynı biçimde gerçek ve rastgele bir onay kodu alır. Durum sorgusu: `GET https://getclickfix.com/api/v1/public/meta/data-deletion/<code>` → API envelope içinde `confirmation_code` ve `status` (`pending` / `completed`). Kişi kimliği, organizasyon ve eşleşme sayısı açıklanmaz. Durum sayfası bu endpoint'i kullanmalıdır. Kod, durum kaydına erişim anahtarıdır; cache edilmez. Tamamlama ve tekrar denemeler idempotenttir.

Meta sözleşmesi: https://developers.facebook.com/docs/development/create-an-app/app-dashboard/data-deletion-callback/
