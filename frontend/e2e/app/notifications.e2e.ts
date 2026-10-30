import { OWNER_STATE, api, expect, test } from "./fixtures";

test.use({ storageState: OWNER_STATE });

type Notification = { uuid: string; title: string; read_at: string | null };

test("notifications: a new in-app notification shows in the bell and is marked read", async ({
  page,
}) => {
  const unread = async () =>
    (
      await api<{ count: number }>(
        page.request,
        "GET",
        "/notifications/unread-count",
        undefined,
        null,
      )
    ).count;

  await page.goto("/dashboard");
  const before = await unread();

  // The self-test sends one in-app notification to the signed-in user.
  await api(page.request, "POST", "/notifications/test", {}, null);
  await expect.poll(unread, { timeout: 30_000 }).toBeGreaterThan(before);

  const bell = page.getByRole("button", { name: /okunmamış bildirim$/ });
  await expect(bell).toBeVisible({ timeout: 30_000 });
  await bell.click();
  await expect(page.getByText("Test bildirimi").first()).toBeVisible();

  await page.getByRole("button", { name: "Tümünü okundu işaretle" }).click();
  await expect.poll(unread).toBe(0);
  await expect(
    page.getByRole("button", { name: "Bildirimler", exact: true }),
  ).toBeVisible();

  const { items } = await api<{ items: Notification[] }>(
    page.request,
    "GET",
    "/notifications?limit=5",
    undefined,
    null,
  );
  const latest = items.find((n) => n.title === "Test bildirimi");
  expect(latest?.read_at).toBeTruthy();
});
