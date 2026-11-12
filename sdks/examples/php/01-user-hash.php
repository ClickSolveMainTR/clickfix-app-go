<?php
// Adım 1: user_hash sunucuda hesaplanır ve giriş yapmış kullanıcıya verilir.
require __DIR__.'/vendor/autoload.php'; // composer require clickfix/sdk

use ClickFix\UserHash;

// Panel > Web widget > Güvenlik > Kimlik doğrulama anahtarı (bir kez gösterilir)
$secret = getenv('CLICKFIX_IDENTITY_SECRET');

// Oturum açmış kullanıcının sizin sisteminizdeki kimliği (dış kimlik) ve
// firmasının kimliği (identify'daki company.id). Firma verilirse imza ikisini
// birden kapsar: hex(HMAC-SHA256(anahtar, user_id + "\n" + company_id)).
function userHashFor(string $secret, string $userId, ?string $companyId = null): string
{
    return UserHash::make($secret, $userId, $companyId);
}

// Örnek: /api/support-session uç noktanız bunu döner, tarayıcı identify'a verir
echo json_encode([
    'user_id' => '1001',
    'user_hash' => userHashFor($secret, '1001'), // firmasız: eski formül
    'company_id' => 'F-77',
    'company_user_hash' => userHashFor($secret, '1001', 'F-77'), // company: { id: "F-77" } ile gönderilir
]), PHP_EOL;
