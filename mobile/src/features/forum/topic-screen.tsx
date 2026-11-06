import { Stack, useLocalSearchParams } from 'expo-router';
import * as WebBrowser from 'expo-web-browser';
import { RefreshControl, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { WEB_URL } from '@/core/config';
import { formatShort } from '@/core/format';
import { Button } from '@/ui/button';
import { EmptyState } from '@/ui/empty-state';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';
import { useManualRefresh } from '@/ui/use-manual-refresh';

import { type ForumReply, type ForumTopic, useForumActions, useForumScope, useForumTopic } from './api';
import { acceptedReply } from './core';
import { confirmAction, forumFailed, ForumNotice, useForumT, useSingleFlight } from './parts';

/**
 * One published topic (CF-171) as visitors see it, with the moderator's
 * action from the panel's topic dialog: marking the accepted answer. Text
 * written by visitors is shown as plain text.
 */
export function ForumTopicScreen() {
  const { slug } = useLocalSearchParams<{ slug: string }>();
  const t = useForumT();
  const { org, canManage } = useForumScope();
  const topic = useForumTopic(slug);
  const { accept } = useForumActions();
  const refresh = useManualRefresh(topic);
  const flight = useSingleFlight();
  const title = topic.data?.title ?? t('forum.topic');

  if (!canManage || !slug)
    return (
      <>
        <Stack.Screen options={{ title: t('forum.topic') }} />
        <ForumNotice kind={canManage ? 'missing' : 'forbidden'} />
      </>
    );
  // A topic that was hidden or never existed: the public API answers 404.
  if (isApiError(topic.error) && topic.error.status === 404 && !topic.data)
    return (
      <>
        <Stack.Screen options={{ title: t('forum.topic') }} />
        <ForumNotice kind="missing" />
      </>
    );

  const markAccepted = (data: ForumTopic, reply: ForumReply, replaces: boolean) => {
    const run = () => {
      if (!flight.start()) return;
      accept.mutate(
        { topic: data.uuid, reply: reply.uuid },
        {
          onSuccess: () => toast.success(t('forum.accepted_done')),
          onError: (e) => forumFailed(t, e),
          onSettled: flight.end,
        },
      );
    };
    // Replacing an answer the community already sees asks first.
    if (!replaces) return run();
    confirmAction(t, {
      title: t('forum.accept_replace_title'),
      body: t('forum.accept_replace_body'),
      action: t('forum.accept'),
      run,
    });
  };

  return (
    <>
      <Stack.Screen options={{ title }} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <QueryState query={topic}>
          {(data) => {
            const accepted = acceptedReply(data);
            const replies = data.replies.items;
            return (
              <View className="gap-4">
                <List>
                  <ListRow
                    title={data.title}
                    titleLines={4}
                    subtitle={formatShort(data.created_at)}
                    trailing={accepted ? <StatusPill label={t('forum.answered')} tone="done" /> : undefined}
                  />
                  <View className="px-4 py-3">
                    <Text variant="callout" selectable>
                      {data.body}
                    </Text>
                  </View>
                </List>
                <Button
                  title={t('forum.view_public')}
                  icon="externalLink"
                  variant="outline"
                  onPress={() =>
                    void WebBrowser.openBrowserAsync(
                      `${WEB_URL}/help/${encodeURIComponent(org)}/community/topic/${encodeURIComponent(data.slug)}`,
                    )
                  }
                />
                <View>
                  <SectionHeader title={t('forum.replies', { count: data.replies.total })} />
                  {replies.length ? (
                    <List>
                      {replies.map((r, i) => {
                        const isAccepted = r.uuid === accepted;
                        return (
                          <View
                            key={r.uuid}
                            className={
                              i === replies.length - 1 ? 'gap-2 px-4 py-3' : 'gap-2 border-b border-border px-4 py-3'
                            }>
                            <Text variant="callout" selectable>
                              {r.body}
                            </Text>
                            <View className="flex-row flex-wrap items-center gap-3">
                              <Text variant="footnote" tone="muted">
                                {[formatShort(r.created_at), t('forum.votes', { count: r.votes })].join(' · ')}
                              </Text>
                              {isAccepted ? <StatusPill label={t('forum.accepted')} tone="done" /> : null}
                            </View>
                            {isAccepted ? null : (
                              <Button
                                title={t('forum.accept')}
                                icon="accept"
                                variant="ghost"
                                disabled={accept.isPending}
                                loading={accept.isPending && accept.variables?.reply === r.uuid}
                                onPress={() => markAccepted(data, r, !!accepted)}
                                className="self-start"
                              />
                            )}
                          </View>
                        );
                      })}
                    </List>
                  ) : (
                    <EmptyState icon="reply" title={t('forum.no_replies')} />
                  )}
                  {data.replies.total > replies.length ? (
                    <Text variant="footnote" tone="muted" className="px-4 pt-2">
                      {t('forum.more_replies', { count: replies.length })}
                    </Text>
                  ) : null}
                </View>
              </View>
            );
          }}
        </QueryState>
      </Screen>
    </>
  );
}
