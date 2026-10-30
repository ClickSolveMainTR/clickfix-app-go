export function startOfDay(date: Date) {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate());
}
export function addDays(date: Date, amount: number) {
  const result = new Date(date);
  result.setDate(result.getDate() + amount);
  return result;
}
export function startOfWeek(date: Date, monday: boolean) {
  return addDays(startOfDay(date), -((date.getDay() + (monday ? 6 : 0)) % 7));
}
export function monthDays(date: Date, monday: boolean) {
  const first = startOfWeek(new Date(date.getFullYear(), date.getMonth(), 1), monday);
  return Array.from({ length: 42 }, (_, i) => addDays(first, i));
}
export function sameDay(a: Date, b: Date) {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}
export function localDayKey(date: Date) {
  return `${date.getFullYear()}-${date.getMonth()}-${date.getDate()}`;
}

/** Port of web calendar views/utils.ts layoutTimedSegments: clip days and partition overlap clusters. */
export function layoutEvents<T extends { starts_at: string; ends_at: string }>(events: T[], day: Date) {
  const start = startOfDay(day).getTime();
  const end = addDays(day, 1).getTime();
  const sorted = events
    .filter((e) => Date.parse(e.starts_at) < end && Date.parse(e.ends_at) > start)
    .sort((a, b) => Date.parse(a.starts_at) - Date.parse(b.starts_at));
  const out: { event: T; top: number; height: number; column: number; columns: number }[] = [];
  let group: typeof out = [];
  let groupEnd = 0;
  let lanes: number[] = [];
  const flush = () => {
    for (const item of group) item.columns = lanes.length;
    group = [];
    lanes = [];
    groupEnd = 0;
  };
  for (const event of sorted) {
    const from = Math.max(start, Date.parse(event.starts_at));
    const to = Math.min(end, Date.parse(event.ends_at));
    if (from >= groupEnd) flush();
    let column = lanes.findIndex((last) => last <= from);
    if (column < 0) column = lanes.length;
    const layoutEnd = Math.max(to, from + 24 * 60000);
    lanes[column] = layoutEnd;
    const d = new Date(from);
    const item = {
      event,
      top: d.getHours() * 60 + d.getMinutes(),
      height: Math.max(24, (to - from) / 60000),
      column,
      columns: 1,
    };
    out.push(item);
    group.push(item);
    groupEnd = Math.max(groupEnd, layoutEnd);
  }
  flush();
  return out;
}
