# Web widget SDK (CF-55, CF-150)

Müşterinin sitesine eklenen canlı destek balonu ve onu yöneten JavaScript SDK'sı. Tasarım kararları ve
backend parçaları: [design/widget.md](design/widget.md). Tipler: `https://<app>/widget.d.ts`
(kaynak `frontend/public/widget.d.ts`). Çalışan örnek: [examples/widget](../examples/widget).

## Kurulum

En kısa hali (sayfanın sonuna):

```html
<script src="https://<app>/widget.js" data-widget="<public id>" async></script>
```

Betik yüklenmeden önce çağrı yapmak için kuyruk parçacığı (çağrılar yüklenince sırayla çalışır):

```html
<script>
  window.ClickFix = window.ClickFix || function () {
    (window.ClickFix.q = window.ClickFix.q || []).push(arguments);
  };
  ClickFix("boot", { widgetId: "<public id>", locale: "tr" });
</script>
<script src="https://<app>/widget.js" async></script>
```

Panelde **Web widget → Kurulum** sekmesi HTML, Next.js, React, Vue, WordPress, Shopify ve Google Tag
Manager için hazır kod verir; **Kurulum kontrolü** widget'ın görüldüğü siteleri listeler.

CF-55 (SDK 1.x) parçacıkları ve çağrıları değişmeden çalışır.

## Başlatma seçenekleri (`boot` / `init`)

| Seçenek | Açıklama |
|---|---|
| `widgetId` | Herkese açık widget id (`data-widget` yerine) |
| `locale` | `tr` / `en` (varsayılan: widget ayarı, sonra tarayıcı dili) |
| `theme` | `light` / `dark` / `auto` |
| `position`, `color` | Balon tarafı ve ana renk (`#rrggbb`) |
| `dir` | `ltr` / `rtl` |
| `hideLauncher` | Balonu gizler; kendi düğmenizi kullanın |
| `customLauncherSelector` | Kendi düğmelerinizin CSS seçicisi (`data-clickfix-launcher` her zaman çalışır) |
| `zIndex` | Katman sırası |
| `user` | Başlangıçta `identify` |
| `attributes` | Kişi oluşunca yazılacak öznitelikler |
| `consent` | Çerez yöneticiniz onayı zaten aldıysa `true` |
| `debug` | Olayları konsola yazar |

## Komutlar

Her komut iki biçimde çağrılır: `ClickFix("open", "help")` veya `ClickFix.open("help")`.

| Komut | Ne yapar |
|---|---|
| `boot(options)` / `init(options)` | Widget'ı başlatır |
| `shutdown()` | Widget'ı sayfadan kaldırır |
| `update({...})` | Profil (`name`, `email`, `phone`), çalışma zamanı ayarları (`locale`, `theme`, `position`, `color`, `dir`, `hideLauncher`) ve öznitelikler (diğer tüm sade anahtarlar) |
| `open(view?)` | Paneli açar: `home`, `messages`, `help`, `new` veya `{ conversation: uuid }` |
| `close()`, `toggle()` | Paneli kapatır / değiştirir |
| `show()`, `hide()` | Widget'ın tamamını gösterir / gizler |
| `showLauncher()`, `hideLauncher()` | Yalnız balonu |
| `identify(user, userHash)` veya `identify({ user_id, user_hash, ... })` | Ziyaretçiyi oturum açmış kullanıcınızla eşler |
| `setAttributes({ plan: "pro" })` | Eşlenmiş öznitelikleri kişi alanlarına yazar |
| `track(name, props)` | Sayfa olayı; son 20 olay ziyaretçinin sonraki mesajına eklenir |
| `startConversation(text?, { send? })` / `showNewMessage(text?)` | Yeni konuşmayı hazır metinle açar (`send: true` hemen gönderir) |
| `showMessage(text, { open?, sender? })` | Programatik proaktif mesaj balonu |
| `searchHelp(query)` | Yardım sekmesinde arar |
| `openArticle(url)` | Yardım makalesini yeni sekmede açar (yalnız https) |
| `setLocale("en")`, `setTheme("dark")` | Çalışma zamanında dil / tema |
| `consent(true \| false)` | KVKK/GDPR onayı; `false` saklanan her şeyi siler |
| `getUnreadCount()`, `getVisitorId()`, `isOpen()`, `getState()` | Durum okuma |
| `on(event, fn)`, `once(event, fn)`, `off(event, fn?)` | Olay dinleme |
| `reset()` | Çıkışta çağırın: ziyaretçi unutulur, sonraki açılış yeni ziyaretçidir |
| `startReplay()`, `stopCapture(revoke?)` | Oturum kaydı (CF-126) |

## Olaylar

| Olay | Yük |
|---|---|
| `ready` | `{ protocol_version, sdk_version, widget_id, visitor_id }` |
| `open`, `close`, `view` | `{ view }` |
| `message` | Ekipten (veya `sender_type: "bot"` ile AI asistandan) yanıt: `{ conversation_uuid, message_uuid, text, sender_type }` |
| `message:sent` | Ziyaretçinin mesajı kaydedildi |
| `conversation:started` (`conversation`) | Yeni konuşma |
| `conversation:ended` | Konuşma çözüldü / kapandı: `{ conversation_uuid, status }` |
| `unread` | Okunmamış yanıt sayısı değişti: `{ count }` |
| `csat` (`rating`) | Memnuniyet anketi yanıtlandı: `{ conversation_uuid, score, existing }` |
| `trigger` | Proaktif mesaj gösterildi: `{ id, name, message }` |
| `identify`, `attributes`, `consent`, `visibility`, `reset`, `track`, `error` | Bkz. `widget.d.ts` |

## Kimlik doğrulama

`user_hash = hex(HMAC-SHA256(identity_secret, user_id))` yalnız sunucunuzda hesaplanır. Anahtar panelde
**Güvenlik** bölümünde üretilir ve bir kez gösterilir. Örnekler (Node, Go, PHP, Python) aynı bölümdedir.
Doğrulanan kullanıcının anonim geçmişi aynı kişiye taşınır.

### identify reddedilirse

`identify` (ve `boot({ user })`) reddedilirse widget çalışmaya devam eder ama kullanıcı **anonim kalır**:
geçmişi eşleşmez ve doğrulanmış kullanıcıya özel öznitelikler yazılmaz. SDK bunu `debug` kapalıyken de
tarayıcı konsoluna `[ClickFix] identify was rejected (...)` uyarısıyla yazar ve `error` olayını yayar.
`ClickFix("identify", ...)` çağrısının döndürdüğü promise ayrıca reddedilir.

| Yanıt | Anlamı | Çözüm |
|-------|--------|-------|
| `403 WIDGET_IDENTITY_DISABLED` | Widget için kimlik anahtarı hiç üretilmemiş. | Panelde **Web widget → Güvenlik → Kimlik doğrulama** altında anahtarı üretin, sunucunuza kaydedin ve `user_hash` değerini bu anahtarla hesaplayın. |
| `401 WIDGET_IDENTITY_INVALID` | `user_hash`, `user_id` ile eşleşmiyor. | Sunucunuzdaki anahtarın bu widget'ın **güncel** anahtarı olduğunu (yenilediyseniz eskisi geçersizdir), imzanın `identify`'a verdiğiniz `user_id` dizesinin aynısı üzerinden hesaplandığını ve çıktının hex (base64 değil) olduğunu kontrol edin. |

```js
ClickFix("on", "error", (e) => {
  if (e.code === "WIDGET_IDENTITY_DISABLED" || e.code === "WIDGET_IDENTITY_INVALID") {
    // e.message nedeni ve panelde bakılacak yeri söyler
  }
});
ClickFix("boot", { widgetId: "<id>", user: { user_id: "42", user_hash: "<sunucuda hesaplanan>" } });
```

Panel de aynı durumu gösterir: **Kurulum → Kurulum kontrolü** ve **Güvenlik** bölümleri, widget'ın son
reddedilen denemesini (yalnız hata kodu ve zamanı) uyarı olarak listeler. Geçerli bir `user_hash` ile ilk
doğrulamada uyarı kalkar. Konsol uyarısında, `error` olayında ve bu kayıtta `user_id`, `user_hash` veya
anahtar değeri bulunmaz.

## Öznitelikler

`setAttributes`, `identify({ attributes })`, `update` ve ön sohbet soruları yalnız **öznitelik eşleme**
listesindeki anahtarları yazar (`name`, `email`, `phone` veya `custom:<kişi alanı anahtarı>`). Değerler kişi
özel alanının tipine göre doğrulanır. Anonim işaretli olmayan anahtarları yalnız doğrulanmış kullanıcılar
yazabilir; böylece konsoldan `plan: "enterprise"` yazmak işe yaramaz. Kişi henüz yoksa sonuç
`pending: true` döner ve SDK öznitelikleri ilk mesajla gönderir.

## Kendi düğmeniz

```html
<button data-clickfix-launcher>Destek</button>
<script>ClickFix("boot", { widgetId: "<id>", hideLauncher: true });</script>
```

Düğmeye `data-clickfix-unread` (okunmamış sayısı) ve `aria-expanded` yazılır; rozet için CSS yeterlidir.

## Hedefleme ve proaktif mesajlar

Panelde sayfa kuralları (`içerir`, `eşittir`, `ile başlar`, `ile biter`, `desen (*)`, `içermez`), cihaz ve ülke
seçilir. `/pricing` gibi `/` ile başlayan değer yol ve sorguyla, tam adres tüm URL ile eşleşir. Sunucu
bootstrap'ta ilk sayfayı değerlendirir (ülke yalnız sunucuda bilinir); SDK tek sayfa uygulama
gezinmelerinde (`pushState`, `popstate`, `hashchange`) aynı kuralları tarayıcıda yeniden uygular. Proaktif
mesajlar sayfada geçen süre, kaydırma yüzdesi, çıkış niyeti, yeni/geri gelen ziyaretçi, giriş yapmış
kullanıcı, dil, ülke ve UTM koşullarıyla tetiklenir; gösterim sıklığı ziyaretçi başına bir kez, ziyaret
başına bir kez veya her sayfa olabilir.

## Gizlilik (KVKK / GDPR)

- **Onay iste:** ziyaretçi onaylayana kadar tarayıcıya hiçbir şey yazılmaz; çerez yöneticiniz
  `ClickFix("consent", true)` çağırabilir veya `boot({ consent: true })` verebilir.
- **Çerezsiz mod:** oturum yalnız bellekte tutulur (sayfa yenilenince yeni ziyaretçi).
- **Kalıcılık:** `local` (ziyaretler arası), `session` (sekme), `memory`.
- Ön sohbet formundaki onay kutusu, onay zamanıyla ilk mesaja eklenir.

## Çalışma zamanı

- Shadow DOM içinde çalışır; sitenizin CSS'i widget'a, özel CSS'iniz sitenize sızmaz. Özel CSS'ten
  `@import`, betik adresleri ve https olmayan `url()` değerleri kaldırılır.
- Açık/koyu/otomatik tema, RTL, telefonda tam ekran, klavye ile kullanım (Esc kapatır), `prefers-reduced-motion`.
- Gerçek zamanlı bağlantı (Centrifugo) kopunca yeniden bağlanır, o sırada yoklama yapar. Çevrimdışıyken
  mesajlar kuyrukta bekler ve bağlantı gelince sırayla gönderilir.
- Markdown alt kümesi (kalın, italik, kod, liste, bağlantı); HTML asla yorumlanmaz.
- Paket: `widget.js` ~120 KB (gzip ~38 KB); oturum kaydı modülü yalnız onaydan sonra ayrı yüklenir.

## React

```tsx
import { useClickFix, useClickFixEvent } from "@clickfix/react-widget";
const { ready, unread, open, sdk } = useClickFix({ appOrigin: "https://<app>", widgetId: "<id>" });
useClickFixEvent("message", (m) => console.log(m.text));
```

Kendi arayüzünüzü yazmak için başsız istemci (`createWidgetClient`) de aynı pakettedir:
[sdks/react-widget/README.md](../sdks/react-widget/README.md).

## Geliştirme

```bash
cd frontend
pnpm widget:build   # tsc kontrolü + esbuild → public/widget.js, public/widget-recorder.js
pnpm widget:test    # paylaşılan şema/hedefleme/CSS/markdown testleri (Go ile ortak fixture'lar)
```

Editörün canlı önizlemesi `public/widget-preview.html` içinde aynı `widget.js`'i önizleme modunda çalıştırır
(ağ yok; ayarlar `postMessage` ile gelir).
