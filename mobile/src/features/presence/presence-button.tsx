import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { cn } from '@/ui/cn';
import { OptionSheet } from '@/ui/select';
import { toast } from '@/ui/toast';

import { PRESENCE_STATUSES, type PresenceStatus, useMyPresence, useSetPresence } from './presence';

const DOT: Record<PresenceStatus, string> = {
  online: 'bg-success',
  away: 'bg-warning',
  busy: 'bg-destructive',
  offline: 'bg-muted-foreground',
};

/**
 * The agent's status from the inbox header (CF-82): a dot in the status
 * colour; a tap picks available / break / busy / offline (CF-54 API).
 */
export function PresenceButton() {
  const { t } = useTranslation('settings');
  const me = useMyPresence();
  const set = useSetPresence();
  const [open, setOpen] = useState(false);
  if (!me.data) return null;
  const current = me.data.manual_status;
  return (
    <>
      <OptionSheet<PresenceStatus>
        trigger={
          <Pressable
            accessibilityRole="button"
            accessibilityLabel={`${t('presence')}: ${t(`presence_${current}`)}`}
            hitSlop={8}
            onPress={() => setOpen(true)}
            className="size-9 items-center justify-center active:opacity-60">
            <View className="size-5 items-center justify-center rounded-full border-2 border-border">
              <View className={cn('size-3 rounded-full', DOT[current])} />
            </View>
          </Pressable>
        }
        visible={open}
        title={t('presence')}
        value={current}
        options={PRESENCE_STATUSES.map((s) => ({
          value: s,
          label: t(`presence_${s}`),
          description: s === 'online' ? t('presence_hint') : undefined,
        }))}
        onClose={() => setOpen(false)}
        onPick={(s) => {
          setOpen(false);
          set.mutate(s, { onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)) });
        }}
      />
    </>
  );
}
