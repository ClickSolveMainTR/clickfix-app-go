import { mkdir, writeFile } from "node:fs/promises";
import { join } from "node:path";

import { expect, test } from "@playwright/test";

import { type Ctx, DEFAULT_HOLD, toVtt } from "./composition";
import { compositions } from "./compositions";
import { MOCK_URL } from "./playwright.config";

const CAPTION_ID = "meta-review-caption";

/** English UI from the first paint, the demo organization selected, and a caption strip the steps update. */
const INIT = `
  localStorage.setItem("app.locale", "en");
  localStorage.setItem("clickfix.active-organization", JSON.stringify({ state: { slug: "northwind" }, version: 0 }));
  window.__reviewCaption = function (text) {
    var el = document.getElementById(${JSON.stringify(CAPTION_ID)});
    if (!el) {
      el = document.createElement("div");
      el.id = ${JSON.stringify(CAPTION_ID)};
      el.setAttribute("aria-hidden", "true");
      el.style.cssText = "position:fixed;left:0;right:0;top:0;z-index:2147483647;background:rgba(15,23,42,.92);color:#fff;font:600 17px/1.45 system-ui,-apple-system,Segoe UI,sans-serif;padding:12px 24px;text-align:center;pointer-events:none";
      (document.body || document.documentElement).appendChild(el);
    }
    el.textContent = text;
  };
`;

async function api<T>(path: string, body?: unknown): Promise<T> {
  const res = await fetch(`${MOCK_URL}${path}`, {
    method: body === undefined ? "GET" : "POST",
    headers: { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const json = (await res.json()) as {
    success: boolean;
    data: T;
    error?: { message: string };
  };
  if (!json.success)
    throw new Error(json.error?.message ?? `mock ${path} failed`);
  return json.data;
}

const only = (process.env.REVIEW_ONLY ?? "").trim();

for (const composition of compositions) {
  test(composition.id, async ({ page }, testInfo) => {
    test.skip(Boolean(only) && only !== composition.id, `REVIEW_ONLY=${only}`);
    const started = Date.now();
    await api("/__review/reset", {});
    await page.addInitScript(INIT);

    const cues: Array<{ at: number; text: string }> = [];
    let current = "";
    const show = async (text: string) => {
      current = text;
      cues.push({ at: Date.now() - started, text });
      // The strip lives in the page; re-apply after navigations.
      await page
        .evaluate(
          (value) =>
            (
              window as unknown as { __reviewCaption?: (t: string) => void }
            ).__reviewCaption?.(value),
          text,
        )
        .catch(() => undefined);
    };
    page.on("load", () => {
      if (current)
        void page
          .evaluate(
            (value) =>
              (
                window as unknown as { __reviewCaption?: (t: string) => void }
              ).__reviewCaption?.(value),
            current,
          )
          .catch(() => undefined);
    });

    const ctx: Ctx = {
      page,
      mock: {
        inbound: (message) =>
          api<{ uuid: string }>("/__review/inbound", message),
        sent: () => api("/__review/sent"),
      },
    };

    await page.goto("/login");
    await show(composition.title);
    await page.waitForTimeout(4000);

    for (const step of composition.steps) {
      await show(step.caption);
      if (step.run) await step.run(ctx);
      await show(step.caption);
      await page.waitForTimeout(step.hold ?? DEFAULT_HOLD);
    }
    if (
      composition.steps.some(
        (s) => s.caption.includes("replies") || s.caption.includes("answer"),
      )
    ) {
      expect((await ctx.mock.sent()).length).toBeGreaterThan(0);
    }

    // Collapse the repeated cue each step writes after its action.
    const merged = cues.filter(
      (cue, i) => i === 0 || cue.text !== cues[i - 1].text,
    );
    await mkdir(testInfo.outputDir, { recursive: true });
    await writeFile(
      join(testInfo.outputDir, "captions.vtt"),
      toVtt(merged, Date.now() - started),
    );
    await writeFile(
      join(testInfo.outputDir, "meta-review.json"),
      JSON.stringify(
        {
          id: composition.id,
          title: composition.title,
          permissions: composition.permissions,
        },
        null,
        2,
      ),
    );
  });
}
