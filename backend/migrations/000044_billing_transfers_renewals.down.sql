-- Data-loss note: removes renewal enrollment and receipt linkage; files remain in attachments.
DROP TABLE billing_payment_notices,billing_renewal_reminders;
ALTER TABLE billing_subscriptions DROP COLUMN renewal_enabled;
ALTER TABLE billing_dunning DROP COLUMN last_reminder_day;
DROP INDEX uq_billing_transfer_attachment;
ALTER TABLE billing_bank_transfers DROP COLUMN attachment_uuid;
ALTER TABLE billing_bank_accounts DROP COLUMN branch,DROP COLUMN description;
DROP INDEX uq_billing_invoice_reference;
ALTER TABLE billing_invoices DROP COLUMN payment_reference;
