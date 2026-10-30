# Pazarlama & izleme (CF-144)

Tanıtım sitesinin (`/`, `/features`, `/compare`, `/security`, `/contact`, `/docs`, `/legal`) etiketleri, SEO varsayılanları ve kayıt kaynakları. Platform yöneticisi yönetir, yeniden dağıtım gerekmez.

## Yönetim

- **Sistem ayarları → Pazarlama → Pazarlama & izleme** (`/platform/settings/marketing`, `platform.settings.read` / `write`):
  GTM, GA4, Google Ads dönüşüm kimliği + kayıt etiketi, Meta Pixel, LinkedIn Insight, TikTok Pixel, Microsoft Clarity, Hotjar; site başlığı / açıklaması; Google / Bing / Yandex doğrulama kodları; varsayılan paylaşım görseli; `<head>` ve `<body>` için özel kod.
- **Sistem ayarları → Pazarlama → Kaynaklar** (`/platform/settings/marketing/sources`, `platform.organizations.read`): seçilen tarihlerde açılan organizasyonlar, kaynak / ortam / kampanya kırılımıyla.
- Organizasyon detayında **Kayıt kaynağı** kartı: ilk ve son temas.

## Akış

1. Next.js `(landing)` layout'u `GET /v1/public/marketing`'i 5 dakika önbellekle okur (`marketing-settings` etiketi). Yönetici kaydettiğinde BFF etiketi geçersiz kılar; değişiklik hemen yayına girer.
2. Ziyaretçi çerez kategorilerini seçmeden (zorunlu / analitik / pazarlama) hiçbir üçüncü taraf betiği yüklenmez. Google etiketleri Consent Mode v2 sinyalleriyle (`default` denied, ardından `update`) yüklenir. Seçim `cf_consent` çerezinde 180 gün tutulur; altbilgideki "Çerez tercihleri" bağlantısı tercihleri yeniden açar. Bir kategoriden vazgeçilirse sayfa yeniden yüklenir.
3. UTM (`utm_source/medium/campaign/term/content`), `gclid` / `fbclid` / `msclkid`, dış yönlendiren ve ilk sayfa: analitik izni varsa `cf_ft` (ilk temas, 180 gün, üzerine yazılmaz) ve `cf_lt` (son temas, 30 gün) çerezlerine yazılır. İzin gelene kadar sekmede (`sessionStorage`) bekler.
4. Kayıt (`POST /v1/auth/register`) ve organizasyon açılışı (`POST /v1/onboarding/organizations`) BFF üzerinden geçerken çerezler `X-ClickFix-Attribution` başlığıyla API'ye gider. API'deki `marketing.CaptureSignup` ara katmanı 201 yanıtında `signup_attributions` tablosuna kullanıcı / organizasyon satırını yazar; başlık yoksa organizasyon sahibinin kayıt kaynağını kopyalar.
5. Başarılı kayıttan sonra BFF `cf_conv` çerezini koyar; sonraki sayfada `SignupConversion` (izin varsa) dataLayer `sign_up`, GA4 `sign_up`, Google Ads dönüşümü ve Meta / TikTok `CompleteRegistration` olaylarını bir kez gönderir.

## Güvenlik

- Özel kod parçaları API'de temizlenir: yalnızca `script`, `noscript`, `style`, `meta`, `link`, `img`, `iframe`; `src` / `href` yalnızca https; `on*` işleyicileri ve yorumlar silinir; en fazla 20 KB. Yalnızca `platform.settings.write` yetkisi kaydedebilir, değişiklikler etkinlik kaydına düşer.
- Etiketler yalnızca tanıtım sayfalarında yüklenir; organizasyonların yardım merkezi ve randevu sayfaları hiçbirini almaz. Uygulamaya geçiş (giriş, kayıt) tam sayfa yüklemesiyle olur, etiketler uygulama kabuğunda çalışmaya devam etmez (kayıt dönüşümü dışında).
- CSP: bugün yalnızca `frame-ancestors` ve `base-uri` zorlanır. İleride `script-src` eklenirse şu alanlar izinli olmalıdır: `www.googletagmanager.com`, `www.google-analytics.com`, `connect.facebook.net`, `snap.licdn.com`, `analytics.tiktok.com`, `www.clarity.ms`, `static.hotjar.com` ve özel kodların kullandığı adresler.
- Çerez politikası metni (`/legal/cookies`) hukuk tarafından yukarıdaki kategori ve araçlarla tamamlanmalıdır.
