# Entegrasyon kılavuzu, SDK yayını ve mobil yol (CF-199)

Müşteri ekibine verilen tek entegrasyon kılavuzunun geliştirici tarafı: nerede durur, örnekleri nasıl doğrulanır, SDK'lar nasıl sürümlenir ve yayımlanır, mobil karar nedir.

## Kılavuz

- Yardım merkezi sayfası: `frontend/content/docs/tr/integration-guide.mdx` ve `en/integration-guide.mdx` (`/docs/integration-guide`). Bölüm sırası CF-198'deki kurulum sihirbazıyla aynıdır: müşteriyi tanıt (kimlik doğrulama, dış kimlik, özel alanlar, şirket, olaylar) → sistemini bağla → kanallar (API kanalı, webhook'lar) → mobil webview → AI / bilgi bankası / otomasyon (panel işi, bağlantı).
- Kaynak gerçekler: `backend/docs/public-api.yaml` (ve `backend/docs/public-api/*.md` giriş bölümleri), CF-195 kişi API'si ve dış kimlik, CF-196 özel alan kataloğu (`widget/usecase/attributes.go`), CF-197 sistem bağlama (`modules/integrations/systems`). Bu kodlar değişince kılavuz da değişir.
- Her adımın dört örneği (`widget` JS, Node, Go, curl) `sdks/examples/` altındadır ve sayfaya `sdks/examples/sync-docs.mjs` ile gömülür. Sayfadaki kod bloklarını elle düzenlemeyin; örneği değiştirip `cd sdks && pnpm examples:sync` çalıştırın. CI (`sdks` işi) `pnpm examples:check` ile sayfaların güncel olduğunu, `pnpm examples` ile örneklerin çalıştığını denetler.

## Örnekleri doğrulama

```sh
make guide-examples          # stub API ile: CI'daki koşu
make guide-examples-check    # yalnız derleme, tip kontrolü ve sayfa eşitliği
```

`CLICKFIX_BASE_URL` verilmezse `sdks/examples/stub-server.mjs` ayağa kalkar (anahtar, `Idempotency-Key`, JSON gövde, API kanalı HMAC denetimi, `public-api.yaml` şekilleri). Yerel backend'e karşı koşturmak için `sdks/examples/README.md` içindeki değişkenleri verin; API'nin kendisi değiştiğinde bunu yapın, stub ürünün yerini tutmaz. Mobil uygulama tarafı örnekleri (Flutter, React Native, Swift, Kotlin) kılavuzda sabit metindir ve otomatik teste girmez; bu kılavuzda açıkça söylenir.

## SDK yayını

| Paket | Sürüm nerede | Yayın |
|---|---|---|
| `@clickfix/node` | `sdks/node/package.json`, `CHANGELOG.md` | `main`'de `sdk-publish` işi: npm'de olmayan sürümü `npm publish --access public` ile yayımlar |
| `@clickfix/react-widget` | `sdks/react-widget/package.json`, `CHANGELOG.md` | aynı iş |
| `github.com/ClickSolveMainTR/clickfix-sdks/go` | `sdks/go/VERSION`, `CHANGELOG.md` | `release-tag` işi `go/vX.Y.Z` etiketini basar (yoksa); elle etiket basılmaz |
| `clickfix/laravel` (CF-205) | `sdks/laravel/VERSION`, `CHANGELOG.md` | `sdks-mirror` işi aynada `laravel/vX.Y.Z` etiketini basar ve `laravel/` dizinini `git subtree` ile herkese açık `ClickSolveMainTR/clickfix-laravel` deposuna (`vX.Y.Z`) ayırır; Packagist o depoyu izler. `LARAVEL_MIRROR_KEY` secret'ı yoksa ayırma notla atlanır. PR'da `laravel` işi Testbench matrisini (Laravel 10 × PHP 8.1-8.3, 11 ve 12 × 8.2-8.4, 13 × 8.3-8.4) koşar |
| `clickfix_webview` (pub.dev, CF-203) | `sdks/flutter/clickfix_webview/pubspec.yaml`, `CHANGELOG.md` | `sdk-publish` işi: pub.dev'de olmayan sürümü `dart pub publish --force` ile yayımlar; `PUB_CREDENTIALS` secret'ı yoksa atlar |

- PR'da `sdks` işi `npm pack --dry-run` ve `npm publish --dry-run` koşar; token gerekmez.
- PR'da `flutter` işi (`sdks/flutter/**` değişince) `dart analyze`, `dart format --set-exit-if-changed`, `flutter test`, örnek uygulamada `flutter build bundle` ve `dart pub publish --dry-run` koşar; token gerekmez. Dry-run paket kökünde `LICENSE` ister: dosya yokken adım uyarı verip atlar.
- Flutter yayını için **`PUB_CREDENTIALS`** repository secret'ı gerekir: pub.dev yayıncı hesabında `dart pub token add https://pub.dev` ile üretilen `~/.config/dart/pub-credentials.json` dosyasının içeriği (paketi yayımlama yetkisi olan hesap). Secret yoksa iş özet notuyla atlar, `main` kırılmaz. İlk yayın pub.dev'de paket adını o hesaba bağlar; paket `LICENSE` dosyası olmadan yayımlanamaz.
- `sdk-publish` için **`NPM_TOKEN`** repository secret'ı gerekir (npm'de `@clickfix` kapsamının sahibi hesabın *automation* token'ı; Settings → Secrets and variables → Actions). Secret yoksa iş özet notuyla atlar, `main` kırılmaz.
- Sürüm artırma: paketi değiştiren PR'da `package.json` / `VERSION` sürümünü yükseltin ve `CHANGELOG.md`'ye yazın; kılavuzun **Paketler** tablosundaki sürümleri de güncelleyin. `0.x`'te küçük sürüm kırıcı olabilir, günlükte **Kırıcı** başlığı zorunludur.
- npm kapsam adı: `@clickfix/*` paket adları npm'de boştur (`npm view` 404, kapsam aramasında paket yok); kapsamın bir organizasyon olarak alınıp alınmadığı hesap olmadan doğrulanamaz. Alınamazsa yedek ad `@getclickfix/*`; o durumda iki `package.json`, kılavuzun Paketler tablosu ve `sdks/README.md` birlikte değişir, başka yerde paket adı geçmez.
- Packagist (`clickfix/laravel`): paket adı boştur. Bir kez elle yapılacaklar: `ClickSolveMainTR/clickfix-laravel` adında boş herkese açık depo açmak, yazma yetkili deploy key üretip özel yarısını bu repoya `LARAVEL_MIRROR_KEY` secret'ı olarak koymak, ilk `sdks-mirror` koşusundan sonra depoyu Packagist'te `clickfix/laravel` olarak göndermek ve Packagist'in GitHub kancasını açmak. Sonrası otomatiktir: her yeni `sdks/laravel/VERSION` ayna ve ayrık depoda etiketlenir. `clickfix/laravel`, Packagist'teki `clickfix/sdk`'ya (CF-204) bağımlıdır; o paket yayımlanmadan `composer require clickfix/laravel` çözülmez.
- SDK'lar herkese açık ayna repodan sunulur: her etiketli `main` koşusunda `sdks-mirror` işi `sdks/` ağacını `ClickSolveMainTR/clickfix-sdks` reposuna iter ve paket etiketlerini (`node/vX`, `react-widget/vX`, `visitor-core/vX`, `go/vX`, `flutter/vX`, `laravel/vX`) orada basar (`SDKS_MIRROR_KEY` secret'ı; yoksa iş atlar). Go modül yolu `github.com/ClickSolveMainTR/clickfix-sdks/go`'dur; `go get` ayna repodan çalışır, ana repo private kalabilir. Kaynak her zaman ana repodaki `sdks/`; aynaya elle commit yapılmaz. Lisans: MIT (`sdks/LICENSE`, her pakette kopyası).

## Mobil karar

İssue #114 yorumu (9 Kas): mobil yol tam SDK değil, oturumu taşıyan güvenli webview. Köprü sözleşmesi [webview-bridge.md](./webview-bridge.md), uygulaması `frontend/widget-sdk/src/shared/bridge.ts` + `widget.ts`, barındırılan sayfa `frontend/public/widget-embed.html`, birim testi `frontend/widget-sdk/tests/bridge.test.mjs`. Yerel paketler (CF-201/202/203) bu sözleşmeyi kullanacak; bu işte Flutter paketi yazılmadı.
