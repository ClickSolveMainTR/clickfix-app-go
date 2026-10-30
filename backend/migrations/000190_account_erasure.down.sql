-- Data-loss note: deletion deadlines are removed; erased personal data cannot be restored.
DROP INDEX users_deletion_due_idx;
ALTER TABLE users DROP COLUMN deletion_due_at;
UPDATE notification_templates SET body=CASE language
 WHEN 'tr' THEN E'Merhaba {{name}},\n\nHesabınız isteğiniz üzerine pasifleştirildi ve tüm oturumlarınız kapatıldı. Artık giriş yapamazsınız.\n\nBu işlemi siz yapmadıysanız lütfen destek ekibiyle iletişime geçin.'
 ELSE E'Hello {{name}},\n\nYour account was deactivated at your request and all sessions were signed out. You can no longer sign in.\n\nIf you did not do this, please contact support.' END
 WHERE code='auth.account_deactivated' AND channel='email';
