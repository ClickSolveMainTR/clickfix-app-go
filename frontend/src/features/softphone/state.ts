import type { CallSnapshot } from "./engine";
import type { CallContext, ClientCallEvent } from "./types";

export function callStatus(
  call: CallSnapshot,
  talkSeconds: number,
): ClientCallEvent["client_status"] {
  if (call.error) return "failed";
  if (talkSeconds > 0) return "answered";
  if (call.endReason === "No Answer") return "no_answer";
  if (call.endReason === "Busy") return "busy";
  return "cancelled";
}

export function callPhaseKey(call: CallSnapshot) {
  return call.error ? "error" : call.phase;
}

/** i18n keys under `softphone.failure.*`; see `callFailure`. */
export type CallFailureKind =
  | "pbxRejected"
  | "busy"
  | "forbidden"
  | "notFound"
  | "unavailable"
  | "noAnswer"
  | "rejected"
  | "microphone"
  | "media"
  | "failed";

export interface CallFailure {
  kind: CallFailureKind;
  /** SIP status code when known (shown next to the message: "486"). */
  code: number | null;
  /** Q.850 cause from the PBX `Reason:` header (17 = user busy, used by Netsantral for refused outbound calls). */
  q850: number | null;
}

const reasonCauses: Partial<Record<string, CallFailureKind>> = {
  Busy: "busy",
  Rejected: "forbidden",
  "Not Found": "notFound",
  Unavailable: "unavailable",
  "No Answer": "noAnswer",
  "Microphone Permission": "microphone",
  "User Denied Media Access": "microphone",
  "Bad Media Description": "media",
  "RTP Timeout": "media",
  "WebRTC Error": "media",
};

export function q850Cause(
  reasonHeader: string | null | undefined,
): number | null {
  const match = /cause\s*=\s*(\d+)/i.exec(reasonHeader ?? "");
  return match ? Number(match[1]) : null;
}

/**
 * Why an outgoing call never connected. `null` for normal endings (BYE, local hangup/cancel) or answered calls.
 * An outbound 486 that arrives with `Reason: Q.850;cause=17` before anything rang is the PBX refusing the
 * call itself (Netsantral does this when the account has no TL balance or outbound permission), so it is
 * reported as `pbxRejected` rather than "the customer is busy".
 */
export function callFailure(call: CallSnapshot): CallFailure | null {
  if (call.phase !== "ended" || call.startedAt !== null) return null;
  if (
    call.endReason === "Canceled" ||
    call.endReason === "Terminated" ||
    call.endReason === "BYE"
  )
    return null;
  const code = typeof call.statusCode === "number" ? call.statusCode : null;
  const q850 = q850Cause(call.reasonHeader);
  if (
    call.direction === "outgoing" &&
    (code === 486 || code === 603) &&
    q850 === 17
  )
    return { kind: "pbxRejected", code, q850 };
  if (code === 486 || code === 600) return { kind: "busy", code, q850 };
  if (code === 403 || code === 603) return { kind: "forbidden", code, q850 };
  if (code === 404 || code === 484) return { kind: "notFound", code, q850 };
  if (code === 480 || code === 503 || code === 502)
    return { kind: "unavailable", code, q850 };
  if (code === 408 || code === 487) return { kind: "noAnswer", code, q850 };
  const byReason = reasonCauses[call.endReason ?? ""];
  if (byReason) return { kind: byReason, code, q850 };
  if (call.error || code !== null) return { kind: "failed", code, q850 };
  return call.direction === "incoming"
    ? null
    : { kind: "rejected", code, q850 };
}

export function clientCallEvent(
  call: CallSnapshot,
  context: CallContext,
  beganAt: number,
  endedAt: number,
): ClientCallEvent {
  const talkSeconds =
    call.startedAt === null
      ? 0
      : Math.max(0, Math.floor((endedAt - call.startedAt) / 1000));
  return {
    ...context,
    direction: call.direction === "incoming" ? "inbound" : "outbound",
    phone: call.remote,
    started_at: new Date(beganAt).toISOString(),
    ended_at: new Date(endedAt).toISOString(),
    client_status: callStatus(call, talkSeconds),
    talk_seconds: talkSeconds,
  };
}
