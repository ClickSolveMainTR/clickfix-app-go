/**
 * Home summary (CF-167): which numbers and which pending items the member
 * sees. Free of app imports so it runs under `node --test`. The SLA and
 * appointment rows come from the widget snapshot helpers
 * (`features/widgets/snapshot-core.ts`), which read the same API answers.
 */

export type Counts = { open: number; mine: number; unassigned: number; breached: number };

export type TileKey = 'open' | 'mine' | 'unassigned' | 'breached';
export type Tile = { key: TileKey; value: number; alert: boolean };

/** The inbox numbers in reading order; an SLA breach is the only one that alarms. */
export function countTiles(c: Counts): Tile[] {
  const n = (v: number) => (Number.isFinite(v) && v > 0 ? Math.floor(v) : 0);
  return [
    { key: 'open', value: n(c.open), alert: false },
    { key: 'mine', value: n(c.mine), alert: false },
    { key: 'unassigned', value: n(c.unassigned), alert: false },
    { key: 'breached', value: n(c.breached), alert: n(c.breached) > 0 },
  ];
}

export type PendingKey = 'notifications' | 'setup' | 'incidents';
export type Pending = { key: PendingKey; count: number };

/**
 * What still waits for the member, most pressing first: ongoing incidents,
 * unread notifications, open setup steps. A source the member cannot see (or
 * that has not answered) is passed as null and leaves no row.
 */
export function pendingItems(input: {
  incidents: number | null;
  unread: number | null;
  setup: { completed: number; total: number } | null;
}): Pending[] {
  const out: Pending[] = [];
  if (input.incidents && input.incidents > 0) out.push({ key: 'incidents', count: input.incidents });
  if (input.unread && input.unread > 0) out.push({ key: 'notifications', count: input.unread });
  const left = input.setup ? input.setup.total - input.setup.completed : 0;
  if (left > 0) out.push({ key: 'setup', count: left });
  return out;
}

/** "Merhaba Ayşe" needs a first name; without one the screen uses the plain title. */
export function firstName(name: string | null | undefined) {
  return (name ?? '').trim().split(/\s+/)[0] ?? '';
}
