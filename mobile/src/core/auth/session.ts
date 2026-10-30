import { create } from 'zustand';

import { setActiveOrganization, setSessionExpiredHandler } from '@/core/api/client';
import { queryClient } from '@/core/query';
import { readJSON, storage, writeJSON } from '@/core/storage';
import { clearDrafts } from '@/features/inbox/drafts';
import { clearOutbox } from '@/features/inbox/outbox';
import { unregisterPush } from '@/features/push/push';
import { releaseLock } from '@/features/security/app-lock';
import { closeRealtime } from '@/features/realtime/client';

import { type ApiTokens, fetchMe, logout, type Me, type MeOrganization } from './api';
import { clearTokens, fromApi, loadTokens, setTokens } from './tokens';

type Status = 'loading' | 'signedOut' | 'signedIn';

type Session = {
  status: Status;
  /** Last known session; cached so the app renders before the network answers. */
  me: Me | null;
  /** Organization every `/v1/tenant/*` call is made for; null until one is picked. */
  organization: MeOrganization | null;
  hydrate: () => Promise<void>;
  signIn: (tokens: ApiTokens) => Promise<void>;
  refreshMe: () => Promise<Me>;
  selectOrganization: (slug: string) => void;
  signOut: (opts?: { remote?: boolean }) => Promise<void>;
};

const ME_KEY = 'session.me';
const ORG_KEY = 'session.org';

/** The last used organization if it is still a membership, else the only one. */
function pickOrganization(me: Me, preferred: string | null): MeOrganization | null {
  const orgs = (me.organizations ?? []).filter((o) => o.status === 'active');
  return orgs.find((o) => o.slug === preferred) ?? (orgs.length === 1 ? orgs[0] : null);
}

export const useSession = create<Session>((set, get) => ({
  status: 'loading',
  me: null,
  organization: null,

  hydrate: async () => {
    const tokens = await loadTokens();
    const me = tokens ? readJSON<Me>(ME_KEY) : null;
    const organization = me ? pickOrganization(me, readJSON<string>(ORG_KEY)) : null;
    setActiveOrganization(organization?.slug ?? null);
    set({ status: tokens && me ? 'signedIn' : 'signedOut', me, organization });
  },

  signIn: async (tokens) => {
    // The member just proved who they are: no biometric check on top.
    releaseLock();
    await setTokens(fromApi(tokens));
    const me = await fetchMe();
    writeJSON(ME_KEY, me);
    const organization = pickOrganization(me, readJSON<string>(ORG_KEY));
    setActiveOrganization(organization?.slug ?? null);
    set({ status: 'signedIn', me, organization });
  },

  refreshMe: async () => {
    const me = await fetchMe();
    writeJSON(ME_KEY, me);
    // Roles and permissions may have changed; a removed membership drops the selection.
    const organization = pickOrganization(me, get().organization?.slug ?? null);
    setActiveOrganization(organization?.slug ?? null);
    set({ me, organization });
    return me;
  },

  selectOrganization: (slug) => {
    const organization = get().me?.organizations?.find((o) => o.slug === slug) ?? null;
    if (!organization) return;
    writeJSON(ORG_KEY, slug);
    setActiveOrganization(slug);
    // Every cached list and live channel belongs to the previous organization.
    queryClient.clear();
    closeRealtime();
    set({ organization });
  },

  signOut: async ({ remote = true } = {}) => {
    // Lazy: widgets/sync reads the session store, a static import would be a cycle.
    await import('@/features/widgets/reset').then((m) => m.resetSurfaces({ remote }));
    if (remote) {
      // No pushes for this account on this phone after sign-out.
      await unregisterPush();
      await logout().catch(() => undefined);
    }
    closeRealtime();
    clearOutbox();
    clearDrafts();
    releaseLock();
    await clearTokens();
    storage.remove(ME_KEY);
    setActiveOrganization(null);
    queryClient.clear();
    set({ status: 'signedOut', me: null, organization: null });
  },
}));

setSessionExpiredHandler(() => {
  void useSession.getState().signOut({ remote: false });
});
