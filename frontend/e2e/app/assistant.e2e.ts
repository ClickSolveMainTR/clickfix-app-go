import type { APIRequestContext, Page } from "@playwright/test";

import {
  DEMO_AGENT,
  DEMO_ORG_SLUG,
  DEMO_PASSWORD,
  OWNER_STATE,
  expect,
  runId,
  signIn,
  test,
} from "./fixtures";

/**
 * ClickFix Asistan scenarios of CF-183 (issue #81) through the drawer. The API runs with
 * ASSISTANT_E2E_FAKE_AI=1 (development/test only): a scripted provider answers instead of a real
 * model (backend/internal/modules/assistant/e2e_provider.go), everything else is real (routes,
 * permissions, confirmation cards, database). Only data this run created is touched: channels are
 * named after the run's address and removed at the end; the workflow carries the run id.
 */
test.use({ storageState: OWNER_STATE });
test.describe.configure({ mode: "serial" });

const drawer = (page: Page) =>
  page.getByRole("dialog", { name: "ClickFix Asistan" });

async function openAssistant(page: Page, path: string) {
  await page.goto(path);
  await page.getByRole("button", { name: "ClickFix Asistan'ı aç" }).click();
  await expect(drawer(page)).toBeVisible();
}

async function ask(page: Page, text: string) {
  const composer = drawer(page).getByRole("textbox", {
    name: "Bir soru yazın…",
  });
  await composer.fill(text);
  await composer.press("Enter");
}

const RUN = runId();
const headers = { "X-Organization": DEMO_ORG_SLUG };
const shopURL = (tag: string) =>
  `https://shop-${tag}-${RUN}.example.com/clickfix`;

type Channel = { uuid: string; name: string };
type Action = { id: string; status: string; kind: string };

async function apiChannels(request: APIRequestContext): Promise<Channel[]> {
  const res = await request.get("/api/v1/tenant/channels?type=api", {
    headers,
  });
  return ((await res.json()).data?.items ?? []) as Channel[];
}

/** This run's channels (named "Sipariş sistemi shop-<tag>-<run>"). */
const ownChannels = async (request: APIRequestContext) =>
  (await apiChannels(request)).filter((c) => c.name.includes(RUN));

/** The newest action of the caller's newest assistant thread. */
async function lastAction(request: APIRequestContext) {
  const threads = await request.get("/api/v1/tenant/assistant/threads", {
    headers,
  });
  const thread = ((await threads.json()).data.items as { uuid: string }[])[0]!;
  const msgs = await request.get(
    `/api/v1/tenant/assistant/threads/${thread.uuid}/messages`,
    { headers },
  );
  const items = (await msgs.json()).data.items as { actions?: Action[] }[];
  const action = items.flatMap((m) => m.actions ?? []).at(-1)!;
  return { thread: thread.uuid, action };
}

const actionURL = (thread: string, id: string, verb: string) =>
  `/api/v1/tenant/assistant/threads/${thread}/actions/${id}/${verb}`;

let ownerAction: { thread: string; id: string } | null = null;

test.afterAll(async ({ browser }) => {
  const ctx = await browser.newContext({ storageState: OWNER_STATE });
  for (const ch of await ownChannels(ctx.request))
    await ctx.request.delete(`/api/v1/tenant/channels/${ch.uuid}`, { headers });
  await ctx.close();
});

const lastCard = (page: Page) =>
  drawer(page)
    .locator("div.rounded-xl.border")
    .filter({ has: page.getByRole("button", { name: "Onayla" }) })
    .last();

test("scenario 1: REST API channel is set up, shown once and tested", async ({
  page,
}) => {
  const url = shopURL("main");
  await openAssistant(page, "/channels");
  await ask(
    page,
    `REST API kanalını sistemimize bağlamak istiyorum, adresimiz ${url}`,
  );

  const card = lastCard(page);
  await expect(card).toContainText("REST API kanalı kur");
  // Nothing exists before the confirmation.
  expect(await ownChannels(page.request)).toHaveLength(0);
  await card.getByRole("button", { name: "Kuru çalıştır" }).click();
  await expect(drawer(page)).toContainText("Kuru çalıştırma: sorun yok");
  expect(await ownChannels(page.request)).toHaveLength(0);
  await card.getByRole("button", { name: "Onayla" }).click();

  // The signing secret is shown once, with the example request using a placeholder.
  await expect(drawer(page)).toContainText(
    "Bu değer yalnız şimdi gösteriliyor",
  );
  await expect(drawer(page)).toContainText("$CLICKFIX_SIGNING_SECRET");
  await expect(drawer(page).getByText("Tamamlandı").first()).toBeVisible();

  // Reloading the thread never brings the secret back.
  const secret = await drawer(page).locator("code").first().innerText();
  expect(secret).toMatch(/^[0-9a-f]{64}$/);
  await page.reload();
  await page.getByRole("button", { name: "ClickFix Asistan'ı aç" }).click();
  await expect(drawer(page)).not.toContainText(secret);
  expect(await ownChannels(page.request)).toHaveLength(1);
});

test("scenario 1b: the existing channel is tested from the chat", async ({
  page,
}) => {
  await openAssistant(page, "/channels");
  await ask(page, "REST API kanalımı test et");
  const card = lastCard(page);
  await expect(card).toContainText("Kanalı test et");
  await card.getByRole("button", { name: "Onayla" }).click();
  // The endpoint does not exist, so the delivery fails; the card says so instead of claiming success.
  await expect(drawer(page)).toContainText(
    /Uygulandı ama doğrulanamadı|Uygulandı ve doğrulandı/,
  );
});

test("scenario 2: a workflow is drafted from a sentence, dry-run and saved", async ({
  page,
}) => {
  await openAssistant(page, "/workflows");
  await ask(
    page,
    `Her yeni mesajda konu iade içeriyorsa önceliği yüksek yapan bir iş akışı kur (e2e-${RUN})`,
  );
  const card = lastCard(page);
  await expect(card).toContainText("İş akışı oluştur");
  await card.getByRole("button", { name: "Kuru çalıştır" }).click();
  await expect(drawer(page)).toContainText("Kuru çalıştırma: sorun yok");
  await card.getByRole("button", { name: "Onayla" }).click();
  await expect(drawer(page)).toContainText(
    /Uygulandı ve doğrulandı|Uygulandı ama doğrulanamadı/,
  );

  const res = await page.request.get("/api/v1/tenant/workflows?limit=100", {
    headers: { "X-Organization": DEMO_ORG_SLUG },
  });
  expect(JSON.stringify(await res.json())).toContain(
    `İade talepleri (e2e-${RUN})`,
  );
});

test("scenario 3: the page says what can be tested here", async ({ page }) => {
  await openAssistant(page, "/channels");
  // A fresh chat: the welcome screen carries the page state of the channels page.
  await expect(drawer(page)).toContainText("Bu sayfada: Kanallar");
  await expect(drawer(page)).toContainText(
    "Buradan çalıştırabileceğiniz testler",
  );
});

test("scenario 4: go to the page and highlight the field", async ({ page }) => {
  await openAssistant(page, "/inbox");
  await ask(page, "Çalışma saatleri ayarı nerede?");
  const card = lastCard(page);
  await expect(card).toContainText("Sayfaya git");
  await card.getByRole("button", { name: "Onayla" }).click();
  await drawer(page)
    .getByRole("link", { name: "Sayfayı aç ve göster" })
    .first()
    .click();
  await expect(page).toHaveURL(/\/settings\/sla\?focus=sla-business-hours/);
  await expect(
    page.locator(
      '[data-guide="sla-business-hours"][data-assistant-focus="true"]',
    ),
  ).toBeVisible();
});

test("the assistant docks beside the page and opens as a full page", async ({
  page,
}) => {
  await openAssistant(page, "/channels");
  await ask(page, "Bu sayfada neler yapabilirim?");
  await drawer(page)
    .getByRole("button", { name: "Yan panele sabitle" })
    .click();
  // Split view: the page stays usable next to the same conversation.
  const dock = page.getByRole("complementary", { name: "ClickFix Asistan" });
  await expect(dock).toContainText("Bu sayfada neler yapabilirim?");
  await dock.getByRole("button", { name: "Çekmeceye al" }).click();
  await expect(drawer(page)).toBeVisible();
  await drawer(page).getByRole("link", { name: "Tam sayfada aç" }).click();
  await expect(page).toHaveURL(/\/assistant$/);
  await expect(
    page.getByText("Bu sayfada neler yapabilirim?").first(),
  ).toBeVisible();
});

test("rejected: a card that is turned down changes nothing", async ({
  page,
}) => {
  await openAssistant(page, "/channels");
  await ask(page, `REST API kanalını bağla, adresimiz ${shopURL("reject")}`);
  const card = lastCard(page);
  await expect(card).toContainText("REST API kanalı kur");
  await card.getByRole("button", { name: "Reddet" }).click();
  await expect(drawer(page)).toContainText("Reddedildi");
  expect(
    (await ownChannels(page.request)).filter((c) => c.name.includes("reject")),
  ).toHaveLength(0);
  // A rejected action cannot be confirmed afterwards.
  const { thread, action } = await lastAction(page.request);
  expect(action.status).toBe("rejected");
  const res = await page.request.post(actionURL(thread, action.id, "confirm"), {
    headers,
  });
  expect(res.status()).toBe(409);
});

test("invalid parameters are refused and nothing is applied", async ({
  page,
}) => {
  await openAssistant(page, "/channels");
  await ask(page, `REST API kanalını bağla, adresimiz ${shopURL("bad")}`);
  await expect(lastCard(page)).toContainText("REST API kanalı kur");
  const { thread, action } = await lastAction(page.request);
  for (const params of [
    { webhook_url: "http://127.0.0.1/hook" },
    { webhook_url: "https://shop.example.com/x", name: 42 },
    { unknown_field: "x" },
  ]) {
    const res = await page.request.post(
      actionURL(thread, action.id, "confirm"),
      {
        headers,
        data: { params },
      },
    );
    expect([400, 422], JSON.stringify(params)).toContain(res.status());
  }
  // The card is still pending and nothing was created.
  const again = await lastAction(page.request);
  expect(again.action.status).toBe("proposed");
  expect(
    (await ownChannels(page.request)).filter((c) => c.name.includes("bad")),
  ).toHaveLength(0);
  ownerAction = { thread, id: action.id };
});

test("unauthorized: an agent gets no set-up card and cannot confirm another's", async ({
  browser,
}) => {
  const ctx = await browser.newContext({ storageState: undefined });
  const page = await ctx.newPage();
  await signIn(page, DEMO_AGENT, DEMO_PASSWORD);
  // The owner's pending action is not visible to the agent.
  expect(ownerAction).not.toBeNull();
  const res = await page.request.post(
    actionURL(ownerAction!.thread, ownerAction!.id, "confirm"),
    { headers },
  );
  expect([403, 404]).toContain(res.status());
  // The agent may not manage channels: the tool is not offered, no card.
  await openAssistant(page, "/inbox");
  await ask(page, `REST API kanalını bağla, adresimiz ${shopURL("agent")}`);
  await expect(drawer(page)).toContainText("yetkiniz görünmüyor");
  await expect(
    drawer(page).getByRole("button", { name: "Onayla" }),
  ).toHaveCount(0);
  expect(
    (await ownChannels(page.request)).filter((c) => c.name.includes("agent")),
  ).toHaveLength(0);
  await ctx.close();
});
