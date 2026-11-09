import type { Page } from "@playwright/test";

import { OWNER_STATE, expect, runId, test } from "./fixtures";

/**
 * ClickFix Asistan scenarios of CF-183 (issue #81) through the drawer. The API runs with
 * ASSISTANT_E2E_FAKE_AI=1 (development/test only): a scripted provider answers instead of a real
 * model (backend/internal/modules/assistant/e2e_provider.go), everything else is real (routes,
 * permissions, confirmation cards, database).
 */
test.use({ storageState: OWNER_STATE });
test.describe.configure({ mode: "serial" });

const drawer = (page: Page) => page.getByRole("dialog");

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

const lastCard = (page: Page) =>
  drawer(page)
    .locator("div.rounded-xl.border")
    .filter({ has: page.getByRole("button", { name: "Onayla" }) })
    .last();

test("scenario 1: REST API channel is set up, shown once and tested", async ({
  page,
}) => {
  const url = `https://shop-${runId()}.example.com/clickfix`;
  await openAssistant(page, "/channels");
  await ask(
    page,
    `REST API kanalını sistemimize bağlamak istiyorum, adresimiz ${url}`,
  );

  const card = lastCard(page);
  await expect(card).toContainText("REST API kanalı kur");
  await card.getByRole("button", { name: "Kuru çalıştır" }).click();
  await expect(drawer(page)).toContainText("Kuru çalıştırma: sorun yok");
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
    "Her yeni mesajda konu iade içeriyorsa önceliği yüksek yapan bir iş akışı kur",
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
    headers: { "X-Organization": "demo-anadolu-yazilim" },
  });
  expect(JSON.stringify(await res.json())).toContain("İade talepleri (e2e)");
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
