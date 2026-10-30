import { format, isValid, parseISO } from "date-fns";

/**
 * The access window is edited with `AppDatePicker` (`yyyy-MM-dd`, see the
 * admin-date-picker rule); the API takes RFC 3339 instants. The start is the
 * beginning of the local day, the end the end of the local day, so a window
 * "1 May – 31 May" covers both days fully.
 */
export function isoToDateInput(value: string | null | undefined): string {
  if (!value) return "";
  const parsed = parseISO(value);
  return isValid(parsed) ? format(parsed, "yyyy-MM-dd") : "";
}

function localDayISO(value: string, time: string): string | undefined {
  if (!value) return undefined;
  const parsed = parseISO(`${value}T${time}`);
  return isValid(parsed) ? parsed.toISOString() : undefined;
}

/** `yyyy-MM-dd` → start of that local day as an ISO instant. */
export function startOfDayISO(value: string): string | undefined {
  return localDayISO(value, "00:00:00");
}

/** `yyyy-MM-dd` → end of that local day as an ISO instant. */
export function endOfDayISO(value: string): string | undefined {
  return localDayISO(value, "23:59:59");
}
