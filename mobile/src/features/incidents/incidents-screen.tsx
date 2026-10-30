import { Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { RefreshControl, View } from 'react-native';

import { formatRelative, formatShort } from '@/core/format';
import { Card } from '@/ui/card';
import { EmptyState } from '@/ui/empty-state';
import { SectionHeader } from '@/ui/list';
import { useManualRefresh } from '@/ui/use-manual-refresh';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { type Incident, useIncidents } from './api';

/** Status page incidents: ongoing first with their latest updates, then recently resolved. */
export function IncidentsScreen() {
  const { t } = useTranslation('work');
  const incidents = useIncidents();
  const refresh = useManualRefresh(incidents);
  return (
    <>
      <Stack.Screen options={{ title: t('incidents.title') }} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <QueryState query={incidents}>
          {(d) => {
            const items = d.items ?? [];
            const active = items.filter((i) => !i.resolved_at);
            const past = items.filter((i) => i.resolved_at);
            if (!items.length)
              return (
                <EmptyState icon="check" title={t('incidents.empty')} body={t('incidents.empty_hint')} />
              );
            return (
              <View className="gap-3">
                {active.length ? <SectionHeader title={t('incidents.active')} /> : null}
                {active.map((i) => (
                  <IncidentCard key={i.uuid} incident={i} />
                ))}
                {past.length ? <SectionHeader title={t('incidents.resolved')} /> : null}
                {past.map((i) => (
                  <IncidentCard key={i.uuid} incident={i} />
                ))}
              </View>
            );
          }}
        </QueryState>
      </Screen>
    </>
  );
}

function IncidentCard({ incident: i }: { incident: Incident }) {
  const { t } = useTranslation('work');
  const updates = [...(i.updates ?? [])]
    .sort((a, b) => (b.created_at ?? '').localeCompare(a.created_at ?? ''))
    .slice(0, 3);
  return (
    <Card>
      <View className="flex-row flex-wrap items-center gap-2">
        <StatusPill
          label={i.resolved_at ? t('incidents.state_resolved') : (i.status ?? '')}
          tone={i.resolved_at ? 'done' : 'problem'}
        />
        {i.severity ? <StatusPill label={i.severity} /> : null}
        {i.public ? <StatusPill label={t('incidents.public')} /> : null}
      </View>
      <Text variant="headline">{i.title}</Text>
      <Text variant="footnote" tone="muted">
        {[
          i.started_at ? t('incidents.started', { at: formatShort(i.started_at) }) : null,
          i.resolved_at ? t('incidents.resolved_at', { at: formatShort(i.resolved_at) }) : null,
        ]
          .filter(Boolean)
          .join(' · ')}
      </Text>
      {updates.map((u) => (
        <View key={u.uuid} className="gap-0.5 border-l-2 border-border pl-3">
          <Text variant="footnote" className="font-sans-semibold">
            {[u.status, u.created_at ? formatRelative(u.created_at) : null].filter(Boolean).join(' · ')}
          </Text>
          {u.body ? (
            <Text variant="footnote" selectable>
              {u.body}
            </Text>
          ) : null}
        </View>
      ))}
    </Card>
  );
}
