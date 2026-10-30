import {
  expect,
  test as base,
  type APIRequestContext,
  type Page,
} from "@playwright/test";

import { BASE_URL } from "./playwright.config";

/** Demo organization written by `seed-demo` (backend/internal/demoseed). */
export const DEMO_ORG_SLUG = "demo-anadolu-yazilim";
export const DEMO_ORG_NAME = "Demo · Anadolu Yazılım";
export const DEMO_OWNER = "selin.arslan@anadolu-yazilim.example.com";
export const DEMO_AGENT = "burak.sahin@anadolu-yazilim.example.com";

export const DEMO_PASSWORD = process.env.E2E_DEMO_PASSWORD ?? "";
export const ADMIN_EMAIL = process.env.E2E_ADMIN_EMAIL ?? "";
export const ADMIN_PASSWORD = process.env.E2E_ADMIN_PASSWORD ?? "";

/** Public frontend origin (PUBLIC_FRONTEND_URL of the stack under test). */
export const BASE_URL_ORIGIN = new URL(BASE_URL).origin;

/**
 * The Go API itself, for the endpoints clients call without the BFF: the public API with an API
 * key and the MCP OAuth discovery documents.
 */
export const API_URL = (
  process.env.E2E_API_URL ?? "http://127.0.0.1:8080"
).replace(/\/$/, "");

/** Saved sessions written by auth.setup.ts. */
export const OWNER_STATE = "e2e/app/.auth/owner.json";
export const ADMIN_STATE = "e2e/app/.auth/admin.json";

/** Unique suffix so reruns against the same database never collide. */
export const runId = () =>
  `${Date.now().toString(36)}${Math.floor(Math.random() * 1e4).toString(36)}`;

/**
 * Fills a server-rendered form once React hydrated it: a value typed before hydration is reset.
 */
export async function fillForm(
  page: Page,
  fields: [selector: string, value: string][],
) {
  await expect(async () => {
    for (const [selector, value] of fields)
      await page.locator(selector).fill(value);
    for (const [selector, value] of fields) {
      await expect(page.locator(selector)).toHaveValue(value, {
        timeout: 1_000,
      });
    }
  }).toPass({ timeout: 60_000 });
}

/** Signs in through the login form and waits until the app shell left the login page. */
export async function signIn(page: Page, email: string, password: string) {
  // Not "networkidle": the login page may keep a request open (nightly runs timed out on it), and
  // fillForm already waits for hydration.
  await page.goto("/login", { waitUntil: "domcontentloaded" });
  await fillForm(page, [
    ["#email", email],
    ["#password", password],
  ]);
  await page.getByRole("button", { name: "Giriş yap", exact: true }).click();
  await page.waitForURL((url) => !/\/(platform\/)?login/.test(url.pathname), {
    timeout: 30_000,
  });
}

type Envelope<T> = { success: boolean; data: T };

/**
 * Same-origin BFF call (`/api/v1/*`) with the browser session cookie; tenant calls carry the
 * organization in X-Organization like the app does.
 */
export async function api<T>(
  request: APIRequestContext,
  method: "GET" | "POST" | "PATCH" | "PUT" | "DELETE",
  path: string,
  body?: unknown,
  org: string | null = DEMO_ORG_SLUG,
): Promise<T> {
  const res = await request.fetch(`/api/v1${path}`, {
    method,
    data: body,
    headers: org ? { "X-Organization": org } : {},
  });
  const text = await res.text();
  expect(res.ok(), `${method} ${path} -> ${res.status()} ${text}`).toBeTruthy();
  // 204 No Content has no envelope.
  return text ? (JSON.parse(text) as Envelope<T>).data : (undefined as T);
}

/** Public id of the organization's web widget channel. */
export async function widgetPublicId(
  request: APIRequestContext,
  org: string = DEMO_ORG_SLUG,
) {
  const { items } = await api<{ items: { uuid: string; type: string }[] }>(
    request,
    "GET",
    "/tenant/channels",
    undefined,
    org,
  );
  const widget = items.find((c) => c.type === "widget");
  expect(widget, `${org} has a widget channel`).toBeTruthy();
  return widget!.uuid;
}

/**
 * A visitor opens a conversation from the website widget (public widget API through the BFF
 * relay). Returns the conversation and the visitor's auth header for the widget API.
 */
export async function widgetConversation(
  request: APIRequestContext,
  input: { text: string; name: string; email: string },
  org: string = DEMO_ORG_SLUG,
) {
  const widget = await widgetPublicId(request, org);
  const boot = await api<{ visitor: { token: string } }>(
    request,
    "POST",
    `/widget/${widget}/bootstrap`,
    {},
    null,
  );
  const visitor = { Authorization: `Bearer ${boot.visitor.token}` };
  const created = await request.post(`/api/v1/widget/${widget}/conversations`, {
    headers: visitor,
    data: input,
  });
  expect(created.ok(), await created.text()).toBeTruthy();
  const conversation = (
    (await created.json()) as { data: { conversation_uuid: string } }
  ).data.conversation_uuid;
  return { widget, visitor, conversation };
}

/**
 * A module guide (product tour) may open on the first visit of a page and cover it. The `test`
 * below registers this for every page; Esc is not enough (the tour ignores it while an input has
 * focus), so the handler clicks "Turu geç".
 */
export async function skipGuides(page: Page) {
  const overlay = page.locator("[data-guide-overlay]");
  await page.addLocatorHandler(overlay, async () => {
    // The backdrop is itself a full-screen "Turu geç" button. While the card animates in it is
    // never "stable", so the click is dispatched instead of waiting for actionability.
    await overlay
      .getByRole("button", { name: "Turu geç" })
      .first()
      .dispatchEvent("click");
  });
}

/**
 * React reports hydration mismatches (production: minified errors 418/423/425) and, in
 * development, missing list keys on the console. They do not break a flow, so assertions miss
 * them.
 */
const REACT_CONSOLE_ERRORS =
  /Minified React error #(418|423|425)\b|Hydration failed|hydration mismatch|did not match\. Server|unique "key" prop/i;

/**
 * `test` for the app specs:
 * - `page` skips module guide tours (skipGuides);
 * - a console guard fails a spec when a page of its browser context logged a React hydration or
 *   list-key error. Other console errors (failed requests, third-party noise) are ignored on
 *   purpose so the guard stays deterministic.
 */
export const test = base.extend<{ consoleGuard: void }>({
  page: async ({ page }, provide) => {
    await skipGuides(page);
    await provide(page);
  },
  consoleGuard: [
    async ({ context }, provide) => {
      const errors: string[] = [];
      const record = (url: string, text: string) => {
        if (REACT_CONSOLE_ERRORS.test(text))
          errors.push(`${url}: ${text.slice(0, 500)}`);
      };
      context.on("console", (message) => {
        if (message.type() === "error")
          record(message.page()?.url() ?? "", message.text());
      });
      context.on("weberror", (error) =>
        record(error.page()?.url() ?? "", String(error.error())),
      );
      await provide();
      expect(errors, "React hydration / key errors on the console").toEqual([]);
    },
    { auto: true },
  ],
});

export { expect };
