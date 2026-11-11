import type { AssistantStatus } from "../services/assistant.service";

/**
 * The "Today N/limit" counter after one more reply (CF-207, #150). The
 * server counts a message as soon as it is stored, so the panel bumps the
 * cached status right away and refetches for the exact figure.
 */
export function bumpUsage(
  status: AssistantStatus | undefined,
): AssistantStatus | undefined {
  if (!status) return status;
  const used = Math.min(status.used_today + 1, status.daily_limit);
  if (used === status.used_today) return status;
  return { ...status, used_today: used };
}

/** Whether a run just settled (running → idle), which is when the counter moves. */
export function runSettled(wasRunning: boolean, running: boolean): boolean {
  return wasRunning && !running;
}
