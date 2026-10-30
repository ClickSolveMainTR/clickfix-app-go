"use client";

import { useEffect } from "react";

import { Permission } from "@/config/permissions";
import type { PresenceStatus } from "@/features/presence";
import { useOptionalTenant } from "@/features/tenant/components/tenant-provider";

import {
  workforceService,
  type ActivityState,
} from "../services/workforce.service";

/** The API closes an interval five minutes after its last event. */
const HEARTBEAT_MS = 60_000;

function newClientUuid() {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  // RFC 4122 v4 fallback for insecure (plain http) origins.
  return "10000000-1000-4000-8000-100000000000".replace(/[018]/g, (c) =>
    (
      Number(c) ^
      (crypto.getRandomValues(new Uint8Array(1))[0]! & (15 >> (Number(c) / 4)))
    ).toString(16),
  );
}

function stateOf(status: PresenceStatus): ActivityState {
  if (status === "away") return "away";
  if (status === "offline") return "offline";
  return "active";
}

/**
 * Timesheet heartbeats (CF-107): mirrors the agent's presence status into
 * `/workforce/activity` on every change and once a minute while the tab is
 * visible. A hidden or closed tab stops sending, so the server ends the
 * interval after five minutes. Members without tenant.schedules.read send
 * nothing. Handling time per conversation is not reported from here.
 */
export function useWorkforceClock(status: PresenceStatus | null) {
  const tenant = useOptionalTenant();
  const slug = tenant?.slug ?? "";
  const allowed =
    tenant?.permissions.includes(Permission.TenantSchedulesRead) ?? false;
  const state = status ? stateOf(status) : null;

  useEffect(() => {
    if (!slug || !allowed || !state) return;
    const send = () => {
      workforceService
        .activity(slug, { client_uuid: newClientUuid(), state })
        .catch(() => undefined);
    };
    send();
    if (state === "offline") return;
    const timer = window.setInterval(() => {
      if (document.visibilityState === "visible") send();
    }, HEARTBEAT_MS);
    const onVisible = () => {
      if (document.visibilityState === "visible") send();
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [slug, allowed, state]);
}
