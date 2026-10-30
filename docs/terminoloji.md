# ClickFix terminoloji sözlüğü

Ürünün her yüzeyinde (web paneli, mobil uygulama, e-posta ve bildirimler, yardım merkezi ve rehber içerikleri, dokümantasyon, herkese açık API başvurusu) aynı kavram aynı kelimeyle söylenir. Yeni metin yazarken önce bu tabloya bakın; tabloda olmayan bir kavram için yeni terim seçtiyseniz aynı PR'da buraya ekleyin.

Kaynak dosyalar:

- Web: `frontend/src/locales/{tr,en}/*.json`
- Mobil: `mobile/src/core/i18n/locales/{tr,en}/*.json`
- Backend (e-posta, bildirim, etkinlik kaydı): `backend/internal/platform/i18n/catalog.go`, modüllerdeki e-posta gövdeleri
- Dokümantasyon: `frontend/content/docs/{tr,en}/*.md`
- API: `backend/docs/openapi.yaml` (özetler İngilizce), `backend/docs/public-api.yaml` (üretilir, elle düzenlenmez)

## Sözlük

| Kavram | TR | EN | Nerede | Kullanılmayacak |
|---|---|---|---|---|
| Müşteriyle yürütülen destek kaydı | **Konuşma** | **Conversation** | Gelen kutusu, raporlar, SLA, API, bildirimler | TR: ticket, bilet, vaka, talep (panelde kayıt adı olarak). EN: ticket, case |
| Son müşterinin gözünden aynı kayıt | **Destek talebi** / talebiniz | **Support request** | Yalnızca son müşteriye giden metinler: yardım merkezi portalı, müşteri e-postaları, widget | Panelde "talep" ile konuşmayı kastetmek |
| Konuşmaların toplandığı ekran | **Gelen kutusu** | **Inbox** | Menü, kanallar, mobil | TR: inbox |
| Müşterinin yazdığı yer (WhatsApp, e-posta…) | **Kanal** | **Channel** | Kanallar sayfası, filtreler | TR: hat, bağlantı (kanal anlamında) |
| ClickFix'i kullanan şirketin hesabı | **Organizasyon** | **Organization** | Ayarlar, davetler, faturalama, erişim hataları | TR: işletme, çalışma alanı, kiracı, tenant. EN: workspace, tenant, account (organizasyon anlamında). İstisna: üçüncü tarafın kendi terimi (Slack çalışma alanı, Microsoft kiracı kimliği) |
| Organizasyondaki kullanıcı hesabı | **Üye** | **Member** | Üyeler, davetler, roller | TR: personel, çalışan, kullanıcı (organizasyon üyesi anlamında) |
| Konuşmayı yanıtlayan kişi | **Temsilci** | **Agent** | Atama, raporlar, gelen kutusu | TR: operatör, ajan (insan için), personel |
| Temsilci grupları | **Ekip** | **Team** | Ekipler, yönlendirme | TR: takım, grup, departman |
| Son müşterinin kaydı | **Kişi** | **Contact** | Kişiler sayfası, API | TR: kontak, müşteri kaydı |
| Kişilerin bağlı olduğu firma | **Şirket** | **Company** | Şirketler sayfası, müşteri sağlığı, gelir | TR: firma, hesap |
| Konuşmayı yanıtlayan yapay zekâ | **AI ajanı** | **AI agent** | AI ayarları, kanal ayarları, gelen kutusu | TR: bot, asistan (ajan anlamında), YZ. EN: bot |
| Yapay zekâ (düz yazıda) | **yapay zekâ** (kısa etiketlerde **AI**) | **AI** | Açıklama metinleri | TR: yapay zeka (şapkasız), Yapay Zeka |
| Temsilciye öneri sunan yardımcı | **Copilot** | **Copilot** | Gelen kutusu yan paneli | TR: yardımcı pilot |
| Hazır metin | **Hazır yanıt** | **Canned reply** | Yanıt kutusu, hazır yanıtlar sayfası | TR: şablon mesaj, kalıp yanıt |
| Birden çok eylemi tek tıkla uygulayan kayıt | **Makro** | **Macro** | Gelen kutusu, hazır yanıtlar sayfası | |
| Konuşmaya eklenen etiket | **Etiket** | **Tag** | Her yerde | TR: tag |
| Süre hedefleri | **SLA** (açıklamada "yanıt ve çözüm süresi hedefleri") | **SLA** | SLA politikaları, raporlar | |
| Bir şeyin yapılması | **Eylem** | **Action** | Kurallar, iş akışları, uygulamalar | TR: aksiyon |
| Olaya bağlı otomatik kural | **Otomasyon kuralı** | **Automation rule** | Kurallar sayfası | |
| Çok adımlı görsel akış | **İş akışı** | **Workflow** | İş akışları sayfası | TR: workflow, akış şeması |
| Hata ve geliştirme isteklerinin ürün ekibine iletilmesi | **Ürün talebi** (çoğul: ürün talepleri) | **Product request** | Mobil "Daha fazla", konuşma menüsü, iş akışı eylemi, etkinlik kaydı, plan özellikleri | TR: mühendislik kaydı, mühendislik sorunu, issue. EN: engineering issue, engineering record |
| Ürün taleplerinin açıldığı dış araç (Jira, Linear, GitHub) | **Ürün talebi aracı** | **Issue tracker** | Entegrasyonlar, iş akışı yardım metni | TR: mühendislik aracı, geliştirme aracı |
| Destek makaleleri | **Bilgi bankası**, **makale** | **Knowledge base**, **article** | Bilgi bankası, AI ajanı | TR: KB, wiki |
| Son müşterinin makaleleri okuduğu site | **Yardım merkezi** | **Help center** | Yardım merkezi ayarları | TR: destek portalı (yardım merkezi anlamında) |
| Panelde açılan dış araç | **Uygulama** | **App** | Menü "Uygulamalar", konuşma kenar çubuğu, kişi paneli | TR: eklenti, widget (uygulama anlamında), kenar çubuğu uygulaması (modül adı olarak) |
| Tek tıkla kurulan örnek uygulamalar | **Uygulama galerisi** | **App gallery** | Uygulamalar ayarları | TR: mağaza, market |
| Uygulamanın istediği izinler | **İzinler** (uygulama izinleri) | **Permissions** | Uygulama kartları ve formu | TR: kapsam (uygulamada) |
| Sunucudan sunucuya erişim anahtarı | **API anahtarı** | **API key** | Organizasyon → API ve otomasyonlar | TR: token, API token |
| API anahtarının erişim alanı | **Kapsam** | **Scope** | API anahtarı formu, API başvurusu | |
| Olay bildirimi gönderen adres | **Webhook** | **Webhook** | API, entegrasyonlar | TR: web kancası |
| Bir role verilen hak | **Yetki** | **Permission** | Roller, erişim hataları | TR: izin (rol yetkisi anlamında), permission |
| Bir şeye onay verme (bildirim, kamera, IP) | **İzin** | **Permission / allow** | Tarayıcı ve cihaz izinleri, IP izin listesi | |
| Kayıt kimliği | **Kimlik** (geliştirici ekranında "Kimlik (UUID)") | **ID** | Yalnızca geliştirici ve platform ekranları | TR: UUID (tek başına), id |
| Adres çubuğundaki kısa ad | **Adres** (geliştirici ekranında "Adres (slug)") | **URL name** / **slug** (yalnızca geliştirici ekranında) | Bilgi bankası, forum, organizasyon | TR: slug (tek başına) |
| Hesaba girme / çıkma | **Giriş yap** / **Çıkış yap** | **Sign in** / **Sign out** | Kimlik doğrulama ekranları | TR: login, oturum aç (istisna: "tek oturum açma (SSO)"). EN: log in, login (fiil olarak) |
| Hesaba yeniden kimlik doğrulama (hassas işlem öncesi) | **Yeniden doğrulama** | **Re-authentication** | Güvenlik, step-up iletişim kutuları | TR/EN: step-up |
| Silinen/eski sürümü geri getirme | **Geri yükle** | **Restore** | Sürüm geçmişi, çöp kutusu | TR: geri al (geri yükleme anlamında) |
| Tablo sütunu | **Sütun** | **Column** | Tablolar, içe/dışa aktarma | TR: kolon |
| Kapalı durumdaki kayıt | **Devre dışı** | **Disabled** | Listeler, anahtarlar | TR: pasif |
| Nöbet çizelgesinde geçici değişiklik | **Nöbet değişikliği** | **Override** | Nöbet | TR: override |
| Organizasyonun resmi kimlik bilgileri | **Resmi bilgiler** | **Legal details** | Organizasyon ayarları, faturalama | TR: şirket bilgileri (Şirket = müşteri şirketi ile karışır) |
| Temsilcinin uzmanlığı (yönlendirme) | **Beceri** | **Skill** | Yönlendirme, ekipler | TR: yetenek |
| Sınırlı lisanslı temsilci | **Sınırlı temsilci** | **Light agent** | Üyeler, faturalama | TR: hafif temsilci |
| Görüntülü odayı düzenleyen | **Düzenleyen** | **Host** | Görüntülü odalar | TR: host |
| İş akışı düzenleme alanı | **Tuval** | **Canvas** | İş akışları | TR: kanvas |
| Ekip içi yazışma | **Ekip sohbeti** | **Team chat** | Ekip sohbeti | TR: iç sohbet |
| Konuşmayı başka birine bırakma | **Devretmek** | **Hand over** | AI ajanı, atama | TR: escalate, eskale etmek |
| AI kullanım birimi | **Token** (yalnızca AI kullanım ve çalışma kayıtlarında) | **Token** | AI ayarları, kullanım | API anahtarı anlamında token |
| Üçüncü taraf kanal erişim bilgisi | **Erişim anahtarı (token)** | **Access token** | Kanal kurulumu | Tek başına "token" |
| Elektronik posta | **E-posta** | **Email** | Her yerde | EN: e-mail |
| Platformu işleten ekip | **Platform yönetimi**, **platform yöneticisi** | **Platform admin** | Erişim hataları, süper yönetici ekranları | TR: süper admin, backend |
| Ön ödemeli kullanım birimi (AI kredisi, görüntülü görüşme dakikası) | **Kredi**, **kredi paketi** (ürün olarak **kredi ürünü**) | **Credit**, **credit pack** (**credit product**) | Faturalama, platform kredi ürünleri | TR: jeton, kontör |
| Kredi bakiyesinin elle değiştirilmesi | **Kredi ekle / düş** (kayıtta **düzeltme**) | **Add / deduct credits** (**adjustment**) | Organizasyon cüzdanı | TR: manuel bakiye, override |
| Kredi bakiyesinin otomatik tamamlanması | **Otomatik yükleme** | **Auto top-up** | Faturalama | TR: oto şarj, auto-recharge |
| Kimin hesap açabileceği | **Kayıt**: **Kayıt açık** / **Davetle** / **Kapalı** | **Sign-up**: **Open** / **Invite only** / **Closed** | Platform ayarları, /register | TR: registration, self-serve |

## Yazım kuralları

### Düğmeler ve menüler

- Düğmeler kısa ve emir kipindedir (2. tekil): **Kaydet, Sil, Ekle, Oluştur, Gönder, Kopyala, Kur, Bağla, Yenile, Tekrar dene, İptal**.
- Aynı iş her yerde aynı fiille söylenir: oluşturma **Oluştur** (yeni kayıt) veya **Ekle** (listeye/kayda bir şey eklemek), değişikliği kaydetme **Kaydet**, kaldırma **Sil**, vazgeçme **İptal**. "Güncelle" düğmesi yerine **Kaydet**.
- Onay iletişim kutularında birincil düğme eylemi tekrar eder ("Sil", "İptal et"), "Evet/Tamam" yazılmaz.
- EN: imperative, sentence case — **Save, Delete, Add, Create, Send, Copy, Install, Connect, Refresh, Try again, Cancel**.

### Açıklamalar, ipuçları ve hatalar

- Yönergeler kibar çoğul kiple yazılır: "girin", "seçin", "deneyin". "Lütfen" yalnızca gerçekten ricada kullanılır.
- Başarı mesajı geçmiş zamanlı kısa cümledir: "Uygulama kaydedildi." "Başarıyla" eklenmez.
- Hata mesajı ne olduğunu ve ne yapılacağını söyler: "Uygulama yüklenemedi. Bağlantınızı kontrol edip tekrar deneyin." İç kod, HTTP durumu, uç nokta, "backend", "payload", "request" gibi geliştirici terimleri kullanıcıya gösterilmez.
- Boş durum, ekranın ne işe yaradığını ve ilk adımı söyler: "Henüz uygulama yok. Galeriden bir uygulama kurun veya kendi uygulamanızın adresini ekleyin."
- Yetki hatası: "… görüntüleme yetkiniz yok. Organizasyon yöneticinizden yetki isteyin."

### Değişkenler ve ekler

- `{{name}}`, `{{app}}`, `{{count}}` gibi değişkenlerden sonra Türkçe ek getirilmez (`{{name}}'a`, `{{name}}'in`); ek, değişkenin değerine göre yanlış olabilir. Cümle ekten bağımsız kurulur:
  - Yanlış: `{{name}} organizasyonuna katıldınız` → doğru: `Katıldığınız organizasyon: {{name}}` veya `"{{name}}" organizasyonuna` (organizasyon kelimesi eki taşır).
  - Yanlış: `{{name}}'e mesaj gönder` → doğru: `Mesaj gönder: {{name}}`.
- Sayı ve çoğul: Türkçede sayıdan sonra isim tekil kalır ("3 konuşma", "3 konuşmalar" değil). İngilizce için `_one` / `_other` anahtarları kullanılır.

### Tarih, saat ve sayı

- Tarih ve sayılar her zaman `Intl` ile, seçili dile göre biçimlenir (`tr-TR` / `en-US`); elle `GG.AA.YYYY` birleştirilmez.
- TR: `29 Eki 2026 14:30`, ondalık virgül (`1.234,5`), yüzde işareti başta (`%42`). EN: `Oct 29, 2026, 2:30 PM`, `1,234.5`, `42%`.
- Saat dilimi organizasyonun saat dilimidir; e-postalarda saat diliminin adı yazılır.

### Ürün ve marka adları

- ClickFix, WhatsApp, Instagram, Messenger, Telegram, Slack, Microsoft Teams, Google Chat, Jira, Linear, GitHub, Zapier, Make, n8n, Cal.com, Calendly yazıldığı gibi yazılır; Türkçe ek kesme işaretiyle eklenir ("WhatsApp'tan", "Slack'e").
- KVKK, SLA, SSO, SCIM, API, AI kısaltmaları büyük harflidir.

## Ürün talepleri (eski adıyla "Mühendislik") neden yeniden adlandırıldı?

"Mühendislik kayıtları/sorunları" ekip içi bir geliştirme kavramıdır; ClickFix kullanan bir destek ekibi ve onların müşterileri için anlamsızdır, ayrıca "sorun" kelimesi ürünün kendisinde hata varmış gibi okunur. Bu modül, müşteri konuşmalarından doğan **hata bildirimlerini ve geliştirme isteklerini** ürün ekibinin aracına (ClickFix'in kendi listesi, Jira, Linear, GitHub) taşır ve konuşmalarla bağlı tutar. Destek ekibinin gözünden bunlar müşteri adına ürüne iletilen **taleplerdir**; bu yüzden tek bir terim seçildi:

- TR: **Ürün talebi** / **Ürün talepleri**, EN: **Product request** / **Product requests**.
- "Hata ve geliştirme talepleri" uzun olduğu için yalnızca açıklama satırında kullanılır ("Konuşmalardan iletilen hata ve geliştirme talepleri").
- Kod içindeki `engineering` modül, izin ve olay adları değişmez (API ve veritabanı uyumluluğu); yalnızca görünen metinler değişir.
- `tenant.tickets.*` yetkileri ve `ticket.*` bildirim olayları bu modüle aittir; panelde "Ürün talepleri" olarak görünür. Özel alanlardaki `ticket` varlığı henüz hiçbir kayda bağlı değildir ve ayrı bir ürün kararı bekler.

## Dil notları

- EN metinlerde ABD yazımı kullanılır: "email", "color", "organization".
- Yüzde: TR `%25`, EN `25%`.
