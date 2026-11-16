# Masaüstü imzalama ve güncelleyici anahtarları (CF-240)

ClickFix Masaüstü (`desktop/apps/agent`) ve ClickFix Destek (`desktop/apps/support`) paketlerinin imza anahtarlarının üretimi, saklanması, döndürülmesi ve iptali. Tasarım: [docs/design/masaustu.md](../design/masaustu.md), 7. bölüm (Tehdit 2) ve 8. bölüm.

Ders: AnyDesk 2024'te üretim sistemleri ele geçirildi ve kod imzalama sertifikaları iptal edilip yeniden dağıtım yapılmak zorunda kalındı. Buradan çıkan kurallar:

- Anahtarlar geliştirici makinesinde durmaz.
- İşletim sistemi imzası ile güncelleyici imzası ayrı anahtarlardır.
- İptal ve döndürme prosedürü olay olmadan önce yazılıdır.

## 1. Anahtarlar

| Anahtar | Ne imzalar | Nerede durur | Kim erişir |
|---|---|---|---|
| Tauri updater özel anahtarı (minisign) | Güncelleme paketleri (`.app.tar.gz`, `-setup.exe`); uygulama yalnız bu imzayla güncellenir | GitHub Actions secret `TAURI_SIGNING_PRIVATE_KEY` (+ `TAURI_SIGNING_PRIVATE_KEY_PASSWORD`). Şifreli çevrim dışı yedek (bkz. 3) | Yalnız `desktop-release` iş akışı |
| Tauri updater açık anahtarı | Uygulamaya gömülür (`plugins.updater.pubkey`) | GitHub Actions variable `TAURI_UPDATER_PUBKEY` (gizli değil) | Herkes |
| Apple Developer ID Application sertifikası | macOS `.app`, sidecar ve DMG kod imzası | Actions secret `APPLE_CERTIFICATE` (base64 `.p12`) + `APPLE_CERTIFICATE_PASSWORD`. Kimlik adı `APPLE_SIGNING_IDENTITY` | Yalnız iş akışı; iş sonunda geçici anahtar zinciri silinir |
| App Store Connect API anahtarı | `notarytool` ile noter başvurusu | Actions secret `APPLE_API_KEY` (key id), `APPLE_API_ISSUER`, `APPLE_API_KEY_P8` (`.p8` içeriği) ve `APPLE_TEAM_ID` | Yalnız iş akışı |
| Windows Authenticode | Windows `.exe` ve NSIS kurulumu | **Karar bekliyor** (bkz. 5). İş akışı `WINDOWS_SIGN_COMMAND` secret'ını kullanır | - |

Mobil uygulamayla aynı Apple ekibi kullanılır, ancak Developer ID sertifikası mobil dağıtım sertifikasından ayrıdır. API anahtarı yalnız "Developer" rolüyle açılır.

Repoda yalnız adlar geçer. Değerler hiçbir zaman commit edilmez, PR'a, issue'ya veya günlüğe yazılmaz.

## 2. Güncelleyici anahtarını üretme

Temiz, güvenilir bir makinede ve tek seferlik bir dizinde:

```bash
cd desktop
pnpm install --frozen-lockfile
umask 077
key_dir="$(mktemp -d)"
pnpm --filter @clickfix-desktop/support exec tauri signer generate -w "$key_dir/clickfix-updater.key"
# Güçlü bir parola girin (parola yöneticisinde üretin).
```

Çıktı iki dosyadır: `clickfix-updater.key` (özel) ve `clickfix-updater.key.pub` (açık).

1. GitHub → Settings → Secrets and variables → Actions:
   - **Secret** `TAURI_SIGNING_PRIVATE_KEY`: `clickfix-updater.key` dosyasının içeriği.
   - **Secret** `TAURI_SIGNING_PRIVATE_KEY_PASSWORD`: parola.
   - **Variable** `TAURI_UPDATER_PUBKEY`: `clickfix-updater.key.pub` dosyasının içeriği.
2. Çevrim dışı yedek: özel anahtarı parolasından **ayrı** şifreli bir ortama (ör. `age` ile şifrelenmiş dosya, iki ayrı USB, kasada) koyun. Parola parola yöneticisinde, en az iki yetkili kişide durur.
3. Makinedeki dosyaları güvenli silin (`rm -P` macOS, `shred -u` Linux). Anahtar geliştirici makinesinde kalmaz.

Açık anahtar `tauri.conf.json`'a yazılmaz; iş akışı derleme sırasında değişkenden ekler. Bu yüzden repodaki yapılandırmada `pubkey` boştur ve yerel derlemeler güncelleme denetimi yapmaz.

## 3. Apple sertifikası ve API anahtarı

1. Apple Developer → Certificates → **Developer ID Application** (G2 Sub-CA). CSR, anahtarın üretildiği makinede (tercihen geçici bir kullanıcı hesabında) oluşturulur.
2. Anahtar Zinciri'nden sertifika + özel anahtar `.p12` olarak, güçlü parolayla dışa aktarılır.
3. `base64 -i developer-id.p12 | pbcopy` → secret `APPLE_CERTIFICATE`; parola → `APPLE_CERTIFICATE_PASSWORD`.
4. `security find-identity -v -p codesigning` çıktısındaki ad (ör. `Developer ID Application: <Şirket> (<TEAMID>)`) → `APPLE_SIGNING_IDENTITY`; ekip kimliği → `APPLE_TEAM_ID`.
5. App Store Connect → Users and Access → Integrations → **Team Keys**: "Developer" rolüyle anahtar oluşturun. Key ID → `APPLE_API_KEY`, Issuer ID → `APPLE_API_ISSUER`, indirilen `.p8` içeriği → `APPLE_API_KEY_P8`.
6. `.p12`, `.p8` ve CSR dosyaları güvenli silinir. Yedek gerekirse 2. adımdaki şifreli çevrim dışı ortama konur.

Apple gizli değerlerinden biri eksikse iş akışı durmaz; macOS paketini **imzasız iç test** olarak etiketler (artefakt adı `-imzasiz-ic-test` ile biter, `manifest.json` içinde `os_signed: false`). Bu paketler müşteriye dağıtılmaz; yayınlama betiği `--allow-unsigned` olmadan reddeder, indirme sayfası da uyarı gösterir.

## 4. Release ve yayınlama

1. `desktop/VERSION`'ı bir PR ile yükseltin (`Cargo.toml`, iki `tauri.conf.json` ve iki `package.json` aynı sürümde olmalı; `make desktop-check` denetler). Beta için ön sürüm kullanın: `0.2.0-beta.1`.
2. Actions → **desktop-release** → Run workflow: `app` (`support`, `agent`, `both`) ve `channel` (`beta`, `stable`). Git etiketi basılmaz.
3. Bitince artefaktları indirin (her hedef bir dizin: paketler, `.sig`, `SHA256SUMS`, `manifest.json`).
4. Yerel deneme (bitiş kriteri):
   - macOS: `spctl -a -vvv -t install <dmg>` "accepted, source=Notarized Developer ID" demeli; uygulama Gatekeeper uyarısız açılmalı.
   - Güncelleme: bir önceki sürümü kurun, yeni sürümü yayınlayın, uygulamayı açıp kapatın; ikinci açılışta yeni sürüm çalışmalı (günlük: "güncelleme indirildi; uygulama kapanınca kurulacak").
5. Yayınlayın (`platform.desktop_releases.manage` yetkili hesapla):

   ```bash
   cd desktop
   CLICKFIX_API_URL=https://api.getclickfix.com CLICKFIX_EMAIL=... CLICKFIX_PASSWORD=... CLICKFIX_TOTP=... \
     node scripts/publish-release.mjs ~/Downloads/clickfix-support-0.2.0-stable-* --notes "Sürüm notu"
   ```

6. Sorun çıkarsa geri çekin: `POST /v1/platform/desktop/releases/{uuid}/withdraw` (`{"reason":"..."}`). Geri çekilen sürüm updater'dan ve `/indir`'den kalkar. O sürüme geçmiş istemciler ancak daha yüksek bir sürüm yayınlanınca düzelir; geri çekme sürüm düşürmez.

## 5. Windows imza kararı (açık)

Azure Artifact Signing Türkiye'deki şirketlere açık değil. Seçenekler:

- Bulut HSM'li OV sertifika: SSL.com eSigner, DigiCert KeyLocker. Anahtar HSM'den çıkmaz; CI kimlik bilgisiyle imzalar.
- AB'deki bir şirket üzerinden Azure Artifact Signing.

Karar verilince:

1. Sağlayıcının imza aracını (CLI) `desktop-release.yml`'deki "Windows Authenticode" adımında kurun ve kimlik bilgilerini yeni secret'lar olarak ekleyin.
2. `WINDOWS_SIGN_COMMAND` secret'ına Tauri `bundle.windows.signCommand` biçiminde komutu yazın (`%1` imzalanacak dosyadır).
3. İş akışı imzalı paketi `Get-AuthenticodeSignature` ile doğrular.

O zamana kadar Windows paketleri imzasız iç testtir.

## 6. Anahtar döndürme (planlı)

Takvim: Apple sertifikası süresi dolmadan 30 gün önce; updater anahtarı yılda bir veya yetkili kişi ayrıldığında.

**Updater anahtarı:** uygulama yalnız gömülü açık anahtarla imzalanmış güncellemeyi kabul eder. Bu yüzden sıra önemlidir:

1. Yeni anahtar çiftini 2. bölümdeki gibi üretin ama henüz secret'ı değiştirmeyin.
2. `TAURI_UPDATER_PUBKEY` değişkenini **yeni** açık anahtarla güncelleyin; `TAURI_SIGNING_PRIVATE_KEY` **eski** anahtarda kalsın.
3. Bir sürüm çıkarıp yayınlayın: eski anahtarla imzalı, içinde yeni açık anahtar gömülü. İstemciler bunu kabul eder ve artık yeni anahtara güvenir.
4. İstemcilerin çoğu geçtikten sonra (sürüm dağılımını izleyin, en az 2 hafta) `TAURI_SIGNING_PRIVATE_KEY` ve parolasını yeni anahtarla değiştirin.
5. Eski anahtarın çevrim dışı yedeğini imha edin ve tarihi bu belgeye kaydedin.

Bu ara sürümü atlayan istemciler (çok eski sürüm) yeni anahtarla imzalı güncellemeyi reddeder; onlara `/indir`'den yeniden kurulum gerekir.

**Apple sertifikası:** yeni Developer ID sertifikası alın, secret'ları değiştirin, bir sürüm çıkarın. Eski sertifikayla imzalı ve noterli paketler süre dolduktan sonra da açılır (zaman damgalı); iptal edilmediği sürece eskisini hemen iptal etmeyin.

## 7. İptal (anahtar sızdı veya şüphe var)

Önce zararı durdurun, sonra yeniden imzalayın.

1. **Dağıtımı durdurun:** etkilenen sürümleri geri çekin (platform yönetim ucu). Gerekirse API'de `/v1/public/desktop/releases/...` ucunu geçici kapatın; istemciler yalnız bu uçtan güncelleme alır.
2. **GitHub:** sızan secret'ları hemen silin, Actions günlüklerini ve `desktop-release` çalıştırma geçmişini inceleyin, repo erişimlerini daraltın.
3. **Apple sertifikası sızdıysa:** Apple Developer'da sertifikayı iptal edin (Revoke) ve Apple Developer Support'a bildirin. İptal, o sertifikayla imzalı **tüm** paketleri (meşrular dahil) Gatekeeper'da geçersiz kılar; bu yüzden hemen yeni sertifikayla yeniden imzalı bir sürüm hazırlayın. API anahtarı sızdıysa App Store Connect'te iptal edin.
4. **Updater anahtarı sızdıysa:** saldırgan geçerli imzalı güncelleme üretebilir, ama istemciye ulaşmak için uç noktamızı da ele geçirmesi gerekir. Yine de anahtar güvenilmez sayılır:
   - Yeni anahtar çifti üretin.
   - Kalan güvenli yol, sızan anahtarla imzalı son bir sürümle yeni açık anahtarı dağıtmaktır (6. bölüm, 2-3. adım). Sızıntı sonrası bunu yapmak bile riskliyse, istemcileri yeniden kuruluma yönlendirin: `/indir` ve uygulama içi uyarı, müşterilere ve işletmelere e-posta.
5. **Windows sertifikası:** sağlayıcıdan iptal isteyin; HSM kimlik bilgilerini döndürün.
6. **Bildirim:** etkilenen işletmelere ve gerekiyorsa KVKK kapsamında Kurul'a bildirim değerlendirmesi. Olay kaydı `docs/security/` altına tarihli olarak yazılır.

## 8. Gizli değer ve değişken listesi

| Ad | Tür | Zorunlu |
|---|---|---|
| `TAURI_SIGNING_PRIVATE_KEY` | secret | Evet (yoksa iş akışı durur) |
| `TAURI_SIGNING_PRIVATE_KEY_PASSWORD` | secret | Anahtar parolalıysa |
| `TAURI_UPDATER_PUBKEY` | variable | Evet |
| `APPLE_CERTIFICATE` | secret | macOS imzası için |
| `APPLE_CERTIFICATE_PASSWORD` | secret | macOS imzası için |
| `APPLE_SIGNING_IDENTITY` | secret | macOS imzası için |
| `APPLE_TEAM_ID` | secret | macOS noteri için |
| `APPLE_API_KEY` | secret | macOS noteri için |
| `APPLE_API_ISSUER` | secret | macOS noteri için |
| `APPLE_API_KEY_P8` | secret | macOS noteri için |
| `WINDOWS_SIGN_COMMAND` | secret | Windows imzası için (karar sonrası) |
| `DESKTOP_RELEASE_HOST` | variable | Hayır (varsayılan `https://getclickfix.com`) |
