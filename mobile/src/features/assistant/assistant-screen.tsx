import * as Haptics from 'expo-haptics';
import { Stack, useFocusEffect, useLocalSearchParams } from 'expo-router';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import * as Clipboard from 'expo-clipboard';
import { Button } from '@/ui/button';
import { useTranslation } from 'react-i18next';
import { FlatList, ActivityIndicator, Pressable, ScrollView, View } from 'react-native';
import { KeyboardAvoidingView, useKeyboardState } from 'react-native-keyboard-controller';
import { useHeaderHeight } from 'expo-router/react-navigation';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { isApiError } from '@/core/api/errors';
import { formatShort, formatTime } from '@/core/format';
import { errorMessage } from '@/core/i18n';
import { useHelpAvailability, useScreenGuides, useSetupGuides } from '@/features/guides/api';
import { MessageComposer } from '@/ui/message-composer';
import { MessageBubble, messageBubbleClass } from '@/features/inbox/thread/message-row';
import { MessageBody } from '@/features/inbox/thread/message-body';
import { FormSheet } from '@/ui/form-sheet';
import { ActionMenu } from '@/ui/menu';
import { QueryState } from '@/ui/query-state';
import Animated, { FadeIn, FadeOut, useReducedMotion } from 'react-native-reanimated';
import { cn } from '@/ui/cn';
import { ConnectionBanner } from '@/ui/connection-banner';
import { EmptyState } from '@/ui/empty-state';
import { HeaderButton } from '@/ui/header-button';
import { Icon } from '@/ui/icon';
import { Input } from '@/ui/input';
import { Markdown } from '@/ui/markdown';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import { ActionCard } from './action-card';
import {
  type AssistantMessage,
  type ThreadContext,
  useAskAssistant,
  useAssistantStatus,
  useAssistantThreads,
  useEscalate,
  useThreadMessages,
} from './api';
import { lastSuggestions, messagesLeft, visibleMessages } from './core';

/** Questions a new conversation suggests before the member types anything. */
const STARTERS = ['starter_setup', 'starter_channels', 'starter_team', 'starter_screen'] as const;

/**
 * ClickFix Asistan (CF-142): the platform's AI for support and setup. A
 * chat like the conversation screen (same bubbles and composer), Markdown
 * answers, quick replies, proposed actions to confirm or reject, the
 * conversation history and a hand-off to ClickFix support. Opened from
 * More, any screen's help menu (with that screen as context) and guide steps.
 */
export function AssistantScreen() {
  const { t } = useTranslation('help');
  const params = useLocalSearchParams<{ route?: string; guide?: string; prompt?: string; thread?: string }>();
  const available = useHelpAvailability((s) => s.assistant);
  const status = useAssistantStatus();
  const [thread, setThread] = useState<string | null>(params.thread ?? null);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [escalateOpen, setEscalateOpen] = useState(false);
  const threads = useAssistantThreads(historyOpen);
  const enabled = available && status.data?.enabled !== false;

  const context: ThreadContext = {
    surface: 'mobile',
    route: params.route || '/assistant',
    ...(params.guide ? { guide_key: params.guide } : {}),
  };

  return (
    <>
      <Stack.Screen
        options={{
          headerLargeTitle: false,
          title: t('assistant.title'),
          headerRight: () =>
            enabled ? (
              <ActionMenu
                title={t('assistant.title')}
                trigger={<HeaderButton icon="more" label={t('menu.title')} />}
                options={[
                  { value: 'new', label: t('assistant.new_thread'), systemImage: 'square.and.pencil' },
                  { value: 'history', label: t('assistant.history'), systemImage: 'clock.arrow.circlepath' },
                  {
                    value: 'support',
                    label: t('assistant.escalate'),
                    systemImage: 'person.crop.circle.badge.questionmark',
                    // No support organization/inbox: escalation would be refused (409).
                    disabled: !thread || status.data?.escalation_available === false,
                  },
                ]}
                onPick={(value) => {
                  if (value === 'history') setHistoryOpen(true);
                  else if (value === 'support') setEscalateOpen(true);
                  else {
                    setThread(null);
                  }
                }}
              />
            ) : null,
        }}
      />
      {!available || (status.isError && isApiError(status.error) && status.error.status === 404) ? (
        <View className="flex-1 justify-center bg-background">
          <EmptyState icon="sparkles" title={t('assistant.unavailable')} />
        </View>
      ) : status.data && !status.data.enabled ? (
        <View className="flex-1 justify-center bg-background">
          <EmptyState
            icon="sparkles"
            title={t('assistant.disabled')}
            body={
              status.data.reason
                ? t(`assistant.reason_${status.data.reason}`, { defaultValue: status.data.reason })
                : undefined
            }
          />
        </View>
      ) : (
        <QueryState query={status}>
          {() => (
            <Chat
              key={thread ?? 'new'}
              thread={thread}
              context={context}
              initialText={thread ? '' : (params.prompt ?? '')}
              left={messagesLeft(status.data)}
              onThread={setThread}
            />
          )}
        </QueryState>
      )}
      <FormSheet
        visible={historyOpen}
        title={t('assistant.history')}
        onCancel={() => setHistoryOpen(false)}
        primary={{
          label: t('assistant.new_thread'),
          onPress: () => {
            setHistoryOpen(false);
            setThread(null);
          },
        }}>
        <ScrollView className="bg-background" contentContainerClassName="p-4 gap-2">
          <QueryState query={threads}>
            {(items) =>
              items.length ? (
                <View className="overflow-hidden rounded-2xl bg-card">
                  {items.map((th) => (
                    <Pressable
                      key={th.uuid}
                      accessibilityRole="button"
                      accessibilityState={{ selected: thread === th.uuid }}
                      onPress={() => {
                        setHistoryOpen(false);
                        setThread(th.uuid);
                      }}
                      className="flex-row items-center gap-3 border-b border-border p-4 active:bg-muted">
                      <Icon name="chat" size={20} tone="muted-foreground" />
                      <View className="flex-1 gap-1">
                        <Text variant="headline">{th.title || t('assistant.untitled')}</Text>
                        <Text variant="caption" tone="muted">
                          {formatShort(th.updated_at)}
                        </Text>
                      </View>
                      {thread === th.uuid ? <Icon name="check" size={18} tone="success" /> : null}
                    </Pressable>
                  ))}
                </View>
              ) : (
                <EmptyState icon="history" title={t('assistant.history_empty')} />
              )
            }
          </QueryState>
        </ScrollView>
      </FormSheet>
      <EscalateSheet thread={thread} visible={escalateOpen} onClose={() => setEscalateOpen(false)} />
    </>
  );
}

type Row = AssistantMessage | { uuid: '__pending'; role: 'pending'; content: string; created_at: string };

function Chat({
  thread,
  context,
  initialText,
  left,
  onThread,
}: {
  thread: string | null;
  context: ThreadContext;
  initialText: string;
  left: number | null;
  onThread: (uuid: string) => void;
}) {
  const { t } = useTranslation('help');
  const insets = useSafeAreaInsets();
  const headerHeight = useHeaderHeight();
  const keyboardVisible = useKeyboardState((state) => state.isVisible);
  const messages = useThreadMessages(thread);
  const ask = useAskAssistant(context);
  const [text, setText] = useState(initialText);
  const [pending, setPending] = useState<string | null>(null);
  const [live, setLive] = useState<AssistantMessage | null>(null);
  const [statusText, setStatusText] = useState('');
  const abort = useRef<AbortController | null>(null);
  const activeThread = useRef(thread);
  const liveRef = useRef<AssistantMessage | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const listRef = useRef<FlatList<Row>>(null);
  const follow = useRef(true);
  const mounted = useRef(true);
  const [jump, setJump] = useState(false);
  const stop = () => abort.current?.abort();
  useFocusEffect(useCallback(() => () => abort.current?.abort(), []));
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      abort.current?.abort();
      if (timer.current) clearTimeout(timer.current);
    };
  }, []);
  const flush = () => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = null;
    if (mounted.current) setLive(liveRef.current ? { ...liveRef.current } : null);
  };
  const onEvent = (name: string, data: unknown) => {
    if (!mounted.current) return;
    if (name === 'message.created')
      liveRef.current = { uuid: '__live', role: 'assistant', content: '', created_at: new Date().toISOString() };
    else if (name === 'status') setStatusText((data as { text: string }).text);
    else if (name === 'delta' && liveRef.current) liveRef.current.content += (data as { text: string }).text;
    else if (name === 'suggestions' && liveRef.current) liveRef.current.suggestions = data as string[];
    else if (name === 'action' && liveRef.current)
      liveRef.current.actions = [
        ...(liveRef.current.actions ?? []),
        data as NonNullable<AssistantMessage['actions']>[number],
      ];
    if (!timer.current) timer.current = setTimeout(flush, 40);
  };
  const shown = useMemo(() => visibleMessages(messages.data ?? []), [messages.data]);
  const suggestions = pending ? [] : lastSuggestions(shown);
  const outOfMessages = left === 0;
  // Newest first: the list is inverted, so the latest answer sits above the composer.
  const rows = useMemo<Row[]>(() => {
    const list: Row[] = [...shown];
    if (pending)
      list.push({ uuid: '__pending', role: 'pending', content: pending, created_at: new Date().toISOString() });
    if (live) list.push(live);
    return list.reverse();
  }, [shown, pending, live]);

  const send = (content: string) => {
    const body = content.trim();
    if (!body || ask.isPending || outOfMessages) return;
    void Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
    setPending(body);
    setText('');
    follow.current = true;
    setJump(false);
    abort.current = new AbortController();
    liveRef.current = null;
    setLive(null);
    setStatusText(t('assistant.typing'));
    ask.mutate(
      {
        thread,
        content: body,
        signal: abort.current.signal,
        onEvent,
        onThread: (id) => {
          activeThread.current = id;
        },
      },
      {
        onSuccess: (r) => {
          if (!mounted.current) return;
          flush();
          liveRef.current = null;
          setLive(null);
          setPending(null);
          void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
          if (r.thread !== thread) onThread(r.thread);
        },
        onError: (e) => {
          if (!mounted.current) return;
          flush();
          liveRef.current = null;
          setLive(null);
          setPending(null);
          if (activeThread.current && activeThread.current !== thread) onThread(activeThread.current);
          if (abort.current?.signal.aborted) return;
          setText(body);
          toast.error(
            isApiError(e) && e.status === 429
              ? t('assistant.limit_reached')
              : errorMessage(isApiError(e) ? e.code : undefined),
          );
        },
      },
    );
  };

  return (
    <KeyboardAvoidingView behavior="padding" keyboardVerticalOffset={headerHeight} className="flex-1 bg-background">
      <ConnectionBanner offlineText={t('common:offline_body')} className="px-4" />
      <FlatList
        ref={listRef}
        onScroll={(e) => {
          follow.current = e.nativeEvent.contentOffset.y < 100;
          setJump(!follow.current);
        }}
        scrollEventThrottle={100}
        onContentSizeChange={() => {
          if (follow.current) listRef.current?.scrollToOffset({ offset: 0, animated: false });
        }}
        inverted
        data={rows}
        keyExtractor={(m) => m.uuid}
        className="flex-1"
        contentContainerClassName="gap-2 px-3 py-3"
        keyboardDismissMode="interactive"
        keyboardShouldPersistTaps="handled"
        accessibilityLabel={t('assistant.title')}
        renderItem={({ item }) => <MessageRow message={item} thread={thread} />}
        ListHeaderComponent={pending ? <TypingIndicator status={statusText} /> : undefined}
        ListEmptyComponent={
          thread ? (
            <View style={{ transform: [{ scaleY: -1 }] }}>
              <QueryState query={messages}>
                {() => <EmptyState icon="chat" title={t('assistant.untitled')} />}
              </QueryState>
            </View>
          ) : (
            <View style={{ transform: [{ scaleY: -1 }] }}>
              <Welcome onPick={send} route={context.route ?? '/assistant'} />
            </View>
          )
        }
      />
      <View
        className="gap-2 border-t border-border bg-background px-3 pt-2"
        style={{ paddingBottom: keyboardVisible ? 8 : Math.max(insets.bottom, 8) }}>
        {jump ? (
          <Button
            variant="secondary"
            title={t('assistant.latest')}
            onPress={() => {
              follow.current = true;
              setJump(false);
              listRef.current?.scrollToOffset({ offset: 0, animated: true });
            }}
          />
        ) : null}
        {pending ? (
          <Button variant="secondary" title={t('assistant.stop')} onPress={stop} />
        ) : shown.some((m) => m.role === 'assistant') ? (
          <Button
            variant="ghost"
            title={t('assistant.regenerate')}
            onPress={() => send([...shown].reverse().find((m) => m.role === 'user')?.content ?? '')}
          />
        ) : null}
        {suggestions.length ? (
          <ScrollView
            horizontal
            showsHorizontalScrollIndicator={false}
            contentContainerClassName="gap-2"
            keyboardShouldPersistTaps="handled">
            {suggestions.map((s) => (
              <SuggestionChip key={s} label={s} onPress={() => send(s)} />
            ))}
          </ScrollView>
        ) : null}
        {outOfMessages ? (
          <Text variant="footnote" tone="muted">
            {t('assistant.limit_reached')}
          </Text>
        ) : left !== null && left <= 5 ? (
          <Text variant="footnote" tone="muted">
            {t('assistant.left', { count: left })}
          </Text>
        ) : null}
        <MessageComposer
          value={text}
          onChangeText={setText}
          inputDisabled={outOfMessages || ask.isPending}
          placeholder={t('assistant.placeholder')}
          label={t('assistant.placeholder')}
          sendLabel={t('assistant.send')}
          disabled={!text.trim() || outOfMessages || ask.isPending}
          maxLength={4000}
          onSend={() => send(text)}
        />
      </View>
    </KeyboardAvoidingView>
  );
}

function SuggestionChip({ label, onPress }: { label: string; onPress: () => void }) {
  return (
    <Pressable
      accessibilityRole="button"
      onPressIn={() => void Haptics.selectionAsync()}
      onPress={onPress}
      className="max-w-72 rounded-full border border-border bg-card px-3.5 py-2 active:bg-muted">
      <Text variant="footnote" className="font-sans-semibold" numberOfLines={1}>
        {label}
      </Text>
    </Pressable>
  );
}

/** A new conversation: what the assistant does and a few questions to start with. */
function Welcome({ onPick, route }: { onPick: (q: string) => void; route: string }) {
  const { t } = useTranslation('help');
  const setup = useSetupGuides();
  const page = useScreenGuides(route);
  const pageTitle = page.data?.find((g) => g.kind === 'page')?.title;
  const remaining =
    setup.data?.items.filter((g) => g.progress.status !== 'completed' && g.progress.status !== 'skipped').slice(0, 3) ??
    [];
  const starters = [
    ...new Set([
      ...(pageTitle ? [t('assistant.starter_context', { title: pageTitle })] : []),
      ...remaining.map((g) => g.title),
      ...STARTERS.map((k) => t(`assistant.${k}`)),
    ]),
  ].slice(0, 4);
  return (
    <View className="gap-4 px-2 py-8">
      <View className="items-center gap-2">
        <View className="size-14 items-center justify-center rounded-full bg-signal">
          <Icon name="sparkles" size={26} tone="foreground" />
        </View>
        <Text variant="title" className="text-center">
          {t('assistant.welcome_title')}
        </Text>
        <Text variant="callout" tone="muted" className="text-center">
          {t('assistant.welcome_body')}
        </Text>
      </View>
      <View className="gap-2">
        {starters.map((question, index) => (
          <Pressable
            key={question}
            accessibilityRole="button"
            onPressIn={() => void Haptics.selectionAsync()}
            onPress={() => onPick(question)}
            className="flex-row items-center gap-3 rounded-xl border border-border bg-card px-4 py-3 active:bg-muted"
            style={{ borderCurve: 'continuous' }}>
            <Icon
              name={(['checklist', 'channel', 'staff', 'sparkles'] as const)[index]}
              size={20}
              tone="muted-foreground"
            />
            <Text variant="callout" className="flex-1">
              {question}
            </Text>
            <Icon name="forward" size={16} tone="muted-foreground" />
          </Pressable>
        ))}
      </View>
    </View>
  );
}

function MessageRow({ message: m, thread }: { message: Row; thread: string | null }) {
  const { t } = useTranslation('help');
  const mine = m.role === 'user' || m.role === 'pending';
  const actions = m.role === 'assistant' ? (m.actions ?? []) : [];
  return (
    <View className={cn('max-w-[88%] gap-1.5', mine ? 'self-end' : 'self-start')} style={{ maxWidth: '92%' }}>
      {m.content ? (
        <ActionMenu
          context
          title={t('assistant.title')}
          options={[{ value: 'copy', label: t('assistant.copy'), systemImage: 'doc.on.doc' }]}
          onPick={() => {
            void Clipboard.setStringAsync(m.content);
          }}
          trigger={
            <MessageBubble
              accessible
              accessibilityLabel={`${mine ? t('assistant.you') : t('assistant.title')}: ${m.content}`}
              className={cn(messageBubbleClass({ inbound: !mine }), m.role === 'pending' && 'opacity-70')}>
              {mine ? <MessageBody body={m.content} /> : <Markdown source={m.content} />}
            </MessageBubble>
          }
        />
      ) : null}
      {thread ? actions.map((a) => <ActionCard key={a.id} thread={thread} action={a} />) : null}
      {m.role !== 'pending' ? (
        <Text variant="caption" tone="muted" className={cn('px-1', mine ? 'self-end' : 'self-start')}>
          {formatTime(m.created_at)}
        </Text>
      ) : null}
    </View>
  );
}

/** Hand-off to ClickFix support: an optional note, then the ticket reference. */
function EscalateSheet({ thread, visible, onClose }: { thread: string | null; visible: boolean; onClose: () => void }) {
  const { t } = useTranslation('help');
  const escalate = useEscalate(thread);
  const [note, setNote] = useState('');
  return (
    <FormSheet
      visible={visible}
      title={t('assistant.escalate')}
      onCancel={onClose}
      dirty={!!note.trim()}
      primary={{
        label: t('assistant.escalate_send'),
        loading: escalate.isPending,
        disabled: !thread,
        onPress: () =>
          escalate.mutate(note, {
            onSuccess: (r) => {
              toast.success(t('assistant.escalated', { ref: r.ticket_ref }));
              setNote('');
              onClose();
            },
            onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)),
          }),
      }}>
      <ScrollView className="bg-background" contentContainerClassName="gap-4 p-5" keyboardShouldPersistTaps="handled">
        <Text variant="callout" tone="muted">
          {t('assistant.escalate_body')}
        </Text>
        <Input
          label={t('assistant.escalate_note')}
          value={note}
          onChangeText={setNote}
          multiline
          maxLength={2000}
          className="min-h-28 py-3"
        />
      </ScrollView>
    </FormSheet>
  );
}

function TypingIndicator({ status }: { status?: string }) {
  const { t } = useTranslation('help');
  const reduced = useReducedMotion();
  return (
    <Animated.View
      entering={reduced ? undefined : FadeIn.duration(180)}
      exiting={reduced ? undefined : FadeOut.duration(120)}
      accessibilityLiveRegion="polite"
      accessibilityLabel={t('assistant.typing')}
      className="my-3 flex-row items-center gap-2 self-start rounded-2xl border border-border bg-card px-4 py-3">
      <ActivityIndicator size="small" />
      <Text variant="footnote" tone="muted">
        {status || t('assistant.typing')}
      </Text>
    </Animated.View>
  );
}
