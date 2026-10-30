-- ClickFix branding for boilerplate defaults (QA): every new user received an
-- "App'e hoş geldiniz" e-mail and the letterhead (PDF exports, e-mail header,
-- /v1/app/config) said "App · Platform & CMS console". Only rows still holding
-- the untouched boilerplate text are changed, so admin edits survive.

UPDATE app_settings
   SET company_name = 'ClickFix'
 WHERE id = 1 AND company_name = 'App';

UPDATE app_settings
   SET tagline = 'Çok kanallı müşteri destek platformu'
 WHERE id = 1 AND tagline = 'Platform & CMS console';

UPDATE notification_templates SET subject = 'ClickFix''e hoş geldiniz'
 WHERE code = 'auth.welcome' AND language = 'tr' AND subject = 'App''e hoş geldiniz';
UPDATE notification_templates SET body = 'Merhaba {{name}}, ClickFix hesabınız başarıyla oluşturuldu.'
 WHERE code = 'auth.welcome' AND language = 'tr' AND channel = 'email'
   AND body = 'Merhaba {{name}}, App hesabınız başarıyla oluşturuldu.';

UPDATE notification_templates SET subject = 'Welcome to ClickFix'
 WHERE code = 'auth.welcome' AND language = 'en' AND subject = 'Welcome to App';
UPDATE notification_templates SET body = 'Hello {{name}}, your ClickFix account was created successfully.'
 WHERE code = 'auth.welcome' AND language = 'en' AND channel = 'email'
   AND body = 'Hello {{name}}, your App account was created successfully.';
