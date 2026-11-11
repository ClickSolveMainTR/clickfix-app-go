/**
 * Time stamps of the conversation (CF-210). A message of today shows only
 * the clock; an older one adds the day, and one of another year the year.
 * The pattern is date-fns syntax for `datetime()` in `@/lib/utils/format`.
 */
export function messageTimePattern(value: Date, now: Date = new Date()) {
  if (
    value.getFullYear() === now.getFullYear() &&
    value.getMonth() === now.getMonth() &&
    value.getDate() === now.getDate()
  ) {
    return "HH:mm";
  }
  if (value.getFullYear() === now.getFullYear()) return "d MMM HH:mm";
  return "d MMM yyyy HH:mm";
}

/** Near the composer limit: the counter is shown from 80% on. */
export function showCharCount(length: number, max: number) {
  return max > 0 && length >= Math.floor(max * 0.8);
}
