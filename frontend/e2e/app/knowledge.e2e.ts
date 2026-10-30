import { DEMO_ORG_SLUG, OWNER_STATE, expect, runId, test } from "./fixtures";

test.use({ storageState: OWNER_STATE });

test("a published article is visible in the public help center", async ({
  page,
  browser,
}) => {
  const id = runId();
  const title = `E2E iade süreci ${id}`;
  const slug = `e2e-iade-sureci-${id}`;

  await page.goto("/settings/knowledge/new");
  await page.locator("#kb-title").fill(title);
  const body = page.locator('[contenteditable="true"]').first();
  await body.click();
  await page.keyboard.type(
    "İade talepleri 14 gün içinde Faturalandırma ekibine iletilir.",
  );
  await page.locator("#kb-slug").fill(slug);
  await page.getByRole("button", { name: "Kaydet ve yayınla" }).click();
  await expect(page.getByText("Makale yayınlandı")).toBeVisible();

  // Anonymous visitor.
  const visitor = await browser.newContext({ locale: "tr-TR" });
  const help = await visitor.newPage();
  await help.goto(`/help/${DEMO_ORG_SLUG}/article/${slug}`);
  await expect(help.getByRole("heading", { name: title })).toBeVisible();
  await expect(help.getByText("İade talepleri 14 gün içinde")).toBeVisible();
  // Seeded collection page lists the seeded public articles.
  await help.goto(`/help/${DEMO_ORG_SLUG}/faturalandirma`);
  await expect(
    help.getByText("Faturalarımı nereden indirebilirim?"),
  ).toBeVisible();
  // Internal articles never reach the public help center.
  await expect(
    help.getByText("İç prosedür: Kurumsal müşteri eskalasyonu"),
  ).toHaveCount(0);
  await visitor.close();
});
