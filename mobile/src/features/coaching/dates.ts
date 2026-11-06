import { formatDateTime } from '@/core/date-format';
import { appLocale, displayTimeZone } from '@/core/format';

import { otherYear } from './core';

/** "6 Kas", or "6 Kas 2025" for a date outside the current year; "—" when there is none. */
export function coachingDate(iso: string | null | undefined) {
  return (
    formatDateTime(
      appLocale(),
      { day: 'numeric', month: 'short', ...(otherYear(iso) ? { year: 'numeric' } : {}), timeZone: displayTimeZone() },
      iso,
    ) || '—'
  );
}
