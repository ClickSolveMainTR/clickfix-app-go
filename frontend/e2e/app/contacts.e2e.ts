import { OWNER_STATE, api, expect, runId, test } from "./fixtures";

test.use({ storageState: OWNER_STATE });

test("contacts: the list loads, a new contact is created and opened from search", async ({
  page,
}) => {
  const id = runId();
  const name = `E2E Kişi ${id}`;
  const email = `kisi-${id}@example.com`;

  await page.goto("/contacts");
  await expect(
    page.getByRole("heading", { name: "Kişiler", exact: true }),
  ).toBeVisible();
  // Seeded demo contacts fill the list.
  await expect(page.locator("tbody tr").first()).toBeVisible();
  await expect(page.getByText("Henüz kişi yok")).toHaveCount(0);

  // Create: the new contact opens in the side sheet.
  await page.getByRole("button", { name: "Yeni kişi" }).click();
  const dialog = page.getByRole("dialog", { name: "Yeni kişi" });
  await dialog.getByLabel("Ad soyad").fill(name);
  await dialog.getByLabel("E-posta").fill(email);
  await dialog.getByRole("button", { name: "Oluştur" }).click();
  await expect(dialog).toBeHidden();
  const sheet = page.getByRole("dialog", { name });
  await expect(sheet).toBeVisible();
  await expect(sheet.getByLabel("E-posta")).toHaveValue(email);
  await page.keyboard.press("Escape");
  await expect(sheet).toBeHidden();

  // Search finds it; a row click opens the same record.
  await page.getByPlaceholder("Ad, e-posta veya telefon ara…").fill(email);
  const row = page.locator("tbody tr", { hasText: name });
  await expect(row).toHaveCount(1);
  await row.click();
  await expect(sheet).toBeVisible();
  await expect(sheet.getByLabel("Ad soyad")).toHaveValue(name);

  // The API has the same record.
  const { items } = await api<{ items: { email: string; name: string }[] }>(
    page.request,
    "GET",
    `/tenant/contacts?q=${encodeURIComponent(email)}&limit=5`,
  );
  expect(items.map((c) => c.name)).toContain(name);
});
