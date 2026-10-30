import {
  OWNER_STATE,
  api,
  expect,
  runId,
  test,
  widgetConversation,
  widgetPublicId,
} from "./fixtures";

test.use({ storageState: OWNER_STATE });

type WidgetMessage = {
  direction: string;
  sender_type: string;
  content: string;
};

test.describe("inbox", () => {
  test("widget conversation → inbox → assign → reply → resolve", async ({
    page,
  }) => {
    const id = runId();
    const question = `E2E: faturamı nasıl indiririm? ${id}`;
    const answer = `Faturalandırma > Faturalar ekranından PDF indirebilirsiniz. ${id}`;

    // A visitor writes from the website widget (public widget API through the BFF relay).
    const widget = await widgetPublicId(page.request);
    const boot = await api<{ visitor: { token: string } }>(
      page.request,
      "POST",
      `/widget/${widget}/bootstrap`,
      {},
      null,
    );
    const visitor = { Authorization: `Bearer ${boot.visitor.token}` };
    const created = await page.request.post(
      `/api/v1/widget/${widget}/conversations`,
      {
        headers: visitor,
        data: {
          text: question,
          name: "Widget Ziyaretçisi",
          email: `ziyaretci-${id}@example.com`,
        },
      },
    );
    expect(created.ok(), await created.text()).toBeTruthy();
    const conversation = (
      (await created.json()) as { data: { conversation_uuid: string } }
    ).data.conversation_uuid;

    // The conversation shows up in the inbox.
    await page.goto("/inbox?view=all&status=open");
    const item = page.locator(`#conv-${conversation}`);
    await expect(item).toBeVisible({ timeout: 30_000 });
    await item.click();
    const pane = page.getByRole("region", { name: "Konuşma", exact: true });
    await expect(pane.getByText(question)).toBeVisible();

    // Assign to me.
    await pane.getByRole("button", { name: /^Ata\b/ }).click();
    await page.getByRole("option", { name: /\(siz\)/ }).click();
    await expect(
      pane.getByRole("button", { name: /Selin Arslan/ }),
    ).toBeVisible();

    // Reply.
    await pane.getByPlaceholder("Yanıtınızı yazın…").fill(answer);
    await pane.getByRole("button", { name: /^Gönder/ }).click();
    await expect(pane.getByText(answer)).toBeVisible();

    // The visitor sees the agent's answer in the widget.
    await expect
      .poll(async () => {
        const res = await page.request.get(
          `/api/v1/widget/${widget}/conversations/${conversation}/messages`,
          { headers: visitor },
        );
        const body = (await res.json()) as { data: { items: WidgetMessage[] } };
        return body.data.items.some(
          (m) => m.direction === "out" && m.content === answer,
        );
      })
      .toBe(true);

    // Resolve.
    await pane.getByRole("button", { name: /^Çöz/ }).click();
    await expect(
      pane.getByRole("button", { name: /^Yeniden aç/ }),
    ).toBeVisible();
  });

  test("a macro replies and sets the status", async ({ page }) => {
    await page.goto("/inbox?view=all&status=open");
    const first = page
      .getByRole("listbox", { name: "Konuşmalar" })
      .getByRole("option")
      .first();
    await first.click();
    const pane = page.getByRole("region", { name: "Konuşma", exact: true });
    await pane.getByRole("button", { name: /^Makrolar/ }).click();
    await page.getByPlaceholder("Makro ara…").fill("Bilgi bekleniyor");
    await page.getByRole("option", { name: /Bilgi bekleniyor/ }).click();
    await expect(
      pane
        .getByText("konuyu inceleyebilmemiz için ekran görüntüsü", {
          exact: false,
        })
        .last(),
    ).toBeVisible();
  });

  test("a filtered inbox is saved as a view", async ({ page }) => {
    const name = `E2E acil ${runId()}`;
    await page.goto("/inbox?view=all&status=open&pr=urgent");
    // With an active filter the filter bar overflows into the empty conversation pane, which then
    // swallows pointer clicks on "Görünümler"; open it from the keyboard instead.
    const views = page.getByRole("button", { name: /Görünümler/ });
    await views.focus();
    await page.keyboard.press("Enter");
    await page.getByRole("option", { name: /Bu görünümü kaydet/ }).click();
    await page.getByPlaceholder("Görünüm adı").fill(name);
    await page.getByRole("button", { name: "Kaydet", exact: true }).click();
    await expect(page).toHaveURL(/[?&]sv=/);
    await expect(
      page.getByRole("button", { name: new RegExp(name) }),
    ).toBeVisible();
  });

  test("bulk actions resolve the selected conversations and undo restores them", async ({
    page,
  }) => {
    const id = runId();
    const visitors = [`E2E Toplu A ${id}`, `E2E Toplu B ${id}`];
    const conversations: string[] = [];
    for (const [i, name] of visitors.entries()) {
      const { conversation } = await widgetConversation(page.request, {
        text: `E2E toplu işlem ${i} ${id}`,
        name,
        email: `toplu-${i}-${id}@example.com`,
      });
      conversations.push(conversation);
    }
    const status = async (uuid: string) =>
      (
        await api<{ status: string }>(
          page.request,
          "GET",
          `/tenant/conversations/${uuid}`,
        )
      ).status;

    await page.goto("/inbox?view=all&status=open");
    for (const uuid of conversations) {
      await expect(page.locator(`#conv-${uuid}`)).toBeVisible({
        timeout: 30_000,
      });
    }
    for (const name of visitors) {
      await page.getByRole("checkbox", { name: `Seç: ${name}` }).check();
    }
    const bar = page.getByRole("toolbar", { name: "Toplu işlemler" });
    await expect(bar).toContainText("2 seçili");
    await bar.getByRole("button", { name: "Çöz" }).click();

    const toast = page
      .locator("[data-sonner-toast]")
      .filter({ hasText: "2 konuşma güncellendi" });
    await expect(toast).toBeVisible();
    await expect(bar).toBeHidden();
    for (const uuid of conversations) {
      await expect.poll(() => status(uuid)).toBe("resolved");
    }

    // Undo from the toast puts both back.
    await toast.getByRole("button", { name: "Geri al" }).click();
    await expect(
      page
        .locator("[data-sonner-toast]")
        .filter({ hasText: "2 konuşmadaki değişiklik geri alındı" }),
    ).toBeVisible();
    for (const uuid of conversations) {
      await expect.poll(() => status(uuid)).toBe("open");
    }
  });
});
