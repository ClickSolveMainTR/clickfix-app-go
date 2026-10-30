# Mobil widget'lar, Live Activities, App Intents (CF-152)

ClickFix mobil uygulamasının işletim sistemi yüzeyleri: ana ekran ve kilit
ekranı widget'ları, Live Activities / Dynamic Island, Siri / Kestirmeler /
Spotlight ve iOS 18 Denetim Merkezi kontrolleri. Android'de Gelen kutusu ve
Durum widget'ları.

## Neyle yapıldı

| Yüzey | Teknoloji | Yer |
|---|---|---|
| WidgetKit widget'ları, kilit ekranı aksesuarları, Denetim Merkezi kontrolleri, Live Activities + Dynamic Island | `@bacons/apple-targets` ile tek SwiftUI uzantı hedefi | `mobile/targets/widgets/` |
| App Intents, App Shortcuts (Siri), Spotlight varlıkları | aynı hedefin `_shared/` klasörü (ana uygulamaya da derlenir) | `mobile/targets/widgets/_shared/` |
| App Group / paylaşılan Keychain / ActivityKit / Spotlight köprüsü | yerel Expo modülü | `mobile/modules/clickfix-widget-bridge/` |
| Uygulama tarafı (snapshot, token, Live Activity yönetimi, ayar) | TypeScript | `mobile/src/features/widgets/` |
| Ana hedef ayarları (entitlement, Info.plist, `CLICKFIX_APP`) | config plugin | `mobile/plugins/with-clickfix-surfaces.js` |
| Android widget'ları | `react-native-android-widget` | `mobile/src/features/widgets/android/` |

Hepsi config plugin'lerle üretilir; `npx expo prebuild --clean` aynısını
yeniden kurar. `ios/` ve `android/` commit edilmez.

## Kimlikler (bundle id'den türetilir)

Ana bundle id `app.json` `ios.bundleIdentifier` (`com.getclickfix.app`).

| Ne | Değer |
|---|---|
| App Group | `group.<bundleId>` → `group.com.getclickfix.app` |
| Widget uzantısı | `<bundleId>.widgets` |
| Paylaşılan Keychain grubu | `$(AppIdentifierPrefix)<bundleId>.shared` |
| Info.plist anahtarları | `ClickFixAppIdentifierPrefix`, `ClickFixAppGroup`, `ClickFixKeychainGroup` |
| Swift derleme koşulu | `CLICKFIX_APP` (sadece ana uygulama; App Shortcuts sağlayıcısı) |

Ana uygulamanın `keychain-access-groups` listesinde **ilk** sıra uygulamanın
kendi grubu (`$(AppIdentifierPrefix)<bundleId>`) olur; böylece refresh token
dahil uygulamanın kendi Keychain kayıtları uzantıya açılmaz. Sadece widget
token'ı açıkça paylaşılan gruba yazılır.

## Paylaşılan veri (App Group `UserDefaults`)

Anahtar `clickfix.widget.snapshot.v1`, değer JSON metni (UTF-8). Uygulama
giriş sonrası, ön plana gelişte, realtime/push olaylarında (gelen mesaj,
atama, durum, randevu) yazar ve `WidgetCenter.reloadAllTimelines()` +
`ControlCenter.reloadAllControls()` çağırır.

```jsonc
{
  "v": 1,
  "generated_at": "2026-10-30T09:12:00Z",
  // ready | signed_out | no_org | no_permission
  "state": "ready",
  "locale": "tr",                     // tr | en (uygulama dili)
  "org": { "slug": "acme", "name": "Acme" },   // ready dışında null olabilir
  "user": { "name": "Ayşe Yılmaz" },
  "can": { "inbox": true, "appointments": true, "presence": true },
  "presence": "online",               // online | away | busy | offline | null
  "inbox": {                          // can.inbox false ise null
    "unread": 4,                      // okunmamış mesajı olan açık konuşma
    "mine": 7,                        // bana atanmış açık konuşma
    "open": 23,
    "breached": 1,
    "top": [                          // en fazla 5, son mesaja göre
      {
        "uuid": "…", "contact": "Mehmet Kaya", "channel": "whatsapp",
        "preview": "Siparişim ne zaman gelir?", "at": "2026-10-30T09:10:00Z",
        "unread": 2, "direction": "in"  // in | out | internal
      }
    ]
  },
  "sla": [                            // SLA'sı 60 dk içinde dolan/dolmuş, en fazla 5, en yakın önce
    { "uuid": "…", "contact": "…", "channel": "email", "subject": "…",
      "due_at": "2026-10-30T09:40:00Z", "breached": false }
  ],
  "today": [                          // bugünkü randevularım (bitmemiş), başlangıca göre, en fazla 6
    { "uuid": "…", "title": "Kurulum görüşmesi", "with": "Mehmet Kaya",
      "starts_at": "…", "ends_at": "…",
      "kind": "video",                // video | phone | in_person | custom_url
      "join": "clickfix:///video/<room uuid>" }   // yoksa null
  ]
}
```

Uzantılar `v` bilinmiyorsa veya JSON bozuksa "uygulamayı aç" durumunu
gösterir. Zamanlar ISO 8601 (UTC).

Diğer App Group anahtarları:

| Anahtar | Yazan | Anlam |
|---|---|---|
| `clickfix.widget.presenceOverride` | widget / kontrol / intent | `{"status":"busy","at":"…"}`: API'ye gönderilen son durum; uygulama bir sonraki snapshot'ta siler |
| `clickfix.widget.pendingRoute` | iOS 17'de uygulamayı açan intent | `"/search?q=…"`; uygulama ön plana gelince okuyup siler ve oraya gider |

## Widget token'ı (paylaşılan Keychain)

Arka planda çalışan intent'ler (durum değiştirme) API'yi kullanıcının ana
refresh token'ı ile değil, **dar kapsamlı, kısa ömürlü, iptal edilebilir** bir
widget token'ı ile çağırır.

- Uygulama `POST /v1/mobile/widget-token` (Bearer + `X-Organization`) ile
  alır: gövde `{ "device_id": "<kurulum id>" }`, yanıt
  `{ "token", "expires_at" }`. Ömür 72 saat; uygulama her ön plana gelişte,
  12 saatten eskiyse yeniler. Aynı cihaz için eskisi iptal edilir.
- Keychain: servis `com.getclickfix.widget`, hesap `credential`, erişim
  `kSecAttrAccessibleAfterFirstUnlock`, grup `ClickFixKeychainGroup`. Değer
  JSON: `{ "token", "expires_at", "api_url", "org" }`.
- Token sadece `Authorization: Widget <token>` ile ve sadece
  `/v1/mobile/widget/*` rotalarında geçerlidir. Oturum (refresh token)
  iptal edilirse, çıkışta (`DELETE /v1/mobile/widget-token`) veya süre
  dolunca geçersizdir. Platform rolü taşımaz; organizasyon üyeliği ve
  izinleri her istekte yeniden çözülür.
- Rotalar: `PUT /v1/mobile/widget/presence` (`{ "status" }`, `PresenceMe`
  döner), `GET /v1/mobile/widget/counts` (`ConversationCounts`,
  `tenant.conversations.read` gerekir).

## Live Activities

SwiftUI görünümleri `targets/widgets` içinde; ActivityKit tipleri uzantıda ve
köprü modülünde birebir aynıdır (ActivityKit tipi adından eşler). Tarihler
Unix **saniye** (`Double`), ActivityKit'in 2001 referanslı `Date` çözümüne
takılmamak için.

| Tip | Sabit öznitelikler | `content-state` |
|---|---|---|
| `CallActivityAttributes` | `room`, `title`, `subtitle`, `startedAt` | `{ muted, kind: "video"\|"voice" }` |
| `UrgentConversationAttributes` | `conversation` (uuid) | `{ contact, channel, subject, preview, dueAt, breached }` |

Görüşme etkinliğini uygulama başlatır (LiveKit odası açılınca), sessize alma
ve bitirme `clickfix:///call/mute` / `clickfix:///call/end` ile görüşme
ekranına döner. Acil konuşma etkinliğini uygulama snapshot'tan (bana atanmış,
SLA'sı 15 dk içinde dolan veya dolmuş) ya da backend push-to-start ile başlatır.

Token'lar `POST /v1/mobile/live-activity-tokens` ile kaydedilir:
`{ device_id, kind: "push_to_start"|"update", activity_type: "call"|"urgent_conversation", activity_id?, ref?, token, environment: "production"|"sandbox" }`.
`DELETE /v1/mobile/live-activity-tokens/{token}` siler. Ayar
`GET|PUT /v1/mobile/live-activity-settings` (`{ "enabled": bool }`); kapalıysa
veya kullanıcının push tercihi kapalıysa backend Live Activity push'u
göndermez.

Backend `sla.warning` olayında (konuşma, atanmış ajan) acil konuşma
etkinliğini push-to-start ile başlatır (`attributes-type`
`UrgentConversationAttributes`), `sla.breached` ile günceller; konuşma çözülünce
veya ajan yanıt verince bitirir.

## Deep link'ler (`clickfix://`)

| Link | Ekran |
|---|---|
| `clickfix:///inbox` | Gelen kutusu |
| `clickfix:///conversation/<uuid>` | Konuşma |
| `clickfix:///agenda` | Ajanda (bugün) |
| `clickfix:///appointment/<uuid>` | Randevu |
| `clickfix:///video/<uuid>` | Görüntülü oda (katıl) |
| `clickfix:///search?q=<metin>` | Arama |
| `clickfix:///assistant` | Asistan |
| `clickfix:///new-conversation` | Yeni konuşma (kişi seçimi) |
| `clickfix:///presence?set=<status>` | Durumu değiştir (Android widget'ı ve iOS 17 intent'leri) |
| `clickfix:///call/end`, `clickfix:///call/mute` | Süren görüşmeyi bitir / sessize al |

## Apple Developer ve EAS'te sahibin yapacakları

1. Identifiers → App Groups: `group.com.getclickfix.app` oluştur.
2. `com.getclickfix.app`: App Groups (yukarıdaki grup), Push Notifications
   açık. Live Activity push için ayrı bir anahtar gerekmez; APNs Auth Key
   (.p8) "Apple Push Notifications service" yetkisiyle oluşturulur.
3. `com.getclickfix.app.widgets` identifier'ını oluştur, aynı App Group'u ekle.
4. Keychain Sharing için ek capability gerekmez (aynı takım öneki).
5. EAS: `eas credentials` ile uzantı için provisioning profile üret
   (ilk `eas build` sorar). Bundle id ve proje ayarları değişmez.
6. Backend ortamı: `APNS_KEY_ID`, `APNS_TEAM_ID`, `APNS_PRIVATE_KEY`
   (.p8 içeriği, base64), `APNS_TOPIC` (`com.getclickfix.app`). Boşsa Live
   Activity push'u kapalıdır, başka hiçbir şey etkilenmez.
