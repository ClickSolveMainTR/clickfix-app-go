import { ADMIN_STATE, DEMO_ORG_NAME, expect, test } from "./fixtures";

test.use({ storageState: ADMIN_STATE });

test("platform admin sees organizations, users and system health", async ({
  page,
}) => {
  await page.goto("/platform/organizations");
  await expect(
    page.getByRole("heading", { name: "Organizasyonlar" }),
  ).toBeVisible();
  await expect(page.getByText(DEMO_ORG_NAME).first()).toBeVisible();

  await page.goto("/platform/users");
  await expect(
    page.getByRole("heading", { name: "Kullanıcılar" }),
  ).toBeVisible();

  await page.goto("/platform/health");
  await expect(
    page.getByRole("heading", { name: "Sistem sağlığı" }),
  ).toBeVisible();
});
