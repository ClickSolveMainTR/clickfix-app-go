# Entegrasyon kılavuzu, SDK yayını ve mobil yol (CF-199)

Müşteri ekibine verilen tek entegrasyon kılavuzunun geliştirici tarafı: nerede durur, örnekleri nasıl doğrulanır, SDK'lar nasıl sürümlenir ve yayımlanır, mobil karar nedir.

## Kılavuz

- Yardım merkezi sayfası: `frontend/content/docs/tr/integration-guide/` ve `en/integration-guide/` (`/docs/integration-guide`, adım başına sayfa; paket tablosu `frontend/content/generated/packages.*.mdx` dosyasından `make sdks` ile üretilir). Bölüm sırası CF-198'deki kurulum sihirbazıyla aynıdır: müşteriyi tanıt (kimlik doğrulama, dış kimlik, özel alanlar, şirket, olaylar) → sistemini bağla → kanallar (API kanalı, webhook'lar) → mobil webview → AI / bilgi bankası / otomasyon (panel işi, bağlantı).
- Kaynak gerçekler: `backend/docs/public-api.yaml` (İngilizce; TR çevirisi ve örnekler `backend/docs/public-api/config.yaml` içinde, ortak kurallar `frontend/content/docs/*/api.mdx`), CF-195 kişi API'si ve dış kimlik, CF-196 özel alan kataloğu (`widget/usecase/attributes.go`), CF-197 sistem bağlama (`modules/integrations/systems`). Bu kodlar değişince kılavuz da değişir.
- Her adımın beş örneği (`widget` JS, Node, Go, PHP, curl) `sdks/examples/` altındadır ve sayfaya `sdks/examples/sync-docs.mjs` ile gömülür. Sayfadaki kod bloklarını elle düzenlemeyin; örneği değiştirip `cd sdks && pnpm examples:sync` çalıştırın. CI (`sdks` işi) `pnpm examples:check` ile sayfaların güncel olduğunu, `pnpm examples` ile örneklerin çalıştığını denetler. `sdks/examples/` herkese açık aynaya gitmez (`sdks-mirror` onu dışarıda bırakır ve aynada varsa siler, CF-212); çalıştırılabilir örnek uygulamalar (Node/Express, Next.js, React Native, Expo, Flutter, Go, PHP, Laravel, curl) yalnız herkese açık [`ClickSolveMainTR/clickfix-examples`](https://github.com/ClickSolveMainTR/clickfix-examples) reposundadır ve orada kendi CI'ı ile yayımlanmış paketlere karşı derlenir.

## Örnekleri doğrulama

```sh
make guide-examples          # stub API ile: CI'daki koşu
make guide-examples-check    # yalnız derleme, tip kontrolü ve sayfa eşitliği
```

`CLICKFIX_BASE_URL` verilmezse `sdks/examples/stub-server.mjs` ayağa kalkar (anahtar, `Idempotency-Key`, JSON gövde, API kanalı HMAC denetimi, `public-api.yaml` şekilleri). Yerel backend'e karşı koşturmak için `sdks/examples/README.md` içindeki değişkenleri verin; API'nin kendisi değiştiğinde bunu yapın, stub ürünün yerini tutmaz. Mobil uygulama tarafı örnekleri (Flutter, React Native, Swift, Kotlin) kılavuzda sabit metindir ve otomatik teste girmez; bu kılavuzda açıkça söylenir.

## SDK yayını

| Paket | Sürüm nerede | Yayın |
|---|---|---|
| `@getclickfix/node` | `sdks/node/package.json`, `CHANGELOG.md` | `main`'de `sdk-publish` işi: npm'de olmayan sürümü `npm publish --access public` ile yayımlar |
| `@getclickfix/visitor-core` | `sdks/visitor-core/package.json`, `CHANGELOG.md` | aynı iş (CF-201; react-widget'tan önce yayımlanır) |
| `@getclickfix/react-widget` | `sdks/react-widget/package.json`, `CHANGELOG.md` | aynı iş |
| `@getclickfix/react-native` | `sdks/react-native/package.json`, `CHANGELOG.md` | aynı iş (visitor-core'dan sonra) |
| `@getclickfix/expo` | `sdks/expo/package.json`, `CHANGELOG.md` | aynı iş (react-native'den sonra) |
| `clickfix/laravel` (CF-205) | `sdks/laravel/CHANGELOG.md` (en yeni `## X.Y.Z` başlığı sürümdür) | `sdks-mirror` işi (#144) `sdks/laravel`'i herkese açık `ClickSolveMainTR/clickfix-laravel` deposuna iter ve orada `vX.Y.Z` etiketler; Packagist o depoyu izler. PR'da `laravel` işi `composer validate` ve Testbench matrisini (Laravel 10 × PHP 8.1-8.3, 11 ve 12 × 8.2-8.4, 13 × 8.3-8.4) koşar |
| `clickfix_webview` (pub.dev, CF-203) | `sdks/flutter/clickfix_webview/pubspec.yaml`, `CHANGELOG.md` | `sdk-publish` işi: pub.dev'de olmayan sürümü `dart pub publish --force` ile yayımlar; `PUB_CREDENTIALS` secret'ı yoksa atlar |
| `github.com/ClickSolveMainTR/clickfix-sdks/go` | `sdks/go/VERSION`, `CHANGELOG.md` | `sdks-mirror` işi ayna repoda `go/vX.Y.Z` etiketini basar (yoksa); elle etiket basılmaz |
| `clickfix/sdk` (PHP, CF-204) | `sdks/php/CHANGELOG.md` en üst girdi (ve `Client::USER_AGENT`; `composer.json`'a `version` yazılmaz, `composer validate --strict` buna uyarı verir) | `sdks-mirror` işi (#144) `sdks/php`'yi herkese açık `ClickSolveMainTR/clickfix-php` reposuna iter ve orada `vX.Y.Z` etiketini basar; Packagist (`clickfix/sdk`) o repoyu izler |

- PR'da `sdks` işi `pnpm publish --dry-run` koşar; token gerekmez. `npm publish` değil `pnpm publish` kullanılır: react-widget'ın `workspace:^` bağımlılığı (`@getclickfix/visitor-core`) ancak böyle gerçek sürüme çevrilir.
- PR'da `flutter` işi (`sdks/flutter/**` değişince) `dart analyze`, `dart format --set-exit-if-changed`, `flutter test`, örnek uygulamada `flutter build bundle` ve `dart pub publish --dry-run` koşar; token gerekmez. Dry-run paket kökünde `LICENSE` ister: dosya yokken adım uyarı verip atlar.
- Flutter yayını için **`PUB_CREDENTIALS`** repository secret'ı gerekir: pub.dev yayıncı hesabında `dart pub token add https://pub.dev` ile üretilen `~/.config/dart/pub-credentials.json` dosyasının içeriği (paketi yayımlama yetkisi olan hesap). Secret yoksa iş özet notuyla atlar, `main` kırılmaz. İlk yayın pub.dev'de paket adını o hesaba bağlar; paket `LICENSE` dosyası olmadan yayımlanamaz.
- PHP yayını `sdks-mirror` işinden olur (`SDKS_MIRROR_TOKEN` secret'ı). Packagist `composer.json`'ı repo kökünde ve `vX.Y.Z` etiketlerini ister; bu yüzden `sdks/php` ayrı `clickfix-php` reposuna itilir. Packagist'te `clickfix/sdk` adı boştur; paketi `https://github.com/ClickSolveMainTR/clickfix-php` adresiyle bir kez `packagist.org/packages/submit` ile kaydedip GitHub hook'unu açın; sonraki etiketler kendiliğinden gelir.
- `sdk-publish` için **`NPM_TOKEN`** repository secret'ı gerekir (npm'de `@clickfix` kapsamının sahibi hesabın *automation* token'ı; Settings → Secrets and variables → Actions). Secret yoksa iş özet notuyla atlar, `main` kırılmaz.
- Sürüm artırma: paketi değiştiren PR'da `package.json` / `VERSION` sürümünü yükseltin ve `CHANGELOG.md`'ye yazın; kılavuzun **Paketler** tablosu (`frontend/content/generated/packages.*.mdx` ve `packages.json`) bu dosyalardan `make sdks` ile üretilir, `make sdks-check` eskiyse kırmızı yanar. Sürüm elle hiçbir sayfaya yazılmaz. `0.x`'te küçük sürüm kırıcı olabilir, günlükte **Kırıcı** başlığı zorunludur.
- npm kapsam adı: `@clickfix` org adı npm'de başkasına ait çıktığı için yeni paketler `@getclickfix/*` adıyla yayımlanır (`@getclickfix/visitor-core`, CF-201). `@getclickfix/node` ve `@getclickfix/react-widget` adları koordinatörün ayrı PR'ında `@getclickfix/*`'a taşınacak; o değişiklikte iki `package.json`, kılavuzun Paketler tablosu ve `sdks/README.md` birlikte değişir, başka yerde paket adı geçmez.
- Packagist (`clickfix/laravel`): paket adı boştur. Bir kez elle yapılacak: ilk `sdks-mirror` koşusundan sonra `ClickSolveMainTR/clickfix-laravel` deposunu Packagist'te `clickfix/laravel` olarak göndermek ve GitHub kancasını açmak. Depo ve itme/etiketleme `sdks-mirror` işinde tanımlıdır (#144); Laravel için ayrı yayın işi veya secret yoktur. `clickfix/laravel`, Packagist'teki `clickfix/sdk`'ya (CF-204) bağımlıdır; o paket yayımlanmadan `composer require clickfix/laravel` çözülmez.
- SDK'lar herkese açık ayna repodan sunulur: her etiketli `main` koşusunda `sdks-mirror` işi `sdks/` ağacını (`examples/` hariç) `ClickSolveMainTR/clickfix-sdks` reposuna iter ve paket etiketlerini (`node/vX`, `visitor-core/vX`, `react-widget/vX`, `react-native/vX`, `expo/vX`, `go/vX`, `flutter/vX`) orada basar; PHP ve Laravel kendi aynalarında `vX.Y.Z` alır (`SDKS_MIRROR_TOKEN` secret'ı; yoksa iş atlar). Go modül yolu `github.com/ClickSolveMainTR/clickfix-sdks/go`'dur; `go get` ayna repodan çalışır, ana repo private kalabilir. Kaynak her zaman ana repodaki `sdks/`; aynaya elle commit yapılmaz. Lisans: MIT (`sdks/LICENSE`, her pakette kopyası).

## Herkese açık metinler (CF-212)

Aynalara giden her şey (`sdks/` altında `examples/` dışındaki dosyalar: README, CHANGELOG, kaynak yorumları, üretilen istemciler) ve yardım merkezi sayfaları (`frontend/content/docs`, `frontend/content/generated`, `frontend/public/widget.d.ts`, `backend/docs/public-api.yaml`) iç takip numarası (`CF-` + sayı) içermez; aynaya giden Markdown İngilizcedir (`sdks/**/README.md`, `sdks/**/CHANGELOG.md`). `sdks/scripts/public-text.mjs` bunu denetler ve `pnpm check` içinde çalışır (CI `sdks` işi, `make sdks-check`). Üretilen Go ve TypeScript istemcileri OpenAPI açıklamalarındaki referansları `scripts/public-api.mjs` içindeki `scrub` ile temizlenmiş haliyle alır; `openapi.yaml`'daki iç notlar kalabilir.

## Mobil karar

İssue #114 yorumu (9 Kas): mobil yol tam SDK değil, oturumu taşıyan güvenli webview. Köprü sözleşmesi [webview-bridge.md](./webview-bridge.md), uygulaması `frontend/widget-sdk/src/shared/bridge.ts` + `widget.ts`, barındırılan sayfa `frontend/public/widget-embed.html`, birim testi `frontend/widget-sdk/tests/bridge.test.mjs`. Yerel paketler (CF-201/202/203) bu sözleşmeyi kullanır. CF-201 köprüye `clickfix:session` (iki yönlü oturum devri) ekledi, çekirdek `@getclickfix/visitor-core` köprü tiplerini dışa aktarır ve bir test (`sdks/tests/visitor-core.test.mjs`) `widget.d.ts`, `shared/bridge.ts` ve çekirdeğin mesaj türlerini eşit tutar.
