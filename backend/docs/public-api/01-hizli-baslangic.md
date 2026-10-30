# Hızlı başlangıç

ClickFix REST API'si; konuşma açmanızı, mesaj ve iç not eklemenizi, kişi ve şirket verinizi senkron tutmanızı ve olaylara webhook ile abone olmanızı sağlar. Bu başvuruda yalnızca herkese açık ve kararlı uç noktalar yer alır; panelin kendi kullandığı iç uç noktalar burada listelenmez ve API anahtarıyla çağrılamaz.

**Temel adres:** `https://api.getclickfix.com/v1/public-api`

Beş dakikada ilk isteğiniz:

1. Panelde **Organizasyon → API ve otomasyonlar → API anahtarları** sayfasını açın ve **Yeni API anahtarı**'na basın.
2. Anahtara bir ad verin ve yalnızca ihtiyacınız olan kapsamları seçin. Bu örnek için `contacts:read` ve `conversations:write` yeterli.
3. **Anahtar oluştur**'a basın ve `cf_sk_` ile başlayan anahtarı hemen kopyalayın. Anahtar yalnızca bir kez gösterilir.
4. Anahtarı sunucunuzda bir ortam değişkenine koyun ve kişileri listeleyin:

```bash
export CLICKFIX_API_KEY="cf_sk_..."

curl "https://api.getclickfix.com/v1/public-api/contacts?limit=5" \
  -H "Authorization: Bearer $CLICKFIX_API_KEY"
```

5. Bir kişi ve bir kanal (`inbox_uuid`) seçip konuşma açın. Yazma isteklerinde `Idempotency-Key` zorunludur:

```bash
curl -X POST "https://api.getclickfix.com/v1/public-api/conversations" \
  -H "Authorization: Bearer $CLICKFIX_API_KEY" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: siparis-1042-destek" \
  -d '{
    "contact_uuid": "KISI_UUID",
    "inbox_uuid": "KANAL_UUID",
    "subject": "Teslimat sorusu"
  }'
```

Yanıt `201` ve `data.uuid` alanında konuşmanın kimliğini döner. Aynı isteği aynı `Idempotency-Key` ile tekrar gönderirseniz yeni konuşma açılmaz, ilk yanıt aynen döner.

Sonraki adımlar: aşağıdaki **Kimlik doğrulama ve kapsamlar** ve **Webhook'lar** bölümleri, ardından uç nokta başvurusu. Kullanım kılavuzu: [getclickfix.com/docs/api](https://getclickfix.com/docs/api).
