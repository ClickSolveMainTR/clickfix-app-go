import { DEMO_ORG_SLUG, expect, runId, test } from "./fixtures";

// The seeded "Kurulum oturumu" type: in person, public, no video room involved.
const TYPE = "kurulum";

type Slot = { starts_at: string };

const isoDay = (d: Date) => d.toISOString().slice(0, 10);

test("public booking page: a visitor books a slot and cancels it from the manage link", async ({
  page,
}) => {
  const id = runId();
  const name = `E2E Randevu ${id}`;

  // First day with a free slot, from the day after tomorrow on (no min-notice edge cases).
  let day = "";
  for (let i = 2; i < 16 && !day; i++) {
    const from = isoDay(new Date(Date.now() + i * 86_400_000));
    const to = isoDay(new Date(Date.now() + (i + 1) * 86_400_000));
    const res = await page.request.get(
      `/api/v1/public/booking/${DEMO_ORG_SLUG}/${TYPE}/slots?from=${from}&to=${to}`,
    );
    expect(res.ok(), await res.text()).toBeTruthy();
    const { items } = ((await res.json()) as { data: { items: Slot[] } }).data;
    if (items.length > 0) day = from;
  }
  expect(day, "a free slot within two weeks").not.toBe("");

  // The page opens on "today" in the browser; pin the browser clock to that day (09:00 local).
  await page.clock.setFixedTime(new Date(`${day}T06:00:00Z`));

  await page.goto(`/book/${DEMO_ORG_SLUG}`);
  await expect(page.getByText("Görüşme planlayın")).toBeVisible();
  await page.getByRole("link", { name: /Kurulum oturumu/ }).click();
  await expect(page).toHaveURL(new RegExp(`/book/${DEMO_ORG_SLUG}/${TYPE}$`));

  const times = page.getByRole("listbox", { name: "Uygun saatler" });
  await expect(times).toBeVisible();
  await times.getByRole("option").first().click();
  await page.getByLabel("Adınız").fill(name);
  await page.getByLabel("E-posta adresiniz").fill(`randevu-${id}@example.com`);
  await page.getByRole("button", { name: "Randevuyu onayla" }).click();

  await expect(
    page.getByRole("heading", {
      name: /Randevunuz oluşturuldu|Talebiniz alındı/,
    }),
  ).toBeVisible();
  await expect(page.getByText("Kurulum oturumu")).toBeVisible();

  // Manage link: cancel.
  await page
    .getByRole("link", { name: "Yeniden planla veya iptal et" })
    .click();
  await expect(page).toHaveURL(/\/book\/manage\//);
  await expect(
    page.getByRole("heading", { name: "Randevunuz", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Randevuyu iptal et" }).click();
  await page.getByLabel("Neden (isteğe bağlı)").fill("E2E iptal");
  await page.getByRole("button", { name: "Randevuyu iptal et" }).last().click();
  await expect(
    page.getByRole("heading", { name: "Randevu iptal edildi" }),
  ).toBeVisible();
});
