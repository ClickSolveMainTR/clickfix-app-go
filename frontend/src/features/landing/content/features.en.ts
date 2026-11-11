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
      {
        title: "ClickFix Assistant: a helper that gets things done",
        body: "Opens in the panel with ⌘J; tells you what you left undone on the page you are on, sets up channels, workflows, SLAs, webhooks and the widget through an approval card, dry-runs and verifies the result. The same capabilities reach Claude and Cursor through the MCP server.",
      },
      {
        title: "Reads your system",
        body: 'Reads the concepts you defined under "Connect your system" (invoice, order, licence) from your product by the customer\'s external id, follows the playbook and runs approved actions. One working-hours source keeps the out-of-office text, the widget state and the SLA in agreement; every new conversation is acknowledged within 10 seconds.',
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
      "ClickFix Assistant: setup and tests with an approval card",
      "Reads your system through Connect your system",
      "One working-hours source and a 10-second instant reply",
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
    summary:
      "Companies, contacts, verified customer identity, external ids, custom fields, health score and one timeline.",
    eyebrow: "CRM-lite",
    title: "Know the customer behind the conversation.",
    lead: "Contacts link to their companies; every company's conversations, revenue and health score on one screen. Your own customer number arrives as the external id, verified customers carry a badge, and custom fields such as plan and licence sit on the card.",
    description:
      "Company and contact records, customers verified with a signed user_hash, external ids, custom fields, a 0-100 customer health score and a timeline of every interaction.",
    chips: [
      "Companies",
      "Contacts",
      "Verified",
      "External id",
      "Custom fields",
      "Health score",
    ],
    highlights: [
      {
        title: "Verified customer",
        body: "A user signed in on your site or app is identified with a user_hash signed on your server; the company id is part of the signature. The panel separates verified, anonymous and self-named contacts and filters by them.",
      },
      {
        title: "External id",
        body: "The contact's and the company's id in your system is a first-class field: shown on the card, searchable, returned by the API and webhooks. Create the contact by external id through the API and send the first message before the customer visits your site.",
      },
      {
        title: "Custom fields: define, send, done",
        body: "Define a field once; the widget and the API send the same key, an undefined field shows as a warning. List fields are a catalog: SLA, workflows and article targeting use the same options; sensitive fields never reach the AI.",
      },
      {
        title: "Health score and timeline",
        body: "A weighted 0-100 score from usage, revenue and product-event signals; conversations, notes and events per contact and company in order. Which screen the customer wrote from shows in the conversation header.",
      },
    ],
    capabilities: [
      "Company and contact records",
      "Verified / anonymous / self-named distinction",
      "Contact and company external ids",
      "Custom field catalog and undefined-field warning",
      "Company revenue and usage signals",
      "Weighted health score",
      "Contact and company timeline",
      "Screen context (which screen they wrote from)",
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
    name: "Mobile",
    summary:
      "iOS and Android app for your team; in-app support for your customers (webview bridge, Flutter package, app key).",
    eyebrow: "Mobile",
    title:
      "Your inbox in your pocket, your support inside your customer's app.",
    lead: "Reply from the field with the ClickFix iOS and Android app. Add a support screen to your own mobile app with a secure webview that carries the session: Flutter, React Native, iOS and Android connect with the same bridge contract.",
    description:
      "The ClickFix iOS and Android app; in-app support for customer apps with the webview bridge, the Flutter clickfix_webview package, the visitor-core client and app keys.",
    chips: [
      "iOS",
      "Android",
      "Push notifications",
      "Webview bridge",
      "Flutter",
      "React Native",
    ],
    highlights: [
      {
        title: "Push notifications and replies",
        body: "Notifications for new and assigned conversations; conversation history, replies, internal notes and your online status from the app.",
      },
      {
        title: "In-app support (webview)",
        body: "Your app opens the widget page on getclickfix.com in a secure webview; the user is identified with a user_hash signed on your server, deep links open a conversation directly, external links open in the system browser.",
      },
      {
        title: "Flutter package and native core",
        body: "clickfix_webview (pub.dev) implements the bridge contract as ClickFixController and ClickFixWebView. With @getclickfix/visitor-core and an app key you call the widget without a webview and hand the session over to it.",
      },
      {
        title: "From notification to conversation",
        body: "Send your own push from the message.sent webhook event; the notification's deep link opens the support screen on that conversation. The unread badge comes from the widget's unread event.",
      },
    ],
    capabilities: [
      "iOS and Android team app",
      "Push notifications and online status",
      "Webview bridge contract v1 (identify, track, open, session)",
      "Flutter clickfix_webview package",
      "visitor-core and bridge helpers for React Native",
      "App keys (secure access without an Origin)",
      "Deep links and notification-to-conversation",
      "Turkish and English UI",
    ],
  },
  developers: {
    name: "Developer platform",
    summary:
      "REST API, signed webhooks, official SDKs (Node.js, Go, PHP, Laravel, React, Flutter), the API channel and an MCP server.",
    eyebrow: "Developers",
    title: "Connect ClickFix to your own systems.",
    lead: 'A REST API with scoped keys, signed webhooks, official SDKs in six languages, an API channel that turns your own chat into a channel, "Connect your system" that lets the AI read your product, and an integration guide with a running sample on every step.',
    description:
      "ClickFix REST API, signed webhooks, Node.js / Go / PHP / Laravel / React / Flutter SDKs, the API channel, an MCP server and a step-by-step integration guide.",
    chips: ["REST API", "Webhooks", "SDKs", "OpenAPI", "API channel", "MCP"],
    highlights: [
      {
        title: "REST API and reference",
        body: "Scoped API keys, a consistent response envelope and safe retries with Idempotency-Key. Every endpoint has schemas, example request/response, curl/Node.js/Go/PHP samples and a playground you can try in the browser; docs in Turkish and English.",
      },
      {
        title: "Official SDKs",
        body: "@getclickfix/node, the Go module, clickfix/sdk (PHP), clickfix/laravel, @getclickfix/react-widget and @getclickfix/visitor-core; all MIT licensed, open source and published with runnable example apps (clickfix-examples).",
      },
      {
        title: "Integration guide",
        body: "Nine steps in the same order as the setup wizard: identity verification and the company signature, external ids, custom fields, events, connect your system, API channel, webhooks, mobile. Widget, Node.js, Go, PHP and curl samples on every step, executed on every release.",
      },
      {
        title: "Signed webhooks and the API channel",
        body: "Every delivery is HMAC-SHA256 signed and timestamped; failed deliveries are retried for up to 24 hours. With the API channel your own chat app becomes a channel: you send the customer message, the agent reply arrives by webhook.",
      },
      {
        title: "Connect your system",
        body: "Define the concepts (invoice, order, licence) and actions of your accounting, e-commerce or SaaS product on one screen; the AI and your agents read them from your system by the customer's external id and run approved actions.",
      },
      {
        title: "Automation platforms and MCP",
        body: "Zapier, Make and n8n through REST hook subscriptions and polling triggers; ClickFix tools from clients such as Claude and Cursor through the MCP server. Sidebar apps embed in the conversation screen with a signed context token.",
      },
    ],
    capabilities: [
      "Scoped API keys",
      "Bilingual API reference with playground",
      "curl / Node.js / Go / PHP sample on every endpoint",
      "Idempotency-Key support",
      "HMAC-SHA256 signed webhooks",
      "Node.js, Go, PHP, Laravel, React and Flutter SDKs",
      "Runnable example apps in clickfix-examples",
      "API channel (your own chat as a channel)",
      "Connect your system: concepts, endpoints, actions, AI playbook",
      "REST hook subscriptions and an MCP server",
      "Sidebar apps",
      "llms.txt and Markdown output of every docs page",
    ],
  },
};
