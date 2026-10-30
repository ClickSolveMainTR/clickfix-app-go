import { Icon } from '@/ui/icon';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { humanLabel } from './labels';
import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { formatRelative } from '@/core/format';
import { useDebounced } from '@/core/use-debounced';
import { Chips } from '@/ui/chips';
import { ListRow } from '@/ui/list';
import { PagedList } from '@/ui/paged-list';
import { SearchField } from '@/ui/search-field';

import { type Issue, ISSUE_STATUSES, type IssueStatus, useIssues } from './api';

type Filter = 'active' | IssueStatus;

/** Engineering issues (Plain / Linear style): search, status filter, newest change first. */
export function IssuesScreen() {
  const { t } = useTranslation('conversation');
  const [filter, setFilter] = useState<Filter>('active');
  const [text, setText] = useState('');
  const q = useDebounced(text, 300);
  const list = useIssues(filter === 'active' ? null : filter, q);
  const shown =
    filter === 'active' && list.data
      ? {
          ...list,
          data: {
            ...list.data,
            pages: list.data.pages.map((p) => ({
              ...p,
              items: p.items.filter((i) => i.status !== 'done' && i.status !== 'canceled'),
            })),
          },
        }
      : list;
  return (
    <>
      <Stack.Screen options={{ title: t('hub:more.engineering') }} />
      <PagedList<Issue>
        query={shown}
        keyOf={(i) => i.uuid}
        emptyIcon="bug"
        empty={t('issue.empty_title')}
        emptyBody={t('issue.empty_body')}
        header={
          <View className="gap-3">
            <SearchField value={text} onChangeText={setText} placeholder={t('issue.search')} />
            <Chips<Filter>
              value={filter}
              onChange={setFilter}
              options={[
                { value: 'active', label: t('issue.active') },
                ...ISSUE_STATUSES.map((s) => ({ value: s, label: t(`issue.status_${s}`) })),
              ]}
            />
          </View>
        }
        renderItem={(i, _idx, last) => (
          <ListRow
            leading={
              <Icon
                name={
                  i.status === 'done'
                    ? 'accept'
                    : i.status === 'in_progress'
                      ? 'clock'
                      : i.status === 'canceled'
                        ? 'close'
                        : 'statusOpen'
                }
                tone={
                  i.status === 'done'
                    ? 'success'
                    : i.status === 'in_progress'
                      ? 'warning'
                      : 'muted-foreground'
                }
                size={22}
              />
            }
            title={i.title}
            subtitle={
              <View className="gap-1.5 pt-1">
                <View className="flex-row items-center gap-2">
                  <StatusPill
                    label={t(`issue.status_${i.status}`)}
                    tone={i.status === 'done' ? 'done' : i.status === 'in_progress' ? 'progress' : 'neutral'}
                  />
                  <View accessibilityLabel={t(`inbox:priority.${i.priority}`)}>
                    <Icon
                      name={
                        i.priority === 'urgent' || i.priority === 'high'
                          ? 'up'
                          : i.priority === 'low'
                            ? 'down'
                            : 'remove'
                      }
                      tone={i.priority === 'urgent' ? 'destructive' : 'muted-foreground'}
                      size={15}
                    />
                  </View>
                </View>
                {i.labels.length ? (
                  <View className="flex-row flex-wrap gap-1">
                    {i.labels.map((l) => (
                      <View key={l} className="rounded-md bg-secondary px-2 py-0.5">
                        <Text variant="caption" tone="muted">
                          {humanLabel(l)}
                        </Text>
                      </View>
                    ))}
                  </View>
                ) : null}
              </View>
            }
            trailing={
              <Text variant="caption" tone="muted">
                {formatRelative(i.updated_at)}
              </Text>
            }
            onPress={() => router.push(`/engineering/${i.uuid}`)}
            last={last}
          />
        )}
      />
    </>
  );
}
