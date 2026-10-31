# Zorunlu referanslar

Bu dosya, repodaki zorunlu referans kurallarının tek kaynağıdır (eskiden itsplan'deki "Repo kuralları ve zorunlu referanslar" dokümanının kopyasıydı).

## Kural

- Bir maddeye başlamadan önce maddenin **Referans** bölümündeki ve **Referans repo** alanındaki kod okunur.
- Aynı işi yapan kod referans repolarda varsa **sıfırdan yazılmaz**; taşınır (port edilir) ve bu reponun konvansiyonlarına uyarlanır.
- Referanstan sapmak gerekiyorsa nedeni PR açıklamasına ve maddeye yorum olarak yazılır.
- PR açıklamasında "Kullanılan referanslar" bölümü zorunludur: repo + yol + neyin alındığı.
- Referansı olmayan maddelerde açıklamada "Referans: yok (yeni yazılacak)" yazar; yine de önce aşağıdaki haritada benzeri aranır.

## Taban konvansiyonlar (nextjs-go-boilerplate)

- Backend modülü: `backend/internal/modules/<ad>/` (handler, usecase, repository, model, routes.go)
- SQL: migration + sqlc; OpenAPI (`backend/docs/openapi.yaml`) aynı değişiklikte güncellenir
- Listeler `pkg/apiquery`, yanıtlar `pkg/response`
- Yan etkiler `internal/platform/events` / outbox üzerinden; handler'dan doğrudan SMTP yok
- Önemli değişiklikler `activity.Recorder` ile kaydedilir
- Frontend: `frontend/src/features/<ad>/`; tarayıcı sadece BFF'i (`/api/v1/...`) çağırır
- Ayrıntı: `docs/EXTENDING.md` ve `.agents/skills/*`

## Referans haritası

| Alan | Repo | Yol |
|---|---|---|
| Platform, auth, RBAC, bildirim, realtime, arama, depolama, ioengine, bulkengine, step-up, OIDC, QR giriş | nextjs-go-boilerplate | `backend/internal/platform/*`, `backend/internal/modules/*`, `.agents/skills/*` |
| Plan, abonelik, entitlement, PayTR, iyzico, banka hesapları, webhooks | comcode-app-go | `modules/plans`, `subscriptions`, `billing`, `bankaccounts`, `banks`, `webhooks`, `platform/entitlement`, `platform/payment/paytr.go`, `platform/payment/iyzico.go` |
| Faturalama yönetimi, fatura, teklif, sözleşme, WhatsApp, MCP, AI, livepush | otopoly-go | `modules/billing`, `quotes`, `contracts`, `integrations/whatsapp`, `mcp`, `ai`, `livepush`, `notifycenter`, `legal` |
| Ödeme, e-fatura (UBL), iyzico sahte sunucu, SMS, public API, kanallar, destek erişimi | rentopia-app-go | `modules/payments`, `invoices/ubl`, `cmd/iyzico-fake`, `sms`, `publicapi`, `channels`, `supportaccess`, `platformhealth`, `whatsapp` |
| WhatsApp, Instagram, Messenger, ses, MCP + MCP auth, AI asistan, entitlement, kupon, onboarding, onay, mobil | takwim-app-go | `modules/whatsapp`, `instagram`, `messenger`, `voicecall`, `mcp`, `mcpauth`, `aiassist`, `entitlements`, `coupons`, `onboarding`, `consents`, `smsprovider`, `internal/demo`, `mobile/` |
| e-Fatura/e-Arşiv, WhatsApp, AI, MCP, mobil, CI | olexfilms-ecosystem | `modules/einvoice`, `whatsapp`, `ai`, `mcp`, `mobile/`, `.github/workflows/ci.yml` |
| UBL-TR kütüphanesi | go-ubltr | kök |
| WhatsApp kanal servisi | easytrade-whatsapp-channel-go | kök |
| Resend inbound e-posta | 216detay-mail-system | kök |
| Ürün mantığı: ticket AI ajanları, widget, SLA, iş bağlamı, token defteri, iç sohbet | clickfix-app (Laravel) | `app/Ai/*`, `docs/ticket-ai-agents/*`, `docs/widget/*`, `app/Models/*`, `app/Services/*` |
| Sadece fikir ve akış (AGPL kod kopyalanmaz) | Chatwoot, Zammad, Plane, Twenty, Fider, Cachet, OneUptime, GoAlert, OpenReplay | - |
