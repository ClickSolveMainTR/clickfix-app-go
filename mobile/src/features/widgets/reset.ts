import { resetLiveActivities } from './live-activities';
import { resetWidgets } from './sync';

/**
 * Sign-out (CF-152): widgets show the signed-out state, the widget token is
 * revoked while the access token still works, Live Activities end and
 * Spotlight forgets the conversations. Never blocks sign-out.
 */
export async function resetSurfaces(opts: { remote: boolean }) {
  await Promise.all([resetWidgets(opts), resetLiveActivities()]).catch(() => undefined);
}
