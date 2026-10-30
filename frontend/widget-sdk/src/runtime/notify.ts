/** Sound and browser notifications for agent replies while the page is
 * in the background (both opt-in through the widget's features). */

let ctx: AudioContext | null = null;

export function chime() {
  try {
    const AC =
      window.AudioContext ??
      (window as unknown as { webkitAudioContext?: typeof AudioContext })
        .webkitAudioContext;
    if (!AC) return;
    ctx ??= new AC();
    const o = ctx.createOscillator();
    const g = ctx.createGain();
    o.type = "sine";
    o.frequency.setValueAtTime(880, ctx.currentTime);
    o.frequency.exponentialRampToValueAtTime(660, ctx.currentTime + 0.18);
    g.gain.setValueAtTime(0.0001, ctx.currentTime);
    g.gain.exponentialRampToValueAtTime(0.12, ctx.currentTime + 0.02);
    g.gain.exponentialRampToValueAtTime(0.0001, ctx.currentTime + 0.3);
    o.connect(g).connect(ctx.destination);
    o.start();
    o.stop(ctx.currentTime + 0.32);
  } catch {
    /* autoplay policies: stay silent */
  }
}

/** Asks for notification permission (only from a visitor gesture). */
export function askPermission() {
  try {
    if ("Notification" in window && Notification.permission === "default")
      void Notification.requestPermission();
  } catch {
    /* unsupported */
  }
}

export function notify(title: string, body: string, onClick: () => void) {
  try {
    if (!("Notification" in window) || Notification.permission !== "granted")
      return;
    const n = new Notification(title, {
      body: body.slice(0, 160),
      tag: "clickfix",
    });
    n.onclick = () => {
      window.focus();
      onClick();
      n.close();
    };
  } catch {
    /* unsupported */
  }
}
