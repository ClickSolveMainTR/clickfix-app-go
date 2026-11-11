import { readSSE } from "./sse";
import { ApiError } from "@/lib/api/errors";
import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";
import { tenantFormRequest, tenantRequest } from "@/lib/api/tenant-request";

export type AssistantStatus = components["schemas"]["AssistantStatus"];
export type AssistantThread = components["schemas"]["AssistantThread"];
export type AssistantMessage = components["schemas"]["AssistantMessage"];
export type AssistantAction = components["schemas"]["AssistantProposedAction"];
export type AssistantActionField =
  components["schemas"]["AssistantActionField"];
export type AssistantActionDetail =
  components["schemas"]["AssistantActionDetail"];
export type AssistantPageState = components["schemas"]["AssistantPageState"];
export type AssistantDocument = components["schemas"]["AssistantDocument"];
export type AssistantConfirmResult =
  components["schemas"]["AssistantConfirmResult"];
export type AssistantContext = components["schemas"]["AssistantContext"];
export type AssistantSendResult = components["schemas"]["AssistantSendResult"];
export type AssistantSettings = components["schemas"]["AssistantSettings"];
export type AssistantUsage = components["schemas"]["AssistantUsage"];
export type AssistantSupportWidget =
  components["schemas"]["AssistantSupportWidget"];

const A = "/v1/tenant/assistant";

/**
 * ClickFix Asistan (CF-141): platform-owned AI support and set-up chat.
 * Tenant routes need `tenant.assistant.use`; confirming an action re-checks
 * that action's own permission server-side. Writes are silent: the panel
 * renders its own error state.
 */
export const assistantService = {
  async stream(
    slug: string,
    thread: string,
    content: string,
    context: AssistantContext,
    locale: string,
    signal: AbortSignal,
    emit: (name: string, data: unknown) => void,
  ) {
    // The id travels to the API (X-Request-ID) and into its
    // assistant_stream_failed log line, so a failure seen in the browser
    // console can be matched with the server log (CF-207, #150).
    // randomUUID is missing in insecure contexts (plain http on a LAN):
    // then no id is sent and the server's own id is used.
    const requestId =
      typeof crypto !== "undefined" && typeof crypto.randomUUID === "function"
        ? crypto.randomUUID()
        : undefined;
    const response = await fetch(
      `/api/v1/tenant/assistant/threads/${encodeURIComponent(thread)}/messages/stream?locale=${encodeURIComponent(locale)}`,
      {
        method: "POST",
        credentials: "include",
        signal,
        headers: {
          Accept: "text/event-stream",
          "Content-Type": "application/json",
          "X-Organization": slug,
          ...(requestId ? { "X-Request-ID": requestId } : {}),
        },
        body: JSON.stringify({ content, context }),
      },
    );
    if (!response.ok || !response.body) {
      const payload = await response.json().catch(() => null);
      const code = payload?.error?.code ?? "ASSISTANT_AI_UNAVAILABLE";
      console.error("assistant stream request failed", {
        status: response.status,
        code,
        requestId,
      });
      throw new ApiError({
        status: response.status,
        code,
        message: "Assistant request failed",
        ...(requestId ? { requestId } : {}),
      });
    }
    await readSSE(response.body, (name, data) => {
      if (name === "error") {
        const { code, request_id } = data as {
          code: string;
          request_id?: string;
        };
        const id = request_id || requestId;
        console.error("assistant stream failed", { code, requestId: id });
        throw new ApiError({
          status: 503,
          code,
          message: "Assistant stream failed",
          ...(id ? { requestId: id } : {}),
        });
      }
      emit(name, data);
    });
  },
  /** ClickFix's support widget and the member's identity signature. */
  supportWidget(slug: string) {
    return tenantRequest<AssistantSupportWidget>(
      slug,
      "GET",
      `${A}/support-widget`,
      { silent: true },
    );
  },
  status(slug: string) {
    return tenantRequest<AssistantStatus>(slug, "GET", `${A}/status`, {
      silent: true,
    });
  },
  /** The page's missing steps and tests; null when no module describes it. */
  async pageState(slug: string, route: string, locale: string) {
    const out = await tenantRequest<AssistantPageState | undefined>(
      slug,
      "GET",
      `${A}/page-state`,
      { query: { surface: "web", route, locale }, silent: true },
    );
    return out ?? null;
  },
  threads(slug: string) {
    return tenantRequest<{ items: AssistantThread[] }>(
      slug,
      "GET",
      `${A}/threads`,
      { silent: true },
    );
  },
  createThread(slug: string, context: AssistantContext, signal?: AbortSignal) {
    return tenantRequest<AssistantThread>(slug, "POST", `${A}/threads`, {
      body: { context },
      signal,
      silent: true,
    });
  },
  messages(slug: string, thread: string) {
    return tenantRequest<{ items: AssistantMessage[] }>(
      slug,
      "GET",
      `${A}/threads/${thread}/messages`,
      { silent: true },
    );
  },
  send(
    slug: string,
    thread: string,
    content: string,
    context: AssistantContext,
    locale: string,
  ) {
    return tenantRequest<AssistantSendResult>(
      slug,
      "POST",
      `${A}/threads/${thread}/messages`,
      { body: { content, context }, query: { locale }, silent: true },
    );
  },
  /** Edited card fields are validated again by the server. */
  confirm(
    slug: string,
    thread: string,
    action: string,
    locale: string,
    params?: Record<string, unknown>,
  ) {
    return tenantRequest<AssistantConfirmResult>(
      slug,
      "POST",
      `${A}/threads/${thread}/actions/${encodeURIComponent(action)}/confirm`,
      {
        query: { locale },
        silent: true,
        ...(params ? { body: { params } } : {}),
      },
    );
  },
  /** Preview without changing data (the card's "Dry run"). */
  dryRun(
    slug: string,
    thread: string,
    action: string,
    locale: string,
    params?: Record<string, unknown>,
  ) {
    return tenantRequest<AssistantAction>(
      slug,
      "POST",
      `${A}/threads/${thread}/actions/${encodeURIComponent(action)}/dry-run`,
      {
        query: { locale },
        silent: true,
        ...(params ? { body: { params } } : {}),
      },
    );
  },
  reject(slug: string, thread: string, action: string) {
    return tenantRequest<void>(
      slug,
      "POST",
      `${A}/threads/${thread}/actions/${encodeURIComponent(action)}/reject`,
      { silent: true },
    );
  },
  /** Documents shared in a thread (CF-191); text only, kept 7 days. */
  documents(slug: string, thread: string) {
    return tenantRequest<{ items: AssistantDocument[] }>(
      slug,
      "GET",
      `${A}/threads/${thread}/documents`,
      { silent: true },
    );
  },
  uploadDocument(slug: string, thread: string, file: File) {
    const form = new FormData();
    form.append("file", file);
    return tenantFormRequest<AssistantDocument>(
      slug,
      "POST",
      `${A}/threads/${thread}/documents`,
      form,
      { silent: true },
    );
  },
  linkDocument(slug: string, thread: string, url: string) {
    return tenantRequest<AssistantDocument>(
      slug,
      "POST",
      `${A}/threads/${thread}/documents/link`,
      { body: { url }, silent: true },
    );
  },
  removeDocument(slug: string, thread: string, document: string) {
    return tenantRequest<void>(
      slug,
      "DELETE",
      `${A}/threads/${thread}/documents/${encodeURIComponent(document)}`,
      { silent: true },
    );
  },
  escalate(slug: string, thread: string, note: string, locale: string) {
    return tenantRequest<{ ticket_ref: string }>(
      slug,
      "POST",
      `${A}/threads/${thread}/escalate`,
      { body: { note }, query: { locale }, silent: true },
    );
  },
};

export const platformAssistantService = {
  settings() {
    return platformRequest<AssistantSettings>(
      "GET",
      "/v1/platform/assistant/settings",
    );
  },
  save(body: AssistantSettings) {
    return platformRequest<AssistantSettings>(
      "PUT",
      "/v1/platform/assistant/settings",
      { body },
    );
  },
  usage(days: number) {
    return platformRequest<AssistantUsage>(
      "GET",
      "/v1/platform/assistant/usage",
      { query: { days } },
    );
  },
};
