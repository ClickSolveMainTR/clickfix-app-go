# SDK'lar ve örnek kod

Go ve Node.js için sunucu SDK'ları vardır: `@clickfix/node` (npm, Node 22+), `@clickfix/react-widget` (npm, React 18+) ve `github.com/ClickSolveMainTR/clickfix-sdks/go` (Go modülü, `go/vX.Y.Z` etiketleri). Tüm diller için kullanım örnekleri [SDK'lar ve örnek kod](https://getclickfix.com/docs/sdks) sayfasında, baştan sona kurulum sırası ve her adımda doğrulanmış örnekler [Entegrasyon kılavuzu](https://getclickfix.com/docs/integration-guide) sayfasındadır. SDK'lar kimlik doğrulama, sınırlı tekrar deneme (`Retry-After` dahil), sayfalama ve webhook imza doğrulaması sağlar. Temel adres olarak `https://api.getclickfix.com` verin (sonuna `/v1` eklemeyin). Her uç noktanın sağ panelinde curl, JavaScript, Python ve Go için hazır istek örnekleri de bulunur.

**curl**

```bash
curl -X POST "https://api.getclickfix.com/v1/public-api/conversations/KONUSMA_UUID/messages" \
  -H "Authorization: Bearer $CLICKFIX_API_KEY" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: siparis-1042-not" \
  -d '{ "content": "Sipariş kargoya verildi.", "private": true }'
```

**JavaScript / TypeScript (Node.js 22+)**

```ts
import { createClient } from "@clickfix/node";

const client = createClient({
  baseURL: "https://api.getclickfix.com",
  apiKey: process.env.CLICKFIX_API_KEY!,
});

const created = await client.request(
  "/v1/public-api/conversations",
  "post",
  { contact_uuid: contactUUID, inbox_uuid: inboxUUID, subject: "Teslimat sorusu" },
  { idempotencyKey: "siparis-1042-destek" },
);
```

SDK kullanmadan `fetch` ile:

```ts
const res = await fetch("https://api.getclickfix.com/v1/public-api/contacts?limit=20", {
  headers: { Authorization: `Bearer ${process.env.CLICKFIX_API_KEY}` },
});
const body = await res.json();
if (!body.success) throw new Error(`${body.error.code}: ${body.error.message}`);
```

**Go**

```go
client, err := clickfix.New("https://api.getclickfix.com", os.Getenv("CLICKFIX_API_KEY"))
if err != nil {
    log.Fatal(err)
}
res, err := client.GetV1PublicApiContactsWithResponse(ctx, &clickfix.GetV1PublicApiContactsParams{})
```

**Python (requests)**

```python
import os, requests

API = "https://api.getclickfix.com/v1/public-api"
headers = {"Authorization": f"Bearer {os.environ['CLICKFIX_API_KEY']}"}

contacts = requests.get(f"{API}/contacts", params={"limit": 20}, headers=headers, timeout=30).json()

created = requests.post(
    f"{API}/conversations",
    json={"contact_uuid": contact_uuid, "inbox_uuid": inbox_uuid, "subject": "Teslimat sorusu"},
    headers={**headers, "Idempotency-Key": "siparis-1042-destek"},
    timeout=30,
).json()
```

`Idempotency-Key` için işlemi tanımlayan sabit bir değer (ör. sipariş numarası) kullanın ve tekrar denemelerde aynı değeri gönderin; her denemede yeni değer üretmeyin.
