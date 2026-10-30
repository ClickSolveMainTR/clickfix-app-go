import { useCSSVariable } from 'uniwind';
import { HeaderActions } from '@/features/guides/help-menu';
import { GuideTarget } from '@/ui/guide-target';
import { isApiError } from '@/core/api/errors';
import { formatTime } from '@/core/format';
import { errorMessage } from '@/core/i18n';
import { useHeaderHeight } from 'expo-router/react-navigation';
import { useCompactHeader } from '@/features/shell/compact-header';
import { useHeaderScroll } from '@/features/shell/header-scroll';
import { EmptyState } from '@/ui/empty-state';
import { HeaderButton } from '@/ui/header-button';
import { ActionMenu } from '@/ui/menu';
import { QueryState } from '@/ui/query-state';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';
import { useManualRefresh } from '@/ui/use-manual-refresh';
import { Stack, router } from 'expo-router';
import { memo, useEffect, useMemo, useRef, useState, type ComponentRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, FlatList, Pressable, RefreshControl, ScrollView, SectionList, View } from 'react-native';
import {
  useAppointmentScope,
  useAppointmentsLive,
  useCalendarAppointments,
  useSetAppointmentStatus,
  type AgendaScope,
  type Appointment,
} from './api';
import { addDays, layoutEvents, localDayKey, monthDays, sameDay, startOfWeek } from './calendar-core';
import { NewAppointmentSheet } from './new-appointment-sheet';

type Mode = 'day' | 'week' | 'month' | 'list';
function useEventColor() {
  const values = useCSSVariable([
    '--color-calendar-blue', '--color-calendar-emerald', '--color-calendar-amber',
    '--color-calendar-rose', '--color-calendar-violet', '--color-calendar-sky',
    '--color-calendar-orange', '--color-calendar-teal', '--color-calendar-pink',
    '--color-calendar-slate', '--color-calendar-cancelled',
  ]);
  const names = ['blue', 'emerald', 'amber', 'rose', 'violet', 'sky', 'orange', 'teal', 'pink', 'slate'];
  return (event: Appointment) => {
    if (event.status !== 'cancelled' && event.status !== 'no_show' && /^#[\da-f]{6}$/i.test(event.type.color)) return event.type.color;
    const index = event.status === 'cancelled' || event.status === 'no_show'
      ? 10 : (names.includes(event.type.color) ? names.indexOf(event.type.color) : 1);
    return typeof values[index] === 'string' ? values[index] as string : undefined;
  };
}
const openEvent = (event: Appointment) => router.push(`/appointment/${event.uuid}`);

function EventMenu({ event, children }: { event: Appointment; children: React.ReactElement }) {
  const { t } = useTranslation('hub');
  const { canWrite } = useAppointmentScope();
  const status = useSetAppointmentStatus(event.uuid);
  return (
    <ActionMenu
      context
      fullWidth
      title={event.title}
      trigger={children}
      options={[
        { value: 'open', label: t('agenda.open'), systemImage: 'calendar' },
        ...(event.video_room_uuid ? [{ value: 'video', label: t('agenda.join'), systemImage: 'video' as const }] : []),
        ...(canWrite
          ? [
              { value: 'reschedule', label: t('agenda.reschedule'), systemImage: 'calendar.badge.clock' as const },
              { value: 'cancel', label: t('agenda.cancel'), destructive: true, systemImage: 'xmark' as const },
            ]
          : []),
      ]}
      onPick={(value) => {
        if (value === 'video' && event.video_room_uuid) router.push(`/video/${event.video_room_uuid}`);
        else if (value === 'cancel')
          Alert.alert(t('agenda.cancel'), event.title, [
            { text: t('common:cancel'), style: 'cancel' },
            {
              text: t('common:confirm'),
              style: 'destructive',
              onPress: () =>
                status.mutate(
                  { status: 'cancelled' },
                  { onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)) },
                ),
            },
          ]);
        else
          router.push({
            pathname: '/appointment/[uuid]',
            params: { uuid: event.uuid, ...(value === 'reschedule' ? { reschedule: '1' } : {}) },
          });
      }}
    />
  );
}
const EventRow = memo(function EventRow({ event }: { event: Appointment }) {
  const colorOf = useEventColor();
  return (
    <EventMenu event={event}>
      <Pressable
        onPress={() => openEvent(event)}
        accessibilityRole="button"
        accessibilityLabel={`${event.title}, ${formatTime(event.starts_at)}`}
        className="mx-4 flex-row gap-3 border-b border-border py-3">
        <View style={{ width: 4, borderRadius: 2, backgroundColor: colorOf(event) }} />
        <View className="w-14">
          <Text variant="footnote">{formatTime(event.starts_at)}</Text>
          <Text variant="caption" tone="muted">
            {formatTime(event.ends_at)}
          </Text>
        </View>
        <View className="min-w-0 flex-1">
          <Text numberOfLines={2} className="font-sans-semibold">
            {event.title}
          </Text>
          <Text variant="footnote" tone="muted">
            {[event.type.name, event.location_value, event.invitee.name].filter(Boolean).join(' · ')}
          </Text>
        </View>
      </Pressable>
    </EventMenu>
  );
});

const TimeGrid = memo(function TimeGrid({
  days,
  events,
  width,
}: {
  days: Date[];
  events: Appointment[];
  width: number;
}) {
  const colorOf = useEventColor();
  const reportScroll = useHeaderScroll();
  const scroll = useRef<ComponentRef<typeof ScrollView>>(null);
  const columns = useMemo(() => days.map((d) => layoutEvents(events, d)), [events, days]);
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const timer = setInterval(() => setNow(new Date()), 60_000);
    return () => clearInterval(timer);
  }, []);
  const columnWidth = (width - 44) / days.length;
  return (
    <ScrollView
      scrollEventThrottle={16}
      onScroll={(event) => reportScroll(event.nativeEvent.contentOffset.y - 8 * 60)}
      ref={scroll}
      onLayout={() => scroll.current?.scrollTo({ y: 8 * 60, animated: false })}
      className="flex-1"
      contentContainerStyle={{ height: 1440 }}>
      {Array.from({ length: 24 }, (_, hour) => (
        <View
          key={hour}
          style={{ position: 'absolute', top: hour * 60, left: 0, right: 0 }}
          className="flex-row items-start">
          <Text variant="caption" tone="muted" className="w-11 text-center">
            {String(hour).padStart(2, '0')}:00
          </Text>
          <View className="flex-1 border-t border-border" />
        </View>
      ))}
      {days.map((day, index) => (
        <View
          key={localDayKey(day)}
          style={{ position: 'absolute', left: 44 + index * columnWidth, width: columnWidth, height: 1440 }}
          className="border-l border-border">
          {columns[index].map(({ event, top, height, column, columns }) => (
            <View
              key={event.uuid}
              style={{
                position: 'absolute',
                top,
                height,
                left: (column / columns) * columnWidth + 1,
                width: columnWidth / columns - 2,
              }}>
              <EventMenu event={event}>
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={`${event.title}, ${formatTime(event.starts_at)}–${formatTime(event.ends_at)}`}
                  onPress={() => openEvent(event)}
                  style={{ flex: 1, backgroundColor: colorOf(event), borderRadius: 6, padding: 3, overflow: 'hidden' }}>
                  <Text variant="caption" className="text-on-tile" numberOfLines={days.length > 1 ? 3 : 2}>
                    {event.title}
                  </Text>
                  {height > 42 ? (
                    <Text variant="caption" className="text-on-tile">
                      {formatTime(event.starts_at)}
                    </Text>
                  ) : null}
                </Pressable>
              </EventMenu>
            </View>
          ))}
          {sameDay(day, now) ? (
            <View
              pointerEvents="none"
              className="bg-destructive"
              style={{
                position: 'absolute',
                top: now.getHours() * 60 + now.getMinutes(),
                left: 0,
                right: 0,
                height: 2,
              }}
            />
          ) : null}
        </View>
      ))}
    </ScrollView>
  );
});

export function AgendaScreen() {
  const headerHeight = useHeaderHeight();
  const colorOf = useEventColor();
  const reportScroll = useHeaderScroll();
  const { t, i18n } = useTranslation('hub');
  const compactHeader = useCompactHeader(t('nav:agenda'));
  const locale = i18n.language;
  const monday = locale.startsWith('tr');
  const [mode, setMode] = useState<Mode>('month');
  const [selected, setSelected] = useState(() => new Date());
  const [scope, setScope] = useState<AgendaScope>('mine');
  const [creating, setCreating] = useState(false);
  const [width, setWidth] = useState(0);
  const pages = useRef<FlatList<number>>(null);
  const { canRead, canWrite } = useAppointmentScope();
  const from = new Date(selected.getFullYear(), selected.getMonth() - 1, 1).toISOString();
  const to = new Date(selected.getFullYear(), selected.getMonth() + 3, 1).toISOString();
  const query = useCalendarAppointments(scope, from, to);
  useAppointmentsLive();
  const refresh = useManualRefresh(query);
  const events = useMemo(() => query.data?.items ?? [], [query.data]);
  const byDay = useMemo(() => {
    const map = new Map<string, Appointment[]>();
    for (const event of events) {
      const start = new Date(event.starts_at);
      for (
        let day = new Date(start.getFullYear(), start.getMonth(), start.getDate());
        day.getTime() < Date.parse(event.ends_at);
        day = addDays(day, 1)
      ) {
        const key = localDayKey(day);
        map.set(key, [...(map.get(key) ?? []), event]);
      }
    }
    return map;
  }, [events]);
  const dayEvents = byDay.get(localDayKey(selected)) ?? [];
  const title = selected.toLocaleDateString(locale, { month: 'long', year: 'numeric' });
  const modes: Mode[] = ['day', 'week', 'month', 'list'];
  const shift = (amount: number) => {
    setSelected((date) =>
      mode === 'month'
        ? new Date(date.getFullYear(), date.getMonth() + amount, 1)
        : addDays(date, amount * (mode === 'week' ? 7 : 1)),
    );
    requestAnimationFrame(() => pages.current?.scrollToOffset({ offset: width, animated: false }));
  };
  const dayLabel = (date: Date) =>
    sameDay(date, new Date())
      ? t('agenda.today')
      : sameDay(date, addDays(new Date(), 1))
        ? t('agenda.tomorrow')
        : date.toLocaleDateString(locale, { weekday: 'long', month: 'long', day: 'numeric' });
  const sections = useMemo(
    () =>
      [...byDay.entries()]
        .filter(([, items]) => Date.parse(items[0].ends_at) >= new Date().setHours(0, 0, 0, 0))
        .map(([key, data]) => ({ key, data })),
    [byDay],
  );
  const week = startOfWeek(selected, monday);
  const strip = (
    <View className="flex-row border-b border-border px-1 py-2">
      {Array.from({ length: 7 }, (_, i) => addDays(week, i)).map((date) => (
        <Pressable
          key={date.toISOString()}
          accessibilityRole="button"
          accessibilityLabel={dayLabel(date)}
          accessibilityState={{ selected: sameDay(date, selected) }}
          onPress={() => setSelected(date)}
          className="flex-1 items-center gap-1">
          <Text variant="caption" tone="muted">
            {date.toLocaleDateString(locale, { weekday: 'narrow' })}
          </Text>
          <View
            className={`size-8 items-center justify-center rounded-full ${sameDay(date, new Date()) ? 'bg-primary' : sameDay(date, selected) ? 'border-2 border-primary' : ''}`}>
            <Text tone={sameDay(date, new Date()) ? 'onPrimary' : 'default'}>{date.getDate()}</Text>
          </View>
        </Pressable>
      ))}
    </View>
  );
  return (
    <>
      <Stack.Screen
        options={{
          ...compactHeader,
          headerTransparent: true,
          headerLargeTitle: false,
          headerRight: () => (
            <HeaderActions>
              {
                <View className="flex-row items-center">
                  <Pressable accessibilityRole="button" onPress={() => setSelected(new Date())} className="px-1 py-2">
                    <Text variant="footnote" tone="primary">
                      {t('agenda.today')}
                    </Text>
                  </Pressable>
                  <ActionMenu
                    title={t('agenda.view')}
                    value={mode}
                    trigger={
                      <HeaderButton icon="calendar" guide="agenda.scope" label={t('agenda.view')} onPress={() => {}} />
                    }
                    options={[
                      ...modes.map((value) => ({
                        value,
                        label: t(`agenda.view_${value}`),
                        systemImage: 'calendar' as const,
                      })),
                      {
                        value: 'scope',
                        label: scope === 'mine' ? t('appointments:mobile.mine') : t('appointments:mobile.all'),
                        systemImage: scope === 'mine' ? 'person' : 'person.3',
                        children: [
                          { value: 'mine', label: t('appointments:mobile.mine'), systemImage: 'person' },
                          { value: 'all', label: t('appointments:mobile.all'), systemImage: 'person.3' },
                        ],
                      },
                    ]}
                    onPick={(value) => {
                      if (value === 'mine' || value === 'all') setScope(value);
                      else if (modes.includes(value as Mode)) setMode(value as Mode);
                    }}
                  />
                  {canWrite ? (
                    <HeaderButton icon="add" label={t('agenda.create')} onPress={() => setCreating(true)} />
                  ) : null}
                </View>
              }
            </HeaderActions>
          ),
        }}
      />
      <NewAppointmentSheet visible={creating} onClose={() => setCreating(false)} />
      <GuideTarget id="agenda.list" className="flex-1">
        <View
          className="flex-1 bg-background"
          style={{ paddingTop: headerHeight }}
          onLayout={(e) => setWidth(e.nativeEvent.layout.width)}>
          <View className="flex-row items-center px-4 py-2">
            <Text accessibilityRole="header" className="flex-1 font-sans-bold">
              {title}
            </Text>
            <HeaderButton icon="back" label={t('agenda.previous')} onPress={() => shift(-1)} />
            <HeaderButton icon="forward" label={t('agenda.next')} onPress={() => shift(1)} />
          </View>
          {!canRead ? (
            <EmptyState icon="lockClosed" title={t('calendar:disabled.title')} />
          ) : (
            <QueryState query={query}>
              {() =>
                mode === 'list' ? (
                  <SectionList
                    scrollEventThrottle={16}
                    onScroll={(event) => reportScroll(event.nativeEvent.contentOffset.y)}
                    sections={sections}
                    keyExtractor={(item) => item.uuid}
                    stickySectionHeadersEnabled
                    refreshControl={<RefreshControl {...refresh} />}
                    renderSectionHeader={({ section }) => (
                      <Text className="bg-background px-4 py-2 font-sans-bold">
                        {dayLabel(new Date(section.data[0].starts_at))}
                      </Text>
                    )}
                    renderItem={({ item }) => <EventRow event={item} />}
                    ListEmptyComponent={
                      <EmptyState icon="calendar" title={t('calendar:widget.empty')} />
                    }
                  />
                ) : (
                  <>
                    {mode !== 'month' ? strip : null}
                    {width > 0 ? (
                      <FlatList
                        key={mode}
                        ref={pages}
                        horizontal
                        pagingEnabled
                        data={[-1, 0, 1]}
                        initialScrollIndex={1}
                        getItemLayout={(_, index) => ({ length: width, offset: width * index, index })}
                        showsHorizontalScrollIndicator={false}
                        style={mode === 'month' ? { height: 318, flexGrow: 0, flexShrink: 0 } : { flex: 1 }}
                        onMomentumScrollEnd={(event) => {
                          const delta = Math.round(event.nativeEvent.contentOffset.x / width) - 1;
                          if (delta) shift(delta);
                        }}
                        keyExtractor={String}
                        renderItem={({ item: offset }) => {
                          if (mode !== 'month') {
                            const first = mode === 'week' ? addDays(week, offset * 7) : addDays(selected, offset);
                            const days = Array.from({ length: mode === 'week' ? 7 : 1 }, (_, i) => addDays(first, i));
                            return (
                              <View style={{ width }}>
                                <TimeGrid days={days} events={events} width={width} />
                              </View>
                            );
                          }
                          const month = new Date(selected.getFullYear(), selected.getMonth() + offset, 1);
                          const days = monthDays(month, monday);
                          return (
                            <View style={{ width }}>
                              <View className="flex-row py-1">
                                {days.slice(0, 7).map((date) => (
                                  <Text
                                    key={date.toISOString()}
                                    variant="caption"
                                    tone="muted"
                                    className="flex-1 text-center">
                                    {date.toLocaleDateString(locale, { weekday: 'short' })}
                                  </Text>
                                ))}
                              </View>
                              <View className="flex-row flex-wrap">
                                {days.map((date) => (
                                  <Pressable
                                    key={date.toISOString()}
                                    accessibilityRole="button"
                                    accessibilityLabel={`${dayLabel(date)}, ${(byDay.get(localDayKey(date)) ?? []).length} ${t('agenda.events')}`}
                                    accessibilityState={{ selected: sameDay(date, selected) }}
                                    onPress={() => setSelected(date)}
                                    style={{
                                      width: width / 7,
                                      height: 49,
                                      opacity: date.getMonth() === month.getMonth() ? 1 : 0.35,
                                    }}
                                    className="items-center justify-center gap-1">
                                    <View
                                      className={`size-8 items-center justify-center rounded-full ${sameDay(date, new Date()) ? 'bg-primary' : sameDay(date, selected) ? 'border-2 border-primary' : ''}`}>
                                      <Text tone={sameDay(date, new Date()) ? 'onPrimary' : 'default'}>
                                        {date.getDate()}
                                      </Text>
                                    </View>
                                    <View className="h-1 flex-row gap-1">
                                      {(byDay.get(localDayKey(date)) ?? []).slice(0, 3).map((event) => (
                                        <View
                                          key={event.uuid}
                                          style={{
                                            width: 4,
                                            height: 4,
                                            borderRadius: 2,
                                            backgroundColor: colorOf(event),
                                          }}
                                        />
                                      ))}
                                    </View>
                                  </Pressable>
                                ))}
                              </View>
                            </View>
                          );
                        }}
                      />
                    ) : null}
                    {mode === 'month' ? (
                      <FlatList
                        scrollEventThrottle={16}
                        onScroll={(event) => reportScroll(event.nativeEvent.contentOffset.y)}
                        data={dayEvents}
                        keyExtractor={(item) => item.uuid}
                        ListHeaderComponent={
                          <Text className="border-t border-border px-4 py-3 font-sans-semibold">
                            {dayLabel(selected)}
                          </Text>
                        }
                        renderItem={({ item }) => <EventRow event={item} />}
                        ListEmptyComponent={
                          <EmptyState icon="calendar" title={t('calendar:widget.empty')} />
                        }
                        refreshControl={<RefreshControl {...refresh} />}
                      />
                    ) : null}
                  </>
                )
              }
            </QueryState>
          )}
        </View>
      </GuideTarget>
    </>
  );
}
