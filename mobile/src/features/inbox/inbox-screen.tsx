import { HeaderActions } from '@/features/guides/help-menu';
import { GuideTarget } from '@/ui/guide-target';
import { useCompactHeader } from '@/features/shell/compact-header';
import { router, Stack } from 'expo-router';
import * as Haptics from 'expo-haptics';
import { useSelectionChrome } from '@/features/shell/selection-chrome';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { useSession } from '@/core/auth/session';
import { formatShort } from '@/core/format';
import { errorMessage } from '@/core/i18n';
import { readJSON, writeJSON } from '@/core/storage';
import { useDebounced } from '@/core/use-debounced';
import { IncidentBanner } from '@/features/incidents/incident-banner';
import { PresenceButton } from '@/features/presence/presence-button';
import { Chips } from '@/ui/chips';
import { ConnectionBanner } from '@/ui/connection-banner';
import { EmptyState } from '@/ui/empty-state';
import { HeaderButton } from '@/ui/header-button';
import { Icon } from '@/ui/icon';
import type { IconName } from '@/ui/icons';
import { ActionMenu } from '@/ui/menu';
import { PagedList } from '@/ui/paged-list';
import { SearchField } from '@/ui/search-field';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import {
  type Conversation,
  useConversationList,
  useInboxCounts,
  useInboxLive,
  useInboxScope,
  useQuickUpdate,
  useSavedViews,
  useTags,
  useTeams,
} from './api';
import { BulkBar } from './bulk-bar';
import { ConversationRow, type RowAction, rowActions } from './conversation-row';
import { FilterSheet } from './filter-sheet';
import { activeFilterCount, type AdvancedFilters, type InboxFilters, NO_FILTERS, toggle } from './filters';
import { contactLabel, type InboxStatusTab, type InboxView, type SnoozePreset, snoozeUntil } from './helpers';

const VIEWS: InboxView[] = ['all', 'mine', 'unassigned', 'mentions'];
const STATUSES: InboxStatusTab[] = ['open', 'snoozed', 'resolved', 'all'];

type Stored = { view: InboxView; status: InboxStatusTab; adv: AdvancedFilters; saved: string | null };
const DEFAULTS: Stored = { view: 'all', status: 'open', adv: NO_FILTERS, saved: null };
const storeKey = (org: string) => `inbox.filters.${org}`;

/** View, tab and filters survive app restarts per organization (Front / Zendesk keep the last view). */
function useStoredFilters(org: string) {
  const load = (o: string): Stored => ({ ...DEFAULTS, ...(readJSON<Stored>(storeKey(o)) ?? {}) });
  const [state, setState] = useState(() => ({ org, value: load(org) }));
  // Another organization picked while the tab stays mounted: its own saved filters.
  if (state.org !== org) setState({ org, value: load(org) });
  const update = (patch: Partial<Stored>) =>
    setState((cur) => {
      const next = { ...cur.value, ...patch };
      writeJSON(storeKey(cur.org), next);
      return { org: cur.org, value: next };
    });
  return [state.org === org ? state.value : load(org), update] as const;
}

/**
 * The agent inbox (CF-79): saved views and the panel's quick views, status
 * tabs, search, the filter sheet (team, assignee, priority, channel, tag,
 * unread, SLA, sort), swipe actions, multi-select bulk actions with undo,
 * endless scroll and live updates. On a tablet it is the list pane; then
 * `onOpen` shows a conversation beside it instead of pushing it.
 */
export function InboxScreen({ onOpen, activeUuid }: { onOpen?: (uuid: string) => void; activeUuid?: string } = {}) {
  const { t } = useTranslation('inbox');
  const { org, canRead, canManage, canAssign } = useInboxScope();
  const me = useSession((st) => st.me?.user?.uuid);
  const [stored, update] = useStoredFilters(org);
  const { views: savedViews, counts: viewCounts } = useSavedViews();
  const saved = savedViews.find((v) => v.uuid === stored.saved) ?? null;
  const [search, setSearch] = useState('');
  const q = useDebounced(search, 300);
  const [filtersOpen, setFiltersOpen] = useState(false);
  const [selected, setSelected] = useState<string[] | null>(null);
  const filters: InboxFilters = {
    view: stored.view,
    status: stored.status,
    q,
    adv: stored.adv,
    saved: saved ? { uuid: saved.uuid, filters: saved.filters } : null,
  };
  const list = useConversationList(filters);
  const counts = useInboxCounts();
  useInboxLive(activeUuid);
  const quick = useQuickUpdate();
  const tags = useTags(stored.adv.tags.length > 0);
  const teams = useTeams(!!stored.adv.team && stored.adv.team !== 'mine');

  /**
   * Rows an action just moved out of this tab: hidden at once, back if the
   * call fails. They only apply to the tab they were hidden in and until a
   * page fetched after the action arrives.
   */
  const [hidden, setHidden] = useState<{ tab: string; since: number; ids: ReadonlySet<string> }>({
    tab: '',
    since: 0,
    ids: new Set(),
  });
  const tab = `${stored.view}|${stored.status}|${stored.saved}`;
  const activeHidden =
    hidden.tab === tab && (quick.isPending || list.dataUpdatedAt <= hidden.since) ? hidden.ids : null;
  const shown = useMemo(
    () =>
      activeHidden?.size && list.data
        ? {
            ...list,
            data: {
              ...list.data,
              pages: list.data.pages.map((p) => ({
                ...p,
                items: p.items.filter((c) => !activeHidden.has(c.uuid)),
              })),
            },
          }
        : list,
    [activeHidden, list],
  );

  const hide = (uuid: string, on: boolean) =>
    setHidden((cur) => {
      const ids = new Set(cur.tab === tab ? cur.ids : []);
      if (on) ids.add(uuid);
      else ids.delete(uuid);
      return { tab, since: list.dataUpdatedAt, ids };
    });

  const open = (uuid: string) => (onOpen ? onOpen(uuid) : router.push(`/conversation/${uuid}`));

  const apply = (c: Conversation, a: RowAction, preset?: SnoozePreset) => {
    if (a === 'open') return open(c.uuid);
    if (a === 'snooze' && !preset) return;
    const body =
      a === 'take'
        ? { assignee: me ?? null }
        : a === 'snooze' && preset
          ? { status: 'snoozed' as const, snoozed_until: snoozeUntil(preset).toISOString() }
          : { status: a === 'resolve' ? ('resolved' as const) : ('open' as const) };
    // Leaves the current tab? Hide the row now (the inbox's "done" feel), refetch confirms.
    const leaves = (a === 'resolve' || a === 'snooze') && stored.status !== 'all' && !saved;
    if (leaves) hide(c.uuid, true);
    quick.mutate(
      { uuid: c.uuid, body },
      {
        onSuccess: () =>
          toast.success(
            a === 'snooze' && preset
              ? `${t('status.snoozed')} · ${formatShort(snoozeUntil(preset).toISOString())}`
              : t(`conversation:row.done_${a}`, { name: contactLabel(c) }),
          ),
        onError: (e) => {
          if (leaves) hide(c.uuid, false);
          toast.error(errorMessage(isApiError(e) ? e.code : undefined));
        },
      },
    );
  };

  const selecting = selected !== null;
  useSelectionChrome(selecting);
  const selectedSet = useMemo(() => new Set(selected), [selected]);
  const selectionTitle = selected?.length
    ? t('conversation:bulk.selected', { count: selected.length })
    : t('conversation:bulk.choose');
  const visibleIds = shown.data?.pages.flatMap((p) => p.items.map((c) => c.uuid)) ?? [];
  const allSelected = visibleIds.length > 0 && visibleIds.every((id) => selectedSet.has(id));
  const viewTitle = saved ? saved.name : t(`views.${stored.view}`);
  const compactHeader = useCompactHeader(selecting ? selectionTitle : viewTitle);

  if (!canRead) {
    return (
      <View className="flex-1 justify-center bg-background">
        <EmptyState icon="lockClosed" title={t('no_permission')} />
      </View>
    );
  }

  const nFilters = saved ? 0 : activeFilterCount(stored.adv);
  const toggleSelected = (uuid: string) => setSelected((cur) => toggle(cur ?? [], uuid));

  // One chip per active sheet filter, each clearable (the panel's filter bar).
  const chips: { key: string; label: string; clear: () => void }[] = [];
  const adv = stored.adv;
  const setAdv = (patch: Partial<AdvancedFilters>) => update({ adv: { ...adv, ...patch } });
  if (!saved) {
    if (adv.unread) chips.push({ key: 'unread', label: t('list.unread_only'), clear: () => setAdv({ unread: false }) });
    if (adv.slaBreached)
      chips.push({
        key: 'sla',
        label: `${t('conversation:filters.sla_breached')}${counts.data?.breached ? ` · ${counts.data.breached}` : ''}`,
        clear: () => setAdv({ slaBreached: false }),
      });
    for (const p of adv.priorities)
      chips.push({
        key: `p:${p}`,
        label: t(`priority.${p}`),
        clear: () => setAdv({ priorities: toggle(adv.priorities, p) }),
      });
    for (const c of adv.channels)
      chips.push({
        key: `c:${c}`,
        label: t(`channels:types.${c}`, { defaultValue: c }),
        clear: () => setAdv({ channels: toggle(adv.channels, c) }),
      });
    for (const tg of adv.tags)
      chips.push({
        key: `t:${tg}`,
        label: `#${tags.data?.items.find((x) => x.uuid === tg)?.name ?? '…'}`,
        clear: () => setAdv({ tags: toggle(adv.tags, tg) }),
      });
    if (adv.team)
      chips.push({
        key: 'team',
        label:
          adv.team === 'mine'
            ? t('conversation:filters.my_teams')
            : (teams.data?.items.find((x) => x.uuid === adv.team)?.name ?? t('conversation:filters.team')),
        clear: () => setAdv({ team: null }),
      });
    if (adv.assignee && stored.view === 'all')
      chips.push({
        key: 'assignee',
        label: t('conversation:filters.assignee'),
        clear: () => setAdv({ assignee: null }),
      });
    if (adv.sort !== NO_FILTERS.sort)
      chips.push({
        key: 'sort',
        label: t(`conversation:filters.sort_${adv.sort.replace('-', '')}`),
        clear: () => setAdv({ sort: NO_FILTERS.sort }),
      });
  }

  return (
    <>
      <Stack.Screen
        options={{
          ...compactHeader,
          title: selecting ? selectionTitle : viewTitle,
          headerLargeTitle: selecting,
          headerTitle: selecting ? selectionTitle : compactHeader.headerTitle,
          unstable_headerLeftItems: selecting ? undefined : compactHeader.unstable_headerLeftItems,
          headerLeft: selecting ? undefined : compactHeader.headerLeft,
          headerRight: () =>
            selecting ? null : (
              <HeaderActions>
                <View className="flex-row items-center gap-1">
                  <PresenceButton />
                  <HeaderButton icon="search" label={t('hub:search.title')} onPress={() => router.push('/search')} />
                  <HeaderButton
                    icon="select"
                    guide="inbox.select"
                    label={t('conversation:bulk.select')}
                    onPress={() => setSelected([])}
                  />
                  <View>
                    <HeaderButton
                      icon="filter"
                      guide="inbox.filters"
                      label={
                        nFilters
                          ? `${t('conversation:filters.title')}, ${t('conversation:filters.active', { count: nFilters })}`
                          : t('conversation:filters.title')
                      }
                      onPress={() => setFiltersOpen(true)}
                    />
                    {nFilters ? (
                      <View className="absolute -right-0.5 top-0 min-w-4 items-center rounded-full bg-signal px-1">
                        <Text variant="caption" className="font-sans-bold text-signal-foreground">
                          {nFilters}
                        </Text>
                      </View>
                    ) : null}
                  </View>
                </View>
              </HeaderActions>
            ),
        }}
      />
      {selecting ? (
        <>
          <Stack.Toolbar placement="left">
            <Stack.Toolbar.Button
              hidesSharedBackground
              disabled={!visibleIds.length}
              onPress={() => {
                void Haptics.selectionAsync();
                setSelected(allSelected ? [] : visibleIds);
              }}>
              {t(allSelected ? 'conversation:bulk.deselect_all' : 'conversation:bulk.select_all')}
            </Stack.Toolbar.Button>
          </Stack.Toolbar>
          <Stack.Toolbar placement="right">
            <Stack.Toolbar.Button hidesSharedBackground style={{ fontWeight: '700' }} onPress={() => setSelected(null)}>
              {t('conversation:bulk.finish')}
            </Stack.Toolbar.Button>
          </Stack.Toolbar>
        </>
      ) : null}
      <PagedList<Conversation>
        animateLayout
        query={shown}
        keyOf={(c) => c.uuid}
        empty={nFilters || q ? t('conversation:filters.empty_filtered') : t('list.empty_title')}
        header={
          <View className="gap-3">
            {!selecting ? (
              <>
                <GuideTarget id="inbox.search">
                  <SearchField value={search} onChangeText={setSearch} placeholder={t('list.search')} />
                </GuideTarget>
                <View className="min-w-0">
                  <GuideTarget id="inbox.views">
                    <GuideTarget id="inbox.tabs">
                      <Chips<string>
                        value={stored.saved ?? stored.view}
                        onChange={(v) => {
                          if ((VIEWS as string[]).includes(v)) update({ view: v as InboxView, saved: null });
                          else update({ saved: v });
                        }}
                        options={[
                          ...VIEWS.map((v) => ({
                            value: v,
                            icon: (v === 'mine'
                              ? 'user'
                              : v === 'unassigned'
                                ? 'unassigned'
                                : v === 'mentions'
                                  ? 'mention'
                                  : 'inbox') as IconName,
                            label: t(`views.${v}`),
                            count:
                              v === 'mine'
                                ? counts.data?.mine
                                : v === 'unassigned'
                                  ? counts.data?.unassigned
                                  : undefined,
                          })),
                          ...savedViews.map((v) => ({ value: v.uuid, label: v.name, count: viewCounts[v.uuid] })),
                        ]}
                        trailing={
                          !saved ? (
                            <ActionMenu<InboxStatusTab>
                              title={t('conversation:bulk.status')}
                              value={stored.status}
                              trigger={
                                <Pressable
                                  accessibilityRole="button"
                                  accessibilityLabel={`${t('conversation:bulk.status')}: ${t(`status_tabs.${stored.status}`)}`}
                                  className="flex-row items-center gap-1 rounded-full border border-border bg-card px-2.5 py-2">
                                  <Icon
                                    name={
                                      stored.status === 'open'
                                        ? 'statusOpen'
                                        : stored.status === 'snoozed'
                                          ? 'snoozed'
                                          : stored.status === 'resolved'
                                            ? 'accept'
                                            : 'inbox'
                                    }
                                    size={14}
                                    tone="muted-foreground"
                                  />
                                  <Text variant="footnote" className="font-sans-medium">
                                    {t(`status_tabs.${stored.status}`)}
                                  </Text>
                                  <Icon name="chevronDown" size={11} tone="muted-foreground" />
                                </Pressable>
                              }
                              options={STATUSES.map((v) => ({ value: v, label: t(`status_tabs.${v}`) }))}
                              onPick={(status) => update({ status })}
                            />
                          ) : null
                        }
                      />
                    </GuideTarget>
                  </GuideTarget>
                </View>
                {chips.length ? (
                  <View className="flex-row flex-wrap gap-2">
                    {chips.map((c) => (
                      <Pressable
                        key={c.key}
                        accessibilityRole="button"
                        accessibilityLabel={`${c.label}, ${t('list.clear_filter')}`}
                        onPress={c.clear}
                        className="flex-row items-center gap-1 rounded-full bg-secondary px-3 py-1.5">
                        <Text variant="footnote" className="font-sans-semibold">
                          {c.label}
                        </Text>
                        <Icon name="close" size={14} tone="muted-foreground" />
                      </Pressable>
                    ))}
                    {chips.length > 1 ? (
                      <Pressable
                        accessibilityRole="button"
                        onPress={() => update({ adv: NO_FILTERS })}
                        className="px-2 py-1.5">
                        <Text variant="footnote" tone="primary" className="font-sans-semibold">
                          {t('conversation:filters.reset')}
                        </Text>
                      </Pressable>
                    ) : null}
                  </View>
                ) : null}
              </>
            ) : null}
            <ConnectionBanner offlineText={t('conversation:offline_list')} className="rounded-lg" />
            <IncidentBanner />
          </View>
        }
        renderItem={(c, _i, last) => (
          <GuideTarget id="inbox.row" enabled={_i === 0}>
            <GuideTarget id="inbox.swipe" enabled={_i === 0}>
              <ConversationRow
                conversation={c}
                last={last}
                actions={rowActions(c, me, { manage: canManage, assign: canAssign })}
                onAction={(a) => apply(c, a)}
                onSnooze={(p) => apply(c, 'snooze', p)}
                selecting={selecting}
                selected={selectedSet.has(c.uuid)}
                onSelect={canManage || canAssign ? () => toggleSelected(c.uuid) : undefined}
                active={activeUuid === c.uuid}
              />
            </GuideTarget>
          </GuideTarget>
        )}
      />
      {selecting ? <BulkBar selected={selected} onDone={() => setSelected(null)} /> : null}

      <FilterSheet
        visible={filtersOpen}
        value={stored.adv}
        showAssignee={stored.view === 'all'}
        onClose={() => setFiltersOpen(false)}
        onApply={(a) => update({ adv: a, saved: null })}
      />
    </>
  );
}
