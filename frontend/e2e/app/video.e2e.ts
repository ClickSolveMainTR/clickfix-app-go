import type { APIRequestContext, Browser, Page } from "@playwright/test";

import { ADMIN_STATE, OWNER_STATE, api, expect, runId, test } from "./fixtures";

test.use({ storageState: OWNER_STATE });

// Video rooms (CF-139) without a LiveKit server: rooms, links and the lobby live in the API; only
// an admitted participant gets a LiveKit token. These specs stop at the lobby, so they assert UI
// states and availability messages, never media.

type Policy = Record<string, unknown> & { enabled: boolean };
type Room = { uuid: string; name: string; pin_required: boolean };

/** Platform video policy (platform admin); the module is off on a fresh installation. */
async function setPlatformVideo(browser: Browser, enabled: boolean) {
  const admin = await browser.newContext({ storageState: ADMIN_STATE });
  const { policy } = await api<{ policy: Policy }>(
    admin.request,
    "GET",
    "/platform/video/settings",
    undefined,
    null,
  );
  if (policy.enabled !== enabled) {
    await api(
      admin.request,
      "PUT",
      "/platform/video/settings",
      { policy: { ...policy, enabled } },
      null,
    );
  }
  await admin.close();
}

/** Organization video settings: on, with an optional "PIN required" tightening. */
async function setOrgVideo(request: APIRequestContext, requirePin: boolean) {
  await api(request, "PUT", "/tenant/video/settings", {
    enabled: true,
    policy: requirePin ? { require_pin: true } : {},
    storage: { mode: "platform" },
  });
}

async function openCreateDialog(page: Page) {
  await page.goto("/video");
  const create = page.getByRole("button", { name: "Oda oluştur" }).first();
  await expect(create).toBeEnabled();
  await create.click();
  return page.getByRole("dialog", { name: "Oda oluştur" });
}

async function roomByName(request: APIRequestContext, name: string) {
  const { items } = await api<{ items: Room[] }>(
    request,
    "GET",
    "/tenant/video/rooms?limit=50",
  );
  const room = items.find((r) => r.name === name);
  expect(room, `room ${name}`).toBeTruthy();
  return room!;
}

async function guestLink(request: APIRequestContext, room: string) {
  const link = await api<{ code: string }>(
    request,
    "POST",
    `/tenant/video/rooms/${room}/links`,
    { role: "guest" },
  );
  return `/r/${link.code}`;
}

test.describe("video rooms", () => {
  test.afterAll(async ({ browser }) => {
    const owner = await browser.newContext({ storageState: OWNER_STATE });
    await setOrgVideo(owner.request, false);
    await owner.close();
  });

  test("the rooms page explains why video is unavailable", async ({
    page,
    browser,
  }) => {
    await setPlatformVideo(browser, false);
    await page.goto("/video");
    await expect(
      page.getByText(
        "Görüntülü görüşme platform yönetimi tarafından kapatıldı.",
      ),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Oda oluştur" }).first(),
    ).toBeDisabled();
  });

  test("a room without a PIN: the guest page asks only for a name and waits in the lobby", async ({
    page,
    browser,
  }) => {
    const name = `E2E oda ${runId()}`;
    await setPlatformVideo(browser, true);
    await setOrgVideo(page.request, false);

    const dialog = await openCreateDialog(page);
    await expect(
      dialog.getByLabel("PIN (isteğe bağlı, 4-12 hane)"),
    ).toBeVisible();
    await dialog.getByLabel("Oda adı").fill(name);
    await dialog.getByRole("button", { name: "Oda oluştur" }).click();
    await expect(dialog).toBeHidden();
    await expect(page.getByText(name)).toBeVisible();

    const room = await roomByName(page.request, name);
    expect(room.pin_required).toBe(false);
    const link = await guestLink(page.request, room.uuid);

    // The customer opens the link without an account.
    const guest = await browser.newContext({ locale: "tr-TR" });
    const join = await guest.newPage();
    await join.goto(link);
    await expect(join.getByRole("heading", { name })).toBeVisible();
    await expect(join.getByText("Demo · Anadolu Yazılım")).toBeVisible();
    await expect(
      join.getByText("görüşmeyi düzenleyen kişinin sizi içeri alması"),
    ).toBeVisible();
    await expect(join.getByLabel("PIN")).toHaveCount(0);
    const submit = join.getByRole("button", { name: "Görüşmeye katıl" });
    await expect(submit).toBeDisabled();
    await join.getByLabel("Adınız").fill("E2E Misafir");
    await submit.click();
    await expect(join.getByText("Bekleme odasındasınız.")).toBeVisible();
    await guest.close();

    // The host sees the guest waiting.
    await expect
      .poll(async () => {
        const { items } = await api<{
          items: { display_name: string; status: string }[];
        }>(
          page.request,
          "GET",
          `/tenant/video/rooms/${room.uuid}/participants`,
        );
        return items.map((p) => `${p.display_name}:${p.status}`);
      })
      .toContain("E2E Misafir:waiting");

    await api(page.request, "POST", `/tenant/video/rooms/${room.uuid}/end`, {});
  });

  test("with the PIN policy a room needs a PIN and the guest must enter it", async ({
    page,
    browser,
  }) => {
    const name = `E2E PIN oda ${runId()}`;
    const pin = "482615";
    await setPlatformVideo(browser, true);
    await setOrgVideo(page.request, true);

    // The API refuses a room without a PIN.
    const refused = await page.request.post("/api/v1/tenant/video/rooms", {
      headers: { "X-Organization": "demo-anadolu-yazilim" },
      data: { name: `${name} (PIN yok)` },
    });
    expect(refused.status()).toBe(422);
    expect(await refused.text()).toContain("VIDEO_PIN_REQUIRED");

    const dialog = await openCreateDialog(page);
    const pinField = dialog.getByLabel("PIN (zorunlu, 4-12 hane)");
    await expect(pinField).toBeVisible();
    await dialog.getByLabel("Oda adı").fill(name);
    const create = dialog.getByRole("button", { name: "Oda oluştur" });
    await expect(create).toBeDisabled();
    await pinField.fill(pin);
    await expect(create).toBeEnabled();
    await create.click();
    await expect(dialog).toBeHidden();

    const room = await roomByName(page.request, name);
    expect(room.pin_required).toBe(true);
    const link = await guestLink(page.request, room.uuid);

    const guest = await browser.newContext({ locale: "tr-TR" });
    const join = await guest.newPage();
    await join.goto(link);
    await expect(join.getByRole("heading", { name })).toBeVisible();
    await join.getByLabel("Adınız").fill("E2E PIN Misafiri");
    await join.getByLabel("PIN").fill("0000");
    await join.getByRole("button", { name: "Görüşmeye katıl" }).click();
    await expect(
      join.getByText("PIN hatalı. Kontrol edip tekrar deneyin."),
    ).toBeVisible();
    await join.getByLabel("PIN").fill(pin);
    await join.getByRole("button", { name: "Görüşmeye katıl" }).click();
    await expect(join.getByText("Bekleme odasındasınız.")).toBeVisible();
    await guest.close();

    await api(page.request, "POST", `/tenant/video/rooms/${room.uuid}/end`, {});
  });

  test("an ended room's link says the call is over", async ({
    page,
    browser,
  }) => {
    const name = `E2E biten oda ${runId()}`;
    await setPlatformVideo(browser, true);
    await setOrgVideo(page.request, false);
    const room = await api<Room>(page.request, "POST", "/tenant/video/rooms", {
      name,
    });
    const link = await guestLink(page.request, room.uuid);
    await api(page.request, "POST", `/tenant/video/rooms/${room.uuid}/end`, {});

    const guest = await browser.newContext({ locale: "tr-TR" });
    const join = await guest.newPage();
    await join.goto(link);
    await join.getByLabel("Adınız").fill("E2E Geç Kalan");
    await join.getByRole("button", { name: "Görüşmeye katıl" }).click();
    await expect(join.getByText("Bu görüşme sona erdi.")).toBeVisible();
    await guest.close();
  });
});
