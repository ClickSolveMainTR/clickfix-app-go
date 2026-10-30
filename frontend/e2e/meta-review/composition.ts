import type { Page } from "@playwright/test";

/**
 * A review video is a "composition": an ordered list of steps, each with the English caption shown
 * on screen (and written to the .vtt file) and an optional action in the panel. The dry run reads
 * the same list without a browser, so captions, timing and permission coverage are checked before
 * anything is recorded.
 */
export type UseCase = "messenger" | "instagram" | "whatsapp";

export type InboundMessage = {
  channel: UseCase;
  contact: string;
  content: string;
  subject?: string;
  attributes?: Record<string, unknown>;
};

export type Ctx = {
  page: Page;
  mock: {
    inbound(message: InboundMessage): Promise<{ uuid: string }>;
    sent(): Promise<Array<{ channel: string; content: string }>>;
  };
};

export type Step = {
  caption: string;
  /** How long the caption stays on screen after the action, in ms. */
  hold?: number;
  run?: (ctx: Ctx) => Promise<void>;
};

export type Composition = {
  id: UseCase;
  /** Title card, also the first caption. */
  title: string;
  /** Meta permissions and features this video demonstrates; each must be named in a caption. */
  permissions: string[];
  steps: Step[];
};

export const DEFAULT_HOLD = 2500;
/** Upper bound Meta is comfortable with; a longer video is cut by reviewers. */
export const MAX_SECONDS = 180;
export const MAX_CAPTION = 140;

export const REVIEWER = {
  email: "alex.morgan@northwind.example",
  // Accepted by the mock API only; never a real credential.
  password: "review-only-password",
};

function cueTime(ms: number): string {
  const h = Math.floor(ms / 3_600_000);
  const m = Math.floor((ms % 3_600_000) / 60_000);
  const s = Math.floor((ms % 60_000) / 1000);
  const rest = Math.floor(ms % 1000);
  const pad = (n: number, w = 2) => String(n).padStart(w, "0");
  return `${pad(h)}:${pad(m)}:${pad(s)}.${pad(rest, 3)}`;
}

/** WebVTT from caption start times (ms from the video start); each cue ends where the next begins. */
export function toVtt(
  cues: Array<{ at: number; text: string }>,
  endAt: number,
): string {
  const lines = ["WEBVTT", ""];
  cues.forEach((cue, i) => {
    const end = i + 1 < cues.length ? cues[i + 1].at : endAt;
    lines.push(
      String(i + 1),
      `${cueTime(cue.at)} --> ${cueTime(end)}`,
      cue.text,
      "",
    );
  });
  return lines.join("\n");
}

/** Problems that would make the composition unfit for review; empty when it is fine. */
export function lint(composition: Composition): string[] {
  const problems: string[] = [];
  const captions = [
    composition.title,
    ...composition.steps.map((s) => s.caption),
  ];
  for (const caption of captions) {
    if (!caption.trim()) problems.push("empty caption");
    if (caption.length > MAX_CAPTION)
      problems.push(`caption longer than ${MAX_CAPTION}: "${caption}"`);
    if (/[çğıöşüÇĞİÖŞÜ]/.test(caption))
      problems.push(`caption is not English: "${caption}"`);
  }
  const text = captions.join("\n");
  for (const permission of composition.permissions) {
    if (!text.includes(permission))
      problems.push(`permission ${permission} is never named in a caption`);
  }
  const seconds = estimateSeconds(composition);
  if (seconds > MAX_SECONDS)
    problems.push(`estimated ${seconds}s, over ${MAX_SECONDS}s`);
  return problems;
}

/** Caption holds plus a rough allowance for each action (typing, navigation). */
export function estimateSeconds(composition: Composition): number {
  const ms = composition.steps.reduce(
    (sum, s) => sum + (s.hold ?? DEFAULT_HOLD) + (s.run ? 2000 : 0),
    4000,
  );
  return Math.round(ms / 1000);
}
