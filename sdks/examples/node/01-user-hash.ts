// Adım 1: user_hash sunucuda hesaplanır ve giriş yapmış kullanıcıya verilir.
import { identityHash } from "@clickfix/node";

// Panel > Web widget > Güvenlik > Kimlik doğrulama anahtarı (bir kez gösterilir)
const secret = process.env.CLICKFIX_IDENTITY_SECRET!;

// Oturum açmış kullanıcının sizin sisteminizdeki kimliği (dış kimlik)
export function userHashFor(userId: string): string {
  return identityHash(secret, userId);
}

// Örnek: /api/support-session uç noktanız bunu döner, tarayıcı identify'a verir
console.log(
  JSON.stringify({ user_id: "1001", user_hash: userHashFor("1001") }),
);
