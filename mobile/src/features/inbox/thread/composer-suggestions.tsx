import { ClipboardImageOffer, useClipboardImage } from './clipboard-image';
import { api, orgHeader, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { useSession } from '@/core/auth/session';
import { WEB_URL } from '@/core/config';
import { errorMessage } from '@/core/i18n';
import { roleLabel } from '@/core/roles';
import { useMembers } from '@/features/admin/api';
import { useCopilot } from '@/features/copilot/copilot';
import { useKnowledgeScope } from '@/features/knowledge/api';
import { useArticleSearch } from '@/features/search/api';
import { useTeamPresence } from '@/features/team/api';
import { Avatar } from '@/ui/avatar';
import { FormSheet } from '@/ui/form-sheet';
import { Icon } from '@/ui/icon';
import type { IconName } from '@/ui/icons';
import { MessageComposer } from '@/ui/message-composer';
import { nativeChoice } from '@/ui/native-choice';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';
import { useEffect, useRef, useState, type ComponentRef, type Dispatch, type SetStateAction } from 'react';
import { useTranslation } from 'react-i18next';
import {
  AccessibilityInfo,
  ActivityIndicator,
  Alert,
  DeviceEventEmitter,
  FlatList,
  Pressable,
  StyleSheet,
  TextInput,
  View,
  type TextInputProps,
} from 'react-native';
import Animated, { FadeInDown, FadeOutDown, ReduceMotion } from 'react-native-reanimated';
import {
  renderCanned,
  type LocalFile,
  useAgents,
  useCannedResponses,
  useInboxScope,
  useTags,
  useUpdateConversation,
  type ConversationDetail,
} from '../api';
import { useAddTag, useApplyMacro, useMacros } from '../conversation-api';
import { SNOOZE_PRESETS, snoozeUntil } from '../helpers';
import { insertToken, parseTrigger, type Mention, type Trigger } from './composer-tokens';

type Row = {
  id: string;
  label: string;
  detail?: string;
  keyword?: string;
  section?: string;
  icon?: IconName;
  member?: Mention;
  online?: boolean;
  pick: () => void;
};
const DISMISS = 'clickfix.composer.dismiss';
export function dismissComposerSuggestions() {
  DeviceEventEmitter.emit(DISMISS);
}

type Props = Omit<React.ComponentProps<typeof MessageComposer>, 'onChangeText'> & {
  value: string;
  onChangeText: Dispatch<SetStateAction<string>>;
  conversation?: ConversationDetail;
  renderContext?: string;
  mentions?: Mention[];
  onMention?: (member: Mention) => void;
  allowMentions?: boolean;
  onNote?: () => void;
  onReply?: () => void;
  onAttach?: () => void;
  onAi?: () => void;
  onPasteImage?: (file: LocalFile) => boolean | void;
};

/** Shared inline surface. No modal or keyboard dismissal when selecting a suggestion. */
export function InlineComposer({
  conversation,
  renderContext = conversation?.uuid,
  mentions = [],
  onMention,
  allowMentions = false,
  onNote,
  onReply,
  onAttach,
  onAi,
  onPasteImage,
  ...props
}: Props) {
  const { t } = useTranslation('conversation');
  const input = useRef<ComponentRef<typeof TextInput>>(null);
  const list = useRef<FlatList<Row>>(null);
  const [caret, setCaret] = useState(props.value.length);
  const [focused, setFocused] = useState(false);
  const clipboard = useClipboardImage(!!onPasteImage && !props.inputDisabled);
  const [dismissed, setDismissed] = useState<string | null>(null);
  const [stage, setStage] = useState<'assign' | 'priority' | 'tag' | null>(null);
  const [active, setActive] = useState(0);
  const [focusRequest, setFocusRequest] = useState<{ caret: number; expected: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const [customSnooze, setCustomSnooze] = useState(false);
  const [snoozeDate, setSnoozeDate] = useState(() => new Date(Date.now() + 3600000));
  const [openedAt] = useState(() => new Date());
  const [aiText, setAiText] = useState<string | null>(null);
  const trigger = focused ? parseTrigger(props.value, caret) : null;
  const key = trigger ? `${trigger.start}:${trigger.kind}:${trigger.query}` : null;
  const open = !!trigger && key !== dismissed;
  const query = trigger?.query ?? '';
  const scope = useInboxScope();
  const agents = useAgents(open && (trigger?.kind === '@' || stage === 'assign'));
  const members = useMembers(open && trigger?.kind === '@' ? query : '');
  const presence = useTeamPresence();
  const canned = useCannedResponses(query, open && trigger?.kind === '/' && scope.canCanned);
  const macros = useMacros(open && trigger?.kind === '/' && scope.canCanned && !!conversation);
  const { canRead: canKb } = useKnowledgeScope();
  const articles = useArticleSearch(query, open && trigger?.kind === '/' && canKb);
  const tags = useTags(open && stage === 'tag');
  const update = useUpdateConversation(conversation?.uuid ?? '');
  const runMacro = useApplyMacro(conversation?.uuid ?? '');
  const addTag = useAddTag(conversation?.uuid ?? '');
  const copilot = useCopilot(renderContext ?? '');
  const org = useSession((s) => s.organization?.slug ?? '');
  const fail = (e: unknown) => toast.error(errorMessage(isApiError(e) ? e.code : undefined));
  const replace = (replacement: string, token: Trigger | null = trigger) => {
    if (!token) return;
    const next = insertToken(props.value, token, replacement);
    props.onChangeText((current) => (current === props.value ? next.text : current));
    setCaret(next.caret);
    setDismissed(key);
    setStage(null);
    setFocusRequest({ caret: next.caret, expected: next.text });
  };
  const clear = () => replace('');
  const enterStage = (next: 'assign' | 'priority' | 'tag') => {
    if (!trigger) return;
    const text = props.value.slice(0, trigger.start) + '/' + props.value.slice(trigger.end);
    props.onChangeText(text);
    setCaret(trigger.start + 1);
    setDismissed(null);
    setStage(next);
    setActive(0);
    setFocusRequest({ caret: trigger.start + 1, expected: text });
  };
  const action = (run: () => void) => {
    clear();
    run();
  };
  const filter = (label: string) => label.toLocaleLowerCase().includes(query.toLocaleLowerCase());
  const rows: Row[] = [];
  if (open && ((trigger?.kind === '@' && allowMentions) || stage === 'assign')) {
    for (const agent of agents.data?.items ?? []) {
      if (!filter(agent.name)) continue;
      const member = members.data?.pages.flatMap((p) => p.items).find((m) => m.user.uuid === agent.uuid);
      rows.push({
        id: agent.uuid,
        label: agent.name,
        member: agent,
        detail: member?.roles.map((r) => roleLabel(r)).join(' · '),
        online: presence.data?.items.some((p) => p.user_uuid === agent.uuid && p.status === 'online'),
        pick: () => {
          if (stage === 'assign') action(() => update.mutate({ assignee: agent.uuid }, { onError: fail }));
          else {
            onNote?.();
            onMention?.(agent);
            replace(`@${agent.name}`);
          }
        },
      });
    }
  } else if (open && trigger?.kind === '/') {
    if (stage === 'priority') {
      for (const value of ['low', 'normal', 'high', 'urgent'] as const)
        rows.push({
          id: value,
          label: t(`inbox:priority.${value}`),
          pick: () => action(() => update.mutate({ priority: value }, { onError: fail })),
        });
    } else if (stage === 'tag') {
      for (const tag of tags.data?.items ?? [])
        rows.push({
          id: tag.uuid,
          label: tag.name,
          pick: () => action(() => addTag.mutate({ uuid: tag.uuid }, { onError: fail })),
        });
    } else {
      const commands: [string, IconName, boolean, () => void][] = [
        ['not', 'lockClosed', !!onNote, () => action(() => onNote?.())],
        ['yanıt', 'reply', !!onReply, () => action(() => onReply?.())],
        [
          'çöz',
          'check',
          !!conversation && scope.canManage,
          () => action(() => update.mutate({ status: 'resolved' }, { onError: fail })),
        ],
        [
          'ertele',
          'snoozed',
          !!conversation && scope.canManage,
          () =>
            action(() =>
              nativeChoice(
                t('inbox:header.snooze'),
                [
                  ...SNOOZE_PRESETS.map((preset) => ({
                    label: t(`inbox:snooze.${preset}`),
                    onPress: () =>
                      update.mutate(
                        { status: 'snoozed', snoozed_until: snoozeUntil(preset).toISOString() },
                        { onError: fail },
                      ),
                  })),
                  { label: t('inline.custom_date'), onPress: () => setCustomSnooze(true) },
                ],
                t('common:cancel'),
              ),
            ),
        ],
        ['ata', 'user', !!conversation && scope.canAssign, () => enterStage('assign')],
        ['öncelik', 'alert', !!conversation && scope.canManage, () => enterStage('priority')],
        ['etiket', 'tag', !!conversation && scope.canManage, () => enterStage('tag')],
        [
          'ai',
          'sparkles',
          !!onAi || (!!renderContext && scope.canReply),
          () => {
            clear();
            if (onAi) return onAi();
            copilot.mutate(
              { operation: 'suggest_reply' },
              { onSuccess: (r) => setAiText(r.text), onError: fail },
            );
          },
        ],
        ['ek', 'attachment', !!onAttach, () => action(() => onAttach?.())],
      ];
      for (const [keyword, icon, allowed, pick] of commands)
        if (allowed && filter(`${keyword} ${t(`inline.${keyword}`)} ${keyword === 'ai' ? 'asistan' : ''}`))
          rows.push({
            id: keyword,
            label: t(`inline.${keyword}`),
            keyword,
            icon,
            section: t('inline.actions'),
            pick,
          });
      for (const item of scope.canCanned ? (canned.data?.items ?? []) : [])
        if (filter(`${item.title} ${item.shortcut}`) && (renderContext || !/\{\{/.test(item.content)))
          rows.push({
            id: `c:${item.uuid}`,
            label: item.title,
            detail: item.content,
            keyword: item.shortcut ?? 'yanıt',
            icon: 'reply',
            section: t('inline.canned'),
            pick: async () => {
              if (!renderContext) return replace(item.content);
              setBusy(true);
              const token = trigger;
              try {
                const result = await renderCanned(item.uuid, renderContext);
                replace(result.content, token);
              } catch (e) {
                fail(e);
              } finally {
                setBusy(false);
              }
            },
          });
      for (const item of conversation && scope.canCanned ? (macros.data?.items ?? []) : [])
        if (filter(item.name))
          rows.push({
            id: `m:${item.uuid}`,
            label: item.name,
            detail: item.actions.map((a) => t(`macro_action.${a.type}`)).join(' · '),
            keyword: 'makro',
            icon: 'macro',
            section: t('inline.macros'),
            pick: () =>
              Alert.alert(item.name, item.actions.map((a) => t(`macro_action.${a.type}`)).join(' · '), [
                { text: t('common:cancel'), style: 'cancel' },
                {
                  text: t('common:confirm'),
                  onPress: () => action(() => runMacro.mutate(item.uuid, { onError: fail })),
                },
              ]),
          });
      for (const item of canKb ? (articles.data ?? []) : [])
        if (filter(item.title))
          rows.push({
            id: `a:${item.uuid}`,
            label: item.title,
            detail: item.snippet,
            keyword: 'makale',
            icon: 'book',
            section: t('inline.articles'),
            pick: async () => {
              const token = trigger;
              setBusy(true);
              try {
                const article = await unwrap(
                  api.GET('/v1/tenant/knowledge/articles/{uuid}', {
                    params: { header: orgHeader(), path: { uuid: item.uuid } },
                  }),
                );
                replace(
                  article.visibility === 'public' && article.published_version
                    ? `${article.title}: ${WEB_URL}/help/${org}/article/${article.slug}`
                    : article.title,
                  token,
                );
              } catch (e) {
                fail(e);
              } finally {
                setBusy(false);
              }
            },
          });
    }
  }
  useEffect(() => {
    if (!focusRequest || focusRequest.expected !== props.value) return;
    input.current?.focus();
    input.current?.setNativeProps({ selection: { start: focusRequest.caret, end: focusRequest.caret } });
  }, [focusRequest, props.value]);
  useEffect(() => {
    if (open) AccessibilityInfo.announceForAccessibility(t('inline.announced'));
  }, [open, t]);
  useEffect(() => {
    const subscription = DeviceEventEmitter.addListener(DISMISS, () => {
      setDismissed(key);
      setStage(null);
      setAiText(null);
    });
    return () => subscription.remove();
  }, [key]);
  const onKeyPress: TextInputProps['onKeyPress'] = (event) => {
    const key = event.nativeEvent.key;
    if (open && ['ArrowDown', 'ArrowUp', 'Down', 'Up'].includes(key)) {
      event.preventDefault();
      const next = Math.max(
        0,
        Math.min(rows.length - 1, active + (['ArrowDown', 'Down'].includes(key) ? 1 : -1)),
      );
      setActive(next);
      if (rows.length) list.current?.scrollToIndex({ index: next, animated: false });
    } else if (key === 'Escape') {
      setDismissed(trigger ? `${trigger.start}:${trigger.kind}:${trigger.query}` : null);
      setStage(null);
    }
    props.onKeyPress?.(event);
  };
  return (
    <View className="gap-1">
      <FormSheet
        visible={customSnooze}
        title={t('inline.ertele')}
        onCancel={() => setCustomSnooze(false)}
        primary={{
          label: t('common:save'),
          disabled: snoozeDate.getTime() <= openedAt.getTime(),
          loading: update.isPending,
          onPress: () =>
            update.mutate(
              { status: 'snoozed', snoozed_until: snoozeDate.toISOString() },
              { onSuccess: () => setCustomSnooze(false), onError: fail },
            ),
        }}
        sections={[
          {
            fields: [
              {
                key: 'snooze',
                kind: 'date',
                label: t('inline.custom_date'),
                value: snoozeDate,
                min: openedAt,
                onChange: setSnoozeDate,
              },
            ],
          },
        ]}
      />
      {copilot.isPending || busy ? <ActivityIndicator accessibilityLabel={t('common:loading')} /> : null}
      {aiText ? (
        <Pressable
          accessibilityRole="button"
          accessibilityLabel={t('inline.use_ai')}
          onPress={() => {
            props.onChangeText(aiText);
            setAiText(null);
            input.current?.focus();
          }}
          className="rounded-xl bg-secondary p-3">
          <Text>{aiText}</Text>
          <Text tone="signal">{t('inline.use_ai')}</Text>
        </Pressable>
      ) : null}
      {open && (trigger?.kind !== '@' || allowMentions) ? (
        // iOS-style floating suggestion card (like Messages' mention picker): rounded,
        // shadowed, 44 pt rows, hairline separators inset past the leading icon.
        <Animated.View
          entering={FadeInDown.duration(160).reduceMotion(ReduceMotion.System)}
          exiting={FadeOutDown.duration(110).reduceMotion(ReduceMotion.System)}
          className="mx-1 mb-1.5 overflow-hidden rounded-2xl bg-card"
          style={{
            maxHeight: 264,
            borderCurve: 'continuous',
            boxShadow: '0 6px 24px rgba(2, 6, 24, 0.14), 0 0 0 0.5px rgba(2, 6, 24, 0.08)',
          }}>
          <FlatList
            ref={list}
            data={rows}
            keyExtractor={(r) => r.id}
            keyboardShouldPersistTaps="always"
            keyboardDismissMode="none"
            onScrollToIndexFailed={() => {}}
            accessibilityLabel={t('inline.announced')}
            contentContainerStyle={{ paddingVertical: 4 }}
            ListEmptyComponent={
              <Text tone="muted" className="px-4 py-3 text-center">
                {t('common:no_results')}
              </Text>
            }
            renderItem={({ item, index }) => {
              const sectionStart = !!item.section && rows[index - 1]?.section !== item.section;
              const lastInSection = rows[index + 1] == null || rows[index + 1]?.section !== item.section;
              return (
                <View>
                  {sectionStart ? (
                    <Text variant="footnote" tone="muted" className="px-4 pb-1 pt-2.5">
                      {item.section}
                    </Text>
                  ) : null}
                  <Pressable
                    accessibilityRole="button"
                    accessibilityLabel={[item.label, item.detail, item.keyword].filter(Boolean).join(', ')}
                    accessibilityState={{ selected: active === index }}
                    disabled={busy}
                    onPress={item.pick}
                    className={`min-h-11 flex-row items-center gap-3 px-4 py-1.5 active:bg-secondary ${active === index ? 'bg-secondary/70' : ''}`}>
                    {item.member ? (
                      <Avatar
                        name={item.label}
                        size={30}
                        dot={item.online ? 'bg-success' : 'bg-muted-foreground'}
                      />
                    ) : item.icon ? (
                      <View className="h-[30px] w-[30px] items-center justify-center rounded-lg bg-secondary">
                        <Icon name={item.icon} size={16} tone="foreground" />
                      </View>
                    ) : null}
                    <View className="min-w-0 flex-1 justify-center">
                      <Text numberOfLines={1}>{item.label}</Text>
                      {item.detail ? (
                        <Text variant="footnote" tone="muted" numberOfLines={1}>
                          {item.detail}
                        </Text>
                      ) : null}
                    </View>
                    {item.keyword ? (
                      <Text variant="footnote" tone="muted" className="tabular-nums">
                        /{item.keyword.replace(/^\//, '')}
                      </Text>
                    ) : null}
                  </Pressable>
                  {!lastInSection ? (
                    <View
                      className="bg-border"
                      style={{
                        height: StyleSheet.hairlineWidth,
                        marginLeft: item.member || item.icon ? 58 : 16,
                      }}
                    />
                  ) : null}
                </View>
              );
            }}
          />
        </Animated.View>
      ) : null}
      {onPasteImage && !props.inputDisabled ? (
        <ClipboardImageOffer clipboard={clipboard} focused={focused} onImage={onPasteImage} />
      ) : null}
      <MessageComposer
        {...props}
        inputRef={input}
        mentions={mentions}
        onChangeText={(value) => {
          // RN has no image paste event. Selection / multi-character changes refresh the explicit offer.
          if (Math.abs(value.length - props.value.length) !== 1) void clipboard.detect();
          props.onChangeText(value);
          setCaret(value.length);
          setActive(0);
          setDismissed(null);
          if (parseTrigger(value)?.kind !== '/') setStage(null);
        }}
        onSelectionChange={(event) => {
          setCaret(event.nativeEvent.selection.end);
          void clipboard.detect();
          props.onSelectionChange?.(event);
        }}
        onFocus={(event) => {
          setFocused(true);
          void clipboard.detect();
          props.onFocus?.(event);
        }}
        onBlur={(event) => {
          props.onBlur?.(event);
          setFocused(false);
          setStage(null);
        }}
        submitBehavior={open ? 'submit' : 'newline'}
        onSubmitEditing={() => {
          if (open && rows[active] && !busy) rows[active].pick();
        }}
        onKeyPress={onKeyPress}
        onSend={() => {
          setDismissed(key);
          setStage(null);
          setAiText(null);
          props.onSend();
        }}
      />
    </View>
  );
}
