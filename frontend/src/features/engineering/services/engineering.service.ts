import type { ServerListParams } from "@/components/entity";
import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type Issue = components["schemas"]["EngineeringIssue"];
export type IssueStatus = Issue["status"];
export type IssuePriority = Issue["priority"];
export type IssueCreate = components["schemas"]["EngineeringCreateIssue"];
export type IssuePatch = components["schemas"]["EngineeringPatchIssue"];
export type IssueComment = components["schemas"]["EngineeringComment"];
export type IssueConversationLink =
  components["schemas"]["EngineeringConversationLink"];
export type Escalation = components["schemas"]["EngineeringEscalation"];

export type EngineeringIntegration =
  components["schemas"]["EngineeringIntegration"];
export type EngineeringIntegrationConfig =
  components["schemas"]["EngineeringIntegrationConfig"];
export type EngineeringIntegrationInput =
  components["schemas"]["EngineeringIntegrationInput"];
/** Issue trackers with OAuth; `sentry` shares the API but not this screen. */
export type EngineeringProvider = "linear" | "jira" | "github";

export type IssuePage = {
  items: Issue[];
  total: number;
  limit: number;
  offset: number;
};

/** The API sorts by latest change only (`-updated_at`). */
export type ListIssuesParams = Pick<ServerListParams, "limit" | "offset"> & {
  q?: string;
  status?: IssueStatus;
  priority?: IssuePriority;
};

const BASE = "/v1/tenant/engineering";

/**
 * Product requests (`/v1/tenant/engineering/*`, `tenant.tickets.*`): bugs and
 * requests support hands to engineering, with the conversations that
 * reported them. Ported from `mobile/src/features/engineering/api.ts`.
 */
export const engineeringService = {
  list(slug: string, params: ListIssuesParams) {
    return tenantRequest<IssuePage>(slug, "GET", `${BASE}/issues`, {
      query: { ...params, sort: "-updated_at" },
    });
  },

  get(slug: string, uuid: string) {
    return tenantRequest<Issue>(slug, "GET", `${BASE}/issues/${uuid}`);
  },

  /** Opens a request, optionally from a conversation (linked at once). */
  create(slug: string, body: IssueCreate) {
    return tenantRequest<Issue>(slug, "POST", `${BASE}/issues`, { body });
  },

  update(slug: string, uuid: string, body: IssuePatch) {
    return tenantRequest<Issue>(slug, "PATCH", `${BASE}/issues/${uuid}`, {
      body,
    });
  },

  conversations(slug: string, uuid: string, silent = false) {
    return tenantRequest<{ items: IssueConversationLink[] }>(
      slug,
      "GET",
      `${BASE}/issues/${uuid}/conversations`,
      { silent },
    );
  },

  link(
    slug: string,
    uuid: string,
    conversation: string,
    notifyCustomer: boolean,
  ) {
    return tenantRequest<unknown>(
      slug,
      "POST",
      `${BASE}/issues/${uuid}/conversations`,
      {
        body: {
          conversation_uuid: conversation,
          notify_customer: notifyCustomer,
        },
      },
    );
  },

  comments(slug: string, uuid: string) {
    return tenantRequest<{ items: IssueComment[] }>(
      slug,
      "GET",
      `${BASE}/issues/${uuid}/comments`,
    );
  },

  addComment(slug: string, uuid: string, body: string) {
    return tenantRequest<IssueComment>(
      slug,
      "POST",
      `${BASE}/issues/${uuid}/comments`,
      { body: { body } },
    );
  },

  /** Requests that look like this conversation's problem. */
  suggestions(slug: string, conversation: string) {
    return tenantRequest<{ items: Issue[] }>(
      slug,
      "GET",
      `${BASE}/conversations/${conversation}/suggestions`,
      { silent: true },
    );
  },

  /**
   * Masked provider settings (`tenant.settings.write`, like every call below). Secrets are never
   * returned, only whether each one is stored. Errors are rendered by the
   * caller, so every integration call is silent.
   */
  integrations(slug: string) {
    return tenantRequest<{ items: EngineeringIntegration[] }>(
      slug,
      "GET",
      `${BASE}/integrations`,
      { silent: true },
    );
  },

  /**
   * Replaces the provider configuration. Empty or
   * omitted secrets keep the stored ones; `enabled: false` stops syncing.
   */
  saveIntegration(
    slug: string,
    provider: EngineeringProvider,
    body: EngineeringIntegrationInput,
  ) {
    return tenantRequest<EngineeringIntegration>(
      slug,
      "PUT",
      `${BASE}/integrations/${provider}`,
      { body, silent: true },
    );
  },

  /** Provider consent URL with a single-use state (ten minutes). */
  startOAuth(slug: string, provider: EngineeringProvider) {
    return tenantRequest<{ authorize_url?: string }>(
      slug,
      "POST",
      `${BASE}/integrations/${provider}/oauth/start`,
      { silent: true },
    );
  },

  /** Exchanges the callback code in the session that started the flow. */
  completeOAuth(
    slug: string,
    provider: EngineeringProvider,
    body: { state: string; code: string },
  ) {
    return tenantRequest<EngineeringIntegration>(
      slug,
      "POST",
      `${BASE}/integrations/${provider}/oauth/complete`,
      { body, silent: true },
    );
  },

  escalations(slug: string, conversation: string) {
    return tenantRequest<{ items: Escalation[] }>(
      slug,
      "GET",
      `${BASE}/conversations/${conversation}/escalations`,
      { silent: true },
    );
  },
};
