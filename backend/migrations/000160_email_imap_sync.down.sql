-- Data-loss note: drops IMAP cursors and backoff state; messages remain deduplicated.
DROP TABLE IF EXISTS channel_email_imap_states;
