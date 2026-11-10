# Kılavuz örnekleri (CF-199)

[Entegrasyon kılavuzundaki](../../frontend/content/docs/tr/integration-guide.mdx) kod örneklerinin kaynağı burasıdır; sayfalardaki kod bloklarına `sync-docs.mjs` ile gömülür (`pnpm examples:sync`, CI `--check` ile denetler). Örnekler değişince önce burada değiştirin, sonra eşitleyin.

| Klasör | İçerik | Nasıl doğrulanır |
|---|---|---|
| `node/` | `@clickfix/node` ile adım adım TypeScript (Node 22, tip silme ile doğrudan çalışır) | çalıştırılır |
| `go/` | Go modülü; `go run . <adım>` (`replace` ile `../../go`) | çalıştırılır |
| `curl/` | bash + curl + openssl | çalıştırılır |
| `widget/` | tarayıcıdaki `ClickFix(...)` çağrıları | `frontend/public/widget.d.ts` ile `tsc --checkJs` |

```sh
cd sdks
pnpm examples            # stub API ile (CI)
pnpm examples:check      # yalnız derleme ve tip kontrolü
CLICKFIX_BASE_URL=http://localhost:8080 CLICKFIX_API_KEY=cf_sk_... \
CLICKFIX_IDENTITY_SECRET=... CLICKFIX_CHANNEL_INSTANCE=... CLICKFIX_CHANNEL_SECRET=... \
pnpm examples            # gerçek (yerel) backend ile
```

`make guide-examples` kökten aynı işi yapar. `CLICKFIX_BASE_URL` verilmezse `stub-server.mjs` kalkar: `public-api.yaml` şekillerine uyan, anahtar, `Idempotency-Key`, JSON gövde ve API kanalı imzasını denetleyen küçük bir sunucu. Ürünün yerini tutmaz; API'nin kendisi değiştiğinde örnekleri yerel backend'e karşı da koşturun. Yerel backend için gereken değerler: bir organizasyonda `contacts:*`, `companies:read`, `health:write`, `events:write`, `webhooks:write` kapsamlı API anahtarı; widget kimlik anahtarı; bir API kanalının kimliği ve imza anahtarı.
