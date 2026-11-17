# Bildirimler (CF-9, CF-157)

Bu belge ClickFix'te bildirim üreten her olayı, hangi kanaldan gittiğini, metnini ve
tercih kurallarına uyup uymadığını listeler. Kod değişince tablo da güncellenir.

Durum: CF-157 PR'ları (#179 backend içerik modeli, #180 durum sayfası onay sayfası,
#181 web push aksiyonları, #182 mobil push aksiyonları) birleştikten sonraki hâl.
Bir satır sadece bir PR ile gerçekleşiyorsa "PR" sütununda belirtilir.

## Mimari özeti

```
üretici (modül / outbox tüketicisi)
  └─ notifications.Service.Enqueue / NotifyEvent
       ├─ tercih: ana anahtarlar → olay matrisi → sessiz saat → e-posta özeti
       ├─ içerik: content.Build(olay, dil, bilgiler)  → başlık, ön metin, detay, CTA, push metni
       ├─ idempotency: payload.source_event_id (uq_notifications_source_event_channel)
       └─ satır başına teslim (Asynq notifications kuyruğu)
            ├─ inapp → realtime (Centrifugo) + push aynası: web push (VAPID) + Expo
            ├─ email → SMTP (CF-155 markalı layout, #176)
            └─ sms   → kanal sağlayıcısı (şimdilik noop)
```

- Kod: `backend/internal/modules/notifications/` (`content/` içerik, `usecase/preferences.go`
  katalog ve kanal planı, `usecase/expo.go` mobil push, `usecase/push.go` web push,
  `consumer.go` outbox tüketicisi).
- Üye bildirimleri her zaman **in-app satırı** üzerinden gider; push o satırın aynasıdır.
  Ayrı `push` kanalı açmak çift push demektir (randevu host bildiriminde böyle bir hata
  vardı, CF-157'da düzeltildi).

## Tercih kuralları (her kanalda aynı)

| Kural                                         | In-app                                         | Push (web + mobil)                    | E-posta                                            |
| --------------------------------------------- | ---------------------------------------------- | ------------------------------------- | -------------------------------------------------- |
| Ana anahtar (`*_enabled`)                     | `inapp_enabled`                                | `push_enabled` (+ in-app açık olmalı) | `email_enabled`                                    |
| Olay matrisi (`event_channels`)               | Katalog olayları; kilitli kanallar kapatılamaz | Katalog olayları                      | Katalog olayları                                   |
| Sessiz saat (`quiet_hours`)                   | Satır oluşur, okunmamış sayısı artar           | Gönderilmez                           | Sessiz saat bitince gönderilir                     |
| Kritik öncelik (`critical`)                   | —                                              | Sessiz saati deler                    | Sessiz saati ve özeti deler                        |
| E-posta özeti (`email_digest` saatlik/günlük) | —                                              | —                                     | Kritik olmayan katalog e-postaları özette toplanır |
| Güvenlik e-postası (`SecurityEmail`)          | —                                              | —                                     | Tercihlerden bağımsız her zaman gider              |

- Katalog dışı kaynaklar (durum sayfası, faturalama, auth) olay matrisinde görünmez ama
  **sessiz saate uyar** (CF-157; öncesinde uymuyordu).
- Alıcı olarak sadece e-posta adresi olan dış kişilere (müşteri, davetli, abone) giden
  e-postalarda üye tercihi yoktur; bunlar kendi abonelik/iptal akışlarına tabidir.
- Kişi kendi yaptığı işlem için bildirim almaz (aktör alıcılardan çıkarılır).

## İçerik kuralları

Her bildirim şu soruları cevaplar: **kim** (aktör / müşteri), **ne** (olay), **hangi
müşteri / konuşma** (ad, kanal, konuşma no, konu), **ne zaman** (organizasyonun saat
diliminde, saat dilimi adıyla), **ne yapılmalı** (birincil CTA).

- Dil: alıcının dili (`users.locale`), yoksa Türkçe. Tüm içerik tr/en
  (`backend/internal/modules/notifications/content/events.go`).
- Mesaj önizlemesi HTML'den arındırılır, boşluklar birleştirilir, kelime sınırında
  kısaltılır (`content.Preview`; 200–280 karakter).
- SLA: kalan süre ("25 dk kaldı") ya da gecikme ("Süre doldu (1 sa 30 dk)").
- E-posta düz metni CF-155 layout'unun okuduğu biçimdedir: ön metin paragrafı,
  `Etiket: değer` detay satırları (detay tablosu) ve `Etiket: https://…` bağlantıları.
  Birincil CTA satırın `action_url`'üdür (layout'ta düğme).
- **Kilit ekranı:** push metni (web ve mobil) sadece olay türünü ve zararsız
  referansları taşır: kanal, konuşma no, saat, sayı. Müşteri adı, mesaj metni, not,
  tutar, telefon numarası **asla** push'a girmez. In-app başlık/gövde push'a kopyalanmaz.
  Uygulamada ayrıca bir "push'ta içerik göster" ayarı yoktur; işletim sisteminin
  "önizlemeleri gizle" ayarı ek olarak uygulanır. Kaynak: `content.Message.Push` veya
  `pushtext` kataloğu.

## Aksiyonlar ve derin linkler

| Yer        | Aksiyonlar                                                                                                         | Uygulama                                                                                  |
| ---------- | ------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------- |
| E-posta    | Birincil CTA + gerekirse ikincil bağlantı (ör. "Görüşmeye katıl")                                                  | Linkler sayfa açar; durum değiştiren işlem sayfadaki düğmeyle (POST) yapılır              |
| Web push   | "Aç", "Okundu" (en çok 2)                                                                                          | `frontend/public/app-push-sw.js`; tıklama açık sekmeyi hedefe götürür ve okundu işaretler |
| Mobil push | `conversation`: Yanıtla, Okundu, Aç · `conversation_assign`: Bana ata, Yanıtla, Okundu, Aç · `generic`: Okundu, Aç | `mobile/src/features/push/`; Android'de kapalı uygulamada arka plan görevi                |

- Web linkleri: `FRONTEND_URL` + panel yolu (üretimde `https://getclickfix.com/...`).
  Göreli yollar `WithAppURL` ile mutlak yapılır; `//host` ve `javascript:` gibi değerler
  atılır.
- Mobil: push `data.url` panel yolunu taşır, `mobile/src/features/push/links.ts` uygulama
  rotasına çevirir; push'un organizasyonu önce seçilir. Şema `clickfix://`.
- **ClickFix Masaüstü (CF-236):** etkin bir masaüstü oturumu olan kullanıcının
  (`refresh_tokens.client = 'desktop'`) bildirim e-postalarına, birincil eylem uygulama içi
  bir yolsa "Masaüstünde aç" bağlantısı eklenir:
  `https://<kök>/desktop/open?path=<yol>`. Sayfa uygulamayı `clickfix://` ile açar ve
  tarayıcıda devam etme bağlantısı bırakır (e-posta istemcileri özel şemalı bağlantıları
  siler). In-app satırı ve push değişmez. Masaüstü uygulamasında web push aboneliği açılmaz;
  canlı in-app bildirimi uygulamanın yerel bildirimine dönüşür (pencere öndeyse gösterilmez).
- **Açık iş:** e-postadaki `https://getclickfix.com/...` linklerinin uygulamada açılması
  (universal link / App Links) yapılandırılmadı: iOS `associatedDomains`, Android
  `intentFilters` (autoVerify), web'de `/.well-known/apple-app-site-association` ve
  `assetlinks.json` gerekir. Apple Team ID ve imza parmak izi ister, native build gerekir.
- **Kimlik doğrulama / imza:** üye linkleri oturum ister. `/v1/notifications/{uuid}/action`
  HMAC imzalı ve Bearer kimliklidir. Dış kişilere giden linkler imzalı tokenlıdır (davet,
  randevu yönetimi, görüntülü görüşme, durum sayfası).
- **GET ile durum değişikliği yok:** link önizleme botları ve e-posta tarayıcıları linkleri
  açar. Durum sayfası abonelik linki önceden GET ile onaylıyor/iptal ediyordu; #180 ile
  onay sayfasına (`/status/{slug}/subscription/{token}`) gidiyor, işlem `POST` ile
  yapılıyor. Diğer e-posta linkleri zaten sayfa açıyor (davet kabulü, randevu iptali ve
  yeniden planlama sayfada POST). BFF cross-site POST'ları engeller.

## Güvenilirlik

- **Idempotency:** `EnqueueInput.DedupeKey` → `payload.source_event_id`, benzersiz indeks
  `(user_id, channel, source_event_id)`. Aynı olay tekrar gelirse mevcut satır döner,
  yeniden gönderilmez. Outbox tüketicisi olay kimliğini kullanır. Yeni mesaj bildirimi
  konuşma başına 5 dakikalık pencereyle anahtarlanır (mesaj seline karşı). Not:
  `user_id` olmayan (dış alıcı) e-postalarda indeks çalışmaz; bunlar üreticide
  korunur (randevu hatırlatması `AppointmentMarkReminder`, durum aboneliği tokenı).
- **Yeniden deneme:** Asynq teslim görevi; geri çekilmeli yeniden deneme ve kalıcı hatada
  kayıt (dead-letter, `notification_history`) #172 (CF-153) ile geliyor. Expo push ayrı
  kuyrukta yeniden denenir; `DeviceNotRegistered` cihazı devre dışı bırakır.
- **Ayrılmış alan adları (CF-226):** RFC 2606 alan adlarındaki alıcılara (`example.com`,
  `example.net`, `example.org` ve alt alan adları; `*.test`, `*.invalid`, `*.localhost`,
  `*.example`) e-posta denenmez. Satır `cancelled` durumuna alınır, `last_error`
  `skipped: reserved_domain` olur ve geçmişe `skipped` olayı yazılır. Bu bir hata değildir:
  yeniden denenmez, Sentry'ye gitmez, yalnız `notification_skipped` log satırı düşer.
  Sadece `APP_ENV=development` iken (MailHog her adresi kabul eder) bu adreslere de gönderilir.
- **Kalıcı sağlayıcı reddi:** SMTP 5xx (ör. Resend `550 Invalid to`) yeniden denenmez, satır
  hemen dead-letter olur. Sentry'ye hata değil **uyarı** olarak, kanal ve neden başına saatte
  en fazla bir kez düşer (`notification-recipient-rejected` parmak izi, `source` etiketi satırın
  olay/şablon kodu). Yapılandırma, TLS ve kimlik doğrulama hataları hata olarak kalır; ağ,
  zaman aşımı ve 4xx gibi geçici hatalar eskisi gibi geri çekilmeli yeniden denenir.
- **Demo ve seed verisi (karar, CF-226):** demo/seed kullanıcılarının bildirim e-postaları
  ayrıca kapatılmaz. Bu kullanıcılar ayrılmış alan adlarını kullandığı sürece gönderim
  zaten atlanır ve in-app/push bildirimleri çalışmaya devam eder (demo akışı bozulmaz).
  Yeni seed ve örnek verisi yalnız ayrılmış alan adlarını (`example.com`, `*.test`) kullanır.
  `seed-demo`'daki `example.com.tr` adresleri ayrılmış değildir, gerçek bir alana gidebilir;
  bunların `example.com`'a çekilmesi ayrı bir iş olarak izlenir.
- **Takılan satırlar:** `ReclaimStuck` işlemde kalan satırları yeniden teslime verir.
- **Özet:** `ProcessDigests` saatlik/günlük özet e-postası gönderir (kritik olmayan katalog
  e-postaları). Düşük öncelikli olaylar (CSAT, iş tamamlandı) varsayılan olarak e-posta
  göndermez; kullanıcı açarsa özet ayarına tabidir.

## Envanter

Kısaltmalar: **K** = olay matrisinde (katalog), varsayılanlar `in-app / e-posta / push`;
🔒 kilitli kanal; **SS** = sessiz saat; **T** = teslim tekrarına karşı idempotency.

### Gelen kutusu ve konuşmalar (üye)

| Olay                        | Üretici                                                              | Alıcı                       | Kanallar (varsayılan)                       | Başlık (tr) / içerik                                                                                                                                                            | CTA                                 | Push (kilit ekranı)                                                                                   | Tercih / SS / T                        |
| --------------------------- | -------------------------------------------------------------------- | --------------------------- | ------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------- | ----------------------------------------------------------------------------------------------------- | -------------------------------------- |
| `conversation.assigned`     | `inbox/live.go` `MentionNotifier.Assigned` (atama anında)            | Atanan (aktör hariç)        | in-app 🔒 / e-posta ✓ / push ✓ · yüksek     | "Konuşma size atandı · #42 · Deniz Kaya"; "Ali Veli, Deniz Kaya ile olan #42 numaralı konuşmayı size atadı."; son mesaj alıntısı; Müşteri, Kanal, Konuşma, Konu, SLA kalan süre | Konuşmayı aç                        | "Konuşma size atandı" / "WhatsApp · #42"; kategori `conversation` (atanmamışsa `conversation_assign`) | K ✓ / SS ✓ / T — (tek senkron çağrı)   |
| `message.received`          | outbox `messages.received` → `notifications/consumer.go`             | Konuşmanın atananı          | in-app / e-posta ✗ / push ✓ · normal        | "Deniz Kaya yeni mesaj gönderdi · #42"; mesaj alıntısı; aynı detaylar                                                                                                           | Yanıtla                             | "Yeni mesaj" / "WhatsApp · #42"                                                                       | K ✓ / SS ✓ / T ✓ (konuşma başına 5 dk) |
| `conversation.mentioned`    | `MentionNotifier.Mentioned` (not yazılınca)                          | Bahsedilenler (yazar hariç) | in-app 🔒 / e-posta ✓ / push ✓ · yüksek     | "Ali Veli sizden bahsetti · #42"; not alıntısı; detaylar                                                                                                                        | Konuşmayı aç                        | "Sizden bahsedildi" / "WhatsApp · #42"                                                                | K ✓ / SS ✓ / T —                       |
| `conversation.reminder_due` | `conversations/usecase/followups.go` → `MentionNotifier.ReminderDue` | Hatırlatıcıyı kuran         | in-app / e-posta ✗ / push ✓                 | "Hatırlatıcı · #42"; hatırlatıcı notu                                                                                                                                           | Konuşmayı aç                        | "Hatırlatıcı" / "WhatsApp · #42"                                                                      | K ✓ / SS ✓ / T —                       |
| `sla.warning`               | `sla/usecase/clocks.go` → tüketici                                   | Atanan                      | in-app / e-posta ✗ / push ✓ · yüksek        | "SLA süresi yaklaşıyor · #42 · 25 dk kaldı"; Hedef, Son tarih (org saat dilimi)                                                                                                 | Konuşmayı aç                        | "SLA süresi yaklaşıyor" / "WhatsApp · #42 · 25 dk kaldı"                                              | K ✓ / SS ✓ / T ✓                       |
| `sla.breached`              | aynı                                                                 | Atanan                      | in-app 🔒 / e-posta ✓ / push ✓ · **kritik** | "SLA ihlal edildi · #42"; "…ilk yanıt hedefi aşıldı."; gecikme süresi                                                                                                           | Konuşmayı aç                        | "SLA ihlal edildi" / "WhatsApp · #42"                                                                 | K ✓ / SS deler / T ✓                   |
| `csat.submitted`            | outbox `surveys.responded` (kind=csat) → tüketici                    | Atanan                      | in-app / e-posta ✗ / push ✗ · düşük         | "Yeni memnuniyet puanı: 2/5 · #42"; yorum alıntısı                                                                                                                              | Konuşmayı aç                        | "Yeni memnuniyet puanı" / "Bir müşteri 2/5 puan verdi."                                               | K ✓ / SS ✓ / T ✓                       |
| `calls.missed`              | `inbox/netgsm.go`                                                    | Hattın üyeleri              | in-app / e-posta ✗ / push ✓ · yüksek        | "Cevapsız arama"; Hat; arayan numarası metne girmez                                                                                                                             | Aramaları aç                        | "Cevapsız arama" / "Ses kanalınızda cevapsız bir arama var."                                          | K ✓ / SS ✓ / T ✓ (çağrı kimliği)       |
| `teamchat.mentioned`        | `supportoperations/events.go` (`teamchat.message_created`)           | Bahsedilen ekip üyeleri     | in-app / e-posta ✗ / push ✓                 | "Ekip sohbetinde sizden bahsedildi"; "Ali Veli ekip sohbetinde sizden bahsetti."                                                                                                | — (web'de ekip sohbeti sayfası yok) | "Sizden bahsedildi" / "Ekip sohbetinde yeni bir mesaj var."                                           | K ✓ / SS ✓ / T ✓ (olay kimliği)        |

### SLA dışı müşteri ve kalite

| Olay                     | Üretici                                                 | Alıcı        | Kanallar                             | İçerik                                                           | CTA                     | Push                                                                 | Tercih / SS / T   |
| ------------------------ | ------------------------------------------------------- | ------------ | ------------------------------------ | ---------------------------------------------------------------- | ----------------------- | -------------------------------------------------------------------- | ----------------- |
| `company.health_dropped` | `customerhealth` → tüketici                             | Hesap sahibi | in-app / e-posta ✓ / push ✗ · yüksek | "Kayıp riski · Acme A.Ş."; Sağlık skoru, Önceki skor, Risk eşiği | Şirketi aç              | "Kayıp riski" / "Sorumlu olduğunuz bir şirketin sağlık skoru düştü." | K ✓ / SS ✓ / T ✓  |
| `ticket.assigned`        | tüketici (kanonik olayı yayınlayan modül **henüz yok**) | Atanan       | in-app 🔒 / e-posta ✓ / push ✓       | "Ürün talebi size atandı · #7"                                   | (talep sayfası gelince) | "Ürün talebi size atandı"                                            | K ✓               |
| `ticket.commented`       | — (takipçi modeli yok)                                  | —            | —                                    | —                                                                | —                       | —                                                                    | K ✓ (üretici yok) |

### Takvim (randevuların host tarafı)

Üretici `appointments/usecase/notify.go` (`notifyHost`). Host kendi yaptığı değişiklikte
bildirim almaz. Metin host'un dilinde, tarih organizasyonun saat diliminde.

| Olay                      | Varsayılan                           | Başlık (tr)                    | Detaylar                                        | CTA                                   | Push                                       | T                          |
| ------------------------- | ------------------------------------ | ------------------------------ | ----------------------------------------------- | ------------------------------------- | ------------------------------------------ | -------------------------- |
| `appointment.booked`      | in-app / e-posta ✓ / push ✓          | "Yeni randevu: Deniz Kaya"     | Müşteri, Tür, Tarih (saat dilimiyle), Süre, Yer | Randevuyu aç, Görüşmeye katıl (video) | "Yeni randevu" / "10:00 · Ürün demosu"     | randevu + sıra no          |
| `appointment.rescheduled` | in-app / e-posta ✓ / push ✓          | "Randevu yeniden planlandı: …" | + Önceki zaman                                  | aynı                                  | "Randevu yeniden planlandı"                | ✓                          |
| `appointment.confirmed`   | in-app / e-posta ✗ / push ✗          | "Randevu onaylandı: …"         | aynı                                            | aynı                                  | ✓                                          | ✓                          |
| `appointment.cancelled`   | in-app / e-posta ✓ / push ✓          | "Randevu iptal edildi: …"      | aynı                                            | Randevuyu aç                          | ✓                                          | ✓                          |
| `appointment.reminder`    | in-app / e-posta ✗ / push ✓ · yüksek | "Yaklaşan randevu: …"          | aynı                                            | Randevuyu aç, Görüşmeye katıl         | "Yaklaşan randevu" / "10:00 · Ürün demosu" | randevu + offset + sıra no |
| `appointment.no_show`     | in-app / e-posta ✗ / push ✗          | "Randevuya katılım olmadı: …"  | aynı                                            | Randevuyu aç                          | ✓                                          | ✓                          |
| `appointment.completed`   | in-app (katalog dışı, eski metin)    | "Randevu güncellendi: …"       | —                                               | Randevuyu aç                          | `pushtext` genel                           | SS ✓                       |

Hepsi K ✓ / SS ✓.

### Randevunun müşteri tarafı (dış alıcı, e-posta / SMS)

| Olay                                                 | Kanal                                  | Metin                                                                                                                                                | Link                                                           | Not                                                        |
| ---------------------------------------------------- | -------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------- | ---------------------------------------------------------- |
| `appointment.booked/rescheduled/confirmed/cancelled` | e-posta                                | Organizasyonun dilinde; tür şablonu (`ConfirmationTemplate`) özelleştirilebilir; tarih + saat dilimi, Yer, takvime ekle (ICS, Google), video bilgisi | Yönet: `/book/manage/{token}` (yeniden planla / iptal sayfada) | Tercih yok; SS yok                                         |
| `appointment.reminder`                               | e-posta + SMS (aktif SMS kanalı varsa) | `ReminderTemplate`; SMS kısa metin                                                                                                                   | Yönet linki                                                    | Tekrar: `AppointmentMarkReminder` (randevu, offset, sürüm) |

### Nöbet, işler ve bakiye (üye)

| Olay                  | Üretici                                                   | Alıcı                  | Varsayılan                                                   | Başlık (tr) / içerik                                             | CTA                                                          | Push                                                     | Tercih / SS / T                                                                                               |
| --------------------- | --------------------------------------------------------- | ---------------------- | ------------------------------------------------------------ | ---------------------------------------------------------------- | ------------------------------------------------------------ | -------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------- |
| `oncall.page`         | `oncall/service.go`                                       | Politikanın alıcıları  | Politikanın seçtiği kanallar; in-app 🔒 push 🔒 · **kritik** | "ClickFix nöbet uyarısı"; Uyarı başlığı                          | Uyarıyı aç (`/oncall`)                                       | "Nöbet uyarısı" / "Açık bir nöbet uyarısı var."          | K ✓ / SS deler / T ✓ (olay kimliği)                                                                           |
| `exports.ready`       | `exports/usecase/service.go`                              | İşi başlatan           | in-app / e-posta ✗ / push ✓ · düşük                          | "Dışa aktarım hazır · Kişiler"; Kayıt türü, Biçim                | Dışa aktarımı aç                                             | "Dışa aktarım hazır" / "Kişiler dışa aktarımınız hazır." | K ✓ / SS ✓ / T ✓ (iş)                                                                                         |
| `imports.applied`     | `imports/usecase/service.go`                              | İşi başlatan           | in-app / e-posta ✗ / push ✓                                  | "İçe aktarım tamamlandı · Kişiler"; Eklenen, Güncellenen, Hatalı | Sonucu görüntüle                                             | "İçe aktarım tamamlandı"                                 | K ✓ / SS ✓ / T ✓                                                                                              |
| `bulk.completed`      | `bulk/usecase/service.go`                                 | İşi başlatan           | in-app / e-posta ✗ / push ✓                                  | "Toplu işlem tamamlandı"; Başarılı, Başarısız                    | — (iş sayfası yok; eski bozuk link kaldırıldı)               | "…120 kayıt üzerinde toplu işlem tamamlandı."            | K ✓ / SS ✓ / T ✓                                                                                              |
| `bulk.failed`         | aynı                                                      | İşi başlatan           | in-app / e-posta ✗ / push ✓                                  | "Toplu işlem başarısız"; hata alıntısı                           | —                                                            | "Bir toplu işlem tamamlanamadı."                         | K ✓ / SS ✓                                                                                                    |
| `reports.ready`       | `reports/usecase/service.go`                              | Rapor alıcıları        | e-posta (üretici sadece e-posta açar)                        | "Rapor hazır · Haftalık SLA raporu"                              | Raporu görüntüle (imzalı indirme, 7 gün)                     | —                                                        | K ✓ / SS ✓ / T ✓                                                                                              |
| `video.balance_low`   | `videorooms/metering.go`                                  | Faturalama yetkilileri | in-app / e-posta ✓ / push ✗ · yüksek                         | "Görüntülü görüşme dakikanız azalıyor"; Kalan dakika; ne olacağı | Dakika ekle (`/billing`)                                     | "Kalan dakika: 12"                                       | K ✓ / SS ✓ / T (eşik başına bir kez, `VideoMarkLowBalanceNotified`)                                           |
| `automation.executed` | `automation/usecase/service.go` (kural aksiyonu "notify") | Kuralın seçtiği üye    | in-app                                                       | Kural adı / kuralın yazdığı metin (organizasyonun içeriği)       | **Yok** (`action_url` verilmiyor; konuşma sadece payload'da) | `pushtext` genel                                         | K ✗ / SS ✓ / T ✗ (`execution_key` payload'da ama `source_event_id` değil). Takip: konuşma linki + `DedupeKey` |

### Hesap ve güvenlik (üye, `SecurityEmail` → tercihten bağımsız)

DB şablonları (`notification_templates`, tr/en). İçerik modeline taşınmadı: kodlar ve
sıfırlama akışları bu şablonlarla test ediliyor.

| Olay                                                          | Kanal            | Metin (tr konu)                               | Not                                                                                                                                         |
| ------------------------------------------------------------- | ---------------- | --------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| `auth.login_code`                                             | e-posta          | "Giriş kodunuz: {{code}}"                     | Güvenlik, SS yok                                                                                                                            |
| `auth.email_verification`                                     | e-posta          | "E-posta doğrulama kodu"                      | Güvenlik                                                                                                                                    |
| `auth.password_reset`                                         | e-posta          | "Şifre sıfırlama kodu"                        | Güvenlik                                                                                                                                    |
| `auth.oauth_link_code`                                        | e-posta          | "Hesap bağlama kodu: {{code}}"                | Güvenlik                                                                                                                                    |
| `auth.account_deactivation_code` / `auth.account_deactivated` | e-posta          | "Hesap silme onay kodu" / "Hesabınız silindi" | Güvenlik                                                                                                                                    |
| `auth.welcome` (`auth.register`)                              | in-app + e-posta | Hoş geldiniz                                  | Katalog dışı, SS ✓                                                                                                                          |
| `auth.profile_updated`                                        | in-app           | Profil güncellendi                            | Katalog dışı                                                                                                                                |
| **Yeni oturum / yeni cihazdan giriş**                         | —                | —                                             | **Üretilmiyor.** Öneri: `auth.new_session` güvenlik e-postası + in-app (cihaz, konum, zaman, "Bu ben değilim" → oturumları kapatma sayfası) |
| **Şifre değişti**                                             | —                | —                                             | **Üretilmiyor** (`/v1/auth/password/change` sadece yanıt dönüyor). Öneri: `auth.password_changed` güvenlik e-postası                        |

### Davet ve üyelik

| Olay                       | Üretici                                | Alıcı                | Kanal            | Metin                                   | Link                                                   | Not        |
| -------------------------- | -------------------------------------- | -------------------- | ---------------- | --------------------------------------- | ------------------------------------------------------ | ---------- |
| `organizations.invitation` | `organizations/usecase/invitations.go` | Davet edilen e-posta | e-posta (yüksek) | DB şablonu tr/en (davet edenin dilinde) | Daveti kabul et: `/invite/{token}`; kabul sayfada POST | Tercih yok |

### Faturalama ve ödeme (CF-157 kapsamı dışında, dokunulmadı)

`billing/trial_notifications.go` faturalama olaylarını DB şablonlarıyla (`billing.*`,
in-app + e-posta, tr/en) organizasyon sahiplerine gönderir: `billing.welcome`,
`payment_received`, `payment_failed`, `invoice_issued`, `card_expiring`,
`renewal_reminder`, `transfer_pending/approved/rejected`, `quota_warning`,
`quota_exhausted`, `suspended`, `canceled`, `payment_required`,
`reconciliation_mismatch`, `refund_succeeded`, `wallet_low`, `trial_ending`.

- İdempotency: `source_event_id` (olay kimliği) ✓. Katalog dışı: SS ✓, matris ✗.
- Bilinen içerik eksiği: bazı şablonlar fatura UUID'si gibi kullanıcıya anlamsız değerler
  gösteriyor ("Fatura: {{invoice_uuid}}"). Ödeme kodu ayrı PR'da (#171) sertleştiriliyor;
  içerik iyileştirmesi o tarafta yapılmalı.

### Görüntülü görüşme daveti (dış alıcı)

| Olay           | Üretici                          | Kanal                     | Metin                                                                        | Link                                     |
| -------------- | -------------------------------- | ------------------------- | ---------------------------------------------------------------------------- | ---------------------------------------- |
| `video.invite` | `videorooms/service.go` `Invite` | e-posta (davetlinin dili) | Organizasyon, oda, zaman (org saat dilimi), katılım linki; PIN ayrı iletilir | İmzalı katılım linki `/r/{code}` (sayfa) |

### Durum sayfası (dış abone)

| Olay                                              | Üretici                               | Kanal                                                     | Metin                                                        | Link                                                                         | PR   |
| ------------------------------------------------- | ------------------------------------- | --------------------------------------------------------- | ------------------------------------------------------------ | ---------------------------------------------------------------------------- | ---- |
| `status.subscription_requested`                   | `statuspage/usecase/subscriptions.go` | e-posta                                                   | Abonelik onayı; kimin isteği olabileceği, 24 saat geçerlilik | Aboneliği onayla: `/status/{slug}/subscription/{token}` (onay sayfası, POST) | #180 |
| `status.changed`                                  | aynı (`NotifySubscribers`)            | e-posta                                                   | Olay güncellemesi metni                                      | Durum sayfası; Abonelikten ayrıl (onay sayfası)                              | #180 |
| `feedback.changelog_published` / talep yayınlandı | `supportoperations/events.go`         | Oy verenin konuşması varsa konuşmaya mesaj, yoksa e-posta | "İstediğiniz özellik yayınlandı: …"                          | —                                                                            | T ✓  |

- Durum sayfası e-postaları sayfanın dili bilinmediği için Türkçe. Sayfaya dil alanı
  eklenince tr/en yapılabilir.

### Üretilmeyen / ölü dinleyiciler

`notifications/bus.go` içindeki `customers.assigned`, `customers.status_changed`,
`ai.pipeline.escalated`, `ai.draft.created` ve `conversations.assigned` dinleyicileri
pratikte hiçbir şey üretmiyor: ilk dördünü yayınlayan modül yok, sonuncusunun payload'ında
`assigned_user_id` yok (atama `MentionNotifier` ile bildiriliyor). Bu yüzden katalogda
değiller; AI devri bildirimi istenirse yayınlayan tarafla birlikte eklenmeli.

## Yeni bildirim eklemek

1. Olayı kataloğa ekle (`usecase/preferences.go` `eventCatalog`: grup, öncelik,
   varsayılanlar, kilitli kanallar) ve tercih etiketlerini
   `frontend/src/locales/{tr,en}/auth.json` dosyasına yaz
   (`preferences.matrix.event_<anahtar>`; yeni grup için `group_<ad>` ve mobilde
   `GROUP_ORDER`).
2. İçeriği `content/events.go` dosyasına yaz (tr/en; başlık, ön metin, detaylar, CTA,
   kilit ekranı push metni, kategori). `TestCatalogEventsHaveContent` ve
   `TestEveryEventInBothLanguages` eksik içeriği ve push sızıntısını yakalar.
3. Üreticide `Enqueue` / `NotifyEvent` çağır: `SourceEvent` = katalog anahtarı,
   `ContentVars` = bilgiler, `DedupeKey` = olay kimliği. Üyeye push için ayrı `push`
   kanalı açma (in-app aynası yeterli).
4. CTA sadece sayfa açsın. İşlem yapan link gerekiyorsa onay sayfası + POST kullan.
5. Bu tabloyu güncelle.
