/**
 * Timeline layout rules (CF-81), pure so they run under `node --test`:
 * day separators and sender runs, the way chat apps (WhatsApp, Intercom,
 * iMessage) group consecutive bubbles so a burst of messages reads as one.
 */

type TimelineMessage = {
  created_at: string;
  direction: string;
  kind: string;
  private: boolean;
  sender: { uuid?: string | null; name?: string | null };
};

/** Consecutive bubbles closer than this stay in one run. */
export const RUN_GAP_MS = 5 * 60_000;

export type TimelineMark = {
  /** First message of its day: a date separator goes above it. */
  dayStart: boolean;
  /** Last message of a sender run: it shows the name / time line and the bubble tail. */
  runEnd: boolean;
  /** First message of a sender run: it gets the run's top spacing. */
  runStart: boolean;
};

function sameRun(a: TimelineMessage, b: TimelineMessage, dayKey: (iso: string) => string): boolean {
  if (a.kind === 'event' || b.kind === 'event') return false;
  if (a.direction !== b.direction || a.private !== b.private) return false;
  if ((a.sender.uuid ?? a.sender.name ?? '') !== (b.sender.uuid ?? b.sender.name ?? '')) return false;
  if (dayKey(a.created_at) !== dayKey(b.created_at)) return false;
  return Math.abs(new Date(a.created_at).getTime() - new Date(b.created_at).getTime()) <= RUN_GAP_MS;
}

/**
 * Marks for an inverted timeline (`items[0]` is the newest). `dayKey` maps an
 * instant to its calendar day in the display zone.
 */
export function timelineMarks(items: TimelineMessage[], dayKey: (iso: string) => string): TimelineMark[] {
  return items.map((m, i) => {
    const older = items[i + 1];
    const newer = items[i - 1];
    return {
      dayStart: !older || dayKey(older.created_at) !== dayKey(m.created_at),
      runEnd: !newer || !sameRun(m, newer, dayKey),
      runStart: !older || !sameRun(m, older, dayKey),
    };
  });
}

/** `today` / `yesterday` for the separator, or null to print the date. */
export function relativeDay(dayOf: string, today: string, yesterday: string): 'today' | 'yesterday' | null {
  if (dayOf === today) return 'today';
  if (dayOf === yesterday) return 'yesterday';
  return null;
}

/**
 * The composer's "/" shortcut (Intercom, Front macros): a draft that is only
 * a slash and a word asks for canned responses matching that word.
 */
export function slashQuery(draft: string): string | null {
  const m = /^\/([\p{L}\p{N}_-]*)$/u.exec(draft);
  return m ? m[1] : null;
}

/** Display-only groups; cached messages and pagination remain untouched. Newest first. */
export function collapseEvents<T extends TimelineMessage & { attributes?: Record<string, unknown> | null }>(
  items: T[],
  dayKey: (iso: string) => string,
): { message: T; count: number }[] {
  const rows: { message: T; count: number }[] = [];
  let previous: T | undefined;
  for (const message of items) {
    const a = message.attributes;
    const b = previous?.attributes;
    const merge = a?.event === 'merged_from';
    // Merge sources differ by definition; other events must have identical attributes.
    const same = merge
      ? b?.event === a.event && a.actor_uuid === b?.actor_uuid && a.actor_name === b?.actor_name
      : !!a?.event && JSON.stringify(Object.entries(a).sort()) === JSON.stringify(Object.entries(b ?? {}).sort());
    if (
      message.kind === 'event' &&
      previous?.kind === 'event' &&
      same &&
      dayKey(message.created_at) === dayKey(previous.created_at) &&
      Math.abs(Date.parse(message.created_at) - Date.parse(previous.created_at)) <= RUN_GAP_MS
    ) {
      rows[rows.length - 1].count++;
    } else rows.push({ message, count: 1 });
    previous = message;
  }
  return rows;
}
