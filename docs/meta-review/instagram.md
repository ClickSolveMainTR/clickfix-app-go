# Instagram API (Instagram Login): App Review metinleri

Forma İngilizce yapıştırılır. Video: `instagram.mp4` (+ `instagram.vtt`), senaryo
`frontend/e2e/meta-review/instagram.review.ts`.

## Use case summary

ClickFix is a B2B customer support platform. A business connects its Instagram professional account with Instagram
Login so that Direct messages, story replies/mentions and comments on its posts arrive in the ClickFix shared inbox.
Support agents answer DMs and reply publicly to comments. ClickFix never publishes posts or reads other accounts'
content.

## Permissions

### instagram_business_basic

**How we use it:** After the admin connects the account (*Channels → Add channel → Instagram → Connect with Meta*), we
read the account's `user_id` and `username` to label the channel and the conversations. The
`user_id` is also how we route incoming webhooks to the right business.

**Why it is necessary:** Required by every other Instagram permission and to show which account a conversation belongs to.

### instagram_business_manage_messages

**How we use it:** We subscribe the account to `messages, messaging_postbacks, messaging_seen, message_reactions`
(`POST /me/subscribed_apps`). Incoming DMs and story replies become conversations in the ClickFix inbox; the agent's
reply is sent with `POST /{ig-user-id}/messages`. Replies after 24 hours use the `HUMAN_AGENT` tag (within 7 days,
human-written only).

**Why it is necessary:** Answering customers who contact the business by Instagram Direct is the main use case.

### instagram_business_manage_comments

**How we use it:** With the `comments` and `mentions` webhook fields, each new top-level comment on the business's own
posts opens its own conversation in the inbox, showing the commenter's username and the post. The agent's answer in
that conversation is posted as a public reply under the comment (`POST /{comment-id}/replies`). Mentions of the
business in comments on other posts are handled the same way.

**Why it is necessary:** Customers ask support questions in comments; businesses want to answer them from the same inbox.

## Step-by-step instructions for the reviewer

1. Sign in at `https://getclickfix.com/login` with the reviewer account (English UI).
2. **Channels → Add channel → Instagram → Connect with Meta**. Log in to the test Instagram professional account and
   allow the requested permissions. You return to ClickFix and the channel shows as *Connected* with the username.
3. From an Instagram tester account, send a DM to the test account. In ClickFix **Inbox** open the conversation and reply;
   the reply arrives in Instagram Direct.
4. Comment on a post of the test account. The comment appears in the inbox as its own conversation. Type a reply and
   press **Send**; it appears as a public reply under the comment.

## Manual clip

The Instagram login and consent screen is not part of the automated video. If requested, record it with the test
account and insert it after the caption "Connecting the Instagram professional account with Instagram Login".
