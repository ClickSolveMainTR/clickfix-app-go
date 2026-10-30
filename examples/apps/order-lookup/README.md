# Sipariş sorgulama — ClickFix örnek uygulaması

Konuşma kenar çubuğunda müşterinin siparişlerini gösteren, bağımlılıksız (Node.js 22+) küçük bir örnek. Kendi uygulamanızı yazarken başlangıç noktası olarak kullanın; ayrıntılı rehber: [Kendi uygulamanızı yapın](../../../frontend/content/docs/tr/uygulamalar.md).

Ne gösterir:

- `clickfix.ready` → `clickfix.context` el sıkışması ve bağlam belirtecinin (token) yenilenmesi (`public/app.js`)
- Belirtecin sunucuda JWKS ile doğrulanması: EdDSA imzası, `iss`, `aud`, `exp`/`nbf` (`lib/context-token.mjs`)
- Konuşmanın müşteri e-postasının herkese açık API ile okunması (`server.mjs`)
- `clickfix.action` ile konuşmaya iç not ekleme ve müşteriye yanıt gönderme
- Açık/koyu tema, dar kenar çubuğuna uygun düzen, yalnızca ClickFix panelinin çerçeveleyebilmesi (`frame-ancestors`)

## Çalıştırma

```bash
cd examples/apps/order-lookup
CLICKFIX_APP_ID=<uygulama-kimliği> \
CLICKFIX_API_KEY=cf_sk_... \
PANEL_ORIGINS=https://getclickfix.com \
npm start
```

| Değişken | Açıklama |
|---|---|
| `CLICKFIX_APP_ID` | ClickFix'te uygulamayı kaydettikten sonra kartında görünen **Uygulama kimliği**. Belirtecin `aud` değeridir. |
| `CLICKFIX_API_KEY` | `conversations:read` kapsamlı API anahtarı. Boşsa uygulama demo modunda `demo@example.com` siparişlerini gösterir. |
| `CLICKFIX_API_URL` | Varsayılan `https://api.getclickfix.com` |
| `PANEL_ORIGINS` | Uygulamayı çerçeveleyebilecek panel adres(ler)i, virgülle ayrılır. |
| `PORT` | Varsayılan `8787` |

Uygulama HTTPS üzerinden ve ClickFix panelinden **farklı bir adreste** yayında olmalıdır (ör. `https://siparis.ornek.com`). Yerelde denemek için bir HTTPS tüneli (Cloudflare Tunnel, ngrok vb.) kullanabilirsiniz.

## ClickFix'e ekleme

1. **Organizasyon → Uygulama yönetimi** sayfasında galeriden **Sipariş sorgulama** kartının **Kur** düğmesine basın.
2. Adres alanına uygulamanızın HTTPS adresini yazın ve kaydedin.
3. Kartta görünen **Uygulama kimliği**'ni kopyalayıp `CLICKFIX_APP_ID` olarak verin.
4. Bir konuşma açın; sağ paneldeki **Uygulamalar** bölümünde siparişler görünür.

`data/orders.json` örnek veridir; gerçek kullanımda burayı kendi sipariş sisteminize yapılan bir çağrıyla değiştirin.

## Test

```bash
npm test
```

Lisans: MIT.
