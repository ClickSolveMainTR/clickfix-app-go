# ClickFix

**ClickFix** is an AI-first, multi-tenant customer support platform for B2B teams. Conversations from the web widget, mobile apps, WhatsApp, e-mail and the API land in one inbox; an AI assistant answers from your knowledge base and from your own systems (invoices, orders, subscriptions…), and hands over to an agent with the full context.

🌐 [getclickfix.com](https://getclickfix.com) · 📘 [Integration guide](https://getclickfix.com/docs/integration-guide) · 📚 [API reference](https://getclickfix.com/docs/api-reference) · 🔔 [Webhooks](https://getclickfix.com/docs/webhooks) · 🧪 [Example apps](https://github.com/ClickSolveMainTR/clickfix-examples)

## How it fits together

```
 your website ── widget.js / @getclickfix/react-widget ──┐
 your mobile app ── @getclickfix/react-native · expo ────┤  visitor API  (https://getclickfix.com/api/v1/widget/*)
                  └ clickfix_webview · webview bridge ────┤
                                                          ▼
                                                   ClickFix (inbox, AI, automations)
                                                          ▲
 your server ── @getclickfix/node · Go · PHP · Laravel ───┘  public API   (https://api.getclickfix.com/v1/public-api/*)
             ◄── signed webhooks (ClickFix-Webhook-Signature)
```

- **Clients** (browser, app) talk to the visitor API with a public widget id, or a mobile app key bound to your bundle id. They never hold a secret.
- **Your server** holds the API key and the widget's identity secret. It syncs contacts, companies, custom fields and events, verifies webhooks, and signs `user_hash` so the widget knows who the signed-in user is:
  `hex(HMAC-SHA256(identity_secret, user_id))`, or with a company `hex(HMAC-SHA256(identity_secret, user_id + "\n" + company_id))`.

## Packages

| Package | Install | For |
|---|---|---|
| [`@getclickfix/node`](https://www.npmjs.com/package/@getclickfix/node) | `npm install @getclickfix/node` | Node 22+ servers |
| [Go module](https://github.com/ClickSolveMainTR/clickfix-sdks/tree/main/go) | `go get github.com/ClickSolveMainTR/clickfix-sdks/go` | Go 1.24+ servers |
| [`clickfix/sdk`](https://packagist.org/packages/clickfix/sdk) | `composer require clickfix/sdk guzzlehttp/guzzle` | PHP 8.1+ servers |
| [`clickfix/laravel`](https://packagist.org/packages/clickfix/laravel) | `composer require clickfix/laravel` | Laravel 10-13 |
| [`@getclickfix/react-widget`](https://www.npmjs.com/package/@getclickfix/react-widget) | `npm install @getclickfix/react-widget` | React 18+ and Next.js (SSR-safe hooks, UI-free client) |
| [`@getclickfix/react-native`](https://www.npmjs.com/package/@getclickfix/react-native) | `npm install @getclickfix/react-native` | Bare React Native 0.81+: native screens and hooks |
| [`@getclickfix/expo`](https://www.npmjs.com/package/@getclickfix/expo) | `npx expo install @getclickfix/expo` | Expo SDK 54+ |
| [`@getclickfix/visitor-core`](https://www.npmjs.com/package/@getclickfix/visitor-core) | `npm install @getclickfix/visitor-core` | DOM-free visitor client and webview bridge types |
| [`clickfix_webview`](https://pub.dev/packages/clickfix_webview) | `flutter pub add clickfix_webview` | Flutter: secure webview of the hosted widget |

Current versions: [Packages table](https://getclickfix.com/docs/integration-guide/sdks). All SDKs are MIT licensed. Sources: [clickfix-sdks](https://github.com/ClickSolveMainTR/clickfix-sdks) (Node, Go, visitor-core, React, React Native, Expo, Flutter), [clickfix-php](https://github.com/ClickSolveMainTR/clickfix-php), [clickfix-laravel](https://github.com/ClickSolveMainTR/clickfix-laravel). They are read-only mirrors published on every release; please report bugs in their *Issues* tab, fixes ship with the next release.

## Quick start

1. **Embed the widget** (Web widget → Install in the panel):
   ```html
   <script src="https://getclickfix.com/widget.js" data-widget="WIDGET_ID" async></script>
   ```
2. **Identify the signed-in user** with a `user_hash` from your server:
   ```ts
   import { identityHash } from "@getclickfix/node";
   const user_hash = identityHash(process.env.CLICKFIX_IDENTITY_SECRET!, user.id, user.companyId);
   // browser: ClickFix("identify", { user_id: user.id, company: { id: user.companyId } }, user_hash)
   ```
3. **Sync your data** with an API key (Organization → API and automations): contacts by your own user id, companies, custom fields, events.
4. **Receive webhooks** and verify `ClickFix-Webhook-Signature` with `verifyWebhook` (Node), `VerifyWebhook` (Go) or `Webhooks::verify` (PHP).
5. **Go mobile**: native screens with [React Native](https://getclickfix.com/docs/integration-guide/react-native) or [Expo](https://getclickfix.com/docs/integration-guide/expo), or the hosted widget in a [webview](https://getclickfix.com/docs/integration-guide/mobile) (Flutter, iOS, Android).

Every step, with Node, Go, PHP, curl and widget samples: [integration guide](https://getclickfix.com/docs/integration-guide). Complete runnable apps for every stack (Express, Next.js, React Native, Expo, Flutter, Go, PHP, Laravel, curl): [clickfix-examples](https://github.com/ClickSolveMainTR/clickfix-examples).

## Contact

Support and partnerships: [getclickfix.com](https://getclickfix.com)
