import * as WebBrowser from 'expo-web-browser';

import { WEB_URL } from '@/core/config';
import { i18n } from '@/core/i18n';

/** Public legal pages on the web app (`frontend/src/features/landing/constants.ts` LEGAL_DOCS). */
const PATHS = { privacy: '/legal/privacy', terms: '/legal/terms', kvkk: '/legal/data-protection' } as const;

export type LegalPage = keyof typeof PATHS;

/** Opens a legal page in the in-app browser, in the app's language. */
export function openLegalPage(page: LegalPage) {
  const lang = i18n.language === 'en' ? '?lang=en' : '';
  return WebBrowser.openBrowserAsync(`${WEB_URL}${PATHS[page]}${lang}`);
}
