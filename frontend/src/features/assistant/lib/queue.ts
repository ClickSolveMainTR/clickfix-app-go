/**
 * Messages typed while a reply is streaming (CF-207, #150). assistant-ui's
 * composer ignores Enter during a run, so a second question used to pile up
 * in the same box and go out as one merged message. The composer now parks
 * each one here; the thread sends them one by one, in order, as separate
 * messages once the run has settled.
 */
export class PendingMessages {
  private items: string[] = [];
  private listeners = new Set<() => void>();

  /** Queues a message; blank text is dropped. Returns whether it was queued. */
  push(text: string): boolean {
    const trimmed = text.trim();
    if (!trimmed) return false;
    this.items.push(trimmed);
    this.notify();
    return true;
  }

  /** Removes and returns the oldest message, or undefined when empty. */
  next(): string | undefined {
    const item = this.items.shift();
    if (item !== undefined) this.notify();
    return item;
  }

  clear() {
    if (!this.items.length) return;
    this.items = [];
    this.notify();
  }

  get size() {
    return this.items.length;
  }

  /** Snapshot of the queued texts, oldest first. */
  list(): readonly string[] {
    return [...this.items];
  }

  subscribe(listener: () => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  private notify() {
    for (const l of this.listeners) l();
  }
}

/**
 * Whether a queued message should be sent now: the run just settled
 * (running → idle) and something is waiting. A run that pauses on a
 * confirmation card also counts as settled; the queued question then goes
 * out as its own message under the card.
 */
export function shouldSendQueued(
  wasRunning: boolean,
  running: boolean,
  queued: number,
): boolean {
  return wasRunning && !running && queued > 0;
}
