import { expect, type Page } from "@playwright/test";

import { type Composition, type Step, REVIEWER } from "./composition";

const signIn: Step = {
  caption: "A support admin signs in to the ClickFix panel (English interface)",
  run: async ({ page }) => {
    await page.goto("/login");
    await page.locator("#email").fill(REVIEWER.email);
    await page.locator("#password").fill(REVIEWER.password);
    await page.getByRole("button", { name: "Login", exact: true }).click();
    await page.waitForURL((url) => !url.pathname.startsWith("/login"));
  },
};

function openAddChannel(type: "WhatsApp" | "Instagram" | "Messenger"): Step {
  return {
    caption: `Channels → Add channel → ${type}`,
    run: async ({ page }) => {
      await page.goto("/channels");
      await page.getByRole("button", { name: "Add channel" }).first().click();
      await page
        .getByRole("dialog")
        .getByRole("button", { name: type, exact: true })
        .click();
    },
  };
}

async function openConversation(page: Page, contact: string) {
  await page.goto("/inbox");
  const row = page.getByRole("option").filter({ hasText: contact }).first();
  await expect(row).toBeVisible({ timeout: 15_000 });
  await row.click();
  await expect(
    page.getByRole("region", { name: "Conversation", exact: true }),
  ).toBeVisible();
}

async function reply(page: Page, text: string) {
  const box = page.getByRole("textbox", { name: "Reply" });
  await box.click();
  await box.pressSequentially(text, { delay: 25 });
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect(page.getByLabel("Messages").getByText(text)).toBeVisible();
}

const end: Step = { caption: "End of demonstration", hold: 3000 };

export const messenger: Composition = {
  id: "messenger",
  title:
    "ClickFix · Messenger: answering customers who write to the business's Facebook Page",
  permissions: [
    "pages_show_list",
    "pages_manage_metadata",
    "pages_messaging",
    "pages_read_engagement",
    "Human Agent",
  ],
  steps: [
    signIn,
    openAddChannel("Messenger"),
    {
      caption:
        "Connect with Meta opens Facebook Login, where the admin grants the Page permissions (Meta's dialog)",
      hold: 3500,
      run: async ({ page }) => {
        await page.getByRole("button", { name: "Connect with Meta" }).click();
        await page.waitForURL(/\/channels\/connect\/messenger/);
      },
    },
    {
      caption:
        "ClickFix lists the Pages this admin manages (pages_show_list); the admin picks one",
      hold: 3000,
      run: async ({ page }) => {
        await expect(page.getByText("Pick the page to connect:")).toBeVisible();
      },
    },
    {
      caption:
        "The Page is subscribed to our webhook (pages_manage_metadata); its name is read with pages_read_engagement",
      hold: 3500,
      run: async ({ page }) => {
        await page
          .getByRole("button", { name: "Northwind Coffee Machines" })
          .click();
        await page.waitForURL(/\/channels$/);
        await expect(
          page.getByText("Northwind Coffee Machines").first(),
        ).toBeVisible();
      },
    },
    {
      caption: "A customer sends a message to the Page on Messenger…",
      run: async ({ page, mock }) => {
        await mock.inbound({
          channel: "messenger",
          contact: "Olivia Bennett",
          content: "Hi! My espresso machine shows error E4. Can you help?",
        });
        await page.goto("/inbox");
      },
    },
    {
      caption:
        "…and it arrives in the ClickFix shared inbox through the Page webhook",
      hold: 3000,
      run: async ({ page }) => openConversation(page, "Olivia Bennett"),
    },
    {
      caption:
        "The agent replies; ClickFix sends it with the Messenger Send API (pages_messaging)",
      hold: 3500,
      run: async ({ page }) =>
        reply(
          page,
          "Hi Olivia, E4 means the water tank sensor. Please reseat the tank and restart the machine.",
        ),
    },
    {
      caption:
        "The customer answers the next day; the agent can still follow up within 7 days…",
      run: async ({ page, mock }) => {
        await mock.inbound({
          channel: "messenger",
          contact: "Olivia Bennett",
          content: "Still showing E4 after a restart.",
        });
        // Realtime is off in the recording, so the open thread picks the new message up on reload.
        await page.reload();
        await expect(
          page.getByLabel("Messages").getByText("Still showing E4"),
        ).toBeVisible({ timeout: 15_000 });
      },
    },
    {
      caption:
        "…a human-written reply outside 24 hours is sent once with the HUMAN_AGENT tag (Human Agent)",
      hold: 3500,
      run: async ({ page }) =>
        reply(
          page,
          "Thanks Olivia, a technician will call you today to book a free repair.",
        ),
    },
    end,
  ],
};

export const instagram: Composition = {
  id: "instagram",
  title:
    "ClickFix · Instagram: answering Direct messages and comments of the business account",
  permissions: [
    "instagram_business_basic",
    "instagram_business_manage_messages",
    "instagram_business_manage_comments",
  ],
  steps: [
    signIn,
    openAddChannel("Instagram"),
    {
      caption:
        "Connect with Meta opens Instagram Login, where the admin grants the requested permissions (Meta's screen)",
      hold: 3500,
      run: async ({ page }) => {
        await page.getByRole("button", { name: "Connect with Meta" }).click();
        await page.waitForURL(/\/channels(\/connect\/instagram)?/);
      },
    },
    {
      caption:
        "Back in ClickFix the account's user id and username are read (instagram_business_basic)",
      hold: 3500,
      run: async ({ page }) => {
        await page.waitForURL(/\/channels$/);
        await expect(page.getByText("northwind.coffee").first()).toBeVisible();
      },
    },
    {
      caption: "A customer sends a Direct message to the business account…",
      run: async ({ page, mock }) => {
        await mock.inbound({
          channel: "instagram",
          contact: "Emily Carter",
          content: "Do you have the Barista Pro in stock?",
        });
        await page.goto("/inbox");
      },
    },
    {
      caption:
        "…it arrives in the inbox and the agent answers it (instagram_business_manage_messages)",
      hold: 3500,
      run: async ({ page }) => {
        await openConversation(page, "Emily Carter");
        await reply(
          page,
          "Hi Emily, yes! The Barista Pro is in stock and ships tomorrow.",
        );
      },
    },
    {
      caption:
        "Someone comments on a post of the business; each comment opens its own conversation…",
      run: async ({ page, mock }) => {
        await mock.inbound({
          channel: "instagram",
          contact: "jordan.lee",
          subject: "Comment on your post",
          content: "Does this grinder work with decaf beans?",
          attributes: {
            source: "comment",
            thread: "comment:17890001",
            comment_id: "17890001",
            media_id: "17900001",
          },
        });
        await page.goto("/inbox");
      },
    },
    {
      caption:
        "…and the agent's answer is posted as a public reply under it (instagram_business_manage_comments)",
      hold: 3500,
      run: async ({ page }) => {
        await openConversation(page, "jordan.lee");
        await reply(
          page,
          "Yes Jordan, it works with any whole beans, decaf included.",
        );
      },
    },
    end,
  ],
};

/** Stands in for Meta's JS SDK: FB.login opens the facebook.com page below, which posts the FINISH event. */
const FB_SDK_STUB = `
window.FB = {
  init: function () {},
  login: function (cb) {
    window.open("https://www.facebook.com/review/embedded-signup", "es", "width=480,height=640");
    setTimeout(function () { cb({ authResponse: { code: "review-code" } }); }, 2500);
  },
};
if (window.fbAsyncInit) window.fbAsyncInit();
`;
const ES_FINISH_PAGE = `<!doctype html><title>Embedded Signup</title><script>
window.opener && window.opener.postMessage(JSON.stringify({ type: "WA_EMBEDDED_SIGNUP", event: "FINISH",
  data: { waba_id: "201234567890001", phone_number_id: "109876543210001" } }), "*");
setTimeout(function () { window.close(); }, 800);
</script>`;

export const whatsapp: Composition = {
  id: "whatsapp",
  title:
    "ClickFix · WhatsApp: connecting a business number with Embedded Signup and answering customers",
  permissions: [
    "whatsapp_business_management",
    "whatsapp_business_messaging",
    "business_management",
  ],
  steps: [
    signIn,
    openAddChannel("WhatsApp"),
    {
      caption:
        "Embedded Signup: the business picks its WhatsApp account and number in Meta's window (business_management)",
      hold: 3500,
      run: async ({ page }) => {
        await page.route("https://connect.facebook.net/**", (route) =>
          route.fulfill({
            contentType: "application/javascript",
            body: FB_SDK_STUB,
          }),
        );
        await page
          .context()
          .route("https://www.facebook.com/review/**", (route) =>
            route.fulfill({ contentType: "text/html", body: ES_FINISH_PAGE }),
          );
        await page
          .getByRole("button", { name: "Connect WhatsApp with Meta" })
          .click();
      },
    },
    {
      caption:
        "ClickFix subscribes to the account's webhooks and registers the number (whatsapp_business_management)",
      hold: 3500,
      run: async ({ page }) => {
        await expect(page.getByText("+44 20 7946 0958").first()).toBeVisible({
          timeout: 20_000,
        });
      },
    },
    {
      caption:
        "Templates: the business's message templates and their Meta review status",
      hold: 3000,
      run: async ({ page }) => {
        await page.keyboard.press("Escape");
        await page
          .getByRole("button", { name: "Channel actions" })
          .first()
          .click();
        await page.getByRole("menuitem", { name: "Templates" }).click();
        await expect(page.getByText("order_shipped")).toBeVisible();
      },
    },
    {
      caption:
        "A new template is written and submitted to Meta for approval (whatsapp_business_management)",
      hold: 3500,
      run: async ({ page }) => {
        await page.locator("#tpl-name").fill("appointment_reminder");
        await page
          .locator("#tpl-body")
          .fill("Hi {{1}}, your service appointment is on {{2}}.");
        // Meta needs an example for every variable before the template can be submitted.
        await page.locator("#tpl-ex-0").fill("Emma");
        await page.locator("#tpl-ex-1").fill("12 November at 10:00");
        await page.getByRole("button", { name: "Submit for approval" }).click();
        await expect(
          page.getByText("appointment_reminder").first(),
        ).toBeVisible();
        await page.keyboard.press("Escape");
      },
    },
    {
      caption: "A customer writes to the business number on WhatsApp…",
      run: async ({ page, mock }) => {
        await mock.inbound({
          channel: "whatsapp",
          contact: "Emma Clarke",
          content: "Hello, when will my repaired grinder be ready?",
        });
        await page.goto("/inbox");
      },
    },
    {
      caption:
        "…it arrives in the inbox and the agent replies with the Cloud API (whatsapp_business_messaging)",
      hold: 4000,
      run: async ({ page }) => {
        await openConversation(page, "Emma Clarke");
        await reply(
          page,
          "Hi Emma, your grinder is ready. You can pick it up any time after 2 pm today.",
        );
      },
    },
    end,
  ],
};

export const compositions: Composition[] = [messenger, instagram, whatsapp];
