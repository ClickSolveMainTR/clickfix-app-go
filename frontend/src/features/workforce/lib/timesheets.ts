import { differenceInCalendarDays, parseISO } from "date-fns";

import type { Timesheet } from "../services/workforce.service";

/** The timesheet API accepts at most 31 days per request. */
export const MAX_TIMESHEET_DAYS = 31;

export type AgentTime = {
  agent: string;
  active: number;
  away: number;
  handling: number;
  /** Handling seconds per conversation (`null` = deleted conversation). */
  conversations: { uuid: string | null; seconds: number }[];
};

/**
 * Groups the API rows (agent × state × conversation) into one line per
 * agent. Working time is active + handling; away is reported separately.
 */
export function agentTimes(rows: Timesheet[]): AgentTime[] {
  const byAgent = new Map<string, AgentTime>();
  for (const row of rows) {
    let entry = byAgent.get(row.agent_uuid);
    if (!entry) {
      entry = {
        agent: row.agent_uuid,
        active: 0,
        away: 0,
        handling: 0,
        conversations: [],
      };
      byAgent.set(row.agent_uuid, entry);
    }
    if (row.state === "active") entry.active += row.seconds;
    else if (row.state === "away") entry.away += row.seconds;
    else if (row.state === "handling") {
      entry.handling += row.seconds;
      entry.conversations.push({
        uuid: row.conversation_uuid ?? null,
        seconds: row.seconds,
      });
    }
  }
  const list = [...byAgent.values()];
  for (const a of list) a.conversations.sort((x, y) => y.seconds - x.seconds);
  return list.sort((x, y) => y.active + y.handling - (x.active + x.handling));
}

/** Inclusive day range is within the API's 31-day limit. */
export function rangeValid(from: string, to: string) {
  if (!from || !to || from > to) return false;
  return (
    differenceInCalendarDays(parseISO(to), parseISO(from)) < MAX_TIMESHEET_DAYS
  );
}
