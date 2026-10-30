import { mkdir, writeFile } from "node:fs/promises";
import { join } from "node:path";

import { expect, test } from "@playwright/test";

import { DEFAULT_HOLD, estimateSeconds, lint, toVtt } from "./composition";
import { compositions } from "./compositions";

// No page fixture is used, so Playwright starts no browser.
for (const composition of compositions) {
  test(`${composition.id}: captions, permissions, length`, async ({}, testInfo) => {
    expect(lint(composition)).toEqual([]);

    // Caption timeline as the recording would show it, assuming every action takes 2s.
    let at = 0;
    const cues = [{ at, text: composition.title }];
    at += 4000;
    for (const step of composition.steps) {
      cues.push({ at, text: step.caption });
      at += (step.run ? 2000 : 0) + (step.hold ?? DEFAULT_HOLD);
    }
    await mkdir(testInfo.outputDir, { recursive: true });
    await writeFile(
      join(testInfo.outputDir, `${composition.id}.preview.vtt`),
      toVtt(cues, at),
    );
    console.log(
      `${composition.id}: ${composition.steps.length} steps, ~${estimateSeconds(composition)}s, permissions ${composition.permissions.join(", ")}`,
    );
  });
}
