/**
 * ClickFix Asistan inside the support widget (CF-193).
 *
 * The panel embeds ClickFix's own support widget and passes
 * `init({ assistant: { organization } })`. The mode only turns on when the
 * widget script is served from the page's own origin (the ClickFix panel):
 * the calls then go to the panel's same-origin `/api/v1/tenant/assistant/*`
 * with the member's session, on the `widget` surface, where the server
 * offers only the allowlisted capabilities and re-checks permissions on
 * confirm. On a customer's site the option is ignored.
 */

export type AssistantOptions = { organization: string };

export type AssistantAction = {
  id: string;
  title: string;
  summary?: string;
  status: string;
  risk?: string;
  result?: string;
  focus?: { route: string; target?: string } | null;
  verification?: { status: string; message: string } | null;
};

export type AssistantMessage = {
  uuid?: string;
  role: "user" | "assistant" | "tool";
  content: string;
  actions?: AssistantAction[];
  suggestions?: string[];
};

const SLUG = /^[a-z0-9][a-z0-9-]{0,62}$/;
const TARGET = /^[a-z0-9][a-z0-9-]{0,63}$/;

/** The assistant mode is on only on the panel's own origin. */
export function assistantAllowed(
  appOrigin: string,
  pageOrigin: string,
  opts: AssistantOptions | undefined,
): opts is AssistantOptions {
  return Boolean(
    opts &&
    typeof opts.organization === "string" &&
    SLUG.test(opts.organization) &&
    appOrigin === pageOrigin,
  );
}

/** "Go to the page" link: internal paths only (same rule as the drawer). */
export function assistantHref(
  focus: AssistantAction["focus"] | undefined,
): string | null {
  const route = focus?.route ?? "";
  if (
    !route.startsWith("/") ||
    route.startsWith("//") ||
    route.includes("\\") ||
    /[?#\s\u0000-\u001f\u007f]/.test(route)
  )
    return null;
  const target = focus?.target;
  if (!target || !TARGET.test(target)) return route;
  return `${route}?focus=${encodeURIComponent(target)}`;
}

type Fetch = typeof fetch;

export class AssistantError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
  ) {
    super(code);
  }
}

/** Same-origin client of the tenant assistant routes. */
export class AssistantClient {
  thread: string | null = null;

  constructor(
    private readonly org: string,
    private readonly locale: () => string,
    private readonly fetchFn: Fetch = (...a) => fetch(...a),
  ) {}

  private async call<T>(
    path: string,
    body?: unknown,
    empty = false,
  ): Promise<T> {
    const res = await this.fetchFn(
      `/api/v1/tenant/assistant${path}${path.includes("?") ? "&" : "?"}locale=${encodeURIComponent(this.locale())}`,
      {
        method: "POST",
        credentials: "same-origin",
        headers: {
          "Content-Type": "application/json",
          "X-Organization": this.org,
        },
        body: body === undefined ? undefined : JSON.stringify(body),
      },
    );
    if (empty && res.status === 204) return undefined as T;
    const json = (await res.json().catch(() => null)) as {
      data?: T;
      error?: { code?: string };
    } | null;
    if (!res.ok || !json || json.data === undefined)
      throw new AssistantError(
        res.status,
        json?.error?.code ?? `HTTP_${res.status}`,
      );
    return json.data;
  }

  private context(route: string) {
    return { surface: "widget", route: route.slice(0, 200) };
  }

  async send(content: string, route: string): Promise<AssistantMessage> {
    if (!this.thread) {
      const t = await this.call<{ uuid: string }>("/threads", {
        context: this.context(route),
      });
      this.thread = t.uuid;
    }
    const out = await this.call<{ reply: AssistantMessage }>(
      `/threads/${encodeURIComponent(this.thread)}/messages`,
      { content, context: this.context(route) },
    );
    return out.reply;
  }

  confirm(actionId: string): Promise<AssistantMessage> {
    return this.call<AssistantMessage>(this.actionPath(actionId, "confirm"));
  }

  reject(actionId: string): Promise<void> {
    return this.call<void>(
      this.actionPath(actionId, "reject"),
      undefined,
      true,
    );
  }

  private actionPath(actionId: string, verb: string) {
    if (!this.thread) throw new AssistantError(0, "NO_THREAD");
    return `/threads/${encodeURIComponent(this.thread)}/actions/${encodeURIComponent(actionId)}/${verb}`;
  }
}
