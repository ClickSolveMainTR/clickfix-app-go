# Messenger from Meta: App Review metinleri

Forma İngilizce yapıştırılır. Video: `messenger.mp4` (+ `messenger.vtt`), senaryo
`frontend/e2e/meta-review/messenger.review.ts`.

## Use case summary

ClickFix is a B2B customer support platform (shared inbox, SLAs, routing, AI drafts). A business connects its own
Facebook Page in ClickFix so that messages customers send to that Page on Messenger arrive in the ClickFix shared inbox,
where the business's support agents read and answer them. ClickFix only acts on Pages the signed-in admin explicitly
selects, and only sends messages as a reply to a customer who wrote to the Page first.

## Permissions and features

### pages_show_list

**How we use it:** After the business admin signs in with Facebook from *Channels → Add channel → Messenger → Connect
with Meta*, ClickFix calls `GET /me/accounts` and shows the list of Pages the admin manages, so the admin can pick the
one Page to connect. We store only the chosen Page's id, name and Page access token (encrypted at rest, never shown in
the browser).

**Why it is necessary:** Without it the admin cannot choose which Page receives the support conversations.

### pages_manage_metadata

**How we use it:** Right after the admin selects the Page, ClickFix calls `POST /{page-id}/subscribed_apps` with the
fields `messages, messaging_postbacks, message_deliveries, message_reads, message_echoes`, so new Messenger messages
for that Page are delivered to our webhook (`https://api.getclickfix.com/v1/webhooks/channels/messenger`). When the
business disconnects the channel we remove the subscription.

**Why it is necessary:** It is the only way to receive the Page's incoming messages in real time.

### pages_messaging

**How we use it:** When a customer writes to the connected Page, the message appears as a conversation in the ClickFix
inbox. The agent types a reply and presses *Send*; ClickFix sends it with the Send API (`POST /{page-id}/messages`,
`messaging_type: RESPONSE`) and shows delivery/read receipts in the thread.

**Why it is necessary:** This is the core feature: answering customers who contact the business on Messenger.

### pages_read_engagement

**How we use it:** When the Page is connected and on the periodic health check, ClickFix reads the Page's `id` and
`name` (`GET /{page-id}?fields=id,name`) to label the channel and its conversations in the inbox. The same token is
used to read back recent Page messages (`/{page-id}/conversations`) when a webhook delivery was missed, so no
customer message is lost.

**Why it is necessary:** Agents need to see which Page a conversation belongs to, and the inbox must stay complete.

### Human Agent

**How we use it:** Support cases often need more than 24 hours (e.g. waiting for a technician). ClickFix keeps the
reply box open for 7 days after the customer's last message. If Messenger rejects an agent's reply because the
standard 24 hour window has closed, ClickFix resends that same human-written reply once with
`messaging_type: MESSAGE_TAG` and `tag: HUMAN_AGENT`. Automated, AI or promotional messages are never sent with this
tag. The video's last step shows such a follow-up written by an agent.

## Step-by-step instructions for the reviewer

1. Open `https://getclickfix.com/login` and sign in with the reviewer account from *App verification details*.
   The panel is in English.
2. Go to **Channels → Add channel → Messenger → Connect with Meta**. Sign in with Facebook and grant the requested
   permissions to the test Page.
3. Pick the test Page in *Pick the page to connect* and confirm. The channel appears as *Connected*.
4. From a Facebook account with the tester role, send a message to the test Page on Messenger.
5. In ClickFix open **Inbox → All conversations**: the conversation appears. Open it, type a reply and press **Send**.
   The reply arrives in Messenger.

## Manual clip

The Facebook login and permission dialog is not part of the automated video. If requested, record it with the test
account and insert it after the caption "Connecting the Page with Facebook Login".
