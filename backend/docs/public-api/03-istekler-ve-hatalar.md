# Yanıtlar ve hatalar

Tüm yanıtlar aynı zarfı kullanır. Başarılı yanıt:

```json
{
  "success": true,
  "data": { "uuid": "8f0e…", "status": "open" },
  "meta": { "request_id": "01JB…" }
}
```

Hatalı yanıt:

```json
{
  "success": false,
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Invalid input; writes require Idempotency-Key",
    "details": [{ "field": "subject", "message": "too long", "code": "max" }]
  },
  "meta": { "request_id": "01JB…" }
}
```

Hata yönetimini `error.code` değerine göre yapın; `message` metni bilgilendirme amaçlıdır ve değişebilir. Bize ulaştığınızda `meta.request_id` değerini paylaşın; isteği bu değerle buluruz.

| HTTP | `error.code` | Ne zaman | Ne yapmalı |
|---|---|---|---|
| 400 | `VALIDATION_ERROR` | Gövde geçersiz, alan kuralı ihlal edildi veya yazma isteğinde `Idempotency-Key` yok | `details` alanına bakıp isteği düzeltin; aynı isteği tekrar denemeyin |
| 401 | `UNAUTHENTICATED` | Anahtar yok, hatalı, iptal edilmiş veya süresi dolmuş | Anahtarı kontrol edin, gerekirse yenisini oluşturun |
| 403 | `FORBIDDEN` | Anahtarın kapsamı veya anahtarı oluşturan üyenin yetkisi yetmiyor | Anahtara kapsamı ekleyin (yeni anahtar) veya üyenin rolünü güncelleyin |
| 403 | `ENTITLEMENT_REQUIRED`, `PLAN_LIMIT_EXCEEDED`, `SUBSCRIPTION_READ_ONLY` | Organizasyonun planı bu özelliği içermiyor, kota doldu veya abonelik salt okunur | Organizasyon yöneticinizle planı gözden geçirin |
| 404 | `NOT_FOUND` | Kayıt yok, başka organizasyona ait veya uç nokta herkese açık API'de yok | Kimliği ve yolu kontrol edin |
| 409 | `IDEMPOTENCY_CONFLICT` | Aynı `Idempotency-Key` farklı bir istekle kullanıldı veya ilk istek hâlâ işleniyor | Yeni işlem için yeni anahtar üretin; işlenen istek için kısa bir süre sonra aynı anahtarla tekrar deneyin |
| 410 | `CURSOR_EXPIRED` | Tetikleyici imleci artık geçerli değil | Kaynakları yeniden okuyup `cursor=0` ile baştan başlayın |
| 429 | `RATE_LIMITED` | Anahtarın dakikalık limiti aşıldı | `Retry-After` saniye kadar bekleyip tekrar deneyin |
| 502 | `RESULT_TOO_LARGE` | Yanıt çok büyük | `limit` değerini düşürün veya filtre ekleyin |
| 500 | `INTERNAL_ERROR` | Beklenmeyen sunucu hatası | Üstel bekleme ile tekrar deneyin; sürerse `request_id` ile bize yazın |

# Sayfalama

Liste uç noktaları `limit` (varsayılan 20, en fazla 100) ve `offset` parametrelerini kabul eder ve sonucu tek bir yapıda döner:

```json
{
  "success": true,
  "data": { "items": [], "total": 134, "limit": 20, "offset": 40 }
}
```

Sonraki sayfa için `offset` değerini `limit` kadar artırın; `items` boş geldiğinde veya `offset + limit >= total` olduğunda durun. Kişi listesi ayrıca `q` ile arama destekler.

Sayfalama anlık görüntü almak içindir; çok sayıda kaydı sürekli senkron tutmak için sayfa sayfa yoklamak yerine **webhook** veya **tetikleyici yoklama** (`GET /triggers`, imleç tabanlı) kullanın.

# Idempotency

`GET` dışındaki her istekte `Idempotency-Key` başlığı **zorunludur** (en fazla 200 bayt).

- Her mantıksal işlem için benzersiz bir değer üretin, ör. `siparis-1042-iade` veya bir UUID.
- Ağ hatası veya zaman aşımında aynı isteği **aynı anahtar ve aynı gövdeyle** tekrar gönderin. ClickFix ilk tamamlanan yanıtı saklar ve aynen döner; işlem iki kez yapılmaz.
- Anahtar 24 saat saklanır. Aynı anahtarı farklı bir gövde, yöntem veya yolla kullanırsanız `409 IDEMPOTENCY_CONFLICT` döner.
- Saklanan yanıt bir hata ise (ör. `400`), aynı anahtarla tekrar denemek yine aynı hatayı döner. Girdiyi düzelttiyseniz yeni bir anahtar kullanın.
- Mesaj gönderirken `client_id` alanı ek bir tekrar koruması sağlar.

# Hız sınırları ve boyutlar

- Her API anahtarının dakikalık bir istek limiti vardır: varsayılan **120**, anahtar oluştururken 1–6000 arasında ayarlanabilir.
- Limit aşıldığında `429 RATE_LIMITED` ve `Retry-After` başlığı (saniye) döner. Bu süre kadar bekleyin, ardından üstel bekleme ve rastgele sapma (jitter) ile tekrar deneyin.
- Birden çok entegrasyon için ayrı anahtar kullanın; limit anahtar başına uygulanır.
- İstek gövdesi en fazla **128 KB** olabilir.
- `400`, `401`, `403` ve `404` yanıtlarını tekrar denemeyin; önce isteği düzeltin.

# Sürümleme

- API'nin sürümü yoldadır: `/v1/...`. `v1` içinde yalnızca **geriye uyumlu** değişiklikler yapılır: yeni uç nokta, yeni isteğe bağlı alan, yeni olay türü, yeni hata kodu.
- İstemciniz tanımadığı yanıt alanlarını ve olay türlerini yok saymalıdır.
- Bir alanı kaldırmak, anlamını değiştirmek veya zorunlu alan eklemek gibi kırıcı değişiklikler yalnızca yeni bir ana sürümle (`/v2`) gelir ve önceden duyurulur; eski sürüm geçiş süresi boyunca çalışmaya devam eder.
- Bu başvuruda yer almayan uç noktalar herkese açık API'nin parçası değildir ve haber verilmeden değişebilir.
