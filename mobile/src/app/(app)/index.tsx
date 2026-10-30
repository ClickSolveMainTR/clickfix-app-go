import { Redirect } from 'expo-router';

import { useSession } from '@/core/auth/session';

/**
 * Signed-in entry for `/`. The sign-in screen owns `/` only while signed out,
 * so after sign-in (or a deep link to the root) send the user to the inbox,
 * or to the organization picker until one is chosen.
 */
export default function AppIndex() {
  const hasOrganization = useSession((s) => !!s.organization);
  return <Redirect href={hasOrganization ? '/inbox' : '/organizations'} />;
}
