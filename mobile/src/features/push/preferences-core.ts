/**
 * Notification preference rules shared by the screen and the optimistic
 * update; free of app imports so they run under `node --test`.
 */

type Channels = Record<string, Record<string, boolean>>;
export type Prefs = {
  event_channels: Channels;
  quiet_hours: { enabled: boolean; start: string; end: string; timezone: string };
  [k: string]: unknown;
};
export type PrefsUpdate = { event_channels?: Channels; quiet_hours?: Prefs['quiet_hours']; [k: string]: unknown };
export type EventType = { key: string; defaults: Record<string, boolean>; locked: string[] };

/** The server's merge rules, applied locally for the optimistic state (event_channels merge per cell). */
export function applyUpdate<P extends Prefs>(p: P, u: PrefsUpdate): P {
  const event_channels = { ...p.event_channels };
  for (const [ev, cells] of Object.entries(u.event_channels ?? {}))
    event_channels[ev] = { ...event_channels[ev], ...cells };
  return {
    ...p,
    ...u,
    event_channels,
    quiet_hours: u.quiet_hours ? { ...p.quiet_hours, ...u.quiet_hours } : p.quiet_hours,
  } as P;
}

/** Whether `channel` is on for an event: the stored choice, else the default; locked channels are always on. */
export function channelOn(p: Prefs, ev: EventType, channel: string): boolean {
  if (ev.locked.includes(channel)) return true;
  const stored = p.event_channels[ev.key]?.[channel];
  return stored ?? ev.defaults[channel] ?? false;
}
