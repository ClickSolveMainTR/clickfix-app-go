import { Centrifuge, type Subscription, UnauthorizedError } from 'centrifuge';
import { useEffect, useRef } from 'react';
import { AppState } from 'react-native';
import { create } from 'zustand';

import { api, unwrap } from '@/core/api/client';
import { originOf, resolveRealtimeUrl } from '@/core/api/endpoints';
import { isApiError } from '@/core/api/errors';
import { API_URL, REALTIME_URL, WEB_URL } from '@/core/config';

/**
 * One Centrifugo WebSocket for the signed-in user (`centrifuge` JS SDK; React
 * Native has a global WebSocket), ported from otopoly-mobile-app's
 * centrifuge-driver. The first connection token also names the socket URL;
 * later tokens come through `getToken`. Each channel gets its own
 * subscription token (`/v1/realtime/subscription-token`). The socket closes
 * in the background and reconnects on foreground; lists refetch then anyway.
 *
 * Realtime is never a precondition: sending goes over HTTP and works with the
 * socket down. Its state (`useRealtime`) only switches screens to polling.
 */

/**
 * Centrifugo checks the handshake's Origin against `allowed_origins` (the
 * panel origins). React Native sends the socket's own origin
 * (`https://rt.<host>`) unless told otherwise, so every phone was refused
 * and the app never got live updates (CF-79). It sends the panel origin, the
 * same one the web uses.
 */
const SOCKET_ORIGIN = originOf(WEB_URL);

type RNWebSocketCtor = new (
  url: string,
  protocols?: string | string[],
  options?: { headers?: Record<string, string> },
) => WebSocket;

function PanelOriginWebSocket(url: string, protocols?: string | string[]) {
  const Ctor = WebSocket as unknown as RNWebSocketCtor;
  return new Ctor(url, protocols, SOCKET_ORIGIN ? { headers: { origin: SOCKET_ORIGIN } } : undefined);
}

/** `off`: not started, switched off or refused; the rest mirror the Centrifuge client. */
export type RealtimeState = 'off' | 'connecting' | 'connected' | 'disconnected';

export const useRealtime = create<{ state: RealtimeState }>(() => ({ state: 'off' }));
const setState = (state: RealtimeState) => useRealtime.setState({ state });

/** 401 / 403 / 404 / 503 are answers ("not for you", "switched off"): stop instead of retrying forever. */
const isRefusal = (e: unknown) => isApiError(e) && [401, 403, 404, 503].includes(e.status);

type Entry = { onData: (data: unknown) => void };

let client: Centrifuge | null = null;
let starting: Promise<void> | null = null;
let refused = false;
const channels = new Map<string, Set<Entry>>();
const subs = new Map<string, Subscription>();

function connectionToken() {
  return unwrap(api.POST('/v1/realtime/connection-token'));
}

function subscriptionToken(channel: string) {
  return unwrap(api.POST('/v1/realtime/subscription-token', { body: { channel } }));
}

function attach(channel: string) {
  if (!client || subs.has(channel)) return;
  const sub = client.newSubscription(channel, {
    getToken: async () => {
      try {
        return (await subscriptionToken(channel)).token;
      } catch (e) {
        if (isRefusal(e)) throw new UnauthorizedError('subscription refused');
        throw e;
      }
    },
  });
  sub.on('publication', (ctx) => {
    for (const entry of channels.get(channel) ?? []) {
      try {
        entry.onData(ctx.data);
      } catch {
        // A listener bug must not stop the others.
      }
    }
  });
  subs.set(channel, sub);
  sub.subscribe();
}

function start() {
  if (client || starting || refused) return;
  starting = (async () => {
    try {
      const first = await connectionToken();
      const url = resolveRealtimeUrl(first.ws_url, API_URL, REALTIME_URL);
      if (!url) return;
      const next = new Centrifuge(url, {
        token: first.token,
        websocket: PanelOriginWebSocket,
        getToken: async () => {
          try {
            return (await connectionToken()).token;
          } catch (e) {
            if (isRefusal(e)) throw new UnauthorizedError('connection refused');
            throw e;
          }
        },
        minReconnectDelay: 1000,
        maxReconnectDelay: 20000,
      });
      next.on('connecting', () => setState('connecting'));
      next.on('connected', () => setState('connected'));
      next.on('disconnected', () => setState('disconnected'));
      client = next;
      channels.forEach((_, channel) => attach(channel));
      next.connect();
    } catch (e) {
      // Realtime switched off or not allowed: screens keep working on pull-to-refresh.
      if (isRefusal(e)) refused = true;
    } finally {
      starting = null;
    }
  })();
}

function detach(channel: string) {
  const sub = subs.get(channel);
  if (!sub) return;
  sub.removeAllListeners();
  sub.unsubscribe();
  client?.removeSubscription(sub);
  subs.delete(channel);
}

/** Drops the socket and every subscription (sign-out, organization switch). */
export function closeRealtime() {
  subs.forEach((_, channel) => detach(channel));
  client?.disconnect();
  client = null;
  refused = false;
  setState('off');
}

AppState.addEventListener('change', (state) => {
  if (!client) return;
  if (state === 'active') client.connect();
  else if (state === 'background') client.disconnect();
});

/**
 * Listens to a channel while the component is mounted; `null` listens to
 * nothing. The handler may change between renders without resubscribing.
 */
export function useChannel(channel: string | null, onData: (data: unknown) => void) {
  const handler = useRef(onData);
  useEffect(() => {
    handler.current = onData;
  }, [onData]);

  useEffect(() => {
    if (!channel) return;
    const entry: Entry = { onData: (d) => handler.current(d) };
    let set = channels.get(channel);
    if (!set) channels.set(channel, (set = new Set()));
    set.add(entry);
    if (client) attach(channel);
    else start();
    return () => {
      set.delete(entry);
      if (set.size === 0) {
        channels.delete(channel);
        detach(channel);
      }
    };
  }, [channel]);
}
