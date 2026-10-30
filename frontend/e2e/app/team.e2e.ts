import { OWNER_STATE, expect, runId, test } from "./fixtures";

test.use({ storageState: OWNER_STATE });

test("the owner invites a teammate", async ({ page }) => {
  const email = `davet-${runId()}@example.com`;
  await page.goto("/settings/members?tab=invitations");
  await page.getByRole("button", { name: "Davet gönder" }).click();
  const drawer = page.getByRole("dialog", { name: "Üye davet et" });
  await drawer.locator("#email").fill(email);
  await drawer.getByRole("combobox").click();
  await page.getByRole("option", { name: /^Temsilci/ }).click();
  // Leave the role list with Tab: Escape would close the drawer too.
  await page.keyboard.press("Tab");
  await expect(page.getByRole("option", { name: /^Temsilci/ })).toBeHidden();
  await drawer.getByRole("button", { name: "Daveti gönder" }).click();
  await expect(
    page.getByText(`${email} adresine davet gönderildi.`),
  ).toBeVisible();
  await expect(page.getByRole("cell", { name: email })).toBeVisible();
});
