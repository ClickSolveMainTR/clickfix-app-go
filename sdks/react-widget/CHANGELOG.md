# @clickfix/react-widget

Sürümleme: [SemVer](https://semver.org). `0.x` sürümlerinde küçük sürüm (`0.N`) kırıcı değişiklik taşıyabilir; her kırıcı değişiklik burada **Kırıcı** başlığıyla yazılır. Sürüm `package.json` içindedir; `main`'e birleşen bir değişiklikte sürüm değiştiyse CI (`sdk-publish`) npm'e yayımlar.

## 0.1.0 - 2026-11-12

İlk yayın (CF-199).

- `loadClickFix`, `clickFix`, `useClickFix`, `useClickFixEvent`: hazır widget'ı (`widget.js`, SDK 2.1) React'ten yükleyen tipli kancalar.
- `createWidgetClient`, `useWidget`, `useConversations`, `useMessages`, `useHelpSearch`, `useTyping`: kendi arayüzünüz için arayüzsüz ziyaretçi istemcisi (aynı kökenli `/api/v1/widget/*`).
- `widget.d.ts` sözleşmesinin kopyası (`ClickFixSDK`, olaylar, `ClickFixBridge` webview köprüsü tipleri).
