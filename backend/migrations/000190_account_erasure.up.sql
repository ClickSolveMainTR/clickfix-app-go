-- CF-127: durable deletion deadline; recovery never depends on Redis availability.
ALTER TABLE users ADD COLUMN deletion_due_at TIMESTAMPTZ;
CREATE INDEX users_deletion_due_idx ON users(deletion_due_at,id) WHERE deleted_at IS NULL AND deletion_due_at IS NOT NULL;
UPDATE users SET deletion_due_at=deactivated_at+interval '7 days' WHERE deactivated_at IS NOT NULL AND deleted_at IS NULL;
UPDATE notification_templates SET body=CASE language
 WHEN 'tr' THEN E'Merhaba {{name}},\n\nHesabınız kapatıldı ve tüm oturumlarınız sonlandırıldı. Kişisel hesap verileriniz {{purge_date}} tarihinde kalıcı olarak silinecektir. Giriş yapmak silme talebini iptal etmez. İptal için bu tarihten önce destek ekibiyle iletişime geçin.'
 ELSE E'Hello {{name}},\n\nYour account is disabled and all sessions have ended. Your personal account data will be permanently erased on {{purge_date}}. Signing in does not cancel deletion. Contact support before this date to cancel.' END
 WHERE code='auth.account_deactivated' AND channel='email';
