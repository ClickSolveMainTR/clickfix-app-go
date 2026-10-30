/**
 * The ongoing call, as the Live Activity buttons reach it: `clickfix:///call/end`
 * and `clickfix:///call/mute` open the app on a tiny route that runs the
 * handler the call screen registered here (CF-152).
 */
type Handlers = { end: () => void; toggleMute: () => void };

let current: Handlers | null = null;

export function setCallHandlers(handlers: Handlers | null) {
  current = handlers;
}

/** Runs a call action; false when no call is running (the activity outlived it). */
export function runCallAction(action: string): boolean {
  if (!current) return false;
  if (action === 'end') current.end();
  else if (action === 'mute') current.toggleMute();
  else return false;
  return true;
}
