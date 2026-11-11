# Webview köprüsü (CF-199)

Mobil uygulamaların widget'ı **oturumu taşıyan güvenli bir webview** içinde göstermesi için uygulama ile widget arasındaki `postMessage` sözleşmesi. Müşteri ekibinin kararı (issue #114, 9 Kas): mobil yol tam SDK değil, bu webview'dir; böylece Flutter, React Native, iOS, Android ve diğer platformlar aynı yolla bağlanır. İleride çıkacak yerel paketler (CF-201 altyapı, CF-202 React Native, CF-203 Flutter) de bu sözleşmeyi kullanır.

Uygulama tarafı paketler: Flutter `clickfix_webview` (`sdks/flutter/clickfix_webview`, CF-203) sözleşmeyi `ClickFixController` + `ClickFixWebView` olarak uygular ve `dart analyze` / `flutter test` ile doğrulanır.

Uygulama: `frontend/widget-sdk/src/shared/bridge.ts` (saf, birim testli: `frontend/widget-sdk/tests/bridge.test.mjs`) ve `frontend/widget-sdk/src/widget.ts` (`attachBridge`). Barındırılan sayfa: `frontend/public/widget-embed.html`. Tipler: `frontend/public/widget.d.ts` (`ClickFixBridgeCommand`, `ClickFixBridgeMessage`, `ClickFixBridge`). Kullanıcı kılavuzu: `frontend/content/docs/tr/integration-guide.mdx` → "Mobil".

## Sürüm

`bridge_version: 1`. Sözleşmede yalnız geriye uyumlu değişiklik yapılır (yeni komut, yeni alan, yeni olay); uygulama tanımadığı alan ve olayları yok saymalıdır. Kırıcı değişiklik yeni bir `bridge_version` ile gelir ve `clickfix:ready` içinde duyurulur.

| Belge sürümü | İş | Değişiklik |
|---|---|---|
| 1.0 | CF-199 | İlk sözleşme: sekiz komut, `ready` / `event` / `link` / `result` mesajları. |
| 1.1 | CF-201 | Geriye uyumlu ekleme, `bridge_version` 1 kaldı: `clickfix:session` komutu ve mesajı (yerel SDK ile iki yönlü oturum devri); tipler `@getclickfix/visitor-core` paketinde de yayımlanır ("SDK çekirdeği ile paylaşım"). |
| 1.2 | CF-206 | Alan değişmedi, `bridge_version` 1 kaldı: `clickfix:identify` içindeki `user.company.id` **firma kimliği** olarak netleşti ve `user_hash` onu da imzalayabilir (`hex(HMAC-SHA256(anahtar, user_id + "\n" + company_id))`). İmzasız `company.id` bağlanmaz; `identify` olayının yükü `company_verified` alanını kazandı. Eski (yalnız `user_id`) imza çalışmaya devam eder. |

## Açma

Köprü üç yoldan açılır; hepsi aynı davranır:

| Yol | Nerede |
|---|---|
| `https://<app>/widget-embed.html?widget=<id>` | Barındırılan sayfa; `hideLauncher: true`, panel hazır olunca açılır. Sorgu: `locale` (`tr`/`en`), `theme` (`light`/`dark`/`auto`), `view` (`home`/`messages`/`help`/`new`), `conversation` (UUID), `consent=1`. |
| `<script src=".../widget.js" data-widget="<id>" data-bridge="true">` | Kendi sayfanızda |
| `ClickFix("boot", { widgetId, bridge: true })` ya da `bridge: { origin: "https://host.example" }` | Kendi sayfanızda; `origin` yalnız iframe içinde gerekir |

Önizleme modunda (`data-preview`) köprü yoktur. `shutdown` köprüyü de kaldırır.

## Taşıma

Widget → uygulama yönünde widget, sayfada bulduğu **ilk** yerel nesneyi kullanır; sıra sabittir:

| Platform | Nesne | Widget ne çağırır |
|---|---|---|
| React Native (`react-native-webview`) | `window.ReactNativeWebView` | `.postMessage(jsonString)` → `onMessage` |
| iOS (WKWebView) | `window.webkit.messageHandlers.ClickFix` | `.postMessage(jsonString)` → `WKScriptMessageHandler` (ad: `ClickFix`) |
| Android (WebView) | `window.ClickFixAndroid` | `.postMessage(jsonString)` → `@JavascriptInterface` (ad: `ClickFixAndroid`) |
| Flutter (`webview_flutter`; hazır paket: `clickfix_webview`, `sdks/flutter/clickfix_webview`, CF-203) | `window.ClickFixFlutter` | `.postMessage(jsonString)` → `JavaScriptChannel` (ad: `ClickFixFlutter`) |
| Üst çerçeve (iframe) | `window.parent` | `.postMessage(obj, origin)`; yalnız `bridge.origin` verilmişse, `"*"` asla |

Yerel nesneler JSON **metin** alır; üst çerçeve yapılandırılmış nesne alır. Hiçbiri yoksa köprü yine kurulur (komutlar işler) ama dışarı mesaj gitmez ve `ClickFixBridge.transport` `null` olur.

Uygulama → widget yönünde iki giriş vardır:

1. `window.ClickFixBridge.receive(message)`: `message` JSON metni ya da nesne. Yerel platformlarda tercih edilen yol (`evaluateJavaScript`, `evaluateJavascript`, `runJavaScript`, `injectJavaScript`). Bir Promise döner; sonuç mesajı ayrıca taşıma üzerinden gelir.
2. `window.postMessage(message)` → `message` DOM olayı. Kabul kuralı:
   - sayfa bir çerçevedeyse yalnız `e.source === window.parent` **ve** `e.origin === bridge.origin` (yapılandırılmamışsa ya da `"*"` ise reddedilir);
   - sayfa çerçeve dışındaysa yalnız sayfanın kendisinden gelen mesajlar (`e.source === window` ya da `null`, `e.origin` boş ya da sayfanın kaynağı);
   - diğer her şey sessizce yok sayılır. `type` alanı `clickfix:` ile başlamayan mesajlar da yok sayılır (sayfadaki başka betiklerin trafiğine karışmaz).

`ClickFixBridge` nesnesi: `{ version: 1, transport: "react-native" | "ios" | "android" | "flutter" | "frame" | null, receive(message) }`. Yalnız köprü açıkken vardır.

## Uygulama → widget: komutlar

Her komut bir JSON nesnesidir; `type` zorunlu, `id` isteğe bağlıdır (≤128 karakter). `id` verilirse widget `clickfix:result` ile cevap verir; verilmezse cevap gelmez. Mesaj metni 64 KB'ı aşamaz.

| `type` | Alanlar | Widget ne yapar | `result.data` |
|---|---|---|---|
| `clickfix:identify` | `user: { user_id, user_hash, name?, email?, phone?, company?: { id?, name?, domain? }, attributes? }` | `ClickFix("identify", user)`; `user_hash` uygulamanın sunucusunda hesaplanır. `company.id` firmanın uygulamadaki kimliğidir (dış firma kimliği); `user_hash` = `hex(HMAC-SHA256(anahtar, user_id + "\n" + company.id))` olarak ikisini birlikte imzalarsa kişi o firmaya bağlanır ve `identify` olayında `company_verified: true` gelir (CF-206). Yalnız `user_id` imzalanmışsa kullanıcı yine doğrulanır ama `company.id` bağlanmaz, `name`/`domain` ipucu kalır | yok |
| `clickfix:track` | `name` (≤100), `properties?` (nesne) | `ClickFix("track", name, properties)`; ekran bağlamına eklenir | yok |
| `clickfix:open` | `view?` (`home`/`messages`/`help`/`new`), `conversation?` (UUID) | `conversation` varsa o konuşmayı, yoksa `view`'i açar | yok |
| `clickfix:close` | – | Paneli kapatır | yok |
| `clickfix:update` | `update` (nesne) | `ClickFix("update", update)`: `locale`, `theme`, `name`, `email`, `phone`, öznitelikler | yok |
| `clickfix:consent` | `granted` (boolean) | `ClickFix("consent", granted)`; `false` saklanan her şeyi siler | yok |
| `clickfix:reset` | – | `ClickFix("reset")`: ziyaretçi unutulur (çıkışta) | yok |
| `clickfix:get-state` | – | – | `ClickFixState` (`open`, `view`, `unread`, `verified`, `visitor_id`, …) |
| `clickfix:session` | `token` (≤2048) | Uygulamanın yerel çekirdekle (`@getclickfix/visitor-core`, uygulama anahtarı) başlattığı ziyaretçi oturumunu devralır: belirteçle yeniden bootstrap olur, aynı ziyaretçi ve konuşmalar gelir; `identify` olayı yayınlanır. Geçersiz belirteç yabancı gibi yeni ziyaretçi başlatır (hata değil). Önizlemede yok sayılır | yok |

Doğrulama `parseCommand` içindedir: `user_id` (≤200) ve `user_hash` (≤128) zorunlu; `session.token` boş olamaz; `name/email/phone/company.*` ≤255; `attributes` yalnız düz değerler (string/number/boolean, anahtar ≤100); fazlalıklar atılır. Hatalı mesajda `id` varsa `clickfix:result` `ok:false` döner; **mesaj içeriği hiçbir zaman** sonuca, konsola ya da `error` olayına yazılmaz.

Hata kodları (`result.error.code`): `INVALID_JSON`, `INVALID_MESSAGE`, `UNKNOWN_COMMAND`, `TOO_LARGE`, komut çalışırken oluşan SDK hataları (`WIDGET_IDENTITY_DISABLED`, `WIDGET_IDENTITY_INVALID`, `RATE_LIMITED`, `NETWORK_ERROR`, …) ve bilinmeyen hatalar için `COMMAND_FAILED`. `error.message` yalnız kodu tekrarlar (`command failed (KOD)`).

## Widget → uygulama: mesajlar

| `type` | Alanlar | Ne zaman |
|---|---|---|
| `clickfix:ready` | `bridge_version: 1`, `protocol_version: 1`, `sdk_version`, `widget_id`, `visitor_id` | Widget bootstrap olup çizildiğinde; `identify` bundan sonra gönderilmelidir (önce gönderilirse de kuyruklanır) |
| `clickfix:event` | `event`, `payload` | Her SDK olayında: `ready`, `open`, `close`, `view`, `message`, `message:sent`, `conversation:started`, `conversation:ended`, `unread`, `identify`, `csat`, `trigger`, `attributes`, `consent`, `visibility`, `reset`, `track`, `error`. Yükler `widget.d.ts` → `ClickFixEventMap` ile aynıdır |
| `clickfix:link` | `url` | Kullanıcı bir yardım makalesine ya da mesajdaki dış bağlantıya dokunduğunda; widget `window.open` **çağırmaz**, uygulama sistem tarayıcısında açar. Uygulama `url`'yi açmadan önce **yeniden doğrular**: yalnız `https:` (geliştirmede `http://localhost`), asla derin bağlantı / intent işleyicisine vermez. Köprü taşıması yoksa eski davranış (`window.open`) sürer |
| `clickfix:result` | `id`, `ok`, `data?`, `error?: { code, message }` | `id` taşıyan her komuttan sonra |
| `clickfix:session` | `token`, `visitor_id` | `ready`, `identify` ve `reset` olaylarından hemen sonra: webview'in o anki ziyaretçi belirteci. Uygulama bunu `client.adoptToken(token)` ile yerel çekirdeğe verebilir (webview kapandıktan sonra aynı oturumla okunmamış sayısı, bildirim vb.). `reset` sonrası gelen belirteç yeni anonim ziyaretçinindir |

Uygulamanın tipik akışı: `ready` → `identify` gönder → `result` bekle → `unread` olayıyla rozet → `close` olayında ekranı kapat → `link` olayında tarayıcı aç → çıkışta `reset`. Yerel çekirdek kullanan uygulamada `identify` yerine `session` gönderilir (aşağıda).

## SDK çekirdeği ile paylaşım (CF-201)

Mobil SDK'lar (CF-202 React Native, CF-203 Flutter) widget uçlarını webview olmadan da çağırır. Bunun için widget ayarlarında **Mobil uygulamalar** (gelişmiş sekme) bölümünde platform + paket adı ile bir **uygulama anahtarı** oluşturulur; anahtar hash'li saklanır, bir kez gösterilir, iptal edilebilir. `/v1/widget/{public_id}/*` uçları `Origin` yerine `X-ClickFix-App-Key` + `X-ClickFix-App-Package` başlıklarını da kabul eder (`WIDGET_APP_KEY_INVALID` 403: bilinmeyen, iptal edilmiş, başka widget'ın anahtarı ya da uyuşmayan paket adı). Ziyaretçi belirteci, `user_hash` ve hız sınırı tarayıcıdakiyle aynıdır; belirteç kaynağa ya da anahtara bağlı değildir, bu yüzden aynı oturum webview'de de sürer.

**Tip paylaşımı.** Köprü mesaj ve komut tipleri üç yerde aynıdır ve bir test (`sdks/tests/visitor-core.test.mjs`) `clickfix:*` türlerinin kümesini eşit tutar:

| Yer | Ne |
|---|---|
| `frontend/widget-sdk/src/shared/bridge.ts` | Widget tarafı: `parseCommand`, `createBridge` (çalıştırır ve doğrular) |
| `frontend/public/widget.d.ts` → `sdks/react-widget/src/clickfix.d.ts` | Entegratörün gördüğü `ClickFixBridgeCommand` / `ClickFixBridgeMessage` |
| `sdks/visitor-core/src/bridge.ts` (`@getclickfix/visitor-core`) | Uygulama tarafı: `BridgeCommand`, `BridgeMessage`, `encodeCommand`, `commandScript` (`injectJavaScript` için), `parseMessage` (widget dışı mesajları eler), `isSafeLink` |

**Oturum devri, uygulama → webview.** Uygulama çekirdekle oturumu başlatır, webview'i açar ve belirteci devreder:

```ts
const client = createVisitorClient({ mode: "app", baseURL: "https://<app>/api", publicId, app: { key, packageName } });
await client.init(); await client.identify({ user_id, user_hash });
// clickfix:ready gelince:
webview.injectJavaScript(commandScript({ type: "clickfix:session", id: "s1", token: client.getToken() }));
```

Widget belirteçle yeniden bootstrap olur, aynı ziyaretçi (ve `identify` edilmişse aynı kullanıcı) ile açılır; `identify` tekrar gönderilmez, `user_hash` webview'e inmez.

**Oturum devri, webview → uygulama.** Widget `ready`, `identify` ve `reset` sonrasında `clickfix:session` mesajı gönderir; uygulama `client.adoptToken(message.token)` ile yerel çekirdeği aynı oturuma geçirir (örneğin webview kapanınca okunmamış sayısını çekirdekten izlemek için). `parseMessage(onMessageData)` yalnız widget mesajlarını döndürür.

Uygulama anahtarı webview'e **girmez**: widget sayfası uygulamanın kendi kaynağında (`https://<app>`) çalıştığı için zaten izinlidir; anahtar yalnız yerel çekirdeğin HTTP başlığındadır.

## Derin bağlantı

- Belirli konuşma: sayfayı `?conversation=<uuid>` ile aç ya da `clickfix:open` gönder.
- Bildirimden konuşmaya: uygulamanın kendi push'u için `message.sent` webhook olayı (`conversation_uuid`, `contact_external_id`); bildirimin derin bağlantısı açılınca webview yukarıdaki adresle açılır.
- Widget'tan dışarı yalnız `clickfix:link` ile çıkılır; webview `<app>` kaynağı dışına gezinmemelidir.

## Güvenlik kuralları

1. Kimlik anahtarı uygulamaya girmez; `user_hash` uygulamanın kimliği doğrulanmış sunucu ucundan gelir.
2. Webview yalnız uygulama kaynağını (`https://<app>`) yükler; başka adrese gezinme engellenir (`shouldOverrideUrlLoading`, `decidePolicyFor`, `onShouldStartLoadWithRequest`, `NavigationDelegate`). Dosya ve içerik erişimi kapalı; üçüncü taraf çerez yok.
3. Köprü `"*"` kaynağını hiçbir yönde kabul etmez; çerçeve dışı sayfada üst pencereden gelen mesajlar işlenmez. `clickfix:link` ile gelen adres uygulamada yeniden doğrulanır (yalnız `https:`), intent/derin bağlantı işleyicisine verilmez.
4. Reddedilen komutlar içerik sızdırmaz (`parseCommand` ve `errorOf` yalnız kod üretir).
5. Oturum webview depolamasındadır: hesap değişimi ve çıkışta `clickfix:reset`; paylaşılan cihazlarda depolama temizliği. `clickfix:session` ile alınan belirteç uygulamanın güvenli depolamasında tutulur (çıkışta silinir); belirteç tek widget ve ziyaretçiye bağlı imzalı bir değerdir, `user_hash` ya da uygulama anahtarı değildir.
7. Uygulama anahtarı (`X-ClickFix-App-Key`) yalnız yerel çekirdekte, uygulamanın yapılandırmasındadır; webview'e ya da sayfaya verilmez, sızdığında panelden iptal edilir ve yenisi oluşturulur. API, anahtarla birlikte `Origin` ya da `Referer` gelen isteği reddeder ve bu başlıklar CORS'ta izinli değildir: sızmış bir anahtar bir web sayfasını allowlist'ten geçiremez.
6. `consent` olmadan hiçbir şey saklanmaz; onayı uygulama iletir. `widget-embed.html?consent=1` adresi onayı kullanıcıya sormadan verir: bu adresi yalnız uygulamanın kendisi, onayı gerçekten aldıktan sonra kurar; dışarıdan gelen bir bağlantıyı olduğu gibi webview'de açmayın.

## Test

- Birim: `cd frontend && pnpm widget:test` (`tests/bridge.test.mjs`: ayrıştırma ve kırpma, taşıma sırası, kaynak kuralı, sonuçlar, olay yönlendirme, `session`).
- Çekirdek: `cd sdks && pnpm test` (`tests/visitor-core.test.mjs`: köprü yardımcıları, tip kümesi eşitliği, uygulama anahtarı başlıkları, oturum yenileme).
- Elle: `widget-embed.html?widget=<id>` sayfasını masaüstü tarayıcıda açıp konsolda `window.ClickFixBridge.receive({type:"clickfix:get-state",id:"1"})` çağırın; taşıma yokken sonuç gitmez ama Promise çözülür. Bir iframe'den denemek için üst sayfada `bridge: { origin }` ile `boot` edin.
