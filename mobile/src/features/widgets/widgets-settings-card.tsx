import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Platform, Switch, View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import { liveActivitiesEnabled, saveLiveActivitiesSetting, setLiveActivitiesEnabledLocal } from './live-activities';

const KEY = ['mobile', 'live-activity-settings'] as const;

/**
 * Settings → Widgets and Live Activities (CF-152): the Live Activities switch
 * (stored on the server, so the backend stops pushing them too) and where the
 * widgets come from.
 */
export function WidgetsSettingsCard() {
  const { t } = useTranslation('settings');
  const qc = useQueryClient();
  const [busy, setBusy] = useState(false);
  const ios = Platform.OS === 'ios';
  const settings = useQuery({
    queryKey: KEY,
    enabled: ios,
    queryFn: async () => {
      const res = await unwrap(api.GET('/v1/mobile/live-activity-settings'));
      setLiveActivitiesEnabledLocal(res.enabled);
      return res;
    },
    initialData: { enabled: liveActivitiesEnabled() },
    initialDataUpdatedAt: 0,
  });
  const enabled = settings.data?.enabled ?? true;

  return (
    <View>
      <SectionHeader title={t('widgets_title')} />
      <List>
        {ios ? (
          <ListRow
            iconTile
            icon="device"
            title={t('live_activities')}
            subtitle={t('live_activities_hint')}
            titleLines={2}
            trailing={
              <Switch
                accessibilityLabel={t('live_activities')}
                value={enabled}
                disabled={busy}
                onValueChange={async (value) => {
                  setBusy(true);
                  qc.setQueryData(KEY, { enabled: value });
                  try {
                    await saveLiveActivitiesSetting(value);
                  } catch {
                    qc.setQueryData(KEY, { enabled: !value });
                    toast.error(t('live_activities_failed'));
                  } finally {
                    setBusy(false);
                  }
                }}
              />
            }
          />
        ) : null}
        <ListRow
          iconTile
          icon="grid"
          title={t('widgets_add')}
          subtitle={<Text tone="muted">{ios ? t('widgets_add_ios') : t('widgets_add_android')}</Text>}
          titleLines={2}
          last
        />
      </List>
    </View>
  );
}
