import { Button } from '@/ui/button';
import { FormSheet } from '@/ui/form-sheet';
import { HeaderButton } from '@/ui/header-button';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { formatShort } from '@/core/format';
import { errorMessage } from '@/core/i18n';
import { Card } from '@/ui/card';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { Markdown } from '@/ui/markdown';
import { QueryState } from '@/ui/query-state';
import { FormScreen } from '@/ui/screen';
import { Select } from '@/ui/select';
import { StatusPill, type StatusTone } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';
import { humanLabel } from './labels';

import {
  ISSUE_STATUSES,
  type IssueStatus,
  useAddIssueComment,
  useEngineeringScope,
  useIssue,
  useIssueComments,
  useIssueConversations,
  useUpdateIssue,
} from './api';

const TONE: Record<IssueStatus, StatusTone> = {
  backlog: 'neutral',
  todo: 'neutral',
  in_progress: 'progress',
  done: 'done',
  canceled: 'neutral',
};

/** One engineering issue: status (changeable with write access), description, linked conversations, comments. */
export function IssueScreen() {
  const { t } = useTranslation('conversation');
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const issue = useIssue(uuid);
  const links = useIssueConversations(uuid);
  const comments = useIssueComments(uuid);
  const update = useUpdateIssue(uuid);
  const add = useAddIssueComment(uuid);
  const { canWrite } = useEngineeringScope();
  const [comment, setComment] = useState('');
  const [commenting, setCommenting] = useState(false);
  const [editing, setEditing] = useState(false);
  const [status, setStatus] = useState<IssueStatus>('todo');
  const [priority, setPriority] = useState<'low' | 'normal' | 'high' | 'urgent'>('normal');
  const fail = (e: unknown) => toast.error(errorMessage(isApiError(e) ? e.code : undefined));

  return (
    <>
      <Stack.Screen
        options={{
          title: t('issue.card_title'),
          headerRight: () =>
            canWrite && issue.data ? (
              <HeaderButton
                icon="edit"
                label={t('people:edit')}
                onPress={() => {
                  setStatus(issue.data!.status);
                  setPriority(issue.data!.priority);
                  setEditing(true);
                }}
              />
            ) : null,
        }}
      />
      <FormScreen
        footer={
          canWrite ? (
            <Button title={t('issue.add_comment')} icon="add" variant="ghost" onPress={() => setCommenting(true)} />
          ) : undefined
        }>
        <QueryState query={issue}>
          {(i) => (
            <View className="gap-4">
              <View className="gap-2">
                <Text variant="title" selectable>
                  {i.title}
                </Text>
                <View className="flex-row flex-wrap items-center gap-2">
                  <StatusPill label={t(`issue.status_${i.status}`)} tone={TONE[i.status]} />
                  <StatusPill
                    label={t(`inbox:priority.${i.priority}`)}
                    tone={i.priority === 'urgent' ? 'problem' : 'neutral'}
                  />
                  {i.labels.map((l) => (
                    <StatusPill key={l} label={humanLabel(l)} />
                  ))}
                </View>
                <Text variant="footnote" tone="muted">
                  {t('issue.updated', { at: formatShort(i.updated_at) })}
                </Text>
              </View>
              {canWrite ? (
                <View>
                  <Select<IssueStatus>
                    label={t('issue.change_status')}
                    value={i.status}
                    onChange={(s) => update.mutate({ status: s, revision: i.revision }, { onError: fail })}
                    disabled={update.isPending}
                    options={ISSUE_STATUSES.map((s) => ({ value: s, label: t(`issue.status_${s}`) }))}
                  />
                </View>
              ) : null}
              {i.description ? (
                <Card>
                  <Markdown source={i.description} />
                </Card>
              ) : null}
              {links.data?.items.length ? (
                <View className="gap-2">
                  <SectionHeader title={t('issue.conversations')} />
                  <List>
                    {links.data.items.map((l, idx, all) => (
                      <ListRow
                        key={l.conversation_uuid}
                        icon="inbox"
                        title={l.subject || l.conversation_uuid.slice(0, 8)}
                        subtitle={[
                          t(`channels:types.${l.channel}`, { defaultValue: l.channel }),
                          l.notify_customer ? t('issue.notifies') : null,
                        ]
                          .filter(Boolean)
                          .join(' · ')}
                        onPress={() => router.push(`/conversation/${l.conversation_uuid}`)}
                        last={idx === all.length - 1}
                      />
                    ))}
                  </List>
                </View>
              ) : null}
              <View className="gap-3">
                <Text variant="headline">{t('issue.comments')}</Text>
                {(comments.data?.items ?? []).map((c) => (
                  <View key={c.uuid} className="gap-0.5 border-b border-border pb-2">
                    <Text variant="footnote" selectable>
                      {c.body}
                    </Text>
                    <Text variant="caption" tone="muted">
                      {[c.source, formatShort(c.created_at)].filter(Boolean).join(' · ')}
                    </Text>
                  </View>
                ))}
              </View>
            </View>
          )}
        </QueryState>
      </FormScreen>
      <FormSheet
        visible={commenting}
        title={t('issue.add_comment')}
        onCancel={() => {
          setCommenting(false);
          setComment('');
        }}
        dirty={!!comment}
        primary={{
          label: t('issue.send_comment'),
          disabled: !comment.trim(),
          loading: add.isPending,
          onPress: () =>
            add.mutate(comment.trim(), {
              onSuccess: () => {
                setComment('');
                setCommenting(false);
              },
              onError: fail,
            }),
        }}
        sections={[
          {
            fields: [
              {
                key: 'comment',
                kind: 'text',
                label: t('issue.add_comment'),
                value: comment,
                onChange: setComment,
                multiline: true,
              },
            ],
          },
        ]}
      />
      <FormSheet
        visible={editing}
        title={t('issue.card_title')}
        onCancel={() => setEditing(false)}
        dirty={status !== issue.data?.status || priority !== issue.data?.priority}
        primary={{
          label: t('common:save'),
          loading: update.isPending,
          onPress: () => {
            if (issue.data)
              update.mutate(
                { status, priority, revision: issue.data.revision },
                { onSuccess: () => setEditing(false), onError: fail },
              );
          },
        }}
        sections={[
          {
            fields: [
              {
                key: 'status',
                kind: 'pick',
                label: t('issue.change_status'),
                value: status,
                onChange: (v) => setStatus(v as IssueStatus),
                options: ISSUE_STATUSES.map((value) => ({ value, label: t(`issue.status_${value}`) })),
              },
              {
                key: 'priority',
                kind: 'pick',
                label: t('issue.priority'),
                value: priority,
                onChange: (v) => setPriority(v as typeof priority),
                options: ['low', 'normal', 'high', 'urgent'].map((value) => ({
                  value,
                  label: t(`inbox:priority.${value}`),
                })),
              },
            ],
          },
        ]}
      />
    </>
  );
}
