import type { Page } from "@playwright/test";

import { BASE_URL } from "./playwright.config";
import { OWNER_STATE, api, expect, runId, test } from "./fixtures";

test.use({ storageState: OWNER_STATE });

/** Optional: E2E_SCREENSHOTS=<dir> keeps screenshots of the editor (PR material). */
const SHOTS = process.env.E2E_SCREENSHOTS ?? "";
const shot = async (page: Page, name: string) => {
  if (SHOTS) await page.screenshot({ path: `${SHOTS}/${name}.png` });
};

type Widget = { uuid: string; name: string };

// CF-150: the widget editor (draft autosave, live preview, publish, history,
// install) and the published settings reaching an embedded widget.
test("widget editor publishes settings that the embedded widget uses", async ({
  page,
}) => {
  const name = `E2E widget ${runId()}`;
  const created = await api<Widget>(page.request, "POST", "/tenant/channels", {
    type: "widget",
    name,
    settings: {},
  });

  await page.goto("/widget");
  await page.getByRole("tab", { name }).click();
  await expect(page.getByTestId("section-appearance")).toBeVisible();

  // The live preview runs the real SDK in preview mode.
  const preview = page.frameLocator('[data-testid="widget-preview-frame"]');
  await expect(preview.locator("#clickfix-widget")).toBeAttached();

  // A design change autosaves the draft.
  await page.getByRole("button", { name: "#059669" }).click();
  await expect(page.getByTestId("save-status")).toContainText(
    "Taslak kaydedildi",
  );
  await expect(page.getByTestId("publish-state")).toContainText(
    "Yayınlanmamış",
  );
  await shot(page, "editor-appearance");

  // Section search finds the channels section; a WhatsApp number is a channel link.
  await page.getByPlaceholder("Ayarlarda ara").fill("whatsapp");
  await page.getByRole("button", { name: "Kanallar" }).click();
  await page.getByLabel("WhatsApp").fill("+90 555 111 22 33");
  await expect(page.getByTestId("save-status")).toContainText(
    "Taslak kaydedildi",
  );
  await page.getByPlaceholder("Ayarlarda ara").fill("");

  // A proactive message with a page rule.
  await page.getByRole("button", { name: "Proaktif mesajlar" }).click();
  await page.getByRole("button", { name: "Proaktif mesaj ekle" }).click();
  await page
    .getByLabel("Mesaj (TR)")
    .fill("Fiyatlar hakkında yardım ister misiniz?");
  await page.getByLabel("Mesaj (EN)").fill("Need help with pricing?");
  await shot(page, "editor-triggers");

  // Publish as v1.
  await page.getByRole("button", { name: "Yayınla", exact: true }).click();
  await page.getByLabel("Neler değişti? (isteğe bağlı)").fill("E2E ilk yayın");
  await page.getByRole("button", { name: "Şimdi yayınla" }).click();
  await expect(page.getByTestId("publish-state")).toContainText("Canlı: v1");

  // History lists the version.
  await page.getByRole("button", { name: "Geçmiş" }).click();
  await expect(page.getByText("E2E ilk yayın")).toBeVisible();
  await shot(page, "editor-history");
  await page.keyboard.press("Escape");

  // Install snippets carry the public id.
  await page.getByRole("button", { name: "Kurulum" }).click();
  await expect(page.getByTestId("snippet-html")).toContainText(created.uuid);
  await shot(page, "editor-install");

  // The published settings reach a widget embedded on a page. The fake page
  // lives on the app origin: browsers block a public origin from loading a
  // loopback stack (private network access), and the app origin is always
  // allowed by the widget API.
  await page.route(`${BASE_URL}/__e2e/shop/**`, (route) =>
    route.fulfill({
      contentType: "text/html",
      body: `<!doctype html><html><body><h1>Shop</h1><script src="${BASE_URL}/widget.js" data-widget="${created.uuid}" async></script></body></html>`,
    }),
  );
  await page.goto(`${BASE_URL}/__e2e/shop/pricing`);
  const host = page.locator("#clickfix-widget");
  await expect(host).toBeAttached({ timeout: 30_000 });
  await expect
    .poll(() =>
      page.evaluate(() => {
        const root = document
          .querySelector("#clickfix-widget")
          ?.shadowRoot?.querySelector(".root") as HTMLElement | null;
        return root?.style.getPropertyValue("--cf-accent") ?? "";
      }),
    )
    .toBe("#059669");
  // The trigger has no page rule, so it fires after its 10 s time on page.
  await expect(host.locator(".proactive")).toContainText("Fiyatlar", {
    timeout: 30_000,
  });
  await host.locator(".launcher").click();
  await expect(host.locator(".panel")).toBeVisible();
  await expect(
    host.locator('a[href="https://wa.me/905551112233"]'),
  ).toBeVisible();
  const state = await page.evaluate(() =>
    (
      window as unknown as {
        ClickFix: { getState(): { open: boolean; visible: boolean } };
      }
    ).ClickFix.getState(),
  );
  expect(state.open).toBe(true);
  expect(state.visible).toBe(true);
  await shot(page, "embed-open");
});
