import { router, Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { RefreshControl, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { formatRelative, formatShort } from '@/core/format';
import { errorMessage } from '@/core/i18n';
import { Avatar } from '@/ui/avatar';
import { Button } from '@/ui/button';
import { Card } from '@/ui/card';
import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { useManualRefresh } from '@/ui/use-manual-refresh';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import {
  type OncallAlert,
  type OncallRota,
  useAcknowledgeAlert,
  useOncallAlerts,
  useOncallNow,
  useOncallScope,
  useRotas,
} from './api';
import { useMemberNames } from './names';

/**
 * On-call (CF-104): open alerts first, acknowledged in one tap (stops the
 * escalation), then each rota with who is on call now.
 */
export function OncallScreen() {
  const { t } = useTranslation('team');
  const alerts = useOncallAlerts();
  const rotas = useRotas();
  const refresh = useManualRefresh({
    isRefetching: alerts.isRefetching || rotas.isRefetching,
    refetch: () => Promise.all([alerts.refetch(), rotas.refetch()]),
  });
  const open = (alerts.data?.items ?? []).filter((a) => !a.resolved_at);
  return (
    <>
      <Stack.Screen options={{ title: t('oncall.title') }} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <SectionHeader title={t('oncall.alerts')} />
        <QueryState query={alerts} rows={2}>
          {() =>
            open.length ? (
              <View className="gap-3">
                {open.map((a) => (
                  <AlertCard key={a.uuid} alert={a} />
                ))}
              </View>
            ) : (
              <EmptyState icon="check" title={t('oncall.no_alerts')} />
            )
          }
        </QueryState>
        <SectionHeader title={t('oncall.rotas')} />
        <QueryState query={rotas} rows={2}>
          {(d) =>
            d.items.length ? (
              <View className="gap-3">
                {d.items.map((r) => (
                  <RotaCard key={r.uuid} rota={r} />
                ))}
              </View>
            ) : (
              <EmptyState icon="alert" title={t('oncall.no_rotas')} />
            )
          }
        </QueryState>
        <SetupLinks />
      </Screen>
    </>
  );
}

/**
 * Read-only views of the on-call setup (CF-168): escalation policies, uptime
 * monitors and status components. A row is listed only for a member who may
 * read what it opens.
 */
function SetupLinks() {
  const { t } = useTranslation('team');
  const { canSetup, canComponents } = useOncallScope();
  const rows = [
    { key: 'policies', icon: 'alert', to: '/more/oncall-policies', show: canSetup },
    { key: 'monitors', icon: 'chart', to: '/more/oncall-monitors', show: canSetup },
    { key: 'components', icon: 'check', to: '/more/status-components', show: canComponents },
  ] as const;
  const shown = rows.filter((r) => r.show);
  if (!shown.length) return null;
  return (
    <View>
      <SectionHeader title={t('setup.title')} />
      <List>
        {shown.map((r, i) => (
          <ListRow
            key={r.key}
            icon={r.icon}
            title={t(`setup.${r.key}`)}
            subtitle={t(`setup.${r.key}_hint`)}
            onPress={() => router.push(r.to)}
            last={i === shown.length - 1}
          />
        ))}
      </List>
    </View>
  );
}

function AlertCard({ alert: a }: { alert: OncallAlert }) {
  const { t } = useTranslation('team');
  const ack = useAcknowledgeAlert();
  const acked = !!a.acknowledged_at;
  return (
    <Card>
      <View className="flex-row items-center gap-2">
        <Icon name="alert" size={18} tone={acked ? 'muted-foreground' : 'destructive'} />
        <Text variant="headline" className="flex-1">
          {a.incident_uuid
            ? t('oncall.alert_incident')
            : a.conversation_uuid
              ? t('oncall.alert_conversation')
              : t('oncall.alert')}
        </Text>
        <StatusPill
          label={acked ? t('oncall.acknowledged') : t('oncall.open')}
          tone={acked ? 'done' : 'problem'}
        />
      </View>
      <Text variant="footnote" tone="muted">
        {[
          t('oncall.started', { at: formatRelative(a.created_at) }),
          acked ? null : t('oncall.next_step', { step: a.next_step, at: formatShort(a.next_notify_at) }),
        ]
          .filter(Boolean)
          .join(' · ')}
      </Text>
      <View className="flex-row gap-2">
        {!acked ? (
          <Button
            title={t('oncall.acknowledge')}
            className="flex-1"
            loading={ack.isPending}
            onPress={() =>
              ack.mutate(a.uuid, {
                onSuccess: () => toast.success(t('oncall.acknowledged')),
                onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)),
              })
            }
          />
        ) : null}
        {a.conversation_uuid ? (
          <Button
            title={t('oncall.open_conversation')}
            variant="outline"
            className="flex-1"
            onPress={() => router.push(`/conversation/${a.conversation_uuid}`)}
          />
        ) : null}
      </View>
    </Card>
  );
}

function RotaCard({ rota: r }: { rota: OncallRota }) {
  const { t } = useTranslation('team');
  const now = useOncallNow(r.uuid);
  const nameOf = useMemberNames();
  const agents = now.data?.agents ?? [];
  return (
    <Card title={`${r.name} · ${r.timezone}`}>
      {agents.length ? (
        agents.map((a) => (
          <View key={a} className="flex-row items-center gap-3">
            <Avatar name={nameOf(a)} size={32} />
            <Text className="flex-1">{nameOf(a)}</Text>
            <StatusPill label={t('oncall.now')} tone="progress" />
          </View>
        ))
      ) : (
        <Text tone="muted">{now.isPending ? t('common:loading') : t('oncall.nobody')}</Text>
      )}
    </Card>
  );
}
