/**
 * Session replay and co-browse (CF-126): asks the visitor, loads the rrweb
 * recorder bundle only after consent, uploads gzip chunks and follows the
 * agent's pointer. Moved out of widget.ts unchanged in behaviour (CF-150).
 */
import type { Transport } from "./api";
import { Realtime, type RealtimeTokens } from "./realtime";
import type { StringKey } from "../shared/i18n";

export type ReplaySession = {
  uuid: string;
  kind: "replay" | "cobrowse";
  status: string;
  control_enabled: boolean;
  timeout_at: string;
};
export type ReplayPolicy = {
  replay_enabled: boolean;
  cobrowse_enabled: boolean;
  control_enabled: boolean;
  block_selectors: string[];
};
type RecorderModule = {
  start(emit: (event: unknown) => void, selectors: string[]): () => void;
};

export type CaptureHost = {
  api: Transport;
  t(key: StringKey): string;
  emit(event: string, payload: unknown): void;
  appOrigin: string;
};

let recorderLoad: Promise<RecorderModule> | null = null;
function loadRecorder(origin: string): Promise<RecorderModule> {
  if (!recorderLoad)
    recorderLoad = new Promise((resolve, reject) => {
      const script = document.createElement("script");
      script.src = origin + "/widget-recorder.js";
      script.onload = () => {
        const mod = (window as unknown as { ClickFixRecorder?: RecorderModule })
          .ClickFixRecorder;
        if (mod) resolve(mod);
        else reject(new Error("Recorder unavailable"));
      };
      script.onerror = () => reject(new Error("Recorder unavailable"));
      document.head.appendChild(script);
    });
  return recorderLoad;
}

export class Capture {
  private capture: {
    session: ReplaySession;
    conversation: string;
    sequence: number;
    events: unknown[];
    stop: () => void;
    live: Realtime | null;
    flushing: boolean;
  } | null = null;
  private captureTimer: number | undefined;
  private captureDeadline: number | undefined;
  private captureUI: HTMLElement | null = null;
  private pointerUI: HTMLElement | null = null;
  private captureGeneration = 0;

  constructor(private host: CaptureHost) {}

  /** Explicit command for the host's own consent UI; the widget still asks the visitor. */
  async startReplay(conversation: string | null) {
    if (!conversation) return;
    const policy = await this.host.api.call<ReplayPolicy>(
      "GET",
      "/replay-policy",
    );
    if (!policy.replay_enabled) return;
    this.askCapture(conversation, null, policy);
  }

  askCapture(
    conversation: string,
    session: ReplaySession | null,
    policy: ReplayPolicy,
  ) {
    if (this.capture || this.captureUI) return;
    const box = document.createElement("div");
    box.setAttribute("data-clickfix-block", "");
    box.style.cssText =
      "position:fixed;bottom:90px;right:24px;z-index:2147483647;background:white;color:#111;padding:18px;border:2px solid #4554ec;border-radius:12px;max-width:320px;font:14px system-ui;box-shadow:0 8px 32px #0003";
    const text = document.createElement("p");
    text.textContent = session
      ? session.control_enabled
        ? this.host.t("captureControlNotice")
        : this.host.t("captureLiveNotice")
      : this.host.t("captureReplayNotice");
    const yes = document.createElement("button");
    yes.textContent = this.host.t("captureAllow");
    const no = document.createElement("button");
    no.textContent = this.host.t("captureDecline");
    const finish = () => {
      box.remove();
      this.captureUI = null;
    };
    yes.onclick = async () => {
      yes.disabled = true;
      try {
        const accepted = session
          ? await this.host.api.call<ReplaySession>(
              "POST",
              `/conversations/${conversation}/sessions/${session.uuid}/consent`,
              { action: "accept" },
            )
          : await this.host.api.call<ReplaySession>(
              "POST",
              `/conversations/${conversation}/sessions`,
              { consent: true },
            );
        finish();
        await this.beginCapture(conversation, accepted, policy);
      } catch {
        finish();
        this.host.emit("error", {
          code: "CAPTURE_UNAVAILABLE",
          message: this.host.t("captureUnavailable"),
        });
      }
    };
    no.onclick = () => {
      finish();
      if (session)
        void this.host.api
          .call(
            "POST",
            `/conversations/${conversation}/sessions/${session.uuid}/consent`,
            { action: "decline" },
          )
          .catch(() => {});
    };
    box.append(text, yes, no);
    document.body.appendChild(box);
    this.captureUI = box;
  }
  private async beginCapture(
    conversation: string,
    session: ReplaySession,
    policy: ReplayPolicy,
  ) {
    if (this.capture) return;
    const generation = ++this.captureGeneration;
    const mod = await loadRecorder(this.host.appOrigin);
    if (
      generation !== this.captureGeneration ||
      Date.parse(session.timeout_at) <= Date.now()
    )
      return;
    const c = {
      session,
      conversation,
      sequence: 0,
      events: [] as unknown[],
      stop: () => {},
      live: null as Realtime | null,
      flushing: false,
    };
    this.capture = c;
    try {
      if (session.kind === "cobrowse") {
        c.live = new Realtime(
          () =>
            this.host.api.call<RealtimeTokens>(
              "POST",
              `/conversations/${conversation}/sessions/${session.uuid}/live-token`,
            ),
          (data) => this.onCaptureEvent(data, policy),
          () => {},
        );
        c.live.start();
      }
      c.stop = mod.start((event) => {
        if (this.capture !== c) return;
        if (c.events.length >= 5000) {
          void this.stopCapture(true);
          return;
        }
        c.events.push(event);
      }, policy.block_selectors);
      this.captureTimer = window.setInterval(
        () => {
          void this.flushCapture();
        },
        session.kind === "cobrowse" ? 1000 : 5000,
      );
      this.captureDeadline = window.setTimeout(
        () => {
          void this.stopCapture(false);
        },
        Math.max(0, Date.parse(session.timeout_at) - Date.now()),
      );
      const box = document.createElement("div");
      box.setAttribute("data-clickfix-block", "");
      box.style.cssText =
        "position:fixed;top:12px;right:12px;z-index:2147483647;background:#fff;color:#111;border:2px solid #4554ec;padding:12px;font:14px system-ui";
      box.append(
        document.createTextNode(
          session.kind === "cobrowse"
            ? this.host.t("captureLive")
            : this.host.t("captureRecording"),
        ),
      );
      const stop = document.createElement("button");
      stop.textContent = this.host.t("captureStop");
      stop.onclick = () => {
        void this.stopCapture(false);
      };
      const revoke = document.createElement("button");
      revoke.textContent = this.host.t("captureRevoke");
      revoke.onclick = () => {
        void this.stopCapture(true);
      };
      box.append(stop, revoke);
      document.body.appendChild(box);
      this.captureUI = box;
    } catch {
      await this.stopCapture(true);
    }
  }
  private async flushCapture() {
    const c = this.capture;
    if (!c || c.flushing || !c.events.length) return;
    if (typeof CompressionStream === "undefined") {
      await this.stopCapture(true);
      return;
    }
    c.flushing = true;
    const events = c.events.splice(0);
    try {
      const raw = JSON.stringify(events);
      if (new TextEncoder().encode(raw).length > 2 * 1024 * 1024)
        throw new Error("Capture too large");
      const compressed = await new Response(
        new Blob([raw]).stream().pipeThrough(new CompressionStream("gzip")),
      ).arrayBuffer();
      if (compressed.byteLength > 512 * 1024)
        throw new Error("Capture too large");
      let binary = "";
      for (const b of new Uint8Array(compressed))
        binary += String.fromCharCode(b);
      await this.host.api.call(
        "POST",
        `/conversations/${c.conversation}/sessions/${c.session.uuid}/chunks`,
        { sequence: c.sequence, data: btoa(binary) },
      );
      c.sequence =
        c.session.kind === "cobrowse" ? (c.sequence + 1) % 300 : c.sequence + 1;
    } catch {
      void this.stopCapture(true);
    } finally {
      c.flushing = false;
    }
  }
  async stopCapture(revoke = false) {
    ++this.captureGeneration;
    const c = this.capture;
    c?.stop();
    c?.live?.stop();
    if (c && !revoke) await this.flushCapture();
    this.capture = null;
    window.clearInterval(this.captureTimer);
    window.clearTimeout(this.captureDeadline);
    this.captureUI?.remove();
    this.captureUI = null;
    this.pointerUI?.remove();
    this.pointerUI = null;
    if (c)
      await this.host.api
        .call(
          "POST",
          `/conversations/${c.conversation}/sessions/${c.session.uuid}/consent`,
          { action: revoke ? "revoke" : "stop" },
        )
        .catch(() => {});
  }
  onCaptureEvent(data: unknown, policy: ReplayPolicy) {
    const ev = data as {
      type?: string;
      session?: ReplaySession;
      event?: { type: string; x: number; y: number };
    };
    if (ev.type === "cobrowse.status" && ev.session?.status !== "active") {
      void this.stopCapture(false);
      return;
    }
    if (
      ev.type !== "cobrowse.pointer" ||
      !ev.event ||
      !this.capture ||
      Date.parse(this.capture.session.timeout_at) <= Date.now()
    )
      return;
    const { x, y, type } = ev.event;
    if (!Number.isFinite(x) || !Number.isFinite(y)) return;
    const target = document.elementFromPoint(x, y);
    const blocked = [
      "input,textarea,select,[contenteditable],[data-private],[data-clickfix-block],.rr-block,.rr-mask",
      ...policy.block_selectors,
    ].join(",");
    if (!target || target.closest(blocked)) return;
    if (type === "control_click") {
      // Only host-marked non-submitting buttons. Never type, focus or write field values.
      if (
        this.capture.session.control_enabled &&
        policy.control_enabled &&
        target instanceof HTMLButtonElement &&
        target.type === "button" &&
        target.hasAttribute("data-clickfix-control")
      )
        target.click();
      return;
    }
    this.pointerUI?.remove();
    const pointer = document.createElement("div");
    pointer.setAttribute("data-clickfix-block", "");
    pointer.style.cssText = `position:fixed;pointer-events:none;z-index:2147483647;border:3px solid #4554ec;border-radius:50%;width:${type === "highlight" ? 40 : 12}px;height:${type === "highlight" ? 40 : 12}px;left:${x}px;top:${y}px`;
    document.body.appendChild(pointer);
    this.pointerUI = pointer;
  }
}
