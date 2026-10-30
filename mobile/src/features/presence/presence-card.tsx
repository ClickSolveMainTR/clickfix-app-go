import { useTranslation } from 'react-i18next';

import { Card } from '@/ui/card';
import { Chips } from '@/ui/chips';
import { Text } from '@/ui/text';

import { PRESENCE_STATUSES, type PresenceStatus, useMyPresence, useSetPresence } from './presence';

/** "My status": available, on a break, busy or offline (CF-54). */
export function PresenceCard() {
  const { t } = useTranslation('settings');
  const me = useMyPresence();
  const set = useSetPresence();
  if (!me.data) return null;
  return (
    <Card title={t('presence')}>
      <Chips<PresenceStatus>
        value={me.data.manual_status}
        onChange={(v) => set.mutate(v)}
        options={PRESENCE_STATUSES.map((s) => ({ value: s, label: t(`presence_${s}`) }))}
      />
      <Text tone="muted">{t('presence_hint')}</Text>
    </Card>
  );
}
