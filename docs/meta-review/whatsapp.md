# Connect on WhatsApp: App Review metinleri

Forma İngilizce yapıştırılır. Video: `whatsapp.mp4` (+ `whatsapp.vtt`), senaryo
`frontend/e2e/meta-review/whatsapp.review.ts`.

## Use case summary

ClickFix is a B2B customer support platform acting as a Tech Provider. A business connects its own WhatsApp Business
Account and phone number to ClickFix with Embedded Signup. Customers' WhatsApp messages arrive in the ClickFix shared
inbox and agents answer them with the Cloud API. Businesses also create and track their message templates (submitted
to Meta for approval) from ClickFix.

## Permissions

### whatsapp_business_management

**How we use it:** After Embedded Signup returns the WABA id and phone number id, ClickFix subscribes our app to the
WABA's webhooks (`POST /{waba-id}/subscribed_apps`), registers the number for the Cloud API, reads the number's display
name and quality rating for the channel health view, and lists/creates/deletes the business's message templates
(*Channels → WhatsApp → Templates*).

**Why it is necessary:** Without it we cannot receive the business's messages or manage the templates it uses.

### whatsapp_business_messaging

**How we use it:** Incoming customer messages (text, media, button replies) arrive through the WhatsApp webhook
(`https://api.getclickfix.com/v1/webhooks/channels/whatsapp`) and appear in the inbox. The agent's reply is sent with
`POST /{phone-number-id}/messages` (text and media). Delivery and read statuses update the thread. The reply box is
closed outside the 24 hour customer service window.

**Why it is necessary:** Messaging customers on behalf of the business is the core feature.

### business_management

**How we use it:** Only during Embedded Signup, to read the business portfolio the admin selected and access the WABA
it shares with ClickFix. We do not manage any other business assets.

## Step-by-step instructions for the reviewer

1. Sign in at `https://getclickfix.com/login` with the reviewer account (English UI).
2. **Channels → Add channel → WhatsApp → Connect WhatsApp with Meta**. In the Meta window select the test business,
   WhatsApp Business Account and test number. The channel appears as *Connected* with the number.
3. Send a WhatsApp message to the test number from a tester phone. Open it in **Inbox** and reply; the reply arrives in WhatsApp.
4. Open **Channels**, the WhatsApp card's menu → **Templates**: the business's templates and their review status are
   listed; create a new template and press **Submit for approval**.

## Manual clip

The Embedded Signup window belongs to Meta and is not part of the automated video. If requested, record it with the test
account and insert it after the caption "Embedded Signup: the business picks its WhatsApp account and number".
