import type { UseQueryResult } from '@tanstack/react-query';
import { type Href, router, Stack, useFocusEffect } from 'expo-router';
import * as WebBrowser from 'expo-web-browser';
import { type ReactNode, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, RefreshControl, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { P, useCan } from '@/core/auth/permissions';
import { useSession } from '@/core/auth/session';
import { formatNumber, formatRelative, formatTime } from '@/core/format';
import { errorMessage } from '@/core/i18n';
import { useUpcomingAppointments } from '@/features/appointments/api';
import { WEB_URL } from '@/core/config';
import { useInboxCounts, useInboxLive } from '@/features/inbox/api';
import { useIncidents } from '@/features/incidents/api';
import { type ApiAppointment, todayAppointments } from '@/features/widgets/snapshot-core';
import { cn } from '@/ui/cn';
import type { IconName } from '@/ui/icons';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Skeleton } from '@/ui/skeleton';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { useManualRefresh } from '@/ui/use-manual-refresh';

import { useOnboardingState, useSlaAtRisk, useUnreadCount } from './api';
import { countTiles, firstName, type PendingKey, pendingItems, pendingView } from './core';

/** Where a pending row leads; the set-up checklist lives on the panel's home. */
const PENDING: Record<PendingKey, { icon: IconName; open: () => void }> = {
  incidents: { icon: 'megaphone', open: () => router.push('/more/incidents' as Href) },
  notifications: { icon: 'bell', open: () => router.push('/notifications' as Href) },
  setup: { icon: 'checklist', open: () => void WebBrowser.openBrowserAsync(`${WEB_URL}/dashboard`) },
};

/**
 * Home summary (CF-167): the open conversations, the SLA clocks about to
 * run out and what still waits for the member, on one screen. The panel's
 * home (`frontend/src/app/(cms)/dashboard/page.tsx`) greets the member and
 * shows the setup checklist and the next appointments; the inbox and SLA
 * numbers are the ones the inbox and the home-screen widget already read.
 * A card whose permission the member lacks is not shown.
 */
export function DashboardScreen() {
  const { t } = useTranslation('hub');
  const can = useCan();
  const me = useSession((s) => s.me);
  const org = useSession((s) => s.organization);
  const canInbox = can(P.conversationsRead);
  const canAppointments = can(P.appointmentsRead);

  const counts = useInboxCounts();
  const sla = useSlaAtRisk();
  const appointments = useUpcomingAppointments('mine');
  const incidents = useIncidents();
  const setup = useOnboardingState();
  const unread = useUnreadCount();

  // Messages and status changes reach the summary while it is open...
  useInboxLive();
  // ...and coming back from a conversation re-reads the SLA rows and the numbers.
  const refetchSla = sla.refetch;
  const refetchCounts = counts.refetch;
  useFocusEffect(
    useCallback(() => {
      if (!canInbox) return;
      void refetchSla();
      void refetchCounts();
    }, [canInbox, refetchSla, refetchCounts]),
  );

  const queries = [counts, sla, appointments, incidents, setup, unread];
  // Only what this member may read was ever started; the rest stays off.
  const started = (q: { fetchStatus: string; isFetched: boolean }) => q.fetchStatus !== 'idle' || q.isFetched;
  const refresh = useManualRefresh({
    isRefetching: queries.some((q) => q.isRefetching),
    refetch: () => Promise.all(queries.filter(started).map((q) => q.refetch())),
  });

  const name = firstName(me?.user?.name);
  const today = todayAppointments((appointments.data?.items ?? []) as ApiAppointment[], new Date());
  const pendingSources = [incidents, setup, unread];
  const pending = pendingItems({
    incidents: canInbox && incidents.data ? (incidents.data.items ?? []).filter((i) => !i.resolved_at).length : null,
    unread: unread.data ? Number(unread.data.count ?? 0) : null,
    setup: setup.data ?? null,
  });
  const pendingState = pendingView(
    pending.length,
    pendingSources.map((q) => ({ enabled: started(q), loaded: q.data !== undefined, failed: q.isError })),
  );

  return (
    <>
      <Stack.Screen options={{ title: t('dashboard.title') }} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <View className="gap-0.5 px-4">
          <Text variant="headline">{name ? t('dashboard.greeting', { name }) : t('dashboard.title')}</Text>
          {org ? (
            <Text variant="footnote" tone="muted">
              {org.name}
            </Text>
          ) : null}
        </View>

        {canInbox ? (
          <View>
            <SectionHeader title={t('dashboard.conversations')} className="pt-1" />
            <Loaded query={counts} skeleton="h-40">
              {(data) => (
                <View className="flex-row flex-wrap gap-3">
                  {countTiles(data).map((tile) => (
                    <Pressable
                      key={tile.key}
                      accessibilityRole="button"
                      accessibilityLabel={`${t(`dashboard.count_${tile.key}`)}, ${formatNumber(tile.value)}`}
                      onPress={() => router.push('/inbox')}
                      className="min-w-[46%] flex-1 gap-1 rounded-lg bg-card p-4 active:opacity-80"
                      style={{ borderCurve: 'continuous' }}>
                      <Text variant="footnote" tone="muted" numberOfLines={1}>
                        {t(`dashboard.count_${tile.key}`)}
                      </Text>
                      <Text
                        variant="title"
                        className={cn('tabular-nums', tile.alert && 'text-destructive')}
                        numberOfLines={1}
                        adjustsFontSizeToFit>
                        {formatNumber(tile.value)}
                      </Text>
                    </Pressable>
                  ))}
                </View>
              )}
            </Loaded>
          </View>
        ) : null}

        {canInbox ? (
          <View>
            <SectionHeader title={t('dashboard.sla')} className="pt-1" />
            <Loaded query={sla} skeleton="h-24">
              {(rows) =>
                rows.length ? (
                  <List>
                    {rows.map((row, i) => (
                      <ListRow
                        key={row.uuid}
                        icon={row.breached ? 'timerOff' : 'timer'}
                        tone={row.breached ? 'danger' : 'default'}
                        title={row.contact || t('dashboard.unknown_contact')}
                        subtitle={[row.subject, formatRelative(row.due_at)].filter(Boolean).join(' · ')}
                        trailing={
                          <StatusPill
                            label={t(row.breached ? 'dashboard.sla_breached' : 'dashboard.sla_due')}
                            tone={row.breached ? 'problem' : 'progress'}
                          />
                        }
                        onPress={() => router.push(`/conversation/${row.uuid}`)}
                        last={i === rows.length - 1}
                      />
                    ))}
                  </List>
                ) : (
                  <Note icon="accept" text={t('dashboard.sla_empty')} />
                )
              }
            </Loaded>
          </View>
        ) : null}

        {/* Like the panel's widget: hidden when the module is off for the organization (403) or fails. */}
        {canAppointments && !appointments.isError ? (
          <View>
            <SectionHeader title={t('dashboard.today')} className="pt-1" />
            <Loaded query={appointments} skeleton="h-24">
              {() =>
                today.length ? (
                  <List>
                    {today.map((a, i) => (
                      <ListRow
                        key={a.uuid}
                        icon="calendar"
                        title={a.title || a.with || t('dashboard.appointment')}
                        subtitle={[`${formatTime(a.starts_at)} – ${formatTime(a.ends_at)}`, a.title ? a.with : '']
                          .filter(Boolean)
                          .join(' · ')}
                        onPress={() => router.push(`/appointment/${a.uuid}`)}
                        last={i === today.length - 1}
                      />
                    ))}
                  </List>
                ) : (
                  <Note icon="calendar" text={t('dashboard.today_empty')} />
                )
              }
            </Loaded>
          </View>
        ) : null}

        <View>
          <SectionHeader title={t('dashboard.pending')} className="pt-1" />
          {pendingState === 'list' ? (
            <List>
              {pending.map((p, i) => (
                <ListRow
                  key={p.key}
                  icon={PENDING[p.key].icon}
                  title={t(`dashboard.pending_${p.key}`, { count: p.count })}
                  onPress={PENDING[p.key].open}
                  last={i === pending.length - 1}
                />
              ))}
            </List>
          ) : pendingState === 'loading' ? (
            <Skeleton className="h-14" />
          ) : pendingState === 'error' ? (
            <List>
              <ListRow icon="error" tone="danger" title={t('dashboard.load_failed')} titleLines={2} />
              <ListRow
                icon="refresh"
                title={t('common:retry')}
                onPress={() => pendingSources.filter((q) => q.isError).forEach((q) => void q.refetch())}
                last
              />
            </List>
          ) : (
            <Note icon="accept" text={t('dashboard.pending_empty')} />
          )}
        </View>
      </Screen>
    </>
  );
}

/** One card's own loading, error (with retry) and content, so a failing card never hides the others. */
function Loaded<D>({
  query,
  skeleton,
  children,
}: {
  query: UseQueryResult<D>;
  skeleton: string;
  children: (data: D) => ReactNode;
}) {
  const { t } = useTranslation('hub');
  if (query.data !== undefined) return <>{children(query.data)}</>;
  if (query.isError) {
    const e = query.error;
    return (
      <List>
        <ListRow
          icon="error"
          tone="danger"
          title={t('dashboard.load_failed')}
          titleLines={2}
          subtitle={errorMessage(isApiError(e) ? e.code : undefined)}
          last
        />
        <ListRow icon="refresh" title={t('common:retry')} onPress={() => void query.refetch()} last />
      </List>
    );
  }
  return <Skeleton className={skeleton} />;
}

function Note({ icon, text }: { icon: IconName; text: string }) {
  return (
    <List>
      <ListRow icon={icon} title={text} titleLines={3} last />
    </List>
  );
}
