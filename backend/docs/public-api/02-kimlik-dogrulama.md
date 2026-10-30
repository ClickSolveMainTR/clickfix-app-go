# Kimlik doğrulama ve kapsamlar

Her istek bir **API anahtarıyla** doğrulanır. Anahtarı `Authorization` başlığında `Bearer` olarak gönderin:

```http
Authorization: Bearer cf_sk_...
```

## API anahtarı oluşturma

1. **Organizasyon → API ve otomasyonlar → API anahtarları** sayfasını açın. Sayfayı görmek için organizasyonda API anahtarlarını yönetme yetkiniz olmalıdır.
2. **Yeni API anahtarı**'na basın; anahtara entegrasyonu tanıtan bir ad verin (ör. "ERP senkronizasyonu").
3. Yalnızca entegrasyonun ihtiyaç duyduğu **kapsamları** seçin.
4. İsterseniz **dakikalık istek limiti** (1–6000, varsayılan 120) ve **son geçerlilik tarihi** belirleyin.
5. **Anahtar oluştur**'a basın ve anahtarı kopyalayın. ClickFix anahtarın yalnızca özetini saklar; anahtar bir daha gösterilmez.

Bir anahtarı aynı sayfadaki **İptal et** ile hemen geçersiz kılabilirsiniz. Anahtar sızdıysa önce iptal edin, sonra yenisini oluşturun.

> API anahtarları yalnızca sunucuda kullanılır. Tarayıcı, mobil uygulama veya herkese açık bir depodaki koda koymayın.

## Anahtar hangi verilere erişir?

- Anahtar oluşturulduğu **organizasyona** bağlıdır; `X-Organization` gibi bir başlık göndermeniz gerekmez.
- Anahtar, onu oluşturan üyenin **güncel** yetkileriyle sınırlıdır. Üyenin yetkisi düşerse, üye pasifleşirse veya organizasyondan çıkarılırsa anahtar da aynı oranda kısıtlanır ya da çalışmaz.
- Bir isteğin geçmesi için hem anahtarın ilgili **kapsamı** hem de üyenin ilgili **yetkisi** gerekir.

## Kapsamlar

| Kapsam | İzin verdiği |
|---|---|
| `conversations:read` | Konuşmaları ve mesajları okuma |
| `conversations:write` | Konuşma oluşturma ve güncelleme, mesaj ve iç not ekleme |
| `contacts:read` | Kişileri okuma |
| `contacts:write` | Kişileri güncelleme |
| `companies:read` | Şirketleri okuma |
| `triggers:read` | Tetikleyici olaylarını yoklama |
| `webhooks:read` | Webhook aboneliklerini okuma |
| `webhooks:write` | Webhook aboneliği oluşturma, güncelleme ve silme |
| `revenue:write` | Şirket gelir verisi gönderme |
| `health:write` | Şirket kullanım sinyali (0–100) gönderme |
| `events:write` | Şirket zaman çizelgesine ürün olayı gönderme |

Her uç noktanın gerektirdiği kapsam, başvurudaki açıklamasında yazar. Kapsam eksikse `403 FORBIDDEN` döner ve mesajda eksik kapsam belirtilir.
