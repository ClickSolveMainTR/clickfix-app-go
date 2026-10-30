import { router } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';

import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

import { useActiveIncidents } from './api';

/** "Ongoing incident" strip at the top of the inbox (agents answer differently during an outage). */
export function IncidentBanner() {
  const { t } = useTranslation('work');
  const active = useActiveIncidents();
  if (!active.length) return null;
  const first = active[0];
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityHint={t('incidents.open_hint')}
      onPress={() => router.push('/more/incidents')}
      className="flex-row items-center gap-2 rounded-lg bg-destructive/12 px-3 py-2 active:opacity-80">
      <Icon name="alert" size={16} tone="destructive" />
      <View className="flex-1">
        <Text variant="footnote" className="font-sans-semibold" numberOfLines={1}>
          {active.length > 1 ? t('incidents.banner_many', { count: active.length }) : t('incidents.banner_one')}
        </Text>
        <Text variant="footnote" tone="muted" numberOfLines={1}>
          {first.title}
        </Text>
      </View>
      <Icon name="forward" size={14} tone="muted-foreground" />
    </Pressable>
  );
}
