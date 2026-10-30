-- Data-loss note: removes quotes and decision evidence; converted subscriptions remain valid.
DROP TABLE enterprise_quotes;
ALTER TABLE billing_profiles DROP COLUMN payment_terms_days;
