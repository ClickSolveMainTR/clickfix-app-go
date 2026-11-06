import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { Alert, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { formatShort } from '@/core/format';
import { useDebounced } from '@/core/use-debounced';
import { Button } from '@/ui/button';
import { Chips } from '@/ui/chips';
import { ListRow } from '@/ui/list';
import { PagedList } from '@/ui/paged-list';
import { SearchField } from '@/ui/search-field';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import {
  type ForumTopicSummary,
  useForumActions,
  useForumCategories,
  useForumScope,
  useForumTopics,
  useModerationQueue,
} from './api';
import { categoryName, flagLabel, isReply, isRisky, type ModerationItem, needsConfirm, type Outcome } from './core';
import { confirmAction, forumFailed, ForumNotice, useForumT, useSingleFlight } from './parts';

type Tab = 'moderation' | 'topics';
const ALL = '';

/**
 * Community forum moderation (CF-171), the phone version of the panel's
 * forum page (`frontend/src/features/forum/components/forum-admin-page.tsx`):
 * the queue of pending posts with approve / reject / spam and the explicit AI
 * review, and the published topics with search and category. Categories and
 * the conversions to and from conversations stay on the panel.
 */
export function ForumScreen() {
  const t = useForumT();
  const { canManage } = useForumScope();
  const [tab, setTab] = useState<Tab>('moderation');
  const queue = useModerationQueue();
  const pending = queue.data?.pages[0]?.total;

  if (!canManage)
    return (
      <>
        <Stack.Screen options={{ title: t('forum.title') }} />
        <ForumNotice kind="forbidden" />
      </>
    );

  const tabs = (
    <Chips<Tab>
      value={tab}
      onChange={setTab}
      options={[
        { value: 'moderation', label: t('forum.tab_moderation'), count: pending },
        { value: 'topics', label: t('forum.tab_topics') },
      ]}
    />
  );
  return (
    <>
      <Stack.Screen options={{ title: t('forum.title') }} />
      {tab === 'moderation' ? <Moderation tabs={tabs} /> : <Topics tabs={tabs} />}
    </>
  );
}

function Moderation({ tabs }: { tabs: React.ReactElement }) {
  const t = useForumT();
  const queue = useModerationQueue();
  const { moderate, assist } = useForumActions();
  // One decision at a time: while a write is on its way every action is off.
  const busy = moderate.isPending || assist.isPending;
  const flight = useSingleFlight();
  const total = queue.data?.pages[0]?.total ?? 0;

  const decide = (item: ModerationItem, outcome: Outcome) => {
    const run = () => {
      if (!flight.start()) return;
      moderate.mutate(
        { uuid: item.uuid, outcome },
        {
          onSuccess: () => toast.success(t(`forum.done_${outcome}`)),
          onError: (e) => forumFailed(t, e),
          onSettled: flight.end,
        },
      );
    };
    if (!needsConfirm(outcome)) return run();
    confirmAction(t, {
      title: t(`forum.confirm_${outcome}_title`),
      body: t('forum.confirm_hide_body'),
      action: t(`forum.${outcome}`),
      destructive: true,
      run,
    });
  };

  const review = (item: ModerationItem) =>
    confirmAction(t, {
      title: t('forum.assist'),
      body: t('forum.assist_hint'),
      action: t('forum.assist_run'),
      run: () => {
        if (!flight.start()) return;
        assist.mutate(item.uuid, {
          onSettled: flight.end,
          onSuccess: (r) =>
            toast.success(r.flagged ? t('forum.ai_flagged', { reason: r.reason }) : t('forum.ai_clean')),
          onError: (e) => (isApiError(e) && e.status === 429 ? toast.error(t('forum.ai_limited')) : forumFailed(t, e)),
        });
      },
    });

  const more = (item: ModerationItem) =>
    Alert.alert(item.title || t(isReply(item) ? 'forum.kind_reply' : 'forum.kind_topic'), undefined, [
      { text: t('forum.assist'), onPress: () => review(item) },
      { text: t('forum.spam'), style: 'destructive', onPress: () => decide(item, 'spam') },
      { text: t('forum.cancel'), style: 'cancel' },
    ]);

  return (
    <PagedList<ModerationItem>
      query={queue}
      keyOf={(p) => p.uuid}
      emptyIcon="accept"
      empty={t('forum.queue_empty')}
      header={
        <View className="gap-3">
          {tabs}
          {total ? (
            <Text variant="footnote" tone="muted" className="px-1">
              {t('forum.queue_count', { count: total })}
            </Text>
          ) : null}
        </View>
      }
      renderItem={(p) => {
        const flag = flagLabel(t, p.flag_reason);
        const acting = busy && (moderate.variables?.uuid === p.uuid || assist.variables === p.uuid);
        return (
          <View className="gap-2 border-b border-border px-4 py-3">
            <View className="flex-row flex-wrap items-center gap-2">
              <StatusPill
                label={t(isReply(p) ? 'forum.kind_reply' : 'forum.kind_topic')}
                icon={isReply(p) ? 'reply' : 'chat'}
              />
              {flag ? <StatusPill label={flag} tone={isRisky(p.flag_reason) ? 'problem' : 'neutral'} /> : null}
              <Text variant="footnote" tone="muted" className="ml-auto">
                {formatShort(p.created_at)}
              </Text>
            </View>
            {p.title ? (
              <Text variant="body" className="font-sans-semibold" selectable>
                {p.title}
              </Text>
            ) : null}
            {/* Visitor text is shown as plain text, never rendered as markup. */}
            <Text variant="callout" tone="muted" numberOfLines={8} selectable>
              {p.body}
            </Text>
            <View className="flex-row flex-wrap gap-2 pt-1">
              <Button
                title={t('forum.approve')}
                icon="check"
                loading={acting && moderate.variables?.outcome === 'approve'}
                disabled={busy}
                onPress={() => decide(p, 'approve')}
                className="flex-1"
              />
              <Button
                title={t('forum.reject')}
                icon="close"
                variant="outline"
                disabled={busy}
                onPress={() => decide(p, 'reject')}
                className="flex-1"
              />
              <Button title={t('forum.more')} icon="more" variant="ghost" disabled={busy} onPress={() => more(p)} />
            </View>
          </View>
        );
      }}
    />
  );
}

function Topics({ tabs }: { tabs: React.ReactElement }) {
  const t = useForumT();
  const [text, setText] = useState('');
  const q = useDebounced(text, 300);
  const [category, setCategory] = useState(ALL);
  const categories = useForumCategories();
  const topics = useForumTopics(q, category);
  return (
    <PagedList<ForumTopicSummary>
      query={topics}
      keyOf={(topic) => topic.uuid}
      emptyIcon="chat"
      empty={q.trim() || category ? t('common:no_results') : t('forum.topics_empty')}
      header={
        <View className="gap-3">
          {tabs}
          <SearchField value={text} onChangeText={setText} placeholder={t('forum.search')} />
          {categories.data?.length ? (
            <Chips<string>
              value={category}
              onChange={setCategory}
              options={[
                { value: ALL, label: t('forum.all_categories') },
                ...categories.data.map((c) => ({ value: c.slug, label: c.name })),
              ]}
            />
          ) : null}
          <Text variant="footnote" tone="muted" className="px-1">
            {t('forum.topics_hint')}
          </Text>
        </View>
      }
      renderItem={(topic, _i, last) => (
        <ListRow
          icon="chat"
          title={topic.title}
          titleLines={2}
          subtitle={[
            categoryName(categories.data, topic.category_slug),
            formatShort(topic.created_at),
            t('forum.votes', { count: topic.votes }),
          ].join(' · ')}
          trailing={topic.accepted_reply_uuid ? <StatusPill label={t('forum.answered')} tone="done" /> : undefined}
          onPress={() => router.push({ pathname: '/more/forum-topic', params: { slug: topic.slug } })}
          last={last}
        />
      )}
    />
  );
}
