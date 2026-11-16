# Masaüstü uygulamaları ve uzaktan destek (CF-231)

Bu belge iki masaüstü ürününü ve onları besleyen backend ve web işlerini tanımlar.

- **ClickFix Masaüstü:** temsilci uygulaması.
- **ClickFix Destek:** müşterinin indirdiği hızlı destek uygulaması.

Uygulama işleri CF-232 ile CF-240 arasındadır (bkz. 9. İş yolları).

## 1. Neden

### Rakipler

| Ürün | Masaüstü uygulaması |
|---|---|
| Front | Var, Electron, macOS paketi yaklaşık 277 MB |
| Crisp | Var |
| Tidio | Var |
| Zendesk, Intercom, Freshdesk, Help Scout, Chatwoot, LiveAgent, HubSpot | Yok, yalnız web ve tarayıcı bildirimi |
| Gorgias | Yalnız Apple Silicon'da çalışan iPad uygulaması |

Kullanıcıların sürekli istediği şeyler:
- ayrı bir pencere,
- güvenilir bildirim,
- tepside rozet,
- kısayol,
- e-postadaki bağlantıdan doğrudan konuşmayı açmak.

Bunları masaüstü uygulamasıyla ilk sunanlardan biri olabiliriz.

### Uzaktan destek

Uzaktan destek (AnyDesk, TeamViewer QuickSupport, Splashtop SOS, Zoho Assist) yardım masalarına hep bir eklenti ile bağlanır. Örnekler:
- Zendesk + TeamViewer
- Freshdesk + Zoho Assist

Bu eklentiler ayrı lisans ister. Biz oturumu doğrudan konuşmanın içinden açarız.

Konuşmanın içinden açmanın ürün avantajı:
- Müşteri kimin bağlandığını görür: doğrulanmış organizasyon adı, logosu ve temsilci adı.
- Kod yalnız var olan bir konuşmaya bağlıdır. Dışarıdan gelen "şu kodu gir" araması işe yaramaz. Bu, dolandırıcılığa karşı en güçlü önlemdir (bkz. 7).

### Zaten elimizde olanlar

| Parça | Nerede | Bu işte kullanımı |
|---|---|---|
| LiveKit SFU, gömülü TURN, kayıt (egress) | `docs/video-rooms.md`, `platform/livekit` | Ekran görüntüsü ve kontrol olayları için medya ve veri kanalı |
| Co-browse (rrweb, onay, işaretçi) | `docs/design/session-replay.md` (CF-126) | Kurulumsuz ilk adım. Uzaktan destek, site dışına çıkmak gereken durumlar içindir |
| QR ile giriş, tek kullanımlık takas | `modules/qrlogin` | Masaüstü giriş akışının kalıbı |
| OAuth 2.1 sunucusu | `modules/mcpauth` | Gerekirse PKCE kalıbı |
| Bildirim hattı (in-app, web push, Expo) | `docs/bildirimler.md` | Masaüstü bildirimleri aynı satırların aynası olur |
| Destek erişimi ve denetim | `modules/supportaccess`, `platform/activity` | Her oturum adımının denetim kaydı |
| Entitlement ve ölçümleme | `platform/entitlement`, `docs/billing` | Paket özelliği ve dakika ölçümü |

## 2. Kararlar

1. **Tauri v2, Go sidecar ile.**
   - Rust tarafı yalnız kabuktur: pencere, tepsi, bildirim, derin bağlantı, güncelleyici, sidecar yönetimi.
   - İş mantığı web uygulamasında ve Go sidecar'dadır.
   - Electron'a göre paket küçüktür (hedef: Destek uygulaması 15 MB altı) ve bellek kullanımı düşüktür.
2. **Temsilci uygulaması, barındırılan web uygulamasını yükler.**
   - Next.js uygulaması SSR ve Auth.js çerezleriyle çalışır, statik paketlenemez.
   - Pencere `https://getclickfix.com` kökünü açar.
   - Tauri IPC yalnız bu kökten gelen çağrılara açıktır (`capabilities` içinde `remote.urls`); başka hiçbir köke açılmaz.
   - Web tarafı masaüstünde çalıştığını `window.__TAURI_INTERNALS__` ile değil, köprü modülüyle anlar: `frontend/src/lib/desktop/bridge.ts`.
3. **Müşteri uygulaması yerel arayüz paketler.**
   - Arayüz küçük ve statiktir (kod girişi, onay ekranları, durum).
   - Uzak kök yüklemez; tüm ağ çağrıları sidecar üzerinden API'ye gider.
4. **Medya LiveKit üzerinden akar.**
   - Her uzaktan destek oturumu bir LiveKit odasıdır: `rs_<uuid>`, `auto_create: false`.
   - Müşteri ekranı bir video izi olarak yayınlanır.
   - Kontrol olayları (fare, klavye) güvenilir veri kanalıyla, yalnız müşteriye gider.
   - NAT ve kurumsal ağlarda var olan TURN/TLS 443 kullanılır.
   - Röle sunucuları kendi altyapımızdadır. Bu, KVKK md. 9 kapsamında yurt dışı aktarım sorusunu önler.
5. **Ekranın yakalanma yolu bir keşifle seçilir (CF-235).** Adaylar:
   - **A:** Webview içinde `getDisplayMedia` ve LiveKit JS. Kodlama ücretsizdir; sorun, WKWebView ve WebView2 desteği ve izin akışıdır.
   - **B:** Go sidecar içinde yerel yakalama (ScreenCaptureKit / Windows Graphics Capture, cgo) ve VP8/H.264 kodlama, LiveKit Go istemcisiyle (Pion).
   - **C:** Rust `scap` ile yakalama ve LiveKit Rust SDK.

   Ölçütler: gecikme (hedef 200 ms altı), CPU, paket boyutu, izin akışı, imzalama karmaşıklığı. Karar bu belgeye eklenir.

   Hangi yol seçilirse seçilsin, **girdi enjeksiyonu Go sidecar'dadır**: robotgo; macOS'ta Accessibility, Windows'ta kullanıcı oturumu düzeyinde.
6. **RustDesk kodu kullanılmaz.** AGPL-3.0 lisanslıdır; ürüne gömmek kaynak açmayı gerektirir. Yalnız mimari fikir olarak okunabilir; kod kopyalanmaz. `docs/REFERANSLAR.md` "Sadece fikir" satırına eklenir.
7. **Gözetimsiz erişim (unattended) v1'de yoktur.** Her oturum müşterinin o anki onayıyla başlar ve uygulama kapanınca biter. Arka planda çalışan servis, otomatik başlatma ve yükseltilmiş yetki (UAC, güvenli masaüstü) kapsam dışıdır.
8. **Linux:**
   - Temsilci uygulaması için hedeftir (AppImage, deb).
   - Destek uygulaması için v1'de yoktur. Wayland'da yakalama ve girdi enjeksiyonu ayrı iştir.

## 3. Depo yapısı

```
desktop/
  Cargo.toml                 # Rust çalışma alanı
  crates/shell/              # ortak Rust: sidecar başlatma, güncelleyici, tek örnek, günlük
  apps/agent/                # ClickFix Masaüstü (src-tauri + küçük yükleme/hata sayfası)
  apps/support/              # ClickFix Destek (src-tauri + statik arayüz: Vite + React, shadcn yok)
  sidecar/                   # Go modülü: github.com/furkanmeclis/clickfix-app-go/desktop/sidecar
    cmd/clickfix-sidecar/    # tek ikili, mod bayrağıyla: --mode=agent | --mode=support
    internal/ipc/            # stdin/stdout satır sınırlı JSON-RPC
    internal/api/            # ClickFix API istemcisi (OpenAPI tipleriyle)
    internal/remote/         # LiveKit katılımı, veri kanalı, girdi enjeksiyonu
    internal/sysinfo/        # destek kaydı için sistem bilgisi (müşteri onayıyla)
  package.json               # pnpm çalışma alanına eklenir
```

- Sidecar ayrı bir Go modülüdür ve `backend` modülünü import etmez. API ile yalnız HTTP üzerinden, OpenAPI sözleşmesiyle konuşur.
- Sidecar ikilisi Tauri `bundle.externalBin` ile `clickfix-sidecar-<target-triple>` adıyla paketlenir.
- macOS'ta uygulamayla birlikte hardened runtime ile imzalanır ve noterden geçer.
- Rust ve sidecar arasında IPC: sidecar yalnız kendi ebeveyninden stdin üzerinden komut alır, ağ portu açmaz.
  - Her komutun bir `id`'si olur, yanıt `id` ile eşlenir.
  - Uzun ömürlü olaylar `event` satırlarıyla gelir.
- Sidecar'ı Rust tarafı (`crates/shell`) doğrudan başlatır; `tauri-plugin-shell` kullanılmaz ve webview'e shell izni verilmez (CF-232).
- Temsilci uygulamasının `remote.urls` capability'si sabit JSON yerine çalışma anında `CLICKFIX_APP_URL`'den üretilir; böylece hazırlık ortamı ve yerel backend de yalnız kendi köküne IPC açar (CF-232).
- Ayrıntılar ve yerel geliştirme: `desktop/README.md`.

## 4. ClickFix Masaüstü (temsilci)

| Özellik | Ayrıntı |
|---|---|
| Pencere | Tek örnek. Pencere konumu ve boyutu korunur. Kapatma tepsiye küçültür (ayarlanabilir) |
| Bildirim | Web uygulaması bir in-app bildirimi gösterdiğinde köprü üzerinden yerel bildirim çıkar. Tıklanınca ilgili konuşma açılır. Web push masaüstünde kapatılır, aynı bildirim iki kez gelmez |
| Rozet ve tepsi | Okunmamış atanmış konuşma sayısı: macOS dock rozeti, Windows görev çubuğu kaplaması, tepsi menüsü (durum: çevrimiçi / meşgul / çevrimdışı; presence modülü) |
| Genel kısayol | Varsayılan `Ctrl/Cmd+Shift+K`: pencereyi öne getirir ve Cmd+K paletini açar |
| Derin bağlantı | `clickfix://conversation/<uuid>`, `clickfix://auth/callback?...`. E-posta bildirimlerinde "Masaüstünde aç" bağlantısı |
| Giriş | Sistem tarayıcısında giriş, uygulamaya tek kullanımlık kodla dönüş (bkz. 6). Google ve Apple gömülü webview'de OAuth'a izin vermez |
| Çoklu organizasyon | Web'deki organizasyon değiştirici aynen çalışır. Ayrı pencere v2'ye kalır |
| Otomatik başlatma | Kullanıcı ayarı. Varsayılan kapalı |
| Güncelleme | Tauri updater, imzalı bildirim dosyası (bkz. 8) |
| Çevrimdışı | Bağlantı koptuğunda yerel hata sayfası ve yeniden deneme. Taslak saklama v2 |

## 5. ClickFix Destek (müşteri) ve uzaktan destek oturumu

### Akış

1. Temsilci konuşmada **Uzaktan destek** düğmesine basar ve mod seçer: yalnız izleme, ya da kontrol isteği.
2. Backend oturumu oluşturur.
   - 9 haneli kod üretir; yalnız özeti (hash) saklanır.
   - Tek kullanımlık katılma bağlantısı üretir.
   - Ömrü 15 dakikadır.
3. Konuşmaya müşteriye görünen bir mesaj düşer: indirme bağlantısı (işletim sistemine göre) ve kod.
4. Müşteri uygulamayı açar.
   - Windows'ta kurulumsuz tek exe, macOS'ta dmg içinde .app.
   - Kodu girer, ya da `clickfix-support://join?t=...` bağlantısı kodu kendisi doldurur.
5. Uygulama `POST /v1/public/remote-support/resolve` ile oturumu çözer ve gösterir:
   - organizasyonun doğrulanmış adını, logosunu,
   - temsilcinin adını, fotoğrafını,
   - istenen yetkiyi,
   - "ClickFix çalışanları ve işletmeler sizden asla para transferi, banka girişi veya kart bilgisi istemez" uyarısını.
6. Onaylar ayrı ayrı alınır:
   - **İzleme onayı.** Zorunludur. Aydınlatma metni ayrı gösterilir, açık rıza ayrı bir kutudur (KVKK 2026/347 ilke kararı).
   - **Kontrol onayı.** Temsilci kontrol istediyse ayrı bir evet gerekir.
   - **Kayıt onayı.** Organizasyon kaydı açtıysa ayrı bir evettir. Hayır derse oturum kayıtsız sürer.
7. macOS'ta ilk kullanımda izin ekranları gösterilir: Ekran Kaydı ve, kontrol için, Erişilebilirlik. Hangi düğmeye basılacağı adım adım anlatılır ve izin verilince uygulama yeniden başlar.
8. Oturum sürerken:
   - Ekranın kenarında her zaman görünen bir çerçeve ve "<Temsilci> ekranınızı görüyor / kontrol ediyor" çubuğu durur.
   - Çubukta tek tıkla **Durdur** ve **Kontrolü geri al** vardır.
   - Kısayol `Ctrl+Alt+Shift+S` oturumu hemen bitirir.
9. Bitiş:
   - Müşteri durdurur, temsilci bitirir, süre dolar (varsayılan en çok 60 dakika) ya da konuşma kapanır.
   - LiveKit odası silinir, sidecar girdi enjeksiyonunu kapatır, uygulama bir özet gösterir.

### Kontrol kapısı

Sidecar bir girdi olayını yalnız şu koşulların hepsi sağlanınca uygular:
- oturum etkindir,
- kontrol onayı verilmiştir ve geri alınmamıştır,
- olay temsilcinin LiveKit kimliğinden gelmiştir,
- hız sınırı aşılmamıştır.

Ek kurallar:
- Klavye olaylarında sistem kısayol listesi (ör. `Ctrl+Alt+Del`) engellenir.
- Pano senkronu ve dosya aktarımı v1'de yoktur; v2 için ayrı onay adımı olur.

### Backend modülü: `modules/remotesupport`

- **Tablo `remote_support_sessions`** (migration 000238), alanları:
  - organization, conversation, agent, contact,
  - `code_hash`, `join_token_hash`, `status` (`pending | joined | active | ended | expired | cancelled`),
  - `mode_requested`, `view_consent_at`, `control_consent_at`, `control_revoked_at`, `recording_consent`,
  - `room_name`, `egress_id`,
  - `client_os`, `client_version`,
  - `ended_reason`, `ended_by`, `started_at`, `ended_at`,
  - organizasyon kapsamlı indeksler.
- **İzinler:** `tenant.remote_support.use`, `tenant.remote_support.control`, `tenant.remote_support.settings`, `platform.remote_support.manage`. Migration, `rbac` ve `frontend/src/config/permissions.ts` üçlüsü ile eklenir; varsayılan olarak owner ve admin rollerine verilir.
- **Ayarlar:** organizasyon düzeyinde açık/kapalı, kontrole izin, kayıt (kapalı / isteğe bağlı), en uzun süre.
- **Entitlement:** `remote_support` özelliği ve dakika ölçümü.
- **Rotalar** (taslak; OpenAPI aynı değişiklikte):
  - `POST /v1/tenant/conversations/{uuid}/remote-support`: oluştur
  - `GET /v1/tenant/remote-support/{uuid}`: durum
  - `POST /v1/tenant/remote-support/{uuid}/token`: temsilcinin LiveKit token'ı; abone olur ve veri gönderir, yayın yapmaz
  - `POST /v1/tenant/remote-support/{uuid}/end`
  - `POST /v1/public/remote-support/resolve`: kod veya bağlantı ile çöz. IP ve kod başına sıkı hız sınırı; 5 yanlış denemede kod geçersiz olur
  - `POST /v1/public/remote-support/{uuid}/consent`: izleme, kontrol, kayıt; geri alma da buradan
  - `POST /v1/public/remote-support/{uuid}/token`: müşterinin token'ı; yalnız ekran izini yayınlar
  - `POST /v1/public/remote-support/{uuid}/end`
  - `POST /v1/public/remote-support/{uuid}/report`: "bu bir dolandırıcılık" bildirimi
  - LiveKit webhook (var olan uç noktaya `rs_` odaları için dal): katılma ve ayrılma durumları
- **Gerçek zamanlı:** temsilci ekranı durum değişikliklerini Centrifugo özel kanalından alır.
- **Denetim:** oluşturma, çözme, her onay, kontrolün geri alınması, bitiş ve bildirim, `activity` ile organizasyonun denetim kaydına yazılır.
- **Kötüye kullanım:**
  - Platform yöneticisi bir organizasyonun uzaktan desteğini tek düğmeyle kapatır (kill switch).
  - Bildirilen oturum işaretlenir.
  - Kısa sürede çok sayıda bildirim alan organizasyonda özellik kendiliğinden askıya alınır.

#### Uygulama notları (CF-233)

Backend `backend/internal/modules/remotesupport` altındadır. Yukarıdaki taslaktan farkları ve netleşen kararlar:

- **Kod ve deneme sayımı.** Kod 9 hanedir (`crypto/rand`). Saklanan iki özet vardır, ikisi de sunucu anahtarıyla HMAC'tir: tüm kodun özeti (`code_hash`) ve ilk 5 hanenin özeti (`code_lookup`). `code_lookup` bekleyen oturumlar arasında tekildir. Yanlış bir kod, aynı 5 haneyle başlayan bekleyen oturuma yazılır; 5. yanlış denemede o oturum `expired` (`attempts`) olur. Böylece "5 yanlış denemede kod geçersiz" kuralı, kod oturumu doğrudan göstermediği halde uygulanabilir.
- **Müşteri kimliği.** Çözme (`resolve`) bir kez `client_secret` döner (256 bit, yalnız özeti saklanır). Sonraki public çağrılar (`consent`, `token`, `end`, `report`) oturum uuid'si ve bu sırla çalışır; başka hiçbir şey açmaz.
- **Hız sınırı.** `resolve`: IP başına dakikada 10, saatte 30; kod öneki ve bağlantı başına 15 dakikada 10. Diğer public uçlar: IP başına dakikada 60, oturum başına 10 dakikada 120. Hepsi `ratelimit.AllowStrict` ile kapalı başarısızdır (Redis yoksa istek reddedilir).
- **Durumlar.** `pending` (kod geçerli) → `joined` (çözüldü) → `active` (müşteri ekran izi ile LiveKit'e katıldı, webhook). Başlamadan biten oturum `cancelled`, süresi dolan `expired`, başlamış oturum `ended` olur. Bitiş nedenleri: `agent`, `customer`, `declined`, `code_expired`, `timeout` (çözüldükten sonra 15 dakika içinde izleme onayı yok), `attempts`, `max_duration`, `conversation_closed`, `platform`, `reported`.
- **Kontrol kapısı (sunucu tarafı).** Temsilci token'ı hiçbir zaman medya yayınlamaz. `canPublishData` yalnız müşteri kontrol onayı vermiş ve geri almamışsa, organizasyon kontrole izin veriyorsa ve temsilcinin `tenant.remote_support.control` izni varsa açıktır. Onay verildiğinde veya geri alındığında bağlı temsilcinin izni LiveKit `UpdateParticipant` ile hemen değiştirilir; yeni token da aynı kurala göre üretilir.
- **Ayarlar** `remote_support_settings` tablosundadır. Organizasyon için varsayılan **kapalıdır** (dolandırıcılık riski nedeniyle yönetici açar); kontrole izin varsayılan açık, kayıt `off`, en uzun süre 60 dakika (5–240), kayıt saklama 30 gün (1–365). Platform durumu (`active | disabled | suspended`) aynı satırdadır.
- **Entitlement.** Özellikler `remote_support.enabled` (boolean, katalog varsayılanı açık) ve `remote_support.minutes` (aylık toplam, katalog varsayılanı sınırsız). Dakika, ekran paylaşımının başladığı andan bitişe kadar yukarı yuvarlanarak oturum başına bir kez ölçülür.
- **Kayıt.** Kayıt onayı ve organizasyonda `optional` kayıt varsa, müşteri katılınca oda için egress başlar ve dosya platform deposuna (`remote-support-recordings/<org>/<oturum>.mp4`) gider. Kayıt bilgileri `video_recordings`'e değil oturum satırına yazılır (o tablo video odasına bağlıdır); silme, video kayıtlarının saklama işiyle aynı zamanlanmış işte (`app:video:recording-purge`) yapılır. Kayıt indirme ucu bu işte yoktur.
- **Konuşma mesajı.** Müşteriye görünen mesaj (indirme bağlantısı `{FRONTEND_URL}/indir?app=support`, İngilizce organizasyonlarda `&lang=en` + kod; CF-240) konuşmanın kanalından otomatik mesaj olarak gider. Kanal penceresi kapalıysa mesaj düşmez (`message_posted: false`) ve temsilci kodu kendisi iletir. Katılma bağlantısı mesaja konmaz, yalnız temsilciye bir kez döner.
- **Doğrulanmış ad.** Ayrı bir doğrulama kaydı olmadığından `verified`, organizasyonun durumunun `active` olmasıdır. Temsilci fotoğrafı henüz yoktur (`avatar_url: null`).
- **Ek rotalar.** `GET /v1/tenant/conversations/{uuid}/remote-support` (konuşmanın oturumları), `GET|PUT /v1/tenant/remote-support/settings`, `GET /v1/platform/remote-support/organizations`, `PUT /v1/platform/remote-support/organizations/{uuid}/state` (kill switch; kapatma bütün açık oturumları hemen bitirir), `GET /v1/platform/remote-support/reports`.
- **Otomatik askıya alma.** 24 saatte 3 bildirim organizasyonu `suspended` yapar ve açık oturumları bitirir; platform yöneticisi `active` ile geri açar.
- **Gerçek zamanlı.** Kanal `remote_support:{uuid}` (Centrifugo namespace `remote_support`); oturumun temsilcisi ve `tenant.remote_support.settings` sahibi abone olabilir.
- **Zamanlanmış iş.** `app:remote-support:sweep` dakikada bir çalışır: kodu dolanları, onay bekleyip zaman aşımına uğrayanları, süre sınırını aşanları, konuşması kapananları ve organizasyonu kapatılanları bitirir.

#### Kontrol olayı biçimi (sürüm 1, CF-237)

Temsilci konsolu olayları üretir, ClickFix Destek sidecar'ı (CF-238) çözer ve kontrol kapısından geçirerek uygular. Başvuru kodu `frontend/src/features/remote-support/lib/control.ts` dosyasındadır (`decodeEvent` alıcının yapması gereken doğrulamanın aynısıdır; testleri `control.test.mjs`). Bu bölüm ile o dosya birlikte değişir.

**Taşıma**

- LiveKit veri kanalı, güvenilir (`reliable: true`), konu (topic) `clickfix.rs.control.v1`.
- Alıcı yalnız müşteridir: `destinationIdentities` müşterinin kimliğidir (`rs:customer:<oturum>`).
- Gönderen kimliği `rs:agent:<oturum>` olmayan olay yok sayılır. LiveKit `canPublishData` iznini yalnız kontrol onayı yürürlükteyken verdiği için başka bir katılımcı zaten yayın yapamaz; sidecar yine de kimliğe bakar.
- Her mesaj tek bir JSON nesnesidir, UTF-8. En çok 1024 bayt; büyüğü atılır.

**Ortak alanlar**

| Alan | Tür | Anlam |
|---|---|---|
| `v` | tamsayı | Ana sürüm. Şu an `1`. Bilinmeyen sürüm atılır |
| `seq` | tamsayı ≥ 1 | Temsilci bağlantısı başına artan sıra. Alıcı sırayı yalnız tanı için kullanır; güvenilir kanal sırayı korur |
| `t` | tamsayı | Temsilcinin saati, Unix ms. Yalnız tanı içindir, olay zamanlaması için kullanılmaz |
| `type` | metin | Aşağıdaki türlerden biri. Bilinmeyen tür atılır |

Bilinmeyen ek alanlar hata değildir, yok sayılır. Böylece sürüm içinde alan eklemek geriye uyumludur; anlamı değişen bir değişiklik `v`'yi artırır ve konu adı `...v2` olur.

**Koordinatlar:** `x` ve `y`, paylaşılan ekranın (video izinin) sol üst köşesine göre 0 ile 1 arasında normalize değerlerdir, 4 ondalık. Konsol, `object-fit: contain` ile çizilen görüntünün siyah bantlarında olay üretmez. Sidecar ekran pikseline şöyle çevirir: `px = round(x × (genişlik − 1))`, `py = round(y × (yükseklik − 1))`; genişlik ve yükseklik yakalanan birincil ekranın gerçek piksel boyutudur (HiDPI ölçeği sidecar'ın işidir). 0–1 dışı değer atılır.

**Türler**

| `type` | Ek alanlar | Not |
|---|---|---|
| `pointer.move` | `x`, `y` | Konsol en çok saniyede 60 gönderir, son konum her zaman gider |
| `pointer.down`, `pointer.up` | `x`, `y`, `button` (`0` sol, `1` orta, `2` sağ) | DOM `button` değerleri |
| `wheel` | `x`, `y`, `dx`, `dy` | Piksel, tamsayı, mutlak değeri en çok 1200. Satır kipi 16, sayfa kipi 800 piksel sayılır |
| `key.down`, `key.up` | `code`, `key`, `mods` {`alt`,`ctrl`,`meta`,`shift`}, `repeat` | `code` fiziksel tuştur (DOM `KeyboardEvent.code`, ör. `KeyA`, `Enter`); düzenden bağımsız eşleme bununla yapılır. `key` bilgi amaçlıdır (en çok 32 karakter) |
| `release` | - | Basılı bütün tuşları ve fare düğmelerini bırak. Konsol odağı kaybedince, klavyeyi geri alınca ve kontrol bitince gönderir |

**Örnek**

```json
{"v":1,"seq":42,"t":1795339200000,"type":"pointer.down","x":0.5123,"y":0.2871,"button":0}
{"v":1,"seq":43,"t":1795339200016,"type":"key.down","code":"KeyC","key":"c","mods":{"alt":false,"ctrl":true,"meta":false,"shift":false},"repeat":false}
```

**Alıcı kuralları (sidecar)**

- Yukarıdaki "Kontrol kapısı" koşullarının hepsi sağlanmadan hiçbir olay uygulanmaz. Kapı kapanınca (onay geri alındı, oturum bitti, bağlantı koptu) sidecar `release` uygular gibi basılı her şeyi bırakır.
- Engelli kısayollar hem konsolda hem sidecar'da atılır: `Ctrl+Alt+Delete`, `Ctrl+Alt+Backspace`, `Ctrl+Shift+Escape`, `Meta+L`, `Meta+Ctrl+Q`, `Meta+Alt+Escape`, `Meta+Shift+Q`. Liste `isBlockedCombo` ile aynıdır.
- Hız sınırı: saniyede 200 olay. Aşan olaylar atılır ve oturum günlüğüne bir kez yazılır.
- `key.up` gelmeden bağlantı koparsa sidecar basılı tuşları kendisi bırakır.

**Konsol tarafı**

- "Kontrolü kullan" basılınca fare ve klavye yakalanır, ekranın çevresi yeşil olur ve "Kontrol sizde" görünür.
- Klavyeyi geri almak için `Esc` iki kez hızlıca (600 ms içinde) basılır; ilk `Esc` müşteriye gider.
- Kontrol izni, onay ya da LiveKit veri izni gidince giriş hemen kilitlenir ve bir uyarı çıkar. Onay yeniden verilse de giriş kendiliğinden açılmaz; temsilci yeniden basar.

#### Uygulama notları (CF-237)

- **Konsol:** konuşma başlığındaki uzaktan destek düğmesi (`tenant.remote_support.use` yoksa görünmez). Pencere mod seçimini, müşteriye gidecek mesajın önizlemesini, kodu (bir kez) ve kopyalama düğmelerini, adım adım durumu (Centrifugo `remote_support:{uuid}`, bağlantı yoksa 4 saniyede bir sorgu), izleyiciyi ve bitince özeti gösterir. Pencereyi kapatmak oturumu bitirmez; düğmedeki yeşil nokta süren oturumu gösterir ve tıklayınca geri açar.
- **Zaman çizelgesi:** oturum bitince konuşmaya `remote_support_ended` sistem olayı düşer (mod, süre, kontrol ve kayıt onayı). Backend'e bu işte eklendi.
- **Ayarlar:** `/settings/remote-support` (`tenant.remote_support.settings`). Paket yoksa yükseltme bilgisi, platform kapattıysa neden gösterilir.
- **Platform:** `/platform/remote-support` (`platform.remote_support.manage`): organizasyon listesi ve kill switch, bildirilen oturumlar.
- **Geliştirme müşterisi:** `/dev/remote-support`, yalnız `NODE_ENV=development` iken açılır (üretimde 404). Kodu girer, onayları verir, `getDisplayMedia` ile ekran paylaşır ve gelen kontrol olaylarını listeler (uygulamaz). İki sekmeyle uçtan uca deneme için LiveKit yerelde çalışmalı ve LiveKit webhook'u backend'e ulaşmalıdır (oturumun `active` olması webhook'la olur).
- **KVKK metinleri:** taslak `docs/design/uzaktan-destek-metinleri.md`; hukuki onaydan geçmeden ürüne konmaz.

## 6. Masaüstü giriş akışı

Kalıp, `qrlogin`'deki tek kullanımlık takastır.

1. Uygulama bir `state` ve PKCE `code_verifier` üretir, sistem tarayıcısında `https://getclickfix.com/desktop/login?state=...&challenge=...` sayfasını açar.
2. Kullanıcı tarayıcıda normal yoldan giriş yapar: şifre, Google, Apple, passkey, SSO.
3. Sayfa backend'den tek kullanımlık, 60 saniyelik bir kod alır ve `clickfix://auth/callback?code=...&state=...` adresine yönlendirir.
4. Uygulama kodu ve `code_verifier`'ı `POST /v1/auth/desktop/exchange` ile takas eder.
   - Normal oturum (refresh token) döner. Cihaz listesinde "ClickFix Masaüstü (macOS)" olarak görünür.
   - Refresh token işletim sisteminin anahtar zincirinde (Keychain / Credential Manager) saklanır.
5. Webview'e oturum çerezi bir takas sayfası üzerinden kurulur: `POST /desktop/session` ve ardından yönlendirme. Token URL'e yazılmaz.

Uygulama notları (CF-234, backend + web; ayrıntı `backend/docs/auth.md`):

- Kod `POST /v1/auth/desktop/codes` ile alınır (oturum açmış tarayıcı, `state` + S256 `code_challenge`), Redis'te özetlenmiş anahtarla 60 saniye tutulur ve bir kez kullanılır. Takas `POST /v1/auth/desktop/exchange` (kod + `code_verifier` + `state` + `client_os` + isteğe bağlı `device_name`); uygulama bunu `/api/mobile/v1` üzerinden çağırır, panel BFF'i bu yolu reddeder. Yanlış verifier üçüncü denemede kodu siler; yeniden kullanım ve yanlış verifier denetim kaydına yazılır.
- Giriş sayfası `callbackUrl` yerine mevcut giriş sayfasının `?next=` parametresini kullanır (aynı işlev, repodaki kalıp).
- Webview ayrı bir oturum alır: uygulama refresh token'ını anahtar zincirinde tutar; aynı token'ı webview ile paylaşmak rotasyonu bozar (her taraf diğerinin token'ını iptal eder). Webview, uygulamanın **erişim token'ını** `POST /desktop/session`'a gönderir (form, JSON veya `Authorization: Bearer`); Next rotası `POST /v1/auth/desktop/web-session` ile yeni bir masaüstü oturumu alır, Auth.js çerezini kurar ve 303 ile yönlendirir. Sadece aynı kökenden gelen POST kabul edilir.
- Oturumlar `refresh_tokens.client = 'desktop'`, `client_os`, `device_name` ile işaretlenir (migration 000239); yenilemede etiket korunur. Cihaz listesi "ClickFix Masaüstü (macOS) · <bilgisayar adı>" gösterir.

## 7. Güvenlik ve uyum

- **Tehdit 1, dolandırıcılık:** "telefonla arayıp uygulamayı kurdurmak".
  - Kod yalnız var olan bir konuşmadan doğar.
  - Uygulama organizasyonu ve temsilciyi doğrulanmış olarak gösterir.
  - Uyarı metni her zaman görünür.
  - Bildirim düğmesi ve kill switch vardır.
  - Kod kısa ömürlüdür ve tek kullanımlıktır.
- **Tehdit 2, imza anahtarının çalınması** (AnyDesk 2024).
  - İmza anahtarları yalnız bulut HSM'de veya GitHub Actions secret'ında tutulur, geliştirici makinesinde durmaz.
  - Güncelleyici anahtarı ayrıdır.
  - Anahtar döndürme prosedürü `docs/security/` altına yazılır.
- **Tehdit 3, sunucu tarafında yetki atlatma** (ScreenConnect CVE-2024-1709).
  - Public uçlar yalnız kod veya token ile çalışır, başka bir şey açmaz.
  - Kiracı ayrımı testleri zorunludur.
- **Tehdit 4, sidecar'ın kötüye kullanılması.**
  - Sidecar port açmaz, yalnız ebeveyn sürecinden komut alır.
  - Girdi enjeksiyonu yalnız kontrol kapısından geçer (5).
- **KVKK:**
  - Aydınlatma metni ve açık rıza metni ayrıdır.
  - Kayıt ayrı bir rızadır.
  - Kayıtların saklama süresi organizasyon ayarıdır (varsayılan 30 gün) ve video odalarındaki kayıt temizliğiyle aynı iş tarafından silinir.
  - Medya, kendi LiveKit ve TURN sunucularımızdan geçer.
- **macOS:** Sequoia, ekran kaydı iznini ayda bir yeniden sorar. Uygulama bunu hata saymaz; izin ekranını yeniden gösterir. Kalıcı izin (persistent content capture) için Apple başvurusu ayrı bir karardır.

## 8. Derleme, imzalama ve dağıtım

- **CI:**
  - `desktop/**` değişince self-hosted Linux runner'da şunlar çalışır: `cargo fmt --check`, `cargo clippy -D warnings`, `cargo test`, sidecar `go vet`, `go test`, `golangci-lint`, destek arayüzünün `pnpm typecheck` / `lint` / `build`, Linux `tauri build --no-bundle` denemesi.
  - Bunlar `desktop/ci/Dockerfile` derleme konteynerinde, `desktop/scripts/check.sh` ile çalışır; runner'a webkit2gtk kurulmaz. Yerelde aynı betik `make desktop-check` ile çalışır.
  - macOS ve Windows derlemeleri repo private olduğu için ücretli dakika harcar. Bu yüzden yalnız `desktop-release` iş akışında çalışır (`workflow_dispatch` ile).
- **Sürüm:** `desktop/` kendi sürümünü taşır (`desktop/VERSION`, semver). Elle git etiketi basılmaz; release iş akışı sürümü dosyadan okur.
- **İmzalama:**
  - macOS: Developer ID Application sertifikası, hardened runtime, `notarytool`. Mobil için kullanılan Apple ekibi kullanılır, sertifika ayrıdır.
  - Windows: Azure Artifact Signing Türkiye'deki şirketlere açık değil. Seçenekler: bulut HSM'li OV sertifika (SSL.com eSigner, DigiCert KeyLocker) ya da AB'deki bir şirket. Bu bir kullanıcı kararıdır. İmzasız derleme yalnız iç test içindir.
- **Release iş akışı (CF-240):** `.github/workflows/desktop-release.yml`, yalnız `workflow_dispatch` (girdiler: `app` = `support`/`agent`/`both`, `channel` = `beta`/`stable`). Hedefler: `darwin-aarch64` ve `darwin-x86_64` (ikisi de `macos-15` runner'ında; universal paket yerine iki paket, Tauri updater platform anahtarlarıyla birebir), `windows-x86_64` (NSIS). Beta kanalı ön sürüm ister (`0.2.0-beta.1`), stable istemez. Çıktı Actions artefaktıdır; API'ye yayınlama `desktop/scripts/publish-release.mjs` ile elle yapılır. Gizli değerlerin eksiği:
  - Updater anahtarı yoksa iş akışı durur.
  - Apple veya Windows imza değerleri yoksa paket **imzasız iç test** olarak etiketlenir (artefakt adı, `manifest.json` `os_signed: false`, indirme sayfasında uyarı) ve yayınlama betiği `--allow-unsigned` olmadan reddeder.
- **Güncelleyici:**
  - Tauri updater imzası zorunludur (`TAURI_SIGNING_PRIVATE_KEY`, `bundle.createUpdaterArtifacts`). Açık anahtar `TAURI_UPDATER_PUBKEY` değişkeninden derleme sırasında yazılır; repodaki `tauri.conf.json`'da boştur ve boşken uygulama güncelleme denetimi yapmaz.
  - Bildirim dosyası `GET /v1/public/desktop/releases/{app}/{target}/{current_version}?channel=` üzerinden API'den gelir (Tauri biçimi, güncelleme yoksa 204). `target`, Tauri'nin `{{target}}-{{arch}}` değeridir. Uç `getclickfix.com/api/v1/...` (BFF) üzerindendir.
  - Sürümler semver önceliğiyle karşılaştırılır. Beta istemci stable sürümleri de alır; stable istemci beta görmez. Geri çekilen sürüm sunulmaz, ama sürüm düşürülmez: düzeltme daha yüksek sürümle gelir.
  - Uygulama açılışta denetler, paketi indirip imzasını doğrular ve **kapanışta kurar** (`clickfix_shell::updates`). Süren bir uzaktan destek oturumu yeniden başlatmayla kesilmez; yeni sürüm sonraki açılışta çalışır.
  - Paketler nesne depolamada durur (`desktop/releases/{app}/{channel}/{version}/{target}/{uuid}/{dosya}`); API `GET /v1/public/desktop/files/{uuid}/{kind}/{name}` ile akıtır. Kayıtlar `desktop_releases` tablosundadır (000240, platform kapsamlı).
  - Yayınlama ve geri çekme: `POST /v1/platform/desktop/releases` (multipart), `POST /v1/platform/desktop/releases/{uuid}/withdraw`; yetki `platform.desktop_releases.manage`.
  - Kanallar: `stable`, `beta`.
- **Anahtarlar:** üretim, saklama, döndürme ve iptal prosedürü [docs/security/masaustu-imzalama.md](../security/masaustu-imzalama.md).
- **İndirme sayfası:** `getclickfix.com/indir` (İngilizce `?lang=en`). İşletim sistemini algılar, iki uygulamayı ayrı gösterir, SHA-256 değerlerini yazar (`GET /v1/public/desktop/downloads`). `?app=support` ClickFix Destek'i öne alır. Konuşmadaki uzaktan destek mesajı da buraya bağlanır. Landing alt bilgisinde "İndir" bağlantısı vardır.

## 9. İş yolları

| CF | İş | Bağımlı | Nerede |
|---|---|---|---|
| CF-232 | Masaüstü iskeleti: `desktop/` çalışma alanı, iki boş uygulama, sidecar modülü, CI işi | - | Bulut |
| CF-233 | Uzaktan destek backend'i: `remotesupport` modülü, migration 000238, RBAC, OpenAPI, LiveKit, onaylar, denetim, kötüye kullanım | - | Bulut |
| CF-234 | Masaüstü giriş akışı: backend takas uçları ve web `/desktop/login` sayfası | - | Bulut |
| CF-235 | Ekran yakalama ve kontrol keşfi (macOS + Windows), karar kaydı | CF-232 | Yerel (gerçek Mac, izin ekranlarına kullanıcı tıklar) |
| CF-236 | ClickFix Masaüstü: kabuk, tepsi, rozet, bildirim köprüsü, kısayol, derin bağlantı, giriş | CF-232, CF-234 | Bulut |
| CF-237 | Temsilci konsolunda uzaktan destek: düğme, kod ve bağlantı, izleyici, kontrol, durumlar, ayarlar ekranı | CF-233 | Bulut |
| CF-238 | Go sidecar: IPC, API istemcisi, LiveKit katılımı, kontrol kapısı, girdi enjeksiyonu, sistem bilgisi | CF-232, CF-235 | Bulut ve yerel test |
| CF-239 | ClickFix Destek: kod girişi, doğrulanmış kimlik, adım adım onay, macOS izin ekranları, çerçeve ve çubuk, durdurma | CF-233, CF-235, CF-238 | Bulut ve yerel test |
| CF-240 | Dağıtım: imzalama, noterleme, güncelleyici, release iş akışı, releases API'si, indirme sayfası | CF-232 (imza kararı kullanıcıda) | Bulut, gizli değerler kullanıcıda |

Dalgalar:
1. **1. dalga:** CF-232, CF-233, CF-234 (birbirinden bağımsız).
2. **2. dalga:** CF-235, CF-236, CF-237.
3. **3. dalga:** CF-238, CF-239, CF-240.

Her iş `cf-<n>-<kisa-ad>` dalında yapılır ve PR `cf-toplu`'ya açılır. Main'e toplu birleştirmeyle girer.

## 10. Kapsam dışı (v1)

Bunlar v1'de yapılmaz:
- gözetimsiz erişim,
- yükseltilmiş yetki ve UAC,
- dosya aktarımı,
- pano senkronu,
- çoklu monitör seçimi (yalnız birincil ekran),
- Linux destek uygulaması,
- mobil cihaz ekranı (iOS ReplayKit, Android MediaProjection),
- temsilci uygulamasında çoklu pencere.
