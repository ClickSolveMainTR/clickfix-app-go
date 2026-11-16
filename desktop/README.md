# ClickFix masaüstü uygulamaları

Bu dizinde iki Tauri v2 uygulaması ve onların ortak Go sidecar'ı durur. Tasarım: [docs/design/masaustu.md](../docs/design/masaustu.md) (CF-231).

| Uygulama | Dizin | Ne yapar |
|---|---|---|
| ClickFix Masaüstü (temsilci) | `apps/agent` | Barındırılan web uygulamasını (`CLICKFIX_APP_URL`, varsayılan `https://getclickfix.com`) bir pencerede açar. Bağlantı yoksa yerel hata sayfası ve "Yeniden dene" gösterir |
| ClickFix Destek (müşteri) | `apps/support` | Pakete gömülü küçük Vite + React + TypeScript arayüzü. Şimdilik "Kodunuzu girin" yer tutucu ekranı |

## Yapı

```
desktop/
  Cargo.toml              Rust çalışma alanı (sürüm = VERSION)
  VERSION                 masaüstü sürümü (semver)
  crates/shell/           ortak Rust: sidecar başlatma/izleme, JSON-RPC istemcisi, tek örnek, günlük, yapılandırma
  apps/agent/             src-tauri + ui/ (yerel yükleme/hata sayfası)
  apps/support/           src-tauri + Vite arayüzü (src/)
  sidecar/                ayrı Go modülü (backend'i import etmez)
    cmd/clickfix-sidecar/ --mode=agent|support
    internal/ipc/         stdin/stdout satır sınırlı JSON-RPC
  scripts/build-sidecar.mjs  sidecar'ı clickfix-sidecar-<target-triple> adıyla derler
  scripts/check.sh        tüm denetimler (make desktop-check ve CI)
  ci/Dockerfile           CI derleme konteyneri
  assets/icon.svg         simge kaynağı (`tauri icon` ile üretilir)
```

## Ön koşullar

| Araç | Sürüm | Not |
|---|---|---|
| Rust | 1.90 veya üstü (CI 1.97) | `rustup component add clippy rustfmt` |
| Go | 1.26.11 | `sidecar/go.mod` |
| Node | 22 (`frontend/.nvmrc`) | |
| pnpm | 11.22.0 | `package.json` `packageManager` |
| Tauri CLI | 2.12.4 | `@tauri-apps/cli`, pnpm ile kurulur; ayrıca kurmaya gerek yok |
| golangci-lint | v2.14.0 | isteğe bağlı yerelde, CI'da zorunlu |

İşletim sistemine göre:

- **macOS:** Xcode Command Line Tools (`xcode-select --install`).
- **Windows:** Visual Studio Build Tools ("Desktop development with C++") ve WebView2 Runtime (Windows 10/11'de genelde kurulu).
- **Linux (Debian/Ubuntu):**

  ```bash
  sudo apt-get install -y build-essential pkg-config libwebkit2gtk-4.1-dev \
    libayatana-appindicator3-dev librsvg2-dev libssl-dev libxdo-dev
  ```

## Geliştirme

Repo kökünden:

```bash
make desktop-dev-agent     # ClickFix Masaüstü (tauri dev)
make desktop-dev-support   # ClickFix Destek (tauri dev, Vite 127.0.0.1:1420)
make desktop-check         # CI ile aynı denetimler
DESKTOP_TAURI_BUILD=1 make desktop-check   # + tauri build --no-bundle (yavaş)
```

Yerel backend ile temsilci uygulaması:

```bash
CLICKFIX_APP_URL=http://localhost:3000 make desktop-dev-agent
```

`CLICKFIX_APP_URL` `https` olmalıdır; `http` yalnız `localhost` ve `127.0.0.1` için kabul edilir. Değer köke indirgenir (yol, sorgu atılır).

`tauri dev` ve `tauri build`, `beforeDevCommand` / `beforeBuildCommand` ile önce sidecar'ı derler. Elle derlemek için: `pnpm sidecar` (bu dizinde).

## Sidecar ve IPC

- Rust kabuğu sidecar'ı uygulama ikilisinin yanından (`bundle.externalBin`) başlatır, `ping` ile doğrular, çökerse 0,5 sn'den 30 sn'ye kadar geri çekilerek yeniden başlatır. 60 sn sağlıklı çalışan süreçte geri çekilme sıfırlanır. Sidecar stderr'i kabuk günlüğüne akar.
- Protokol satır sınırlı JSON'dur:
  - istek `{"id":1,"method":"ping","params":{}}`
  - yanıt `{"id":1,"result":{...}}` ya da `{"id":1,"error":{"code":-32601,"message":"..."}}`
  - olay `{"event":"ready","params":{...}}`
- Hata kodları JSON-RPC 2.0 ile aynıdır (`-32700` bozuk satır, `-32600` geçersiz istek, `-32601` bilinmeyen yöntem, `-32603` iç hata). İstekler eşzamanlı işlenir; yanıtlar `id` ile eşlenir.
- Yöntemler: `ping`, `version`. Sidecar ağ portu açmaz.

## Güvenlik

- **Temsilci:** IPC uzak köke yalnız çalışma anında `CLICKFIX_APP_URL`'den üretilen `remote-app` capability'si ile açılır (`remote.urls` = `<kök>/*`, yalnız ana pencere). Uzak kök yalnız köprü komutlarını çağırabilir: `notify`, `set_badge`, `set_presence`, `open_external`, `version`, `sign_out`. Yerel sayfa (`capabilities/local.json`) yalnız `connect`, `start_login`, `cancel_login`. Komut listesi tek yerdedir (`apps/agent/src-tauri/src/commands.rs`; `build.rs` ve `capability.rs` buradan okur, testler capability dosyalarını denetler). Çekirdek ve eklenti izinleri hiçbir pencereye verilmez. Ana pencere kök dışındaki adreslere gidemez; `target=_blank` / `window.open` ile açılan dış bağlantılar sistem tarayıcısında açılır.
- **Oturum:** refresh token işletim sisteminin anahtar zincirinde (macOS Keychain, Windows Credential Manager, Linux Secret Service) uygulama köküne göre ayrı kayıttadır; erişim token'ı yalnız bellektedir. Token, kod ve verifier günlüğe yazılmaz.
- **Destek:** yerel arayüz yalnız `sidecar_version` komutunu çağırabilir; uzak kök yüklenmez.
- Uygulama komutları `AppManifest` ile tanımlanır, bu yüzden capability'de izin verilmeyen pencere onları çağıramaz.
- Webview'e shell izni verilmez; sidecar'ı yalnız Rust tarafı başlatır.
- İmza ve güncelleyici anahtarları repoda durmaz (CF-240).

## ClickFix Masaüstü (CF-236)

Tasarım: [docs/design/masaustu.md](../docs/design/masaustu.md) §4 ve §6 (uygulama notları ve sapmalar orada).

| Özellik | Nasıl |
|---|---|
| Giriş | Yerel sayfa "Tarayıcıda giriş yap": sistem tarayıcısında `<kök>/desktop/login?state&challenge` (PKCE S256), dönüş `clickfix://auth/callback`, takas `/api/mobile/v1/auth/desktop/exchange`. Webview oturumu `/desktop/session` ile kurulur |
| Derin bağlantı | `clickfix://conversation/<uuid>`, `clickfix://open?path=/...`; ikinci açılış ilk örneğe iletilir (tek örnek + `deep-link`) |
| Bildirim | Web köprüsü `notify`; pencere öndeyse gösterilmez. Masaüstünde web push aboneliği açılmaz |
| Rozet | Okunmamış atanmış konuşma sayısı: dock / başlatıcı sayısı, Windows görev çubuğu kaplaması, tepsi ipucu |
| Tepsi | Göster/gizle, durum, "Kapatınca tepside kalsın", "Oturum açılışında başlat", oturumu kapat, çık |
| Menü | Hakkında, "Güncellemeleri denetle…" (CF-240'a kadar yer tutucu), "Günlükleri aç", Düzen, Pencere |
| Kısayol | Varsayılan `CommandOrControl+Shift+K`: pencereyi öne getirir, Cmd+K paletini açar |
| Pencere | Konum ve boyut korunur (`window-state`, görünürlük hariç) |

Ayarlar, uygulama yapılandırma dizininde `settings.json`'dadır (macOS `~/Library/Application Support/com.getclickfix.desktop/`, Windows `%APPDATA%\com.getclickfix.desktop\`, Linux `~/.config/com.getclickfix.desktop/`):

```json
{ "close_to_tray": true, "autostart": false, "shortcut": "CommandOrControl+Shift+K" }
```

Tepsi menüsü ilk ikisini değiştirir. Kısayol şimdilik yalnız bu dosyadan değişir; boş değer kısayolu kapatır, kaydedilemeyen değerde varsayılan denenir.

Eklenti sürümleri `Cargo.toml` içinde `=` ile sabittir (`tauri-plugin-deep-link`, `-single-instance`, `-notification`, `-global-shortcut`, `-autostart`, `-window-state`, `-opener`, `keyring`). Yükseltme ayrı bir değişiklikle ve aşağıdaki elle denemeyle yapılır.

### Elle deneme (macOS, yerel)

Linux CI derler ve birim testlerini çalıştırır; şunlar gerçek bir Mac'te elle denenir (ekran görüntülerinde kişisel veri olmamalı):

1. `CLICKFIX_APP_URL=http://localhost:3000 make desktop-dev-agent`; yerel sayfada "Tarayıcıda giriş yap", tarayıcıda giriş, uygulamaya dönüş ve panelin açılması. Keychain Access'te `com.getclickfix.desktop` kaydı.
2. Uygulamayı kapatıp açınca yeniden giriş istenmemesi; çıkışta Keychain kaydının silinmesi ve cihaz listesinden oturumun düşmesi.
3. Pencere arkadayken gelen bildirimin yerel bildirim olarak görünmesi, tıklayınca ilgili konuşmanın açılması; pencere öndeyken yerel bildirimin çıkmaması.
4. Dock rozeti ve tepsi ipucunda okunmamış atanmış konuşma sayısı.
5. `open "clickfix://conversation/<uuid>"` ile konuşmanın açılması; uygulama kapalıyken aynı komutla açılış.
6. `Cmd+Shift+K` ile pencerenin öne gelmesi ve paletin açılması.
7. Tepsi: durum seçimi web'deki durumla eşleşir; "Kapatınca tepside kalsın" kapalıyken pencereyi kapatmak uygulamayı kapatır; "Oturum açılışında başlat".

Paketli derlemede `clickfix://` şemasını işletim sistemi kurulumda kaydeder; `tauri dev` ile macOS'ta şema yalnız `.app` paketiyle çalışır (`tauri build --debug --bundles app`).

## Tek örnek ve günlük

İki uygulama da tek örnek çalışır: ikinci açılış var olan pencereyi öne getirir. Günlük, işletim sisteminin uygulama günlük dizinine yazılır (macOS `~/Library/Logs/<identifier>/`, Windows `%LOCALAPPDATA%\<identifier>\logs\`, Linux `~/.local/share/<identifier>/logs/`), 5 MB'ta döner.

## CI

`.github/workflows/ci.yml` içindeki `desktop` işi `desktop/**` değişince çalışır (yalnız `*.md` değişikliği hariç) ve `ci` kontrolünün parçasıdır. İş `ci/Dockerfile` imajını (Dockerfile özetiyle etiketli) kurar ve `scripts/check.sh`'ı `DESKTOP_TAURI_BUILD=1` ile konteynerde, runner kullanıcısıyla çalıştırır. Cargo, Go ve pnpm önbellekleri runner başına `_work/desktop-cache-<runner>` dizinindedir. Disk dolarsa bu dizin silinebilir; sonraki çalışma sıfırdan derler.

macOS ve Windows derlemeleri, imzalama ve dağıtım CF-240 ile `desktop-release` iş akışına gelir.

## Sürüm

`VERSION` tek kaynaktır. Değiştirince `Cargo.toml` (`workspace.package.version`), iki `tauri.conf.json` ve iki `package.json` de güncellenir; `scripts/check.sh` uyuşmazlığı yakalar.

## Release ve güncelleme (CF-240)

- İmzalı paketler yalnız elle tetiklenen `desktop-release` iş akışında derlenir (macOS arm64 + x64, Windows x64). Sürüm `VERSION`'dan okunur, git etiketi basılmaz.
- Paketler Actions artefaktı olarak çıkar; API'ye `node scripts/publish-release.mjs <dizin>...` ile yayınlanır, `/indir` sayfası ve güncelleyici oradan okur.
- Uygulamalar açılışta `GET /api/v1/public/desktop/releases/{app}/{target}/{current_version}` ucunu sorar, yeni paketi indirip imzasını doğrular ve kapanışta kurar (`crates/shell/src/updates.rs`). Geliştirme derlemesinde ve `plugins.updater.pubkey` boşken denetim yapılmaz.
- `bundle.createUpdaterArtifacts` açık olduğundan yerelde paketli derleme (`tauri build`, `--no-bundle` olmadan) updater anahtarı ister. Anahtarsız yerel deneme: `pnpm --filter @clickfix-desktop/support tauri build --config '{"bundle":{"createUpdaterArtifacts":false}}'`.
- Anahtarlar, gizli değerler, döndürme ve iptal: [docs/security/masaustu-imzalama.md](../docs/security/masaustu-imzalama.md).
