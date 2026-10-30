/**
 * Minimal Centrifugo client (JSON protocol, one channel). Reconnects with
 * jittered backoff and asks for fresh tokens on every connect.
 */
export type RealtimeTokens = {
  ws_url: string;
  connection_token: string;
  subscription_token: string;
  channel: string;
};

export class Realtime {
  private ws: WebSocket | null = null;
  private id = 0;
  private stopped = false;
  private attempts = 0;
  private timer: number | undefined;
  connected = false;

  constructor(
    private tokens: () => Promise<RealtimeTokens>,
    private onEvent: (data: unknown) => void,
    private onState: (connected: boolean) => void,
  ) {}

  start() {
    this.stopped = false;
    void this.connect();
  }

  stop() {
    this.stopped = true;
    window.clearTimeout(this.timer);
    this.ws?.close();
    this.ws = null;
    this.setConnected(false);
  }

  /** Reconnect now (the browser came back online). */
  kick() {
    if (this.stopped || this.connected) return;
    window.clearTimeout(this.timer);
    this.attempts = 0;
    void this.connect();
  }

  private setConnected(v: boolean) {
    if (this.connected !== v) {
      this.connected = v;
      this.onState(v);
    }
  }

  private retry() {
    if (this.stopped) return;
    this.attempts++;
    const delay =
      Math.min(30000, 1000 * 2 ** Math.min(this.attempts, 5)) +
      Math.random() * 1000;
    this.timer = window.setTimeout(() => void this.connect(), delay);
  }

  private async connect() {
    if (this.stopped) return;
    let t: RealtimeTokens;
    try {
      t = await this.tokens();
    } catch {
      this.retry();
      return;
    }
    if (!t.ws_url) return;
    let ws: WebSocket;
    try {
      ws = new WebSocket(t.ws_url);
    } catch {
      this.retry();
      return;
    }
    this.ws?.close();
    this.ws = ws;
    const send = (cmd: Record<string, unknown>) => ws.send(JSON.stringify(cmd));
    ws.onopen = () => {
      send({
        id: ++this.id,
        connect: { token: t.connection_token, name: "clickfix-widget" },
      });
      send({
        id: ++this.id,
        subscribe: { channel: t.channel, token: t.subscription_token },
      });
    };
    ws.onmessage = (ev) => {
      for (const line of String(ev.data).split("\n").filter(Boolean)) {
        let msg: Record<string, unknown>;
        try {
          msg = JSON.parse(line) as Record<string, unknown>;
        } catch {
          continue;
        }
        if (Object.keys(msg).length === 0) {
          ws.send("{}"); // server ping → pong
          continue;
        }
        if (msg.error) {
          ws.close();
          continue;
        }
        if (msg.subscribe) {
          this.attempts = 0;
          this.setConnected(true);
        }
        const push = msg.push as
          { channel?: string; pub?: { data?: unknown } } | undefined;
        if (push?.pub && push.channel === t.channel)
          this.onEvent(push.pub.data);
      }
    };
    ws.onclose = () => {
      if (this.ws === ws) this.ws = null;
      this.setConnected(false);
      this.retry();
    };
    ws.onerror = () => ws.close();
  }
}
