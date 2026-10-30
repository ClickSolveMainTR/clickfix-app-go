import { onlineManager } from '@tanstack/react-query';
import * as Network from 'expo-network';
import { useSyncExternalStore } from 'react';

/**
 * Connectivity from the OS (expo-network) drives TanStack Query's
 * `onlineManager`: queries and the send queue pause offline and resume the
 * moment the phone is back. A state of `null` (unknown yet) counts as online
 * so the first request is never held back.
 *
 * Only `isConnected` decides (CF-79). `isInternetReachable` is a guess: on
 * Android it is false whenever the OS has not "validated" the network (a VPN,
 * Private DNS, a carrier or office network that blocks Google's connectivity
 * check), on iOS it lags network changes. Trusting it held every reply in the
 * send queue as "waiting for connection" on phones that were online.
 * Whether the API answers is learned from real requests instead (below).
 */
let started = false;

export function startNetworkWatch() {
  if (started) return;
  started = true;
  onlineManager.setEventListener((setOnline) => {
    const apply = (s: Network.NetworkState) => setOnline(s.isConnected !== false);
    void Network.getNetworkStateAsync()
      .then(apply)
      .catch(() => undefined);
    const sub = Network.addNetworkStateListener(apply);
    return () => sub.remove();
  });
}

/** True while the phone has a connection. */
export function useOnline() {
  return useSyncExternalStore(
    (cb) => onlineManager.subscribe(cb),
    () => onlineManager.isOnline(),
  );
}

/**
 * Whether the last API call got an answer (any HTTP status) or failed in
 * transport (no route, DNS, TLS, timeout). Starts optimistic.
 */
let apiReachable = true;
const reachListeners = new Set<() => void>();

export function reportApiReachable(ok: boolean) {
  if (ok === apiReachable) return;
  apiReachable = ok;
  reachListeners.forEach((fn) => fn());
}

export function isApiReachable() {
  return apiReachable;
}

function subscribeReach(cb: () => void) {
  reachListeners.add(cb);
  return () => {
    reachListeners.delete(cb);
  };
}

/**
 * What the connection banner says: `offline` (no network on the phone),
 * `unreachable` (online, but the server does not answer), or `ok`.
 */
export type ConnectionState = 'ok' | 'offline' | 'unreachable';

export function useConnection(): ConnectionState {
  const online = useOnline();
  const reachable = useSyncExternalStore(subscribeReach, isApiReachable);
  if (!online) return 'offline';
  return reachable ? 'ok' : 'unreachable';
}
