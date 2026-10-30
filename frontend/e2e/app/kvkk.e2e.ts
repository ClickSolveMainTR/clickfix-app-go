import { OWNER_STATE, api, expect, test } from "./fixtures";

test.use({ storageState: OWNER_STATE });

type DataRequest = {
  uuid: string;
  kind: string;
  status: string;
  download_url?: string;
};

// KVKK data subject export has no screen yet; the request goes through the tenant API like the
// mobile and integrations clients use it.
test("a KVKK export request completes with a downloadable file", async ({
  page,
}) => {
  const { items: contacts } = await api<{ items: { uuid: string }[] }>(
    page.request,
    "GET",
    "/tenant/contacts?q=kuzeylojistik&limit=5",
  );
  expect(contacts.length).toBeGreaterThan(0);

  const created = await api<DataRequest>(
    page.request,
    "POST",
    `/tenant/compliance/contacts/${contacts[0].uuid}/requests`,
    { kind: "export" },
  );
  expect(created.kind).toBe("export");

  let done: DataRequest = created;
  await expect
    .poll(
      async () => {
        done = await api<DataRequest>(
          page.request,
          "GET",
          `/tenant/compliance/requests/${created.uuid}`,
        );
        return done.status;
      },
      { timeout: 60_000 },
    )
    .toBe("completed");

  expect(done.download_url).toBeTruthy();
  const file = await page.request.get(done.download_url!, {
    headers: { "X-Organization": "demo-anadolu-yazilim" },
  });
  expect(file.ok()).toBeTruthy();
});
