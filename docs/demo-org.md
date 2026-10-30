# Demo organizasyonu (CF-134)

`seed-demo` komutu **Demo · Anadolu Yazılım** (`demo-anadolu-yazilim`) organizasyonunu oluşturur ya da yeniler.
Satış görüşmeleri, ekran görüntüleri ve uçtan uca testler (`frontend/e2e/app`) bu organizasyonu kullanır.

Tüm veriler kurgudur: kişi ve şirket adları uydurmadır, e-postalar `example.com` / `example.com.tr` alan adlarını,
telefonlar `+90 555 000 xx xx` aralığını kullanır. Gerçek müşteri verisi eklenmez.

## Ne oluşturur

| Alan | İçerik |
|------|--------|
| Kullanıcılar | 7 hesap, `@anadolu-yazilim.example.com`: sahip (Selin Arslan), admin, süpervizör, 4 temsilci |
| Ekipler | Teknik Destek, Müşteri Başarısı, Faturalandırma (üyeler, ekip liderleri, beceriler) |
| Müşteriler | 8 şirket (kademe, hesap sahibi, kurumsal SLA), 26 kişi, 8 bireysel müşteri |
| Kanallar | Destek e-postası, web sitesi sohbeti (widget), WhatsApp ve Instagram (demo amaçlı, kimlik bilgisi yok) |
| Konuşmalar | 120 konuşma, son 60 güne yayılmış; açık, beklemede, çözülmüş, kapanmış; etiketler, iç notlar, SLA hedefleri ve birkaç SLA ihlali |
| Ayarlar | Mesai saatleri, 2 SLA politikası, 8 etiket, 3 makro, 4 hazır yanıt, 4 kayıtlı görünüm |
| Bilgi bankası | 3 koleksiyon, 8 yayınlanmış makale (7'si herkese açık); yardım merkezi `/help/demo-anadolu-yazilim` |
| Anketler | CSAT puanları (çözülen konuşmaların yaklaşık üçte ikisi), NPS anketi ve yanıtları |
| Mühendislik | "Ürün" projesi, konuşmalara bağlı 4 kayıt |
| Hesap sağlığı | Ayar açılır; skorlar seed verisinden hesap sağlığı servisiyle hesaplanır |

Organizasyon, organizasyon usecase'i ile açılır. Bu yüzden sahip üyeliği ve deneme aboneliği (yayınlanmış bir
deneme planı varsa) normal kayıttaki gibi oluşur. Kurulum sihirbazı tamamlanmış olarak işaretlenir.

## Yenileme nasıl çalışır

- Komut tekrar çalıştırılabilir. Organizasyon varsa onun **içeriği silinip yeniden yazılır**: konuşmalar, kişiler,
  şirketler, kanallar, ekipler, ayarlar. Sonuç her seferinde aynı veri setidir (sabit rastgele tohum).
- Korunanlar: üyelikler ve roller, abonelik ve faturalama geçmişi, SSO / API anahtarları, olay günlüğü. Bilgi bankası
  makaleleri de korunur, çünkü sürümleri değiştirilemez; komut makaleleri slug ile günceller ve içerik değiştiyse
  yeni sürüm yayınlar.
- Komut yalnızca slug'ı `demo-anadolu-yazilim` **ve** adı `Demo · Anadolu Yazılım` olan organizasyona dokunur.
  Slug başka bir organizasyondaysa durur.
- Her çalıştırmada demo şifresi **yeniden üretilir**. Önceki şifre geçersiz olur.

Seed, iç notların `private=true`, `direction=internal`, `kind=note` olarak yazılmasını sağlar;
`#` yalnızca fixture işaretidir, mesaj gövdesine taşınmaz. Bu düzeltmeden önce seed edilmiş
veriyi yenilemek için `make seed-demo` tekrar çalıştırılmalıdır (demo şifresi döner).

Seed tamamlanırken mevcut analitik `Backfill` işlemi de çalışır; demo raporları worker
olmadan hazırdır. Canlı kullanımda `/v1/tenant/analytics/dashboard`, worker'ın olaylarla
güncellediği `analytics_conversations` projeksiyonunu okur; okuma sırasında hesaplama yapmaz.

## Yerelde

```sh
make seed-demo                                 # rastgele şifre üretir ve bir kez yazdırır
make seed-demo ARGS="-dev-catalog"             # plan kataloğu boşsa 30 günlük deneme planı yayınlar
DEMO_PASSWORD='Demo12345' make seed-demo       # şifreyi sabitler (E2E bunu kullanır)
make search-reindex                            # demo verisini Cmd+K aramasında görünür yapar
```

`-dev-catalog` yalnızca yerel ve CI veritabanları içindir. Katalogda yayınlanmış deneme planı yoksa "Demo
(geliştirme)" planını yayınlar; aksi halde hiçbir şey yapmaz. `APP_ENV=production` iken komut bu bayrağı reddeder.
Abonelik olmadan organizasyon salt okunur kalır.

`-conversations N` konuşma sayısını, `-json` ise çıktının JSON olarak basılmasını sağlar.

## Production'da

Komutu operatör, Dokploy'da **backend** konteynerinin terminalinden çalıştırır (create-super-admin ile aynı yer;
bkz. [deploy-dokploy.md](./deploy-dokploy.md)):

```sh
/app/seed-demo
/app/search-reindex
```

1. Çıktıdaki giriş bilgileri **bir kez** gösterilir ve hiçbir yerde saklanmaz. Şifreyi parola yöneticisine kaydedin.
   Bu dosyaya, repoya, sohbete veya bir itsplan maddesine yazmayın.
2. Demo hesapları gerçek kullanıcı gibi giriş yapar. Demo bitince komutu tekrar çalıştırarak şifreyi döndürün.
3. Deneme süresi bitince organizasyon salt okunur olur. Bitmeden önce Platform yönetimi → Organizasyonlar → Demo ·
   Anadolu Yazılım → faturalama sekmesinden deneme süresini uzatın (en fazla bir yıl sonrasına).
4. WhatsApp ve Instagram kanalları kimlik bilgisi içermez. Bu kanallardaki konuşmalara yazılan yanıtlar gönderilemedi
   olarak işaretlenir. Canlı demo için e-posta veya widget konuşmalarını kullanın.
5. Production'da `-dev-catalog` kullanılmaz. Planlar platform faturalama ekranından yönetilir.

## Kod

- `backend/cmd/seed-demo`: komut (Dockerfile'da `/app/seed-demo`).
- `backend/internal/demoseed`: veri (`data.go`), seed adımları (`seed.go`), yenileme silmesi (`wipe.go`), geliştirme
  kataloğu (`devcatalog.go`). Satırlar sqlc sorguları ve usecase'ler üzerinden yazılır. Ham SQL yalnızca eski demo
  içeriğini silmek ve konuşmaların tarihini geriye almak için kullanılır.
