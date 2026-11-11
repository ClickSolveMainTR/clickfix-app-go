import { router, Stack, useLocalSearchParams } from 'expo-router';
import * as WebBrowser from 'expo-web-browser';
import { RefreshControl, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { WEB_URL } from '@/core/config';
import { formatShortWithYear } from '@/core/format';
import { Button } from '@/ui/button';
import { EmptyState } from '@/ui/empty-state';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { useManualRefresh } from '@/ui/use-manual-refresh';

import { type ForumReply, type ForumTopic, useForumActions, useForumScope, useForumTopic } from './api';
import { acceptedReply, type PostKind, visibilityCopy } from './core';
import { confirmAction, ForumNotice, useForumT, useSingleFlight } from './parts';

/**
 * One published topic (CF-171) as visitors see it, with the moderator's
 * actions from the panel's topic dialog: marking the accepted answer and
 * taking the topic or a reply down (CF-182). Text written by visitors is
 * shown as plain text.
 */
export function ForumTopicScreen() {
  const { slug } = useLocalSearchParams<{ slug: string }>();
  const t = useForumT();
  const { org, canManage } = useForumScope();
  const topic = useForumTopic(slug);
  const { accept, visibility } = useForumActions();
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
      accept.mutate({ topic: data, reply }, { onSettled: flight.end });
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

  // Taking a post down asks first. A topic taken down is gone from the
  // public read this screen shows, so the screen closes after it.
  const takeDown = (kind: PostKind, uuid: string) => {
    const copy = visibilityCopy('unpublish', kind);
    confirmAction(t, {
      title: t(copy.title),
      body: t(copy.body),
      action: t(copy.action),
      destructive: true,
      run: () => {
        if (!flight.start()) return;
        visibility.mutate(
          { uuid, action: 'unpublish' },
          {
            onSettled: flight.end,
            onSuccess: () => {
              if (kind === 'topic' && router.canGoBack()) router.back();
            },
          },
        );
      },
    });
  };
  const busy = accept.isPending || visibility.isPending;

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
                    subtitle={formatShortWithYear(data.created_at)}
                    trailing={accepted ? <StatusPill label={t('forum.answered')} tone="done" /> : undefined}
                  />
                  <View className="px-4 py-3">
                    <Text variant="callout" selectable>
                      {data.body}
                    </Text>
                  </View>
                </List>
                <View className="flex-row flex-wrap gap-2">
                  <Button
                    title={t('forum.view_public')}
                    icon="externalLink"
                    variant="outline"
                    onPress={() =>
                      void WebBrowser.openBrowserAsync(
                        `${WEB_URL}/help/${encodeURIComponent(org)}/community/topic/${encodeURIComponent(data.slug)}`,
                      )
                    }
                    className="flex-1"
                  />
                  <Button
                    title={t('forum.unpublish')}
                    icon="hide"
                    variant="destructive"
                    disabled={busy}
                    loading={visibility.isPending && visibility.variables?.uuid === data.uuid}
                    onPress={() => takeDown('topic', data.uuid)}
                    className="flex-1"
                  />
                </View>
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
                                {[formatShortWithYear(r.created_at), t('forum.votes', { count: r.votes })].join(' · ')}
                              </Text>
                              {isAccepted ? <StatusPill label={t('forum.accepted')} tone="done" /> : null}
                            </View>
                            <View className="flex-row flex-wrap gap-2">
                              {isAccepted ? null : (
                                <Button
                                  title={t('forum.accept')}
                                  icon="accept"
                                  variant="ghost"
                                  disabled={busy}
                                  loading={accept.isPending && accept.variables?.reply.uuid === r.uuid}
                                  onPress={() => markAccepted(data, r, !!accepted)}
                                />
                              )}
                              <Button
                                title={t('forum.unpublish')}
                                icon="hide"
                                variant="ghost"
                                disabled={busy}
                                loading={visibility.isPending && visibility.variables?.uuid === r.uuid}
                                onPress={() => takeDown('reply', r.uuid)}
                              />
                            </View>
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
