import type { IntegrationScope } from "../services/automation-integrations.service";

/** Allowlist of `IntegrationKeyInput.scopes` (backend `publicapi/model`). */
export const SCOPES: IntegrationScope[] = [
  "conversations:read",
  "conversations:write",
  "contacts:read",
  "contacts:write",
  "companies:read",
  "triggers:read",
  "webhooks:read",
  "webhooks:write",
  "revenue:write",
  "health:write",
  "events:write",
];

/** What Zapier / Make / n8n need: subscribe, poll, create, note, update. */
export const WORKFLOW_SCOPES: IntegrationScope[] = [
  "triggers:read",
  "webhooks:write",
  "conversations:write",
  "contacts:write",
];

export const scopeKey = (scope: string) =>
  `automation_integrations.scopes.${scope.replace(":", "_")}`;

/** Workflow trigger events (integrations/README.md). */
export const TRIGGER_EVENTS = [
  "conversation.created",
  "conversation.resolved",
  "contact.created",
  "csat.submitted",
  "conversation.tag_added",
];

/** Public API endpoints under `/v1/public-api` (integrations/README.md). */
export const ENDPOINTS = [
  {
    method: "POST",
    path: "/subscriptions",
    scope: "webhooks:write",
    key: "subscribe",
  },
  {
    method: "DELETE",
    path: "/subscriptions/{uuid}",
    scope: "webhooks:write",
    key: "unsubscribe",
  },
  {
    method: "GET",
    path: "/triggers?event=conversation.created&cursor=0",
    scope: "triggers:read",
    key: "poll",
  },
  {
    method: "POST",
    path: "/conversations",
    scope: "conversations:write",
    key: "create_conversation",
  },
  {
    method: "POST",
    path: "/conversations/{uuid}/messages",
    scope: "conversations:write",
    key: "add_note",
  },
  {
    method: "PATCH",
    path: "/contacts/{uuid}",
    scope: "contacts:write",
    key: "update_contact",
  },
] as const;
