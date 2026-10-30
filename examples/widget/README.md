# Web widget SDK örneği (CF-150)

Bağımlılıksız (Node.js 22+) küçük bir site: ClickFix web widget'ını yükler ve SDK'nın
neredeyse her çağrısını düğmelerle dener. Sağdaki günlük, widget'ın yaydığı olayları gösterir.
SDK başvurusu: [docs/widget-sdk.md](../../docs/widget-sdk.md).

Ne gösterir:

- Kuyruk parçacığı ile yükleme (`ClickFix("boot", …)` betik yüklenmeden önce çağrılabilir)
- Kendi düğmeniz (`data-clickfix-launcher`, okunmamış sayısı `data-clickfix-unread`), balonu gizleme
- `identify` (kullanıcı kimliği sunucuda HMAC ile imzalanır: `server.mjs` → `/user-hash`), `update`,
  `setAttributes`, `track`
- `startConversation` (hazır mesajla), `showMessage` (programatik proaktif mesaj), `searchHelp`,
  `open("help")`, `setLocale`, `setTheme`, `consent`, `getState`, `reset`, `shutdown`
- Olaylar: `ready`, `open`, `close`, `message`, `unread`, `conversation:started`, `conversation:ended`,
  `csat`, `trigger`, `attributes`, `visibility`, `error`
- Tek sayfa uygulama gezinmesi (`history.pushState`): hedefleme ve proaktif mesajlar her gezinmede yeniden değerlendirilir
- React: [`react/Support.tsx`](react/Support.tsx) (`@clickfix/react-widget` ile tipli hook'lar)

## Çalıştırma

```bash
cd examples/widget
CLICKFIX_APP=https://app.getclickfix.com \
CLICKFIX_WIDGET_ID=<widget public id> \
CLICKFIX_IDENTITY_SECRET=<widget kimlik doğrulama anahtarı> \
npm start
# http://localhost:8788
```

| Değişken | Açıklama |
|---|---|
| `CLICKFIX_APP` | `widget.js`'i sunan ClickFix adresi |
| `CLICKFIX_WIDGET_ID` | Panel → Web widget → Kurulum'daki herkese açık id |
| `CLICKFIX_IDENTITY_SECRET` | İsteğe bağlı. Panel → Web widget → Güvenlik'te üretilen anahtar; yoksa `identify` düğmesi devre dışıdır |
| `PORT` | Varsayılan `8788` |

Widget'ın **İzinli siteler** listesi doluysa `localhost`'u ekleyin. Kimlik doğrulama anahtarı yalnız
sunucuda kalır; tarayıcıya hiçbir zaman gönderilmez.
