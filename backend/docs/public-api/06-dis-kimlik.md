# Dış kimlik ve doğrulanmış müşteri

Kendi sisteminizdeki kullanıcı numarasını ClickFix'te **dış kimlik** (`external_id`) olarak tutarsınız. Kişi ve şirket dış kimlikleri ayrı alanlardır: kişinin `external_id` değeri sizin kullanıcı numaranızdır, şirketin `external_id` değeri sizin firma numaranızdır. İkisi de organizasyon içinde tekildir.

| Kayıt | Alan | Widget `identify` karşılığı |
|---|---|---|
| Kişi | `external_id` | `user_id` |
| Şirket | `external_id` | `company.id` |

## Kişiyi dış kimlikle senkron tutun

`PUT /contacts/by-external-id` kişiyi dış kimliğiyle oluşturur ya da günceller. Aynı dış kimlikle ikinci çağrı yeni kişi açmaz, mevcut kişiyi günceller. Yanıt kişi oluşturulduysa `201`, güncellendiyse `200` döner. Kapsam `contacts:write`; anahtarı oluşturan üyenin kişi yazma yetkisi de aranır.

```bash
curl -X PUT "https://api.getclickfix.com/v1/public-api/contacts/by-external-id" \
  -H "Authorization: Bearer $CLICKFIX_API_KEY" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: kullanici-1001-v3" \
  -d '{ "external_id": "1001", "name": "Ayşe Yılmaz", "email": "ayse@example.com" }'
```

- Eşleşme yalnızca dış kimlikle yapılır. E-posta veya telefon üzerinden başka bir kişi devralınmaz; sitede adresini yazmış anonim bir ziyaretçi bu kişiye karışmaz.
- Başka bir kişide olan bir dış kimliği atamaya çalışırsanız `409 EXTERNAL_ID_EXISTS` döner.
- Her değişiklik için yeni bir `Idempotency-Key` kullanın; aynı anahtar ilk yanıtı tekrar döndürür.

Kişiyi bulmak için `GET /contacts?external_id=1001`, şirketi bulmak için `GET /companies?external_id=F-77` kullanın.

## Müşteri siteye girmeden ilk mesaj

Konuşmayı `contact_external_id` ile açabilirsiniz; ayrı bir kanal gerekmez:

```bash
curl -X POST "https://api.getclickfix.com/v1/public-api/conversations" \
  -H "Authorization: Bearer $CLICKFIX_API_KEY" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: hosgeldin-1001" \
  -d '{ "contact_external_id": "1001", "inbox_uuid": "WIDGET_KANAL_UUID", "subject": "Hoş geldiniz" }'
```

Ardından `POST /conversations/{uuid}/messages` ile mesajı yazın. Kanal web widget'ı ise kullanıcı sitenize girip `identify` ile oturum açtığında konuşmayı ve mesajı widget'ta görür.

## Doğrulanmış, anonim, adını kendisi yazdı

Kişi ve konuşma yanıtlarında `identity_state` alanı vardır:

| Değer | Anlamı |
|---|---|
| `verified` | Kişi widget `identify` çağrısında `user_hash` ile kimliğini kanıtladı. `verified_at` son doğrulama zamanıdır. |
| `anonymous` | Ad, e-posta veya telefon yok. |
| `self_named` | Kişi ya da kanalı bir ad, e-posta veya telefon verdi, ama kanıtlanmadı. |

API ile oluşturulan kişi, kullanıcı `identify` ile oturum açana kadar doğrulanmış sayılmaz.

## Webhook'larda dış kimlik

Konuşma olaylarının `data` alanı `contact_external_id`, `contact_verified`, `company_uuid` ve `company_external_id` taşır; `contact.created` olayı `contact_external_id` taşır. Kendi kaydınızı ek bir istek atmadan eşleştirebilirsiniz.
