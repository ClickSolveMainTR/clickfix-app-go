/** A remote typing indicator expires if no new "start" arrives in time. */
export const TYPING_TTL_MS = 6_000;
/** Minimum gap between two "start" sends (the API allows one per 2 s). */
export const TYPING_SEND_INTERVAL_MS = 2_500;
/** "stop" is sent after this much local inactivity. */
export const TYPING_IDLE_MS = 3_000;

export type TypingEntry = { userUuid: string; name: string; at: number };

/** Applies one `conversation.typing` event to the current typers. */
export function applyTypingEvent(
  current: TypingEntry[],
  event: Record<string, unknown>,
  now: number,
  selfUuid: string | null,
): TypingEntry[] {
  const userUuid = typeof event.user_uuid === "string" ? event.user_uuid : "";
  if (!userUuid || userUuid === selfUuid) return pruneTyping(current, now);
  const rest = current.filter((t) => t.userUuid !== userUuid);
  if (event.state === "start") {
    const name = typeof event.name === "string" ? event.name : "";
    rest.push({ userUuid, name, at: now });
  }
  return pruneTyping(rest, now);
}

/** Drops typers whose last "start" is older than TYPING_TTL_MS. */
export function pruneTyping(current: TypingEntry[], now: number) {
  return current.filter((t) => now - t.at < TYPING_TTL_MS);
}
