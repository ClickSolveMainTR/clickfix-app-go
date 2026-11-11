import type { AppLocale } from "@/config/i18n";

/**
 * Comparison table (`/compare`). Competitor cells only state what
 * their public product and pricing pages say; where a capability is not
 * listed as built-in, the cell says exactly that ("—") instead of "no".
 * Official logos and neutral wordmarks identify competitors (nominative use).
 */
export type CompareValue = "yes" | "plan" | "addon" | "none" | { text: string };

export const COMPETITORS = ["Zendesk", "Intercom", "Freshdesk"] as const;

type Row = {
  label: Record<AppLocale, string>;
  clickfix: CompareValue;
  others: [CompareValue, CompareValue, CompareValue];
};

type Group = { title: Record<AppLocale, string>; rows: Row[] };

const gdpr = {
  tr: { text: "GDPR araçları" },
  en: { text: "GDPR tools" },
};

export function compareGroups(locale: AppLocale): {
  title: string;
  rows: { label: string; clickfix: CompareValue; others: CompareValue[] }[];
}[] {
  const groups: Group[] = [
    {
      title: { tr: "Kanallar", en: "Channels" },
      rows: [
        {
          label: {
            tr: "E-posta, web sohbeti ve WhatsApp tek gelen kutusunda",
            en: "Email, web chat and WhatsApp in one inbox",
          },
          clickfix: "yes",
          others: ["yes", "yes", "yes"],
        },
        {
          label: {
            tr: "Instagram ve Messenger",
            en: "Instagram and Messenger",
          },
          clickfix: "yes",
          others: ["yes", "yes", "yes"],
        },
        {
          label: {
            tr: "Yerel SMS sağlayıcıları (Netgsm, İleti Merkezi) ve İYS kontrolü",
            en: "Turkish SMS providers (Netgsm, İleti Merkezi) and IYS consent check",
          },
          clickfix: "yes",
          others: ["none", "none", "none"],
        },
      ],
    },
    {
      title: { tr: "Yapay zekâ", en: "AI" },
      rows: [
        {
          label: {
            tr: "Bilgi bankasıyla yanıt veren AI ajanı",
            en: "AI agent answering from the knowledge base",
          },
          clickfix: "yes",
          others: ["addon", "addon", "addon"],
        },
        {
          label: {
            tr: "Temsilci asistanı (taslak, özet, çeviri)",
            en: "Agent copilot (draft, summary, translation)",
          },
          clickfix: "yes",
          others: ["addon", "addon", "addon"],
        },
      ],
    },
    {
      title: { tr: "Self servis ve görüşme", en: "Self-service and meetings" },
      rows: [
        {
          label: {
            tr: "Herkese açık yardım merkezi",
            en: "Public help center",
          },
          clickfix: "yes",
          others: ["yes", "yes", "yes"],
        },
        {
          label: {
            tr: "Yerleşik randevu sayfaları ve video odaları",
            en: "Built-in booking pages and video rooms",
          },
          clickfix: "yes",
          others: ["none", "none", "none"],
        },
      ],
    },
    {
      title: { tr: "Türkiye'ye özel", en: "Made for Türkiye" },
      rows: [
        {
          label: {
            tr: "Türk lirası fiyatlandırma",
            en: "Turkish lira pricing",
          },
          clickfix: "yes",
          others: ["none", "none", "none"],
        },
        {
          label: {
            tr: "PayTR, iyzico ve havale/EFT ile ödeme",
            en: "Payment with PayTR, iyzico and bank transfer",
          },
          clickfix: "yes",
          others: ["none", "none", "none"],
        },
        {
          label: {
            tr: "KVKK veri talepleri (dışa aktarma, silme, saklama)",
            en: "KVKK data requests (export, erasure, retention)",
          },
          clickfix: "yes",
          others: [gdpr[locale], gdpr[locale], gdpr[locale]],
        },
      ],
    },
    {
      title: { tr: "Kurumsal ve geliştirici", en: "Enterprise and developers" },
      rows: [
        {
          label: {
            tr: "SSO (SAML / OIDC) ve SCIM",
            en: "SSO (SAML / OIDC) and SCIM",
          },
          clickfix: "yes",
          others: ["plan", "plan", "plan"],
        },
        {
          label: { tr: "REST API ve webhook'lar", en: "REST API and webhooks" },
          clickfix: "yes",
          others: ["yes", "yes", "yes"],
        },
        {
          label: { tr: "iOS ve Android uygulaması", en: "iOS and Android app" },
          clickfix: "yes",
          others: ["yes", "yes", "yes"],
        },
        {
          label: {
            tr: "Resmi SDK'lar: Node.js, Go, PHP, Laravel, React, Flutter (MIT)",
            en: "Official SDKs: Node.js, Go, PHP, Laravel, React, Flutter (MIT)",
          },
          clickfix: "yes",
          others: ["yes", "yes", "yes"],
        },
        {
          label: {
            tr: "Müşteri uygulamasında destek: webview köprüsü ve Flutter paketi",
            en: "Support inside the customer app: webview bridge and Flutter package",
          },
          clickfix: "yes",
          others: ["yes", "yes", "yes"],
        },
        {
          label: {
            tr: "İmzalı müşteri kimliği (user_hash) ve firma imzası, dış kimlik",
            en: "Signed customer identity (user_hash) with company signature, external ids",
          },
          clickfix: "yes",
          others: ["yes", "yes", "yes"],
        },
        {
          label: {
            tr: "Sistemini bağla: kendi API'nizden kayıt okuyan AI ve temsilci",
            en: "Connect your system: AI and agents reading records from your own API",
          },
          clickfix: "yes",
          others: ["none", "addon", "none"],
        },
        {
          label: {
            tr: "Panelde iş yapan AI asistan (kurulum, test, onay kartı)",
            en: "AI assistant that does the setup in the panel (setup, tests, approval card)",
          },
          clickfix: "yes",
          others: ["none", "none", "none"],
        },
        {
          label: {
            tr: "MCP sunucusu (Claude, Cursor) ve llms.txt dokümantasyon",
            en: "MCP server (Claude, Cursor) and llms.txt documentation",
          },
          clickfix: "yes",
          others: ["none", "none", "none"],
        },
      ],
    },
  ];

  return groups.map((group) => ({
    title: group.title[locale],
    rows: group.rows.map((row) => ({
      label: row.label[locale],
      clickfix: row.clickfix,
      others: row.others,
    })),
  }));
}
