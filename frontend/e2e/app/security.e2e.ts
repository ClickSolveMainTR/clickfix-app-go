import { API_URL, OWNER_STATE, api, expect, runId, test } from "./fixtures";

test.use({ storageState: OWNER_STATE });

test("settings/security: a SCIM token is generated and shown once", async ({
  page,
  playwright,
}) => {
  // SCIM rides on the organization's SSO record; a saved (disabled) OIDC config is enough.
  const sso = await api<{ enabled: boolean }>(
    page.request,
    "GET",
    "/tenant/security/sso",
  );
  if (!sso.enabled) {
    await api(page.request, "PUT", "/tenant/security/sso", {
      protocol: "oidc",
      enabled: false,
      enforced: false,
      ip_allowlist: [],
    });
  }

  await page.goto("/settings/security");
  await expect(page.getByText("SCIM ile kullanıcı yönetimi")).toBeVisible();
  await page.getByRole("button", { name: "Token oluştur / yenile" }).click();
  await page
    .getByRole("dialog", { name: "SCIM token yenilensin mi?" })
    .getByRole("button", { name: "Yenile" })
    .click();

  const issued = page.getByRole("dialog", { name: "Yeni SCIM token" });
  await expect(issued).toBeVisible();
  await expect(
    issued.getByText("Token yalnızca bir kez gösterilir"),
  ).toBeVisible();
  const fields = issued.getByRole("textbox");
  const token = await fields.nth(0).inputValue();
  const base = await fields.nth(1).inputValue();
  expect(token.length).toBeGreaterThanOrEqual(20);
  expect(base).toMatch(/\/scim\/[0-9a-f-]{36}\/v2$/);

  // The identity provider authenticates with the token (no session).
  const idp = await playwright.request.newContext();
  const users = await idp.get(`${base}/Users`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(users.status(), await users.text()).toBe(200);
  const wrong = await idp.get(`${base}/Users`, {
    headers: { Authorization: "Bearer not-the-token" },
  });
  expect(wrong.status()).toBe(401);
  await idp.dispose();

  // Closed once, the token is gone from the page.
  await issued.getByRole("button", { name: "Kopyaladım, kapat" }).click();
  await expect(issued).toBeHidden();
  await expect(page.locator(`input[value="${token}"]`)).toHaveCount(0);
});

test("settings/integrations: an API key is created, used once and revoked", async ({
  page,
  playwright,
}) => {
  const name = `E2E anahtar ${runId()}`;

  await page.goto("/settings/integrations");
  await page.getByRole("tab", { name: "API anahtarları" }).click();
  await page.getByRole("button", { name: "Yeni API anahtarı" }).click();
  const form = page.getByRole("dialog", { name: "Yeni API anahtarı" });
  await form.getByLabel("Ad", { exact: true }).fill(name);
  await form.getByRole("checkbox", { name: /contacts:read/ }).check();
  await form.getByRole("button", { name: "Anahtar oluştur" }).click();

  const secret = page.getByRole("dialog", { name: "API anahtarınız" });
  await expect(
    secret.getByText("Bu anahtar yalnızca bir kez gösterilir"),
  ).toBeVisible();
  const key = await secret.getByLabel("Anahtar", { exact: true }).inputValue();
  expect(key.length).toBeGreaterThanOrEqual(20);
  await secret.getByRole("button", { name: "Kopyaladım" }).click();
  await expect(secret).toBeHidden();

  const row = page.getByRole("listitem").filter({ hasText: name });
  await expect(row).toContainText("Etkin");
  await expect(row).toContainText("contacts:read");

  // Integrations call the API host directly with the key.
  const client = await playwright.request.newContext();
  const call = () =>
    client.get(`${API_URL}/v1/public-api/contacts?limit=1`, {
      headers: { Authorization: `Bearer ${key}` },
    });
  const ok = await call();
  expect(ok.status(), await ok.text()).toBe(200);
  // A scope the key does not have is refused.
  const missing = await client.get(`${API_URL}/v1/public-api/conversations`, {
    headers: { Authorization: `Bearer ${key}` },
  });
  expect(missing.status()).toBe(403);

  // Revoke.
  await row.getByRole("button", { name: "İptal et" }).click();
  await page
    .getByRole("dialog", { name: `"${name}" iptal edilsin mi?` })
    .getByRole("button", { name: "İptal et" })
    .click();
  await expect(row).toContainText("İptal edildi");
  await expect(row.getByRole("button", { name: "İptal et" })).toHaveCount(0);
  await expect.poll(async () => (await call()).status()).toBe(401);
  await client.dispose();
});
