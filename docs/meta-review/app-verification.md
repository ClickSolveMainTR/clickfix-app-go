# App verification details (uygulama geneli)

Forma İngilizce yapıştırılır. Gözden geçiren hesabının e-posta ve şifresi buraya **yazılmaz** (repo herkese açık);
formda ilgili alana gönderim sırasında elle girilir.

## App verification text

ClickFix (https://getclickfix.com) is a multi-tenant B2B customer support platform. Each business is an organization in
ClickFix and connects its own WhatsApp Business number, Facebook Page and Instagram professional account; messages from
those channels land in one shared inbox where the business's agents answer them.

To test:

1. Go to https://getclickfix.com/login and sign in with the reviewer account below. The account belongs to a demo
   organization whose panel language is English. No two-factor code is required for this account.
2. Channels are under **Channels** in the left sidebar; conversations are under **Inbox**.
3. The screencasts attached to each permission show the full flow: connecting the channel, receiving a customer message
   and replying from ClickFix.

Reviewer account: (entered in the form at submission time)

## Platform

- Website: https://getclickfix.com
- Privacy policy: https://getclickfix.com/legal/privacy
- Terms of service: https://getclickfix.com/legal/terms
- Data deletion instructions: https://getclickfix.com/account-deletion

## Gönderim öncesi kontrol (Türkçe)

- Gözden geçiren hesabı prod'da, 2FA kapalı, İngilizce dilli bir demo organizasyonda; şifre sadece forma girilir.
- Test Sayfası, test Instagram profesyonel hesabı ve WhatsApp test numarası uygulamaya bağlanmış ve gözden geçirene
  tester rolüyle erişilebilir.
- Webhook'lar doğrulanmış (bkz. `docs/meta-kurulum.md` "Kalanlar").
- Videolar son sürümle yeniden üretildi (Actions → Meta review videos).
