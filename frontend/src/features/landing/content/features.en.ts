import type { FeatureCatalog } from "./features";

export const featuresEn: FeatureCatalog = {
  inbox: {
    name: "Omnichannel inbox",
    summary:
      "Email, WhatsApp, Instagram, Messenger, Telegram, Slack, Teams, Google Chat, SMS and web chat in one list.",
    eyebrow: "Omnichannel inbox",
    title: "Every channel, one inbox.",
    lead: "Whatever channel your customers use, the conversation lands on the same screen. Your team works with assignment, internal notes, SLAs and canned replies without switching tools.",
    description:
      "Email, WhatsApp, Instagram, Messenger, Telegram, Slack, Teams, Google Chat, SMS, API and web widget conversations in a single inbox.",
    chips: [
      "WhatsApp",
      "Instagram",
      "Messenger",
      "Telegram",
      "Email",
      "Slack",
      "Microsoft Teams",
      "Google Chat",
      "SMS",
      "Web widget",
      "API channel",
    ],
    highlights: [
      {
        title: "Official APIs",
        body: "WhatsApp Business Cloud API, Instagram and Messenger connect through the Meta Graph API; Telegram, Slack, Microsoft Teams and Google Chat through their bot APIs. WhatsApp embedded signup and message templates are supported.",
      },
      {
        title: "Email on your own address",
        body: "Forward your support address to ClickFix or connect your mailbox over IMAP; replies go out over SMTP from your own domain.",
      },
      {
        title: "SMS and voice",
        body: "Send SMS with Netgsm, İleti Merkezi or Twilio; inbound SMS via Twilio lands in the conversation too. An IYS consent check runs before sending. Twilio and Netgsm call records are added to the conversation.",
      },
      {
        title: "Web widget",
        body: "A chat window added to your site with one line of code. It opens in your brand colours, recognises the visitor and brings the conversation into the inbox.",
      },
      {
        title: "Smart routing",
        body: "Round-robin, load-balanced and skills-based routing, with agent capacity limits.",
      },
      {
        title: "SLAs and business hours",
        body: "First response and resolution targets, business hours and holidays. Warnings before a target is missed, escalation when it is.",
      },
    ],
    capabilities: [
      "Assignment, priority, tags and statuses",
      "Internal notes and @mentions",
      "Saved views and filters",
      "Conversation merge",
      "Canned replies (macros)",
      "CSAT, NPS and CES surveys",
      "Multi-brand support",
      "Internal team chat",
      "API channel for your own sources",
      "Turkish and English interface",
    ],
  },
  ai: {
    name: "AI",
    summary:
      "AI agent, agent copilot, quality scoring, call transcription, ClickFix Assistant and guides.",
    eyebrow: "AI",
    title: "AI answers first. Your team takes over when needed.",
    lead: "The AI agent answers from your knowledge base and the procedures you define; when it is unsure, it hands the conversation to your team with full context. Your agents work with a copilot on every reply.",
    description:
      "AI agent, copilot, AI quality scoring, voice and video transcription, ClickFix Assistant: auditable AI that works from your knowledge base.",
    chips: [
      "AI agent",
      "Copilot",
      "Triage",
      "Knowledge sources",
      "Procedures",
      "Evaluation",
      "Transcription",
      "Quality score",
    ],
    highlights: [
      {
        title: "AI agent",
        body: "Configured with procedures, tools (including HTTP calls) and knowledge sources. Add sources by crawling your website and test with evaluation scenarios before going live.",
      },
      {
        title: "Agent copilot",
        body: "One click in the inbox for a draft reply, summary, rewrite, tone change, translation, expansion or shortening.",
      },
      {
        title: "Automatic triage",
        body: "Each incoming conversation gets its language, intent, category, sentiment and priority detected; suggested tags and team help routing.",
      },
      {
        title: "AI-assisted setup",
        body: "Describe your website and industry; ClickFix assembles channel, team, knowledge base and automation suggestions from ready-made templates for you to approve.",
      },
      {
        title: "ClickFix Assistant and guides",
        body: "Open ClickFix Assistant with ⌘J in the app: it searches the docs, checks where you are in setup, applies the steps it suggests once you confirm, and hands you to our support team when needed. Every page also has a step-by-step guided tour.",
      },
      {
        title: "Transcripts and summaries",
        body: "Voice calls and video meeting recordings are transcribed and added to the conversation timeline with a short summary. Deleting a recording deletes its transcript too.",
      },
      {
        title: "AI quality scoring",
        body: "Conversations are scored against your own rubrics; AI does the first pass, your QA team confirms it and turns it into coaching notes.",
      },
      {
        title: "Auditable",
        body: "AI replies are logged and can be sent to quality review. Track the agent with resolution and handoff rates.",
      },
    ],
    capabilities: [
      "Answers from knowledge base articles (RAG)",
      "Handoff to a human with context",
      "Draft, summary, translation and tone tools",
      "Intent, category, sentiment and priority detection",
      "Knowledge sources from website crawls",
      "Evaluation scenarios",
      "Replies in Turkish and English",
      "Setup templates for 8 industries",
      "Transcripts and summaries of voice and video recordings",
      "Rubric-based AI quality scoring",
      "MCP server for your own AI tools",
    ],
  },
  "knowledge-base": {
    name: "Knowledge base & help center",
    summary:
      "Articles, a public help center and a community forum; also the AI agent's source.",
    eyebrow: "Self-service",
    title: "Let customers find the answer themselves.",
    lead: "Articles you write in the knowledge base are published in your public help center and become the AI agent's source. A community forum adds a place where customers help each other.",
    description:
      "Knowledge base, a public help center for each brand and a community forum. Your articles also power the AI agent.",
    chips: ["Articles", "Collections", "Search", "Help center", "Forum"],
    highlights: [
      {
        title: "Rich editor",
        body: "Write articles with headings, lists, tables, code blocks and images; organise them in collections.",
      },
      {
        title: "Public help center",
        body: "A separate help center address per brand, Turkish and English pages, a sitemap and search.",
      },
      {
        title: "Community forum",
        body: "Customers ask questions under categories and topics and answer each other and your team.",
      },
      {
        title: "Connected to AI",
        body: "The AI agent grounds its replies in published articles; update an article and the agent's knowledge follows.",
      },
    ],
    capabilities: [
      "Collections and articles",
      "Full-text search",
      "Help center per brand",
      "Turkish and English content",
      "Community forum",
      "Sitemap for search engines",
    ],
  },
  "appointments-video": {
    name: "Appointments & video rooms",
    summary: "Public booking pages and video rooms that run in the browser.",
    eyebrow: "Appointments and video",
    title: "Take bookings, meet in the browser.",
    lead: "Customers pick a time from your public booking page. For video calls, open rooms with nothing to install, with guest links, PINs and end-to-end encryption options.",
    description:
      "Public booking pages, calendars and LiveKit-based video rooms: guest links, PIN, end-to-end encryption, recording and transcripts.",
    chips: [
      "Booking page",
      "Availability",
      "Guest links",
      "PIN",
      "E2EE",
      "Recording",
    ],
    highlights: [
      {
        title: "Booking pages",
        body: "Appointment types, durations and agent availability. Customers manage their booking from their own link.",
      },
      {
        title: "Video in the browser",
        body: "Rooms run on LiveKit; your customer does not need to install an app.",
      },
      {
        title: "Access control",
        body: "Guest links, an optional PIN and per-room required end-to-end encryption.",
      },
      {
        title: "Recording",
        body: "Optional meeting recording with retention and usage limits. Recordings are transcribed and summarised automatically.",
      },
    ],
    capabilities: [
      "Public booking page",
      "Agent calendars and availability",
      "Customer-side booking management",
      "Guest links and PIN",
      "End-to-end encryption option",
      "Recording with retention period",
      "Recording transcript and AI summary",
    ],
  },
  "customer-relationships": {
    name: "Customer relationships",
    summary: "Companies, contacts, health score and one timeline.",
    eyebrow: "CRM-lite",
    title: "Know the customer behind the conversation.",
    lead: "Contacts belong to companies; each company's conversations, revenue and health score are on one screen. Your agent sees the customer's history while replying.",
    description:
      "Company and contact records, a 0-100 customer health score and a timeline of every interaction.",
    chips: ["Companies", "Contacts", "Health score", "Timeline", "Revenue"],
    highlights: [
      {
        title: "Companies and contacts",
        body: "Link contacts to companies, add custom fields, import and export lists.",
      },
      {
        title: "Health score",
        body: "A 0-100 score weighted from several signals, so you spot at-risk customers early.",
      },
      {
        title: "Timeline",
        body: "Conversations, notes and key events per contact and company, in order.",
      },
    ],
    capabilities: [
      "Company and contact records",
      "Custom fields",
      "Company revenue",
      "Weighted health score",
      "Contact and company timeline",
      "CSV / XLSX import and export",
    ],
  },
  engineering: {
    name: "Engineering bridge",
    summary: "Link support conversations to Linear, Jira and GitHub issues.",
    eyebrow: "Engineering bridge",
    title: "Support and engineering on the same issue.",
    lead: "When a bug report comes in, link the conversation to a Linear, Jira or GitHub issue. Your team stays informed as it moves, and affected customers get a follow-up when it is fixed.",
    description:
      "Link support conversations to Linear, Jira, GitHub and Sentry issues, with follow-ups and revenue impact.",
    chips: [
      "Linear",
      "Jira",
      "GitHub",
      "Sentry",
      "Follow-ups",
      "Revenue impact",
    ],
    highlights: [
      {
        title: "Issue linking",
        body: "Open a new issue from a conversation or link an existing one; ClickFix's built-in issues work too.",
      },
      {
        title: "Follow-ups",
        body: "When an issue changes, follow-up tasks are created for linked conversations, so no customer is forgotten.",
      },
      {
        title: "Revenue impact",
        body: "See which companies a bug affects and their combined revenue to prioritise.",
      },
    ],
    capabilities: [
      "Linear, Jira and GitHub connections",
      "Sentry issues",
      "Built-in issues",
      "Conversation-issue links",
      "Follow-up tasks",
      "Affected revenue view",
    ],
  },
  automation: {
    name: "Automation",
    summary: "Rules, a visual workflow editor, macros and SLAs.",
    eyebrow: "Automation",
    title: "Leave repetitive work to rules, the rest to your team.",
    lead: "Draw step-by-step workflows in the visual editor and cut repetitive work with macros and routing. Versions, run history and a test screen show what ran and why.",
    description:
      "Rule-based automation, a visual workflow editor, macros, SLA policies and smart routing.",
    chips: ["Rules", "Workflows", "Macros", "SLA", "Routing"],
    highlights: [
      {
        title: "Rules via the API",
        body: "Event triggers, conditions and actions defined through the API: tag, assign, prioritise, notify.",
      },
      {
        title: "Visual workflow editor",
        body: "Drag and connect steps; version, test and inspect every run.",
      },
      {
        title: "Macros",
        body: "Canned replies and action bundles applied in one click.",
      },
      {
        title: "SLAs and routing",
        body: "SLA policies, business hours, round-robin, load-balanced and skills-based routing.",
      },
    ],
    capabilities: [
      "Event-based triggers",
      "Condition and action rules (API)",
      "Visual workflow editor",
      "Versions, tests and run history",
      "Macros",
      "SLA policies and escalation",
      "Skills-based routing",
    ],
  },
  analytics: {
    name: "Analytics & workforce",
    summary: "Reports, quality assurance, coaching and workforce planning.",
    eyebrow: "Analytics and team",
    title: "See what happens, grow your team.",
    lead: "Start with response time, SLA and satisfaction reports; grow your team with quality reviews, coaching and workforce planning.",
    description:
      "Support reports, CSAT/NPS/CES, quality scoring rubrics, coaching and Erlang-based workforce forecasting.",
    chips: ["Reports", "CSAT / NPS / CES", "Quality", "Coaching", "Workforce"],
    highlights: [
      {
        title: "Reports",
        body: "Volume, response and resolution times, SLA compliance, by channel and agent.",
      },
      {
        title: "Quality assurance",
        body: "Manual reviews and checked AI reviews with scoring rubrics.",
      },
      {
        title: "Coaching",
        body: "Blind calibration sessions, development tasks and KPI badges.",
      },
      {
        title: "Workforce planning",
        body: "Erlang-based staffing forecasts, agent calendars and availability.",
      },
    ],
    capabilities: [
      "Built-in support reports",
      "CSAT, NPS and CES surveys",
      "Quality rubrics",
      "AI-assisted quality reviews",
      "Calibration and development tasks",
      "Erlang forecasts and shift calendars",
    ],
  },
  "status-page": {
    name: "Status page & on-call",
    summary: "A public status page, incident management and on-call schedules.",
    eyebrow: "Incident management",
    title: "When there is an outage, everyone is on the same page.",
    lead: "Publish the status of your components on a public page, announce incidents and maintenance, and notify subscribers. On-call schedules get you to the right person.",
    description:
      "A public status page, incident and maintenance announcements, subscriptions; on-call with rotations, escalation and paging.",
    chips: [
      "Components",
      "Incidents",
      "Maintenance",
      "Subscriptions",
      "On-call",
    ],
    highlights: [
      {
        title: "Status page",
        body: "Per-component status, incident history and planned maintenance announcements.",
      },
      {
        title: "Subscriptions",
        body: "Customers subscribe to status updates and are notified as an incident progresses.",
      },
      {
        title: "On-call",
        body: "Rotations, escalation policies, paging and acknowledgement.",
      },
    ],
    capabilities: [
      "Component and incident management",
      "Planned maintenance announcements",
      "Status subscriptions",
      "On-call rotations",
      "Escalation and paging",
      "Acknowledgement tracking",
    ],
  },
  security: {
    name: "Security & KVKK",
    summary: "KVKK tools, SSO, SCIM, two-factor sign-in and an audit log.",
    eyebrow: "Security and KVKK",
    title: "Customer data, compliant and under control.",
    lead: "KVKK export, erasure and retention are built into the product. Enterprise sign-in, user provisioning and an audit log are ready.",
    description:
      "KVKK data subject requests, retention, SSO (SAML/OIDC), SCIM, two-factor sign-in, passkeys and an audit log.",
    chips: ["KVKK", "SSO", "SCIM", "2FA", "Passkeys", "Audit log"],
    highlights: [],
    capabilities: [],
  },
  mobile: {
    name: "Mobile app",
    summary: "Instant notifications and replies with the iOS and Android app.",
    eyebrow: "Mobile",
    title: "Your inbox in your pocket.",
    lead: "With the iOS and Android app you hear about new conversations instantly, reply on the go and manage your availability.",
    description:
      "The ClickFix iOS and Android app: push notifications, replying to conversations and availability.",
    chips: ["iOS", "Android", "Push", "Availability"],
    highlights: [
      {
        title: "Push notifications",
        body: "Instant notifications for new conversations and conversations assigned to you.",
      },
      {
        title: "Replying",
        body: "See the conversation history, write replies and leave internal notes.",
      },
      {
        title: "Availability",
        body: "Online or away; change your status from the app.",
      },
    ],
    capabilities: [
      "iOS and Android",
      "Push notifications",
      "Replies and internal notes",
      "Availability",
      "Turkish and English interface",
    ],
  },
  developers: {
    name: "Developer platform",
    summary: "REST API, signed webhooks, REST hooks and sidebar apps.",
    eyebrow: "Developers",
    title: "Connect ClickFix to your own systems.",
    lead: "A REST API with scoped keys, signed webhooks, REST hooks for automation platforms and sidebar apps that place your own app next to the conversation.",
    description:
      "ClickFix REST API, signed webhooks, REST hook connections for Zapier/Make/n8n and sidebar apps.",
    chips: ["REST API", "Webhooks", "OpenAPI", "REST hooks", "Sidebar apps"],
    highlights: [
      {
        title: "REST API",
        body: "Scoped API keys, a consistent response envelope and safe retries with an Idempotency-Key on writes.",
      },
      {
        title: "Signed webhooks",
        body: "Every request is signed with HMAC-SHA256 and carries a timestamp; failed deliveries are retried for up to 24 hours.",
      },
      {
        title: "Automation platforms",
        body: "REST hook subscriptions and polling triggers let you connect from tools such as Zapier, Make and n8n.",
      },
      {
        title: "Sidebar apps",
        body: "Embed your own app next to the conversation as an iframe; a signed session token tells you who is looking.",
      },
    ],
    capabilities: [
      "Scoped API keys",
      "OpenAPI schema",
      "Idempotency-Key support",
      "HMAC-SHA256 signed webhooks",
      "Automatic retries",
      "REST hook subscriptions",
      "Sidebar apps",
    ],
  },
};
