import { useState } from 'react';
import { useReducedMotion } from 'react-native-reanimated';
import { SelectionChromeContext } from '@/features/shell/selection-chrome';
import { NativeTabs } from 'expo-router/native-tabs';
import { useTranslation } from 'react-i18next';

import { P, useCan } from '@/core/auth/permissions';
import { useTabTint } from '@/features/shell/tab-tint';
import { useInboxCounts } from '@/features/inbox/api';
import { useUnreadNotifications } from '@/features/notifications/api';

const badge = (n: number) => (n > 99 ? '99+' : String(n));

/**
 * Native tabs (CF-79, CF-138): Inbox, Contacts, Agenda (members who can read
 * appointments), Notifications and "More" for search, the team, reports,
 * video, admin and settings. Five at most, so iOS never folds a tab into
 * its own "More". Inbox shows my open conversations, Notifications the unread.
 */
export default function Tabs() {
  const { t } = useTranslation('nav');
  const [selectionHidden, setSelectionHidden] = useState(false);
  const reducedMotion = useReducedMotion();
  const tint = useTabTint();
  const can = useCan();
  const mine = useInboxCounts().data?.mine ?? 0;
  const unread = useUnreadNotifications();
  return (
    // Each tab contains a native stack whose scroll views own their header insets.
    <SelectionChromeContext value={setSelectionHidden}>
      <NativeTabs tintColor={tint} hidden={selectionHidden} disableHiddenAnimation={reducedMotion}>
        <NativeTabs.Trigger disableAutomaticContentInsets name="inbox">
          <NativeTabs.Trigger.Icon sf="tray.fill" md="inbox" />
          <NativeTabs.Trigger.Label>{t('inbox')}</NativeTabs.Trigger.Label>
          {mine > 0 ? <NativeTabs.Trigger.Badge>{badge(mine)}</NativeTabs.Trigger.Badge> : null}
        </NativeTabs.Trigger>
        <NativeTabs.Trigger disableAutomaticContentInsets name="contacts">
          <NativeTabs.Trigger.Icon sf="person.2.fill" md="group" />
          <NativeTabs.Trigger.Label>{t('contacts')}</NativeTabs.Trigger.Label>
        </NativeTabs.Trigger>
        <NativeTabs.Trigger disableAutomaticContentInsets name="agenda" hidden={!can(P.appointmentsRead)}>
          <NativeTabs.Trigger.Icon sf="calendar" md="calendar_month" />
          <NativeTabs.Trigger.Label>{t('agenda')}</NativeTabs.Trigger.Label>
        </NativeTabs.Trigger>
        <NativeTabs.Trigger disableAutomaticContentInsets name="notifications">
          <NativeTabs.Trigger.Icon sf="bell.fill" md="notifications" />
          <NativeTabs.Trigger.Label>{t('notifications')}</NativeTabs.Trigger.Label>
          {unread > 0 ? <NativeTabs.Trigger.Badge>{badge(unread)}</NativeTabs.Trigger.Badge> : null}
        </NativeTabs.Trigger>
        <NativeTabs.Trigger disableAutomaticContentInsets name="more">
          <NativeTabs.Trigger.Icon sf="ellipsis.circle.fill" md="more_horiz" />
          <NativeTabs.Trigger.Label>{t('more')}</NativeTabs.Trigger.Label>
        </NativeTabs.Trigger>
      </NativeTabs>
    </SelectionChromeContext>
  );
}
