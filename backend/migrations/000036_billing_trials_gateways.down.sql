-- Data-loss note: removes gateway attempt audit, trial enrollment/reminder history and settings.
DROP TABLE billing_payment_attempts;
DROP TABLE billing_gateway_settings;
DROP TABLE billing_trial_reminders;
DROP TABLE billing_trial_enrollments;

DROP INDEX uq_notifications_source_event_channel;
