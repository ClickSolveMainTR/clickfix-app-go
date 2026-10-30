import type { AppLocale } from "@/config/i18n";

/**
 * Sections of the security / KVKK page (`/security`). Each line describes a
 * shipped control (modules/compliance, sso, auth, audit, platform/ai guard,
 * platform/crypto, videorooms); nothing aspirational.
 */
export type SecuritySection = {
  id: "kvkk" | "access" | "isolation" | "ai" | "channels";
  title: string;
  body: string;
  items: string[];
};

const TR: SecuritySection[] = [
  {
    id: "kvkk",
    title: "KVKK araçları ürünün içinde",
    body: "İlgili kişi taleplerini panelden karşılayın; ayrı bir süreç ya da elle veri ayıklama gerekmez.",
    items: [
      "Kişi verisinin dışa aktarılması",
      "Silme ve anonimleştirme talepleri",
      "Veri türüne göre saklama süreleri",
      "Rıza kayıtları",
      "Meta veri silme taleplerinin otomatik karşılanması",
      "SMS gönderiminden önce İYS izin kontrolü",
    ],
  },
  {
    id: "access",
    title: "Kimlik ve erişim",
    body: "Kimin neye erişebildiğini rol ve yetkilerle belirleyin, hassas işlemlerde kimliği yeniden doğrulayın.",
    items: [
      "SSO: SAML ve OIDC",
      "SCIM ile kullanıcı ve grup senkronizasyonu",
      "İki adımlı doğrulama (uygulama veya e-posta kodu)",
      "Passkey ile parolasız giriş",
      "Hassas işlemlerde adım-yukarı doğrulama",
      "Rol tabanlı yetkiler",
    ],
  },
  {
    id: "isolation",
    title: "İzolasyon ve iz kaydı",
    body: "Her işletmenin verisi kendi organizasyonu içinde tutulur; önemli değişiklikler kayda geçer.",
    items: [
      "Organizasyon bazlı veri izolasyonu",
      "Denetim kaydı",
      "Destek ekibimiz hesabınıza yalnızca sizin verdiğiniz süreli izinle girer; her işlemi denetim kaydına düşer",
      "Kanal ve entegrasyon anahtarları şifreli saklanır",
    ],
  },
  {
    id: "ai",
    title: "Sorumlu yapay zekâ",
    body: "AI ajanı ve asistan, müşteri metnini veri olarak ele alır; talimat olarak değil.",
    items: [
      "Kişisel veriler modele gönderilmeden önce maskelenir",
      "Prompt enjeksiyonu denetimi",
      "Yanıtlar yalnızca sağlanan kaynaklara atıf yapar",
      "AI yanıtları kayıt altında ve incelenebilir",
    ],
  },
  {
    id: "channels",
    title: "Güvenli bağlantılar",
    body: "Dış sistemlerle konuşan her uç imzalı ve doğrulanabilir.",
    items: [
      "HMAC-SHA256 imzalı, zaman damgalı webhook'lar",
      "Kapsam tanımlı API anahtarları",
      "Video odalarında PIN ve uçtan uca şifreleme seçeneği",
    ],
  },
];

const EN: SecuritySection[] = [
  {
    id: "kvkk",
    title: "KVKK tools built in",
    body: "Handle data subject requests from the dashboard, without a separate process or manual data extraction.",
    items: [
      "Export of a person's data",
      "Erasure and anonymisation requests",
      "Retention periods per data type",
      "Consent records",
      "Automatic handling of Meta data deletion requests",
      "IYS consent check before sending SMS",
    ],
  },
  {
    id: "access",
    title: "Identity and access",
    body: "Decide who can access what with roles and permissions, and re-verify identity for sensitive actions.",
    items: [
      "SSO: SAML and OIDC",
      "User and group provisioning with SCIM",
      "Two-factor sign-in (authenticator app or email code)",
      "Passwordless sign-in with passkeys",
      "Step-up verification for sensitive actions",
      "Role-based permissions",
    ],
  },
  {
    id: "isolation",
    title: "Isolation and audit",
    body: "Each business's data stays inside its own organization; important changes are recorded.",
    items: [
      "Per-organization data isolation",
      "Audit log",
      "Our support team enters your account only with a time-limited grant from you; every action is audit-logged",
      "Channel and integration keys stored encrypted",
    ],
  },
  {
    id: "ai",
    title: "Responsible AI",
    body: "The AI agent and copilot treat customer text as data, not as instructions.",
    items: [
      "Personal data is masked before it reaches the model",
      "Prompt injection checks",
      "Replies cite only the supplied sources",
      "AI replies are logged and reviewable",
    ],
  },
  {
    id: "channels",
    title: "Secure connections",
    body: "Every endpoint that talks to outside systems is signed and verifiable.",
    items: [
      "HMAC-SHA256 signed, timestamped webhooks",
      "Scoped API keys",
      "PIN and end-to-end encryption option in video rooms",
    ],
  },
];

export function securitySections(locale: AppLocale) {
  return locale === "tr" ? TR : EN;
}
