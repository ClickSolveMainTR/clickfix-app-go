import { FormSheet } from '@/ui/form-sheet';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { useAgents, useChannelTypes, useTags, useTeams } from './api';
import { type AdvancedFilters, type InboxSort, NO_FILTERS, SORTS } from './filters';

const PRIORITIES = ['urgent', 'high', 'normal', 'low'] as const;
const ANY = '__any';
const MY_TEAMS = 'mine';

/**
 * The inbox filter sheet (CF-79), the panel's filter bar on a phone: team,
 * assignee, priority, channel, tag, unread, SLA breached and sort. Changes
 * apply on "Show results"; "Reset" clears them.
 */
export function FilterSheet({
  visible,
  value,
  showAssignee,
  onClose,
  onApply,
}: {
  visible: boolean;
  value: AdvancedFilters;
  /** The assignee filter only makes sense in the "All" view. */
  showAssignee: boolean;
  onClose: () => void;
  onApply: (v: AdvancedFilters) => void;
}) {
  const { t } = useTranslation('inbox');
  const [draft, setDraft] = useState(value);
  const [wasVisible, setWasVisible] = useState(visible);
  if (visible !== wasVisible) {
    setWasVisible(visible);
    if (visible) setDraft(value);
  }
  const tags = useTags(visible);
  const teams = useTeams(visible);
  const agents = useAgents(visible && showAssignee);
  const channels = useChannelTypes(visible);
  const set = <K extends keyof AdvancedFilters>(k: K, v: AdvancedFilters[K]) => setDraft((d) => ({ ...d, [k]: v }));

  return (
    <FormSheet
      visible={visible}
      title={t('conversation:filters.title')}
      onCancel={onClose}
      detents={['large']}
      dirty={JSON.stringify(draft) !== JSON.stringify(value)}
      searchable
      primary={{
        label: t('conversation:filters.apply'),
        onPress: () => {
          onApply(draft);
          onClose();
        },
      }}
      sections={[
        {
          fields: [
            {
              key: 'sort',
              kind: 'pick',
              label: t('conversation:filters.sort'),
              value: draft.sort,
              onChange: (v) => set('sort', v as InboxSort),
              options: SORTS.map((value) => ({
                value,
                label: t(`conversation:filters.sort_${value.replace('-', '')}`),
              })),
            },
            {
              key: 'unread',
              kind: 'toggle',
              label: t('list.unread_only'),
              value: draft.unread,
              onChange: (v) => set('unread', v),
            },
            {
              key: 'sla',
              kind: 'toggle',
              label: t('conversation:filters.sla_breached'),
              value: draft.slaBreached,
              onChange: (v) => set('slaBreached', v),
            },
          ],
        },
        {
          title: t('conversation:filters.priority'),
          fields: [
            {
              key: 'priorities',
              kind: 'multi',
              label: t('conversation:filters.priority'),
              values: draft.priorities,
              onChange: (v) => set('priorities', v),
              options: PRIORITIES.map((value) => ({ value, label: t(`priority.${value}`) })),
            },
          ],
        },
        {
          fields: [
            {
              key: 'team',
              kind: 'pick',
              label: t('conversation:filters.team'),
              value: draft.team ?? ANY,
              onChange: (v) => set('team', v === ANY ? null : v),
              options: [
                { value: ANY, label: t('conversation:filters.any') },
                { value: MY_TEAMS, label: t('conversation:filters.my_teams') },
                ...(teams.data?.items ?? []).map((tm) => ({ value: tm.uuid, label: tm.name })),
              ],
            },
            ...(showAssignee
              ? [
                  {
                    key: 'assignee',
                    kind: 'pick' as const,
                    label: t('conversation:filters.assignee'),
                    value: draft.assignee ?? ANY,
                    onChange: (v: string) => set('assignee', v === ANY ? null : v),
                    options: [
                      { value: ANY, label: t('conversation:filters.any') },
                      ...(agents.data?.items ?? []).map((a) => ({ value: a.uuid, label: a.name })),
                    ],
                  },
                ]
              : []),
          ],
        },
        {
          title: t('conversation:filters.channel'),
          fields: [
            {
              key: 'channels',
              kind: 'multi',
              label: t('conversation:filters.channel'),
              values: draft.channels,
              onChange: (v) => set('channels', v),
              options: (channels.data ?? []).map((value) => ({
                value,
                label: t(`channels:types.${value}`, { defaultValue: value }),
              })),
            },
          ],
        },
        {
          title: t('conversation:filters.tags'),
          fields: [
            {
              key: 'tags',
              kind: 'multi',
              label: t('conversation:filters.tags'),
              values: draft.tags,
              onChange: (v) => set('tags', v),
              options: (tags.data?.items ?? []).map((tag) => ({ value: tag.uuid, label: tag.name })),
            },
          ],
        },
        {
          fields: [
            {
              key: 'reset',
              kind: 'button',
              label: t('conversation:filters.reset'),
              onPress: () => setDraft(NO_FILTERS),
            },
          ],
        },
      ]}
    />
  );
}
