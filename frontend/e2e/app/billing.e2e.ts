import { expect, test } from "@playwright/test";

import { OWNER_STATE } from "./fixtures";

test.use({ storageState: OWNER_STATE });

// Read-only: a trialing organization sees its plan, limits and the pay-now path. No invoice is
// created, so the run never talks to a payment provider.
test("the billing page shows the trial plan and the public price list", async ({
  page,
}) => {
  await page.goto("/billing");
  await expect(
    page.getByRole("heading", { name: "Faturalama ve plan" }),
  ).toBeVisible();
  await expect(page.getByText("Mevcut plan")).toBeVisible();
  await expect(page.getByText("Deneme", { exact: true }).first()).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Şimdi ücretli plana geç" }),
  ).toBeEnabled();
  await expect(page.getByText("Kullanım ve limitler")).toBeVisible();

  const pricing = await page.request.get("/api/v1/public/pricing");
  expect(pricing.ok()).toBeTruthy();
  const body = (await pricing.json()) as { data: unknown };
  expect(JSON.stringify(body.data)).toContain("monthly");
});
