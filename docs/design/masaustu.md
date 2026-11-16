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

## 6. Masaüstü giriş akışı

Kalıp, `qrlogin`'deki tek kullanımlık takastır.

1. Uygulama bir `state` ve PKCE `code_verifier` üretir, sistem tarayıcısında `https://getclickfix.com/desktop/login?state=...&challenge=...` sayfasını açar.
2. Kullanıcı tarayıcıda normal yoldan giriş yapar: şifre, Google, Apple, passkey, SSO.
3. Sayfa backend'den tek kullanımlık, 60 saniyelik bir kod alır ve `clickfix://auth/callback?code=...&state=...` adresine yönlendirir.
4. Uygulama kodu ve `code_verifier`'ı `POST /v1/auth/desktop/exchange` ile takas eder.
   - Normal oturum (refresh token) döner. Cihaz listesinde "ClickFix Masaüstü (macOS)" olarak görünür.
   - Refresh token işletim sisteminin anahtar zincirinde (Keychain / Credential Manager) saklanır.
5. Webview'e oturum çerezi bir takas sayfası üzerinden kurulur: `POST /desktop/session` ve ardından yönlendirme. Token URL'e yazılmaz.

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
  - `desktop/**` değişince self-hosted Linux runner'da şunlar çalışır: `cargo check`, `cargo clippy`, `cargo test`, sidecar `go vet`, `go test`, `golangci-lint`, destek arayüzünün `pnpm typecheck` / `lint` / `build`, Linux `tauri build --no-bundle` denemesi.
  - macOS ve Windows derlemeleri repo private olduğu için ücretli dakika harcar. Bu yüzden yalnız `desktop-release` iş akışında çalışır (`workflow_dispatch` ile).
- **Sürüm:** `desktop/` kendi sürümünü taşır (`desktop/VERSION`, semver). Elle git etiketi basılmaz; release iş akışı sürümü dosyadan okur.
- **İmzalama:**
  - macOS: Developer ID Application sertifikası, hardened runtime, `notarytool`. Mobil için kullanılan Apple ekibi kullanılır, sertifika ayrıdır.
  - Windows: Azure Artifact Signing Türkiye'deki şirketlere açık değil. Seçenekler: bulut HSM'li OV sertifika (SSL.com eSigner, DigiCert KeyLocker) ya da AB'deki bir şirket. Bu bir kullanıcı kararıdır. İmzasız derleme yalnız iç test içindir.
- **Güncelleyici:**
  - Tauri updater imzası zorunludur (`TAURI_SIGNING_PRIVATE_KEY`).
  - Bildirim dosyası `GET /v1/public/desktop/releases/{app}/{target}/{current_version}` üzerinden API'den gelir.
  - Paketler nesne depolamada durur; URL'leri API üretir.
  - Kanallar: `stable`, `beta`.
- **İndirme sayfası:** `getclickfix.com/indir`. İşletim sistemini algılar, iki uygulamayı ayrı gösterir, SHA-256 değerlerini yazar. Konuşmadaki uzaktan destek mesajı da buraya bağlanır.

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
