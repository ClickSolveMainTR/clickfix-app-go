#!/usr/bin/env node
// Fake Go API for the Meta App Review screencasts (CF-2). The real Next.js panel runs with
// API_URL pointing here, so sign-in (NextAuth, server side), the BFF proxy and every page talk
// to this file instead of the Go API. Only the calls the review flows make have real shapes;
// any other GET answers an empty list so no page shows an error toast.
//
// Scenario control (called from the Playwright process, never from the browser):
//   POST /__review/reset                  back to an empty organization (no channels, no conversations)
//   POST /__review/inbound {channel, contact, content, attributes?, subject?}
//                                          a customer message arrives, as the channel webhook would deliver it
//   GET  /__review/sent                    messages the agent sent (to assert in scenarios)
//
// Every name, id and message here is fictional.
import { randomUUID } from "node:crypto";
import { createServer } from "node:http";

const PORT = Number(process.env.REVIEW_MOCK_PORT ?? 18090);
const now = () => new Date().toISOString();

const USER = {
  uuid: "5d1c2f1e-6b7a-4c3d-9e8f-000000000001",
  email: "alex.morgan@northwind.example",
  name: "Alex",
  surname: "Morgan",
};
const ORG = {
  uuid: "5d1c2f1e-6b7a-4c3d-9e8f-0000000000aa",
  slug: "northwind",
  name: "Northwind Support",
};
const TENANT_PERMISSIONS = [
  "tenant.conversations.read",
  "tenant.conversations.read_all",
  "tenant.conversations.reply",
  "tenant.conversations.note",
  "tenant.conversations.assign",
  "tenant.conversations.manage",
  "tenant.channels.read",
  "tenant.channels.manage",
  "tenant.teams.read",
  "tenant.contacts.read",
  "tenant.macros.read",
  "tenant.settings.read",
];

const CAPABILITIES = {
  whatsapp: {
    media: true,
    templates: true,
    typing: false,
    reactions: true,
    delivery_receipts: true,
    read_receipts: true,
    threading: false,
    reply_window: 86400,
    max_text_length: 4096,
  },
  instagram: {
    media: true,
    templates: false,
    typing: false,
    reactions: true,
    delivery_receipts: false,
    read_receipts: true,
    threading: true,
    reply_window: 604800,
    max_text_length: 1000,
  },
  messenger: {
    media: true,
    templates: false,
    typing: true,
    reactions: false,
    delivery_receipts: true,
    read_receipts: true,
    threading: false,
    reply_window: 604800,
    max_text_length: 2000,
  },
};

const META_PAGES = [
  { id: "104223650000001", name: "Northwind Coffee Machines" },
  { id: "104223650000002", name: "Northwind Outlet" },
];

let state;
function reset() {
  state = {
    channels: [],
    conversations: [],
    messages: new Map(),
    sent: [],
    templates: defaultTemplates(),
  };
}
reset();

function defaultTemplates() {
  return [
    {
      id: "tpl_1",
      name: "order_shipped",
      language: "en_US",
      status: "APPROVED",
      category: "UTILITY",
      components: [
        { type: "BODY", text: "Hi {{1}}, your order {{2}} has shipped." },
      ],
    },
    {
      id: "tpl_2",
      name: "repair_ready",
      language: "en_US",
      status: "APPROVED",
      category: "UTILITY",
      components: [
        {
          type: "BODY",
          text: "Hi {{1}}, your machine is repaired and ready for pickup.",
        },
      ],
    },
  ];
}

function channelInstance(type, name, externalId, extra = {}) {
  const at = now();
  return {
    uuid: randomUUID(),
    type,
    name,
    status: "active",
    external_id: externalId,
    credential_hints: extra.credential_hints ?? {},
    settings: extra.settings ?? {},
    capabilities: CAPABILITIES[type],
    webhook_path: "",
    shared_webhook_path: `/v1/webhooks/channels/${type}`,
    health: {
      state: "ok",
      detail: "",
      checked_at: at,
      token_expires_at: null,
      last_webhook_at: null,
      sent_24h: 0,
      failed_24h: 0,
      error_rate: 0,
      reconnect: false,
    },
    last_inbound_at: null,
    last_outbound_at: null,
    last_error: "",
    last_error_at: null,
    created_at: at,
    updated_at: at,
  };
}

function addChannel(instance) {
  state.channels = state.channels.filter((c) => c.type !== instance.type);
  state.channels.push(instance);
  return instance;
}

function inbound({ channel, contact, content, attributes = {}, subject }) {
  const inst = state.channels.find((c) => c.type === channel);
  if (!inst) throw new Error(`no ${channel} channel connected`);
  const at = now();
  let conv = state.conversations.find(
    (c) =>
      c.channel_type === channel &&
      c.contact.name === contact &&
      (c.thread ?? "") === (attributes.thread ?? ""),
  );
  if (!conv) {
    conv = {
      uuid: randomUUID(),
      number: state.conversations.length + 1001,
      channel_type: channel,
      inbox: { uuid: inst.uuid, name: inst.name },
      contact: {
        uuid: randomUUID(),
        name: contact,
        email: "",
        phone: channel === "whatsapp" ? "+44 7700 900123" : "",
      },
      company: null,
      subject: subject ?? "",
      status: "open",
      custom_status: null,
      priority: "normal",
      source: "inbound",
      assignee: null,
      team: null,
      tag_uuids: [],
      custom_fields: {},
      unread_count: 0,
      message_count: 0,
      last_message_preview: "",
      last_message_direction: "in",
      last_message_at: at,
      last_inbound_at: at,
      waiting_since: at,
      first_response_at: null,
      first_response_due_at: null,
      next_response_due_at: null,
      resolution_due_at: null,
      sla_breached_at: null,
      snoozed_until: null,
      resolved_at: null,
      window_open: true,
      window_ends_at: new Date(
        Date.now() + inst.capabilities.reply_window * 1000,
      ).toISOString(),
      created_at: at,
      updated_at: at,
      thread: attributes.thread ?? "",
    };
    state.conversations.unshift(conv);
    state.messages.set(conv.uuid, []);
  }
  const msg = message(conv, {
    direction: "in",
    sender: { type: "contact", uuid: conv.contact.uuid, name: contact },
    content,
    attributes,
    status: "received",
  });
  state.messages.get(conv.uuid).push(msg);
  Object.assign(conv, {
    unread_count: conv.unread_count + 1,
    message_count: conv.message_count + 1,
    last_message_preview: content,
    last_message_direction: "in",
    last_message_at: at,
    last_inbound_at: at,
    updated_at: at,
  });
  inst.last_inbound_at = at;
  inst.health.last_webhook_at = at;
  return conv;
}

function message(conv, fields) {
  const at = now();
  return {
    uuid: randomUUID(),
    conversation_uuid: conv.uuid,
    kind: "text",
    attributes: {},
    private: false,
    error: "",
    client_id: null,
    reply_to: null,
    mentions: [],
    attachments: [],
    sent_at: at,
    delivered_at: null,
    read_at: null,
    created_at: at,
    ...fields,
  };
}

function publicConversation(conv) {
  const rest = { ...conv };
  delete rest.thread;
  return rest;
}

function me() {
  return {
    user: {
      ...USER,
      status: "active",
      is_super_admin: false,
      email_verified: true,
      locale: "en",
    },
    roles: [],
    permissions: [],
    organizations: [
      {
        ...ORG,
        logo_url: "",
        status: "active",
        access_ends_at: null,
        role: { key: "owner", name: "Owner" },
        permissions: TENANT_PERMISSIONS,
      },
    ],
    links: { profile: "", change_password: "", notification_preferences: "" },
    channels: {},
    realtime: { enabled: false, user_channel: `user:${USER.uuid}` },
  };
}

const page = (items) => ({ items, total: items.length, limit: 50, offset: 0 });

/** Returns [status, data] or undefined for "not handled". */
function route(method, path, url, body) {
  // Auth (NextAuth server side + browser hydration).
  if (path === "app/config") {
    return [
      200,
      {
        mode: "platform",
        name: "ClickFix",
        vapid_configured: false,
        github_auth_enabled: false,
        registration_enabled: false,
        auth_methods: {
          password: { login: true, register: false },
          passkey: { login: false },
          github: { login: false },
          google: { login: false },
          facebook: { login: false },
          apple: { login: false },
        },
      },
    ];
  }
  if (path.startsWith("internal/auth/oauth/"))
    return [200, { enabled: false, client_id: "", client_secret: "" }];
  if (path === "auth/login" || path === "auth/refresh")
    return [
      200,
      {
        access_token: "review-access",
        refresh_token: "review-refresh",
        token_type: "Bearer",
        expires_in: 3600,
      },
    ];
  if (path === "internal/auth/users/by-email")
    return [
      200,
      {
        id: USER.uuid,
        email: USER.email,
        name: `${USER.name} ${USER.surname}`,
      },
    ];
  if (path === "auth/me") return [200, me()];

  // CMS shell.
  if (path === "tenant/context")
    return [
      200,
      {
        organization: {
          uuid: ORG.uuid,
          slug: ORG.slug,
          name: ORG.name,
          status: "active",
          access_ends_at: null,
        },
        role: "owner",
        roles: ["owner"],
        permissions: TENANT_PERMISSIONS,
      },
    ];
  if (path === "tenant/conversations/counts") {
    const open = state.conversations.filter((c) => c.status === "open").length;
    return [
      200,
      {
        open,
        mine: 0,
        unassigned: open,
        snoozed: 0,
        breached: 0,
        by_status: { open },
      },
    ];
  }
  if (path.startsWith("tenant/presence"))
    return [
      200,
      {
        status: "online",
        manual_status: "online",
        last_seen_at: now(),
        heartbeat_seconds: 60,
      },
    ];
  if (path === "notifications/unread-count") return [200, { count: 0 }];
  if (path === "search/specs" || path === "tenant/search/specs")
    return [200, { items: [], enabled: false }];
  if (path === "tenant/onboarding")
    return [
      200,
      { active: false, finished_at: null, steps: [], completed: 0, total: 0 },
    ];
  if (path.startsWith("realtime/"))
    return [503, { code: "REALTIME_DISABLED", message: "realtime disabled" }];

  // Channels.
  if (path === "tenant/channels" && method === "GET")
    return [200, { items: state.channels }];
  if (path === "tenant/channels/types") {
    return [
      200,
      {
        items: ["whatsapp", "instagram", "messenger"].map((type) => ({
          type,
          capabilities: CAPABILITIES[type],
          fields: [],
        })),
      },
    ];
  }
  const oauthStart =
    /^tenant\/channels\/oauth\/(messenger|instagram)\/start$/.exec(path);
  if (oauthStart) {
    // Meta's own login dialog is not automated; the scenario captions it and we return straight to the callback.
    const origin =
      url.searchParams.get("origin") ??
      process.env.REVIEW_PANEL_ORIGIN ??
      "http://127.0.0.1:3100";
    return [
      200,
      {
        url: `${origin}/channels/connect/${oauthStart[1]}?code=review-code&state=review-state`,
        state: "review-state",
      },
    ];
  }
  if (path === "tenant/channels/oauth/instagram/complete") {
    return [
      200,
      addChannel(
        channelInstance("instagram", "@northwind.coffee", "17841400000000001", {
          credential_hints: { username: "northwind.coffee" },
        }),
      ),
    ];
  }
  if (path === "tenant/channels/oauth/messenger/complete")
    return [200, { selection: "review-selection", pages: META_PAGES }];
  if (path === "tenant/channels/oauth/messenger/pages") {
    const picked =
      META_PAGES.find((p) => p.id === body?.page_id) ?? META_PAGES[0];
    return [
      200,
      addChannel(channelInstance("messenger", picked.name, picked.id)),
    ];
  }
  if (path === "tenant/channels/whatsapp/signup" && method === "GET")
    return [
      200,
      {
        app_id: "1117894644223620",
        config_id: "942776138660676",
        api_version: "v21.0",
      },
    ];
  if (path === "tenant/channels/whatsapp/signup" && method === "POST") {
    const inst = addChannel(
      channelInstance(
        "whatsapp",
        "Northwind Support WhatsApp",
        body?.phone_number_id || "109876543210001",
        { settings: { display_phone_number: "+44 20 7946 0958" } },
      ),
    );
    return [200, { items: [inst] }];
  }
  const templates =
    /^tenant\/channels\/[^/]+\/whatsapp\/templates(?:\/([^/]+))?$/.exec(path);
  if (templates) {
    if (method === "GET") return [200, { items: state.templates }];
    if (method === "POST") {
      const tpl = {
        id: `tpl_${state.templates.length + 1}`,
        name: body.name,
        language: body.language ?? "en_US",
        status: "PENDING",
        category: body.category,
        components: [{ type: "BODY", text: body.body }],
      };
      state.templates.push(tpl);
      return [200, tpl];
    }
    if (method === "DELETE") {
      state.templates = state.templates.filter((t) => t.name !== templates[1]);
      return [200, { deleted: true }];
    }
  }
  const channel = /^tenant\/channels\/([^/]+)(?:\/(check|reconnect))?$/.exec(
    path,
  );
  if (channel && method !== "GET") {
    const inst = state.channels.find((c) => c.uuid === channel[1]);
    if (inst) return [200, inst];
  }

  // Inbox.
  if (path === "tenant/conversations" && method === "GET") {
    const contact = url.searchParams.get("contact");
    const rows = state.conversations
      .filter((c) => !contact || c.contact.uuid === contact)
      .map(publicConversation);
    return [200, page(rows)];
  }
  if (path === "tenant/conversations/agents")
    return [
      200,
      {
        items: [
          {
            uuid: USER.uuid,
            name: `${USER.name} ${USER.surname}`,
            email: USER.email,
          },
        ],
      },
    ];
  if (path === "tenant/conversation-views/counts") return [200, { counts: {} }];
  const conv = /^tenant\/conversations\/([0-9a-f-]{36})(?:\/(.+))?$/.exec(path);
  if (conv) {
    const row = state.conversations.find((c) => c.uuid === conv[1]);
    if (!row)
      return [404, { code: "NOT_FOUND", message: "conversation not found" }];
    const sub = conv[2] ?? "";
    if (sub === "" && method === "GET")
      return [200, { ...publicConversation(row), tags: [], participants: [] }];
    if (sub === "" && method === "PATCH") {
      Object.assign(row, body ?? {});
      return [200, { ...publicConversation(row), tags: [], participants: [] }];
    }
    if (sub === "messages" && method === "GET")
      return [
        200,
        {
          items: state.messages.get(row.uuid) ?? [],
          has_more: false,
          before: null,
        },
      ];
    if (sub === "messages" && method === "POST") {
      const msg = message(row, {
        direction: body?.private ? "internal" : "out",
        sender: {
          type: "agent",
          uuid: USER.uuid,
          name: `${USER.name} ${USER.surname}`,
        },
        content: body?.content ?? "",
        private: Boolean(body?.private),
        status: "sent",
        client_id: body?.client_id ?? null,
      });
      state.messages.get(row.uuid).push(msg);
      state.sent.push({
        channel: row.channel_type,
        conversation: row.uuid,
        content: msg.content,
      });
      Object.assign(row, {
        message_count: row.message_count + 1,
        last_message_preview: msg.content,
        last_message_direction: "out",
        last_message_at: msg.created_at,
        first_response_at: row.first_response_at ?? msg.created_at,
        waiting_since: null,
      });
      // The provider confirms delivery a moment later (status webhook).
      setTimeout(
        () => Object.assign(msg, { status: "delivered", delivered_at: now() }),
        1500,
      );
      return [200, msg];
    }
    if (sub === "read") {
      row.unread_count = 0;
      return [200, { status: "ok" }];
    }
    if (sub === "presence") return [200, { viewers: [] }];
    if (sub === "sla") return [200, { policy: null, metrics: [] }];
    if (sub === "form") return [200, { form: null, values: {} }];
    if (sub === "links") return [200, { merged_into: null, items: [] }];
    if (sub === "side-conversations") return [200, { items: [], inboxes: [] }];
  }
  return undefined;
}

function send(res, status, payload) {
  const ok = status < 400;
  const envelope = ok
    ? { success: true, data: payload, meta: { request_id: "review" } }
    : {
        success: false,
        error: { code: payload.code, message: payload.message, details: [] },
        meta: { request_id: "review" },
      };
  res.writeHead(status, { "Content-Type": "application/json" });
  res.end(JSON.stringify(envelope));
}

async function readBody(req) {
  const chunks = [];
  for await (const chunk of req) chunks.push(chunk);
  if (!chunks.length) return undefined;
  try {
    return JSON.parse(Buffer.concat(chunks).toString("utf8"));
  } catch {
    return undefined;
  }
}

const server = createServer(async (req, res) => {
  const url = new URL(req.url ?? "/", `http://127.0.0.1:${PORT}`);
  const body = await readBody(req);
  const method = req.method ?? "GET";

  if (url.pathname === "/healthz") return send(res, 200, { status: "ok" });
  if (url.pathname === "/__review/reset") {
    reset();
    return send(res, 200, { status: "ok" });
  }
  if (url.pathname === "/__review/inbound") {
    try {
      return send(res, 200, publicConversation(inbound(body)));
    } catch (err) {
      return send(res, 409, { code: "NO_CHANNEL", message: String(err) });
    }
  }
  if (url.pathname === "/__review/sent") return send(res, 200, state.sent);

  const path = url.pathname.replace(/^\/v1\//, "").replace(/\/$/, "");
  const answer = route(method, path, url, body);
  if (answer) return send(res, answer[0], answer[1]);
  if (process.env.REVIEW_MOCK_LOG)
    console.log(`[mock-api] fallback ${method} /v1/${path}`);
  if (method === "GET")
    return send(res, 200, { items: [], total: 0, limit: 20, offset: 0 });
  return send(res, 200, { status: "ok" });
});

server.listen(PORT, "127.0.0.1", () =>
  console.log(`[mock-api] listening on http://127.0.0.1:${PORT}/v1`),
);
