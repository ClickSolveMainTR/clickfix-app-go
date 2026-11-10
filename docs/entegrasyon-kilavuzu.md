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
| `@getclickfix/visitor-core` | `sdks/visitor-core/package.json`, `CHANGELOG.md` | aynı iş (CF-201; react-widget'tan önce yayımlanır) |
| `@clickfix/react-widget` | `sdks/react-widget/package.json`, `CHANGELOG.md` | aynı iş |
| `clickfix_webview` (pub.dev, CF-203) | `sdks/flutter/clickfix_webview/pubspec.yaml`, `CHANGELOG.md` | `sdk-publish` işi: pub.dev'de olmayan sürümü `dart pub publish --force` ile yayımlar; `PUB_CREDENTIALS` secret'ı yoksa atlar |
| `github.com/ClickSolveMainTR/clickfix-sdks/go` | `sdks/go/VERSION`, `CHANGELOG.md` | `sdks-mirror` işi ayna repoda `go/vX.Y.Z` etiketini basar (yoksa); elle etiket basılmaz |

- PR'da `sdks` işi `pnpm publish --dry-run` koşar; token gerekmez. `npm publish` değil `pnpm publish` kullanılır: react-widget'ın `workspace:^` bağımlılığı (`@getclickfix/visitor-core`) ancak böyle gerçek sürüme çevrilir.
- PR'da `flutter` işi (`sdks/flutter/**` değişince) `dart analyze`, `dart format --set-exit-if-changed`, `flutter test`, örnek uygulamada `flutter build bundle` ve `dart pub publish --dry-run` koşar; token gerekmez. Dry-run paket kökünde `LICENSE` ister: dosya yokken adım uyarı verip atlar.
- Flutter yayını için **`PUB_CREDENTIALS`** repository secret'ı gerekir: pub.dev yayıncı hesabında `dart pub token add https://pub.dev` ile üretilen `~/.config/dart/pub-credentials.json` dosyasının içeriği (paketi yayımlama yetkisi olan hesap). Secret yoksa iş özet notuyla atlar, `main` kırılmaz. İlk yayın pub.dev'de paket adını o hesaba bağlar; paket `LICENSE` dosyası olmadan yayımlanamaz.
- `sdk-publish` için **`NPM_TOKEN`** repository secret'ı gerekir (npm'de `@clickfix` kapsamının sahibi hesabın *automation* token'ı; Settings → Secrets and variables → Actions). Secret yoksa iş özet notuyla atlar, `main` kırılmaz.
- Sürüm artırma: paketi değiştiren PR'da `package.json` / `VERSION` sürümünü yükseltin ve `CHANGELOG.md`'ye yazın; kılavuzun **Paketler** tablosundaki sürümleri de güncelleyin. `0.x`'te küçük sürüm kırıcı olabilir, günlükte **Kırıcı** başlığı zorunludur.
- npm kapsam adı: `@clickfix` org adı npm'de başkasına ait çıktığı için yeni paketler `@getclickfix/*` adıyla yayımlanır (`@getclickfix/visitor-core`, CF-201). `@clickfix/node` ve `@clickfix/react-widget` adları koordinatörün ayrı PR'ında `@getclickfix/*`'a taşınacak; o değişiklikte iki `package.json`, kılavuzun Paketler tablosu ve `sdks/README.md` birlikte değişir, başka yerde paket adı geçmez.
- SDK'lar herkese açık ayna repodan sunulur: her etiketli `main` koşusunda `sdks-mirror` işi `sdks/` ağacını `ClickSolveMainTR/clickfix-sdks` reposuna iter ve paket etiketlerini (`node/vX`, `react-widget/vX`, `visitor-core/vX`, `go/vX`, `flutter/vX`) orada basar; Packagist `composer.json`'ı repo kökünde istediği için `sdks/php` → `clickfix-php`, `sdks/laravel` → `clickfix-laravel` ayrı aynalardır (`vX.Y.Z`, sürüm CHANGELOG'daki en yeni başlık). Secret: `SDKS_MIRROR_TOKEN` (üç repoya Contents yazma yetkili fine-grained token; yoksa iş atlar). Go modül yolu `github.com/ClickSolveMainTR/clickfix-sdks/go`'dur; `go get` ayna repodan çalışır, ana repo private kalabilir. Kaynak her zaman ana repodaki `sdks/`; aynaya elle commit yapılmaz. Lisans: MIT (`sdks/LICENSE`, her pakette kopyası).

## Mobil karar

İssue #114 yorumu (9 Kas): mobil yol tam SDK değil, oturumu taşıyan güvenli webview. Köprü sözleşmesi [webview-bridge.md](./webview-bridge.md), uygulaması `frontend/widget-sdk/src/shared/bridge.ts` + `widget.ts`, barındırılan sayfa `frontend/public/widget-embed.html`, birim testi `frontend/widget-sdk/tests/bridge.test.mjs`. Yerel paketler (CF-201/202/203) bu sözleşmeyi kullanır. CF-201 köprüye `clickfix:session` (iki yönlü oturum devri) ekledi, çekirdek `@getclickfix/visitor-core` köprü tiplerini dışa aktarır ve bir test (`sdks/tests/visitor-core.test.mjs`) `widget.d.ts`, `shared/bridge.ts` ve çekirdeğin mesaj türlerini eşit tutar.
