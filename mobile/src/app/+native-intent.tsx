import { appRoute } from '@/features/push/links';

/**
 * Maps the panel's https links to app routes when the OS hands one to the app
 * (universal / app links, once the domain is associated at release, or a QR
 * scanned with the system camera): `{panel}/login/qr/{id}` opens the QR
 * sign-in approval, `{panel}/inbox?c={uuid}` the conversation.
 */
export function redirectSystemPath({ path }: { path: string; initial: boolean }) {
  // Not a link we know: the router handles it.
  return appRoute(path) ?? path;
}
