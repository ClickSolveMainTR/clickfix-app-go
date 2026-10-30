import type { Page } from "@playwright/test";

import {
  DEMO_OWNER,
  api,
  expect,
  runId,
  test,
  widgetPublicId,
} from "./fixtures";

/** Active organization slug the org switcher persisted. */
async function activeSlug(page: Page) {
  return page.evaluate(() => {
    const raw = localStorage.getItem("clickfix.active-organization");
    return raw
      ? (JSON.parse(raw) as { state: { slug: string } }).state.slug
      : "";
  });
}

test.describe("sign in", () => {
  test("rejects a wrong password", async ({ page }) => {
    await page.goto("/login");
    await page.locator("#email").fill(DEMO_OWNER);
    await page.locator("#password").fill("WrongPassword1");
    await page.getByRole("button", { name: "Giriş yap", exact: true }).click();
    await expect(page.getByText("E-posta veya şifre hatalı")).toBeVisible();
    await expect(page).toHaveURL(/\/login/);
  });
});

test.describe("sign up, onboarding and organizations", () => {
  test("a new user signs up, finishes the setup wizard and switches organizations", async ({
    page,
  }) => {
    const id = runId();
    const orgName = `E2E Servis ${id}`;
    const secondOrg = `E2E İkinci ${id}`;

    // Sign up at /register (CF-143): account and company, then the e-mail code
    // step (skipped here: e2e has no mailbox) opens the organization and
    // lands in the AI-assisted set-up.
    await page.goto("/register");
    await page.locator("#name").fill("Ece");
    await page.locator("#surname").fill("Deneme");
    await page.locator("#email").fill(`e2e-${id}@example.com`);
    await page.locator("#company").fill(orgName);
    await page.locator("#password").fill(`E2e${id}Pass1`);
    await page
      .getByRole("button", { name: "Hesabımı oluştur", exact: true })
      .click();
    await page
      .getByRole("button", { name: "Şimdilik geç, daha sonra doğrularım" })
      .click({ timeout: 30_000 });
    await page.waitForURL((url) => url.pathname === "/getting-started", {
      timeout: 30_000,
    });

    // The set-up checklist of the new organization.
    await page.goto("/setup");
    const steps = page.getByRole("navigation", { name: "Kurulum adımları" });
    await expect(steps).toBeVisible({ timeout: 30_000 });
    const slug = await activeSlug(page);
    expect(slug).not.toBe("");

    // Step: first channel (web widget).
    await page.goto("/setup?adim=channel");
    await page.getByRole("radio", { name: /Web widget/ }).click();
    await page.locator("#channel-name").fill("Web sitesi");
    await page
      .getByRole("button", { name: "Kanalı bağla", exact: true })
      .click();
    await expect(page.getByText("Kanal bağlandı")).toBeVisible();

    // Step: team (solo) and widget (confirmed).
    await page.goto("/setup?adim=team");
    await page
      .getByRole("button", { name: "Şimdilik tek başıma çalışıyorum" })
      .click();
    await page.goto("/setup?adim=widget");
    // A connected widget channel completes this step by itself.
    const confirmWidget = page.getByRole("button", {
      name: "Widget'ı siteme ekledim",
    });
    const widgetDone = page.getByText("Bu adım tamamlandı");
    await expect(confirmWidget.or(widgetDone)).toBeVisible();
    if (await confirmWidget.isVisible()) await confirmWidget.click();
    await expect(widgetDone).toBeVisible();

    // Step: first conversation, sent through the public widget API of the new channel.
    const widget = await widgetPublicId(page.request, slug);
    const boot = await api<{ visitor: { token: string } }>(
      page.request,
      "POST",
      `/widget/${widget}/bootstrap`,
      {},
      null,
    );
    const first = await page.request.post(
      `/api/v1/widget/${widget}/conversations`,
      {
        headers: { Authorization: `Bearer ${boot.visitor.token}` },
        data: {
          text: "Merhaba, ilk mesaj",
          name: "İlk Müşteri",
          email: `ilk-${id}@example.com`,
        },
      },
    );
    expect(first.ok(), await first.text()).toBeTruthy();

    await page.goto("/setup?adim=first_conversation");
    // The step completes as soon as the conversation is in; "Tekrar kontrol et" refreshes it.
    const arrived = page.getByText("İlk konuşmanız geldi.");
    const recheck = page.getByRole("button", { name: "Tekrar kontrol et" });
    await expect(arrived.or(recheck)).toBeVisible();
    if (await recheck.isVisible()) await recheck.click();
    await expect(arrived).toBeVisible();
    await page.getByRole("button", { name: "Kurulumu bitir" }).click();
    await page.waitForURL((url) => url.pathname === "/dashboard");

    // Org switcher: open a second organization, then switch back to the first.
    const switcher = page.getByRole("button", {
      name: "Organizasyon değiştir",
    });
    await switcher.click();
    await page.getByRole("menuitem", { name: "Yeni organizasyon aç" }).click();
    await page.locator("#org-name").fill(secondOrg);
    await page.getByRole("button", { name: "Organizasyonu aç" }).click();
    // A new organization continues in the AI-assisted set-up (CF-140).
    await page.waitForURL((url) => url.pathname === "/getting-started", {
      timeout: 30_000,
    });
    await expect.poll(() => activeSlug(page)).not.toBe(slug);

    await page.goto("/dashboard");
    await switcher.click();
    await expect(
      page.getByRole("menuitem", { name: new RegExp(secondOrg) }),
    ).toBeVisible();
    await page.getByRole("menuitem", { name: new RegExp(orgName) }).click();
    await expect.poll(() => activeSlug(page)).toBe(slug);
    await expect(switcher).toContainText(orgName);
  });
});
