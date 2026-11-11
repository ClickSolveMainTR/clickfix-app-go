# Webhook'lar

Webhook'lar, ClickFix'te bir olay olduğunda (yeni konuşma, gelen mesaj, SLA ihlali…) belirttiğiniz HTTPS adresine imzalı bir `POST` isteği gönderir. Böylece yoklama yapmadan sisteminizi güncel tutarsınız.

## Abonelik

`webhooks:write` kapsamlı bir anahtarla `POST /webhooks` çağırın. Yanıttaki `data.secret` imza anahtarıdır ve **yalnızca oluşturma yanıtında** döner; güvenli bir yerde saklayın.

- Adres `https://` olmalı ve herkese açık bir sunucuya işaret etmelidir; `localhost`, özel IP'ler ve yönlendirmeler (3xx) kabul edilmez.
- Sunucunuz 10 saniye içinde `2xx` dönmelidir. Başarısız teslimatlar artan aralıklarla (1 dk, 5 dk, 30 dk, 2 sa, 6 sa, 12 sa, 24 sa) tekrar denenir; bu yüzden aynı olay birden fazla gelebilir.
- Zapier, Make veya n8n gibi araçlar için `POST /subscriptions` (REST hook) aynı teslimat altyapısını kullanır.

## Olaylar

| Grup | Olaylar |
|---|---|
| Konuşma | `conversation.created`, `conversation.updated`, `conversation.assigned`, `conversation.unassigned`, `conversation.status_changed`, `conversation.priority_changed`, `conversation.resolved`, `conversation.reopened`, `conversation.snoozed`, `conversation.tag_added` |
| Mesaj | `message.received`, `message.sent`, `message.failed` |
| Kişi ve şirket | `contact.created`, `contact.updated`, `contact.merged`, `company.created`, `company.updated` |
| SLA | `sla.warning`, `sla.breached` |
| Memnuniyet | `csat.submitted` |

Listeye zamanla yeni olaylar eklenir; tanımadığınız olay türlerini yok sayın.

## İstek biçimi

| Başlık | Açıklama |
|---|---|
| `ClickFix-Webhook-Id` | Teslimatın benzersiz kimliği. Tekrar denemelerde aynı kalır; tekilleştirmek için kullanın. |
| `ClickFix-Webhook-Event` | Olay adı, ör. `conversation.created` |
| `ClickFix-Webhook-Timestamp` | İsteğin imzalandığı an (Unix saniye) |
| `ClickFix-Webhook-Signature` | `v1=` + onaltılık (hex) HMAC-SHA256 imzası |

```json
{
  "id": "6c1d…",
  "event": "conversation.created",
  "version": 1,
  "occurred_at": "2026-10-29T10:15:00Z",
  "data": { "conversation_uuid": "8f0e…", "status": "open", "channel_type": "whatsapp" }
}
```

`data` içeriği olaya göre değişir ve yalnızca kimlikleri ve özet alanları taşır. Ayrıntı gerekiyorsa kaydı API ile okuyun.

Konuşma olayları kişinin ve şirketin dış kimliğini de taşır (`contact_external_id`, `company_external_id`); ayrıntı için **Dış kimlik ve doğrulanmış müşteri** bölümüne bakın.

## İmza doğrulama

```text
ClickFix-Webhook-Signature = "v1=" + hex(HMAC-SHA256(secret, timestamp + "." + ham_gövde))
```

1. Gövdeyi JSON olarak ayrıştırmadan önce, gelen **ham baytlarla** imzayı hesaplayın.
2. Sonucu başlıktaki değerle **sabit zamanlı** karşılaştırın.
3. Zaman damgası 5 dakikadan eski veya ileriyse isteği reddedin.
4. `ClickFix-Webhook-Id` değerini kaydedin; aynı kimlik tekrar gelirse işlemi yinelemeyin.
5. Olayı kalıcı olarak kaydettikten sonra `2xx` dönün; uzun işleri arka planda yapın.

**Node.js**

```js
import crypto from "node:crypto";

export function verifyClickFix(rawBody, timestamp, signature, secret) {
  const age = Math.abs(Date.now() / 1000 - Number(timestamp));
  if (!signature || !Number.isFinite(age) || age > 300) return false;
  const expected =
    "v1=" +
    crypto.createHmac("sha256", secret).update(`${timestamp}.`).update(rawBody).digest("hex");
  const a = Buffer.from(expected);
  const b = Buffer.from(signature);
  return a.length === b.length && crypto.timingSafeEqual(a, b);
}
```

**Python**

```python
import hashlib, hmac, time

def verify_clickfix(raw_body: bytes, timestamp: str, signature: str, secret: str) -> bool:
    try:
        if abs(time.time() - int(timestamp)) > 300:
            return False
    except (TypeError, ValueError):
        return False
    mac = hmac.new(secret.encode(), f"{timestamp}.".encode() + raw_body, hashlib.sha256)
    return hmac.compare_digest("v1=" + mac.hexdigest(), signature or "")
```

**Go**

```go
err := clickfix.VerifyWebhook(secret,
    r.Header.Get("ClickFix-Webhook-Timestamp"),
    r.Header.Get("ClickFix-Webhook-Signature"),
    rawBody, time.Now(), 5*time.Minute)
```

## API kanalı

Kendi sohbet uygulamanızı veya sisteminizi ClickFix'e bir kanal olarak bağlamak için **API kanalı** kullanılır. Kanal kurulduğunda size bir gelen adresi (`/v1/webhooks/channels/api/{instance}`) ve imza anahtarı verilir. Müşteri mesajlarını bu adrese gönderirsiniz (aşağıda **API kanalı** başlığı altında), temsilcilerin yanıtları ise kanalda tanımladığınız adrese `apiChannelReply` webhook'u olarak gelir. İmza başlıkları `X-ClickFix-Timestamp` ve `X-ClickFix-Signature: sha256=…` biçimindedir; hesaplama yukarıdakiyle aynıdır.
