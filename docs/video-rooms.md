# Görüntülü görüşme odaları (CF-139)

LiveKit tabanlı görüntülü görüşme modülü: müşteriye e-postayla giden bağlantıyla **giriş yapmadan** katılım,
isteğe bağlı PIN, bekleme odası (host onayı), ekran paylaşımı, sohbet, kamera/mikrofon kontrolleri, uçtan uca
şifreleme (E2EE), kayıt ve dakika bazlı kredi faturalaması. Randevularla (CF-138) birlikte kullanılmak üzere
küçük bir API sunar.

## Mimari

```
Tarayıcı / mobil ──wss──> Traefik (TLS) ──> livekit:7880        (sinyal)
Tarayıcı / mobil ──DTLS-SRTP (UDP 50000-50100, TCP 7881, TURN)──> livekit   (medya)
backend ──http (iç ağ)──> livekit:7880 /twirp/...                 (oda, katılımcı, egress API)
livekit ──webhook (iç ağ)──> backend:8080 /v1/public/video/webhook (imzalı JWT)
livekit-egress ──Redis (psrpc)──> livekit ; egress ──S3──> SeaweedFS veya organizasyonun bucket'ı
```

- Backend: `backend/internal/modules/videorooms` (politika, oda, lobi, kayıt, ölçümleme) ve
  `backend/internal/platform/livekit` (erişim token'ı, webhook doğrulama, Twirp JSON istemcisi; protobuf SDK'sı
  bilinçli olarak eklenmedi).
- LiveKit: `deploy/livekit/livekit.yaml` (sırsız sabit ayarlar) + `deploy/livekit/entrypoint.sh` (rtc, webhook ve
  TURN bölümlerini ortamdan üretir, anahtarları `LIVEKIT_KEYS`, Redis'i `REDIS_HOST/REDIS_PASSWORD` ile verir).
  Anahtar yoksa konteyner çökmeden boşta bekler.
- Redis paylaşılır (kuyrukla aynı örnek): LiveKit çok düğümlü yönlendirme ve egress RPC'si için Redis ister;
  anahtarları `asynq:*` ile çakışmaz.
- `room.auto_create: false`: odalar yalnız API ile (ilk token verilmeden önce `CreateRoom`) açılır.

## Katılım akışı

| Kim | Nasıl |
| --- | --- |
| Host / temsilci (panel, mobil) | `POST /v1/tenant/video/rooms/{uuid}/join` → doğrudan kabul. Oda host'u veya `tenant.video.manage` sahibi host, diğer üyeler `agent` rolüyle girer |
| Misafir (müşteri) | `/r/<kod>` sayfası → `GET /v1/public/video/rooms/{kod}` → ad (+PIN) → `POST .../join`. Lobi açıksa `waiting` + bilet döner; sayfa `POST /v1/public/video/ticket` ile bekler, host kabul edince token gelir |

- **Katılım bağlantısı**: `oda uuid + rol (guest/agent) + bitiş` imzalı kod (HMAC-SHA256, `APP_ENCRYPTION_KEY`'den
  türetilmiş anahtar), 51 karakter. `agent` bağlantısı yalnız giriş yapmış organizasyon üyeleri içindir.
  Bağlantılar varsayılan olarak planlanan bitişten 24 saat sonra, planlanmamış odalarda 7 gün sonra geçersizdir.
- **PIN**: 4-12 hane, `APP_ENCRYPTION_KEY` ile şifreli saklanır, sabit zamanlı karşılaştırılır. Deneme sınırı:
  adres başına dakikada 10, oda bağlantısı başına 10 dakikada 30. Davet e-postasında PIN yoktur.
- **Bekleme odası**: Lobide bekleyen misafire token ve E2EE anahtarı **verilmez**; yalnız bilet (32 bayt rastgele,
  veritabanında SHA-256'sı) verilir. Kabul / ret / çıkarma: `POST .../participants/{p}/admit|deny|remove`.
- **İzinler**: LiveKit token'ı odanın ayarlarına göre `canPublishSources` (mikrofon; politika izin verirse kamera,
  ekran paylaşımı) ve `canPublishData` (sohbet) içerir; istemci tarafı düğmeler sadece görünümdür, sınır sunucudadır.

## Şifreleme ve kayıt (ödünleşim)

- **Taşıma**: Sinyal Traefik üzerinden TLS (wss). Medya her zaman DTLS-SRTP ile şifreli. TURN/TLS isteğe bağlı.
  Backend → LiveKit API çağrıları ve webhook'lar yalnız özel `app` ağında akar; webhook'lar `LIVEKIT_API_SECRET`
  ile imzalı JWT ve gövde SHA-256'sı ile doğrulanır, olay kimlikleri tekrar işlenmez.
- **Uçtan uca (E2EE)**: Her oda için rastgele 32 baytlık ortak anahtar üretilir, `APP_ENCRYPTION_KEY` ile şifreli
  saklanır ve yalnız **kabul edilmiş** katılımcılara join yanıtında verilir. İstemci LiveKit
  `ExternalE2EEKeyProvider` ile medyayı ve veri kanalını şifreler; LiveKit sunucusu içeriği göremez.
- **Kayıt**: LiveKit Egress odaya gizli bir katılımcı olarak katılıp medyayı sunucuda birleştirir; şifreli
  medyayı kaydedemez. Bu yüzden:
  - Oda oluşturulurken seçilir: `record=true` olan oda **E2EE kapalı** açılır ve katılımcılara "bu görüşme kaydedilebilir"
    uyarısı gösterilir. `record=false` olan odada kayıt hiç başlatılamaz.
  - Politika: `allow_recording` (platform → plan → organizasyon, organizasyon yalnız kapatabilir) ve
    `e2ee_required` (açıksa her oda E2EE'dir ve kayıt imkânsızdır). Veritabanında
    `CHECK (NOT (e2ee AND recording))` kısıtı vardır.
- **Kayıt deposu**: Organizasyon kendi S3 bilgilerini girdiyse (Ayarlar → Görüntülü görüşme; anahtarlar
  `APP_ENCRYPTION_KEY` ile şifreli, endpoint yalnız genel `https`) kayıt doğrudan oraya yüklenir ve bizde silinmez.
  Girmediyse platform deposuna (SeaweedFS) gider ve `recording_retention_hours` (varsayılan 24, platform yöneticisi
  değiştirir; plan ve organizasyon kısaltabilir) dolunca `app:video:recording-purge` Asynq işi (10 dakikada bir)
  nesneyi ve kaydı siler. İndirme her zaman API üzerinden akış olarak yapılır (bucket adresi verilmez).

## Politika katmanları

`video_platform_settings` (tam politika) → `video_plan_policies` (plan bazında alanları **değiştirir**) →
`video_org_settings.policy` (organizasyon yalnız **sıkılaştırır**: izinleri kapatır, gereksinim ekler, sınırları
düşürür). Alanlar: modül açık, en uzun süre, en fazla katılımcı, kamera / ekran paylaşımı / sohbet / kayıt /
misafir katılımı izinleri, PIN zorunlu, bekleme odası zorunlu, E2EE zorunlu, kayıt saklama süresi, eş zamanlı kayıt
ve en uzun kayıt süresi (egress sınırları), düşük bakiye eşiği.

Modülün kullanılabilmesi için dördü birden gerekir: LiveKit yapılandırılmış (`LIVEKIT_*`), platform politikası
açık, plan `video.enabled` hakkını veriyor, organizasyon ayarı açık. `GET /v1/tenant/video/availability` hangisinin
eksik olduğunu söyler.

## Krediler ve ölçümleme

- Birim: **katılımcı-dakika** (`video_minutes`). Her LiveKit bağlantısı (bir katılımcının bir girişi) kapanınca
  dakikaya yukarı yuvarlanır ve cüzdandan düşülür (`video:<sid>` anahtarıyla, tekrar eden webhook iki kez düşmez).
  Kayıt egress'i gizli katılımcıdır, faturalanmaz. Neden katılımcı-dakika: maliyet (bant genişliği, SFU yükü)
  katılımcı sayısıyla doğrusal artar; oda-dakika iki kişilik görüşmeyle on kişilik toplantıyı aynı fiyatlardı.
- Mevcut cüzdan / defter / hak motoru kullanılır, kopyalanmadı: `billing_credit_ledger.unit` artık
  `TRY | ai_credits | video_minutes`; lotlar, FIFO tahsis (en erken biten önce), süre dolumu ve oto-yükleme aynı kod.
- Plan: `video.minutes` limit özelliği her dönem cüzdana plan lotu olarak yüklenir (AI kredileri gibi, dönem
  sonunda biter). `video.minutes` **sınırsız** ise ölçümleme yapılmaz.
- Paket: Platform → Faturalama → Kredi paketleri ekranında `unit=video_minutes` ile paket (fiyat TRY, miktar,
  geçerlilik günü) tanımlanır; organizasyonlar mevcut ödeme akışıyla (PayTR / iyzico, fatura) satın alır.
- Bakiye 0 iken yeni oda açılamaz ve yeni token verilmez (`402 VIDEO_MINUTES_EXHAUSTED`). Her dakika çalışan
  `app:video:sweep` işi, açık bağlantıların birikmiş dakikası bakiyeyi aşarsa canlı odayı kapatır; en uzun süreyi
  aşan ve bağlantı süresi dolan odaları da kapatır. Bakiye yetmezse kalan kısım borç yazılmaz, odada
  `unbilled_minutes` olarak görünür.
- Düşük bakiye: bakiye eşik (`low_balance_minutes`, varsayılan 60) altına inince sahip ve yöneticilere günde en
  fazla bir kez uygulama içi + e-posta bildirimi gider.

## Randevu entegrasyonu (CF-138 / CF-139)

`location_kind=video` olan türün internal, public sayfa ve widget rezervasyonları
`videorooms.Service.RoomForAppointment` çağırır. Randevu ve oda aynı transaction'da
kaydedilir; `appointments.video_room_uuid` oda UUID'sini tutar. Şema alanı zaten
bulunduğu için bu bağlantı yeni migration gerektirmez.

- Açılış: başlangıçtan `VIDEO_APPOINTMENT_OPEN_BEFORE_MINUTES` dakika önce (varsayılan 10).
- Kapanış: randevu bitişinden `VIDEO_APPOINTMENT_GRACE_MINUTES` dakika sonra (varsayılan 15).
- Her iki ayar 0–120 dakikayla sınırlandırılır ve `compose.prod.yml` backend ortamına aktarılır.
- Pencerenin tamamı etkin platform/plan/organizasyon süre sınırına sığmalıdır.
  PIN zorunluysa kriptografik rastgele 6 haneli PIN üretilir. Misafir girişi, modül
  etkinliği, plan hakkı ve dakika bakiyesi mevcut video usecase'i tarafından denetlenir.
- Oda oluşturma bağlantıları kendiliğinden e-posta göndermez. Randevu servisi
  onay/güncelleme/hatırlatma e-postalarına konuk bağlantısı ve PIN'i ekler.
  ICS DESCRIPTION ve Google/Outlook açıklamaları aynı konuk bilgisini taşır.
- Public manage yanıtı `video.guest_link`, `pin`, `host_joined`, `guest_joined` içerir.
  Temsilci takvim/detay yanıtı ayrıca `video.host_link` taşır. Bu imzalı bağlantı
  yine organizasyon üyeliği ve oturum gerektirir; public yanıtta bulunmaz.
- Yeniden planlama aynı odanın penceresini ve host'unu transaction içinde günceller;
  canlı oda başka bir pencereye taşınamaz. Eski bağlantı da kayıtlı pencereye tabidir.
  Yeni onay/hatırlatmalar yeni bitiş zamanına imzalanmış bağlantıyı taşır.
- İptal odayı kapatır; video sweep pencere bitiminde açık/canlı odaları sonlandırır.
  Katılım API'si pencere dışında token vermez; sweep dakikalık çalışır.
- Randevu hatırlatma sweep'i pencere sonunda gerçek LiveKit `video_sessions`
  kayıtlarını katılımcı rolleriyle birleştirir. Host ve konuk katılmışsa `completed`,
  aksi halde `no_show` olur. Lobi/admission tek başına katılım sayılmaz.
  Her iki katılım bayrağı ayrıca yanıtta gösterilir; iptal edilmiş randevu korunur.
- Video kapalı, yapılandırılmamış, misafir girişine kapalı, süre sınırının dışında
  veya bakiyesizse **rezervasyon başarılıdır**. `video_warning` ve `location_value`
  temsilcinin bağlantı bilgileri için iletişime geçeceğini açıklar.
  Commit sonrasında organizasyon kapsamlı `appointments.updated` (payload `event=appointment.video_unavailable`) activity
  olayı yazılır. Veritabanı gibi beklenmeyen hatalar transaction'ı geri alır.

Doğrudan video API sözleşmesi korunur:
`PUT /v1/tenant/video/appointments/{appointment_uuid}/room` (`tenant.video.use`),
`{name, starts_at, ends_at, host_uuid?, guest_email?, guest_name?, locale?, pin?}`.
`guest_email` yalnız bu doğrudan çağrıda ayrı video daveti gönderir. Normal rezervasyon
akışında bu alan verilmez. `video_rooms.appointment_uuid` mevcut şemada yabancı
anahtarsızdır; randevu servisi organizasyon kapsamlı kaydı oluşturur ve bağlar.

Testler: `appointments/usecase/video_test.go`; dış LiveKit HTTP servisi sahtedir.
Ayrıntılı randevu davranışı: [Randevular](appointments.md).

## Yetkiler ve denetim

- `tenant.video.use` (tüm varsayılan rollere): oda oluşturma, katılma, bağlantı, davet, lobi.
  Odayı bitirme, katılımcı çıkarma ve kayıt: oda host'u veya `tenant.video.manage`.
- `tenant.video.manage` (owner, admin): organizasyon ayarları, kayıt listesi / indirme / silme.
- `platform.video.manage` (super admin): platform politikası, plan kuralları.
- Denetim kaydı (`activity`): `video.room_created`, `video.room_joined`, `video.room_ended`, `video.link_created`,
  `video.guest_invited`, `video.participant_admit|deny|remove`, `video.recording_started|stopped|deleted|downloaded`,
  `video.settings_updated`, `video.platform_settings_updated`, `video.plan_policy_updated|deleted`,
  `video.appointment_room`.

## Portlar ve Dokploy

| Port | Protokol | Yayın | Ne |
| --- | --- | --- | --- |
| 7880 | TCP | Traefik (`video.<domain>`, HTTPS) | Sinyal + API. Dışarıya doğrudan açılmaz |
| 7881 | TCP | host | ICE/TCP (UDP kapalı ağlar) |
| 50000-50100 | UDP | host | Medya (ICE). Aralık `LIVEKIT_RTC_PORT_START/END` |
| 3478 | UDP | host | TURN/UDP (`LIVEKIT_TURN_DOMAIN` doluysa) |
| 50200-50300 | UDP | host | TURN relay aralığı |
| 5349 | TCP | Traefik TCP router (TLS sonlandırma) | TURN/TLS (`external_tls: true`) |

Aynı sunucuda ikinci bir LiveKit çalışacaksa port aralıkları değiştirilmelidir.

### TURN

Kurumsal ağlarda yalnız 443/TLS açık olabilir. `LIVEKIT_TURN_DOMAIN=turn.<domain>` verildiğinde LiveKit gömülü
TURN'ü açar. TURN/TLS için Traefik'e (Dokploy → Traefik → Dynamic config) şu TCP router eklenir; Traefik sertifikayı
Let's Encrypt ile alır ve düz TCP'yi `livekit:5349`'a iletir:

```yaml
tcp:
  routers:
    clickfix-turn:
      entryPoints: [websecure]
      rule: HostSNI(`turn.getclickfix.com`)
      service: clickfix-turn
      tls:
        certResolver: letsencrypt
  services:
    clickfix-turn:
      loadBalancer:
        servers:
          - address: clickfix-livekit:5349
```

## Yerel geliştirme

`make local-dev` altyapısı `livekit` servisini de başlatır (`compose.local.yml`, geliştirme anahtarları
`.env.example` ile aynı). Kayıt denemek için: `docker compose -f compose.local.yml --profile video-recording up -d`.
Platform paneli → Görüntülü görüşme ekranından modülü açın, plana `video.enabled` ve `video.minutes` verin.

## Mobil

`@livekit/react-native` + `@livekit/react-native-webrtc` (yerel modüller; dev client yeniden derlenmeli) ve
`@livekit/react-native-expo-plugin` + `@config-plugins/react-native-webrtc` config plugin'leri. Temsilci
konuşma ekranından, müşteri `clickfix://r/<kod>` veya `https://<domain>/r/<kod>` bağlantısından katılır.
iOS'ta kamera / mikrofon izin metinleri ve arka plan ses modu, Android'de `CAMERA`, `RECORD_AUDIO`,
`MODIFY_AUDIO_SETTINGS`, `BLUETOOTH_CONNECT` izinleri eklenmiştir.
