import type { WebphoneConfig } from "./types";

/**
 * Browser softphone engine (legacy SoftphoneWidget / GelenCagriModal on JsSIP). The UI only talks to
 * this interface; the JsSIP implementation is loaded lazily and tests replace it through
 * `window.__CLICKFIX_SIP_ENGINE_FACTORY__` (same pattern as the realtime socket factory), so no test
 * ever opens a SIP WebSocket.
 */

export type RegistrationState =
  "disconnected" | "connecting" | "registered" | "failed";
export type CallDirection = "incoming" | "outgoing";
export type CallPhase = "ringing" | "connecting" | "active" | "ended";

export interface CallSnapshot {
  id: string;
  direction: CallDirection;
  remote: string;
  remoteName: string | null;
  phase: CallPhase;
  held: boolean;
  muted: boolean;
  startedAt: number | null;
  endReason: string | null;
  /** SIP status the call failed with (486 Busy Here, 403 Forbidden, 404 Not Found…); null for BYE/cancel/local errors. */
  statusCode?: number | null;
  /** `Reason:` header (`Q.850;cause=17`) of the final response when the PBX sent one. */
  reasonHeader?: string | null;
  error?: boolean;
}

export interface SoftphoneEvents {
  onRegistration: (state: RegistrationState, reason?: string) => void;
  onCall: (call: CallSnapshot | null) => void;
  onRemoteStream: (stream: MediaStream | null) => void;
}

export interface SoftphoneEngine {
  start: () => void;
  stop: () => void;
  call: (target: string) => void;
  answer: () => void;
  hangup: () => void;
  setHold?: (held: boolean) => void;
  setMuted: (muted: boolean) => void;
  sendDtmf: (tone: string) => void;
  transfer: (target: string) => void;
}

export interface SoftphoneEngineConfig {
  websocketUrl: string;
  domain: string;
  username: string;
  password: string;
  iceServers: RTCIceServer[];
  /** SIP `From` display name (legacy widget: user's full name, fallback "ClickFix"). */
  displayName?: string;
}

export type SoftphoneEngineFactory = (
  config: SoftphoneEngineConfig,
  events: SoftphoneEvents,
) => SoftphoneEngine | Promise<SoftphoneEngine>;

declare global {
  interface Window {
    __CLICKFIX_SIP_ENGINE_FACTORY__?: SoftphoneEngineFactory;
  }
}

/** Legacy `sip_config.stun` default; used only when the settings carry no usable ICE server at all. */
export const defaultIceServers: RTCIceServer[] = [
  { urls: "stun:stun.l.google.com:19302" },
];

/**
 * `webphone.ice_servers` is stored as a string list (`["stun:stun.l.google.com:19302"]`, one per line in
 * Ayarlar). JsSIP/RTCPeerConnection need `{ urls }` objects, so strings are wrapped; objects pass through.
 * Dropping the strings silently (previous behaviour) left the INVITE SDP with host candidates only.
 */
export function normalizeIceServers(value: unknown): RTCIceServer[] {
  if (!Array.isArray(value)) return [];
  const servers: RTCIceServer[] = [];
  for (const entry of value) {
    if (typeof entry === "string") {
      const urls = entry.trim();
      if (/^(stun|stuns|turn|turns):/i.test(urls)) servers.push({ urls });
    } else if (entry && typeof entry === "object" && "urls" in entry) {
      const urls = (entry as { urls: unknown }).urls;
      if (
        typeof urls === "string"
          ? urls.trim()
          : Array.isArray(urls) && urls.length
      )
        servers.push(entry as RTCIceServer);
    }
  }
  return servers;
}

/** `null` when the user has no SIP account or the PBX is disabled. */
export function engineConfigFrom(
  config: WebphoneConfig | null | undefined,
  displayName?: string | null,
): SoftphoneEngineConfig | null {
  if (
    !config?.enabled ||
    !config.sip_websocket_url ||
    !config.sip_domain ||
    !config.sip_username ||
    !config.sip_password
  )
    return null;
  const iceServers = normalizeIceServers(config.ice_servers);
  return {
    websocketUrl: config.sip_websocket_url,
    domain: config.sip_domain,
    username: config.sip_username,
    password: config.sip_password,
    iceServers: iceServers.length ? iceServers : defaultIceServers,
    displayName: displayName?.trim() || "ClickFix",
  };
}

/** Dial string → SIP URI: keeps extensions/numbers (+ and digits, *, #) and appends the PBX domain. */
export function sipTarget(target: string, domain: string) {
  const trimmed = target.trim();
  if (trimmed.startsWith("sip:")) return trimmed;
  let dialable = trimmed.replace(/[^\d+*#]/g, "");
  const digits = dialable.replace(/\D/g, "");
  if (/^(?:90|0090)?5\d{9}$/.test(digits)) dialable = "0" + digits.slice(-10);
  return dialable ? `sip:${dialable}@${domain}` : null;
}

export async function createSoftphoneEngine(
  config: SoftphoneEngineConfig,
  events: SoftphoneEvents,
): Promise<SoftphoneEngine> {
  if (typeof window !== "undefined" && window.__CLICKFIX_SIP_ENGINE_FACTORY__) {
    return window.__CLICKFIX_SIP_ENGINE_FACTORY__(config, events);
  }
  const { createJsSipEngine } = await import("./jssip-engine");
  return createJsSipEngine(config, events);
}
