UPDATE notification_templates SET body = 'Hello {{name}}, your App account was created successfully.'
 WHERE code = 'auth.welcome' AND language = 'en' AND channel = 'email'
   AND body = 'Hello {{name}}, your ClickFix account was created successfully.';
UPDATE notification_templates SET subject = 'Welcome to App'
 WHERE code = 'auth.welcome' AND language = 'en' AND subject = 'Welcome to ClickFix';

UPDATE notification_templates SET body = 'Merhaba {{name}}, App hesabınız başarıyla oluşturuldu.'
 WHERE code = 'auth.welcome' AND language = 'tr' AND channel = 'email'
   AND body = 'Merhaba {{name}}, ClickFix hesabınız başarıyla oluşturuldu.';
UPDATE notification_templates SET subject = 'App''e hoş geldiniz'
 WHERE code = 'auth.welcome' AND language = 'tr' AND subject = 'ClickFix''e hoş geldiniz';

UPDATE app_settings
   SET tagline = 'Platform & CMS console'
 WHERE id = 1 AND tagline = 'Çok kanallı müşteri destek platformu';

UPDATE app_settings
   SET company_name = 'App'
 WHERE id = 1 AND company_name = 'ClickFix';
