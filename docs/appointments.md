# Randevular ve takvim (CF-138)

Organizasyon randevu türünde uygunluk, süre, host seçimi, konum, public erişim,
iptal/yeniden planlama politikası ve hatırlatmaları tanımlar. Temsilci rezervasyonu,
public `/book/{org}/{type}` sayfası ve widget aynı rezervasyon usecase'ini kullanır.
Takvim gün/hafta/ay/ajanda ve sürükle-bırak hareketlerini organizasyon kapsamında sunar.

## Görüntülü randevu

`location_kind=video` için rezervasyon oda UUID'sini otomatik bağlar. Oda yalnız
randevu penceresinde kullanılabilir: varsayılan olarak 10 dakika önce açılır,
süre + 15 dakika toleranstan sonra kapanır. Etkin platform/plan/organizasyon
politikası ile dakika cüzdanı uygulanır. Ayrıntılar ve ortam ayarları
[video odaları](video-rooms.md#randevu-entegrasyonu-cf-138--cf-139) belgesindedir.

Onay, yeniden planlama ve hatırlatma e-postası, ICS açıklaması ve public yönetim
sayfasının API yanıtı konuk bağlantısını ve gerekirse PIN'i içerir. Özel e-posta
şablonları kullanılsa da bu bilgiler standart detaylara eklenir. Temsilci detay ve
takvim API'sinde oturum gerektiren host bağlantısı ayrıca bulunur.

Video kullanılamadığında rezervasyon devam eder; `video_warning` ve konum açıklaması
bağlantı bilgilerini temsilcinin ileteceğini bildirir. Organizasyon activity kaydı
`appointments.updated` eylemi ve `event=appointment.video_unavailable` payload alanıyla tutulur. Bu randevu otomatik olarak
no-show sayılmaz; katılım kanıtı olmayan fallback rezervasyon normal iş akışındadır.

Yeniden planlama aynı odanın başlangıç/bitişini ve host'unu günceller. İşlem
başarısızsa randevunun eski zamanı da korunur. Canlı oda yeni pencereye taşınamaz.
İptal video odasını kapatır. Pencere sonunda dakikalık worker, doğrulanmış LiveKit
katılım kayıtlarından `completed` (host + konuk) veya `no_show` hesaplar.
Lobide beklemek veya token almak katılım sayılmaz; yanıt iki katılım bayrağını da taşır.
Manuel completed/no-show değişikliği de bu kanıtla uyuşmalıdır; pencere bitmeden
video no-show kaydı yapılamaz.

Backend kapsamı: bu değişiklik API sözleşmelerini ve üretilen SDK/tipleri sağlar.
Web/mobil görsel bileşenleri bu alanları kullanarak bağlantı ve uyarıları gösterebilir.
