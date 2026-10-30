import { expect, test as setup } from "@playwright/test";

import {
  ADMIN_EMAIL,
  ADMIN_PASSWORD,
  ADMIN_STATE,
  DEMO_ORG_SLUG,
  DEMO_OWNER,
  DEMO_PASSWORD,
  OWNER_STATE,
  api,
  signIn,
} from "./fixtures";

setup("sign in as the demo owner", async ({ page }) => {
  expect(
    DEMO_PASSWORD,
    "E2E_DEMO_PASSWORD is required (the DEMO_PASSWORD given to seed-demo)",
  ).not.toBe("");
  await signIn(page, DEMO_OWNER, DEMO_PASSWORD);
  await expect(
    page.getByRole("button", { name: "Organizasyon değiştir" }),
  ).toBeVisible();
  await page.evaluate((slug) => {
    // Pin the demo organization for the later specs (same key the org switcher writes).
    const raw = localStorage.getItem("clickfix.active-organization");
    if (!raw?.includes(slug)) {
      localStorage.setItem(
        "clickfix.active-organization",
        JSON.stringify({ state: { slug }, version: 0 }),
      );
    }
  }, DEMO_ORG_SLUG);
  // Module guide tours open on first visits and cover the page; the specs do not need them.
  // The API answers 204, which the BFF reports as 500 until PR #169, so only the result counts.
  await page.request.put("/api/v1/tenant/guides/preferences", {
    headers: { "X-Organization": DEMO_ORG_SLUG },
    data: { auto_start: false },
  });
  await expect
    .poll(
      async () =>
        (
          await api<{ auto_start: boolean }>(
            page.request,
            "GET",
            "/tenant/guides/preferences",
          )
        ).auto_start,
    )
    .toBe(false);
  await page.context().storageState({ path: OWNER_STATE });
});

setup("sign in as the platform admin", async ({ page }) => {
  expect(
    ADMIN_EMAIL,
    "E2E_ADMIN_EMAIL / E2E_ADMIN_PASSWORD are required",
  ).not.toBe("");
  await signIn(page, ADMIN_EMAIL, ADMIN_PASSWORD);
  await expect(page).toHaveURL(/\/platform/);
  // Self-serve sign up is off on a fresh installation; the sign-up spec needs it on.
  await api(
    page.request,
    "PATCH",
    "/platform/auth/settings",
    { registration_enabled: true, password_register_enabled: true },
    null,
  );
  await page.context().storageState({ path: ADMIN_STATE });
});
