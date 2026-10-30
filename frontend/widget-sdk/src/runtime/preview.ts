/**
 * Preview mode (CF-150 editor): the real runtime on demo data, no network.
 * The editor's iframe (public/widget-preview.html) posts the draft settings
 * and the state to show; FakeApi answers every call locally.
 */
import { normalize, type WidgetSettings } from "../shared/schema";
import type { Transport } from "./api";
import type {
  Bootstrap,
  Conversation,
  Message,
  PublicSettings,
  TeamMember,
} from "./types";

export type PreviewView =
  | "home"
  | "messages"
  | "help"
  | "chat"
  | "offline"
  | "prechat"
  | "csat"
  | "consent"
  | "proactive"
  | "closed"
  | "callback";

export type PreviewState = {
  settings: unknown;
  name?: string;
  locale?: "tr" | "en";
  theme?: "light" | "dark" | "auto";
  dir?: "ltr" | "rtl";
  view?: PreviewView;
  team?: TeamMember[];
};

/** The visitor-safe settings (usecase.Public in Go). */
export function publicSettings(s: WidgetSettings): PublicSettings {
  const out: Record<string, unknown> = { ...s };
  for (const key of [
    "allowed_domains",
    "team_member_uuids",
    "attribute_mapping",
    "rate_limit",
  ])
    delete out[key];
  return {
    ...(out as Omit<PublicSettings, "ai" | "attribute_keys">),
    triggers: s.triggers.filter((t) => t.enabled),
    ai: { enabled: s.ai.enabled, human_button: s.ai.human_button },
    attribute_keys: s.attribute_mapping.map((m) => ({
      key: m.key,
      anonymous: m.anonymous,
    })),
  };
}

const DEMO = {
  tr: {
    visitor1: "Merhaba, siparişimin kargo durumunu öğrenebilir miyim?",
    agent1: "Merhaba! Tabii ki 😊 Sipariş numaranızı paylaşır mısınız?",
    visitor2: "#10482",
    agent2:
      "Teşekkürler. Siparişiniz **bugün** kargoya verildi. Takip bağlantısı: https://kargo.example.com/10482",
    articles: [
      [
        "Siparişimi nasıl takip ederim?",
        "Kargo takip numarası ve teslimat süreleri",
      ],
      ["İade ve değişim koşulları", "14 gün içinde ücretsiz iade"],
      ["Ödeme yöntemleri", "Kredi kartı, havale ve kapıda ödeme"],
    ],
  },
  en: {
    visitor1: "Hi, could you tell me where my order is?",
    agent1: "Hi there! Of course 😊 Could you share your order number?",
    visitor2: "#10482",
    agent2:
      "Thanks. Your order shipped **today**. Tracking link: https://shipping.example.com/10482",
    articles: [
      ["How do I track my order?", "Tracking numbers and delivery times"],
      ["Returns and exchanges", "Free returns within 14 days"],
      ["Payment methods", "Cards, bank transfer and cash on delivery"],
    ],
  },
} as const;

const DEFAULT_TEAM: TeamMember[] = [
  { name: "Ayşe", initials: "AY", avatar_url: null },
  { name: "Emre", initials: "EK", avatar_url: null },
  { name: "Mert", initials: "MT", avatar_url: null },
];

export class FakeApi implements Transport {
  token = "preview";
  private settings: WidgetSettings;
  private conversations: Conversation[];
  private messages: Message[];
  readonly view: PreviewView;

  constructor(private state: PreviewState) {
    this.settings = normalize(state.settings);
    this.view = state.view ?? "home";
    const loc = state.locale ?? "tr";
    const d = DEMO[loc];
    const now = Date.now();
    const at = (min: number) => new Date(now - min * 60000).toISOString();
    const msg = (
      uuid: string,
      direction: "in" | "out",
      content: string,
      min: number,
    ): Message => ({
      uuid,
      direction,
      sender_type: direction === "in" ? "contact" : "user",
      sender_name:
        direction === "out" ? (state.team ?? DEFAULT_TEAM)[0]!.name : "",
      kind: "text",
      content,
      status: direction === "in" ? "read" : "delivered",
      client_id: null,
      attachments: [],
      created_at: at(min),
      read_at: direction === "in" ? at(min - 1) : null,
    });
    this.messages = [
      msg("m1", "in", d.visitor1, 9),
      msg("m2", "out", d.agent1, 8),
      msg("m3", "in", d.visitor2, 7),
      msg("m4", "out", d.agent2, 2),
    ];
    const resolved = this.view === "csat";
    this.conversations =
      this.view === "prechat" || this.view === "consent"
        ? []
        : [
            {
              uuid: "c1",
              number: 1042,
              status: resolved ? "resolved" : "open",
              open: !resolved,
              last_message_preview: d.agent2.replace(/\*\*/g, ""),
              last_message_direction: "out",
              last_message_at: at(2),
              unread_count:
                this.view === "closed" || this.view === "home" ? 1 : 0,
              can_rate: resolved && this.settings.csat,
              rating: null,
            },
          ];
  }

  bootstrap(): Bootstrap {
    const s = this.settings;
    const offline = this.view === "offline";
    return {
      widget: {
        id: "preview",
        name: this.state.name || "ClickFix",
        settings: publicSettings(s),
        online: !offline && !s.away_mode,
        team: s.show_team ? (this.state.team ?? DEFAULT_TEAM) : [],
        reply: {
          value:
            s.reply_time.mode === "hidden"
              ? ""
              : s.reply_time.mode === "manual"
                ? s.reply_time.value
                : offline
                  ? "day"
                  : "minutes",
          next_open_at: offline
            ? new Date(Date.now() + 14 * 3600000).toISOString()
            : null,
        },
      },
      visitor: {
        id: "preview",
        token: "preview",
        expires_at: new Date(Date.now() + 3600000).toISOString(),
        verified: false,
        contact: null,
      },
      realtime: { enabled: false },
      limits: {
        max_file_bytes: s.attachments.max_mb << 20,
        max_text_length: 10000,
      },
      conversations: this.conversations,
      request_form: null,
      context: {
        visible: true,
        hidden_reason: "",
        device: "desktop",
        country: "",
        returning: false,
        eligible_triggers: s.triggers.filter((t) => t.enabled).map((t) => t.id),
      },
    };
  }

  async call<T>(method: string, path: string, body?: unknown): Promise<T> {
    const loc = this.state.locale ?? "tr";
    const out = ((): unknown => {
      if (path === "/bootstrap") return this.bootstrap();
      if (path.startsWith("/help/search"))
        return {
          available: true,
          help_center_url: "https://help.example.com",
          items: DEMO[loc].articles.map(([title, excerpt], i) => ({
            title,
            excerpt,
            url: `https://help.example.com/a/${i + 1}`,
          })),
        };
      if (path === "/booking")
        return { available: true, url: "https://example.com/book" };
      if (path === "/replay-policy")
        return {
          replay_enabled: false,
          cobrowse_enabled: false,
          control_enabled: false,
          block_selectors: [],
        };
      if (path === "/conversations" && method === "GET")
        return { items: this.conversations };
      if (/\/messages/.test(path) && method === "GET")
        return {
          conversation: this.conversations[0],
          items: this.messages,
          has_more: false,
        };
      if (
        method === "POST" &&
        (path === "/conversations" || /\/messages$/.test(path))
      ) {
        const text = (body as { text?: string } | undefined)?.text ?? "";
        return {
          conversation_uuid: "c1",
          created: false,
          message: {
            ...this.messages[0]!,
            uuid: "local-" + Math.random().toString(36).slice(2),
            content: text,
            created_at: new Date().toISOString(),
            read_at: null,
            status: "sent",
          },
        };
      }
      return {};
    })();
    return out as T;
  }
}
