import { DEMO_ORG_SLUG, OWNER_STATE, api, expect, test } from "./fixtures";

test.use({ storageState: OWNER_STATE });

type Guide = {
  key: string;
  progress: { status: string; step_index: number };
};

const guides = (request: Parameters<typeof api>[0]) =>
  api<{ items: Guide[] }>(
    request,
    "GET",
    "/tenant/guides?surface=web&route=/contacts&locale=tr",
  );

// Guide progress is saved with PUT .../progress, which the Go API answers with 204 No Content.
// The BFF used to turn that 204 into a 500 (production bug); fixed in PR #169, which this spec
// depends on.
test("guide progress is saved through the BFF (204 from the API)", async ({
  page,
}) => {
  const { items } = await guides(page.request);
  expect(items.length, "the contacts page has a guide").toBeGreaterThan(0);
  const key = items[0].key;
  const progress = async () =>
    (await guides(page.request)).items.find((g) => g.key === key)?.progress;

  const save = async (status: string, step: number) => {
    const res = await page.request.put(
      `/api/v1/tenant/guides/${encodeURIComponent(key)}/progress`,
      {
        headers: { "X-Organization": DEMO_ORG_SLUG },
        data: { status, step_index: step },
      },
    );
    expect(
      res.status(),
      `PUT progress -> ${res.status()} ${await res.text()}`,
    ).toBeGreaterThanOrEqual(200);
    expect(res.status()).toBeLessThan(300);
  };

  await save("in_progress", 1);
  await expect
    .poll(async () => await progress())
    .toMatchObject({ status: "in_progress", step_index: 1 });

  // Leave the tour dismissed so it does not open over later specs.
  await save("skipped", 0);
  await expect.poll(async () => (await progress())?.status).toBe("skipped");
});
