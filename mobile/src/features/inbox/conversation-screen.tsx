import { HeaderActions } from '@/features/guides/help-menu';
import { GuideTarget } from '@/ui/guide-target';
import { channelIcon as conversationChannelIcon } from '@/ui/channel';
import { nativeChoice } from '@/ui/native-choice';
import * as DocumentPicker from 'expo-document-picker';
import * as Haptics from 'expo-haptics';
import * as ImagePicker from 'expo-image-picker';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useHeaderHeight } from 'expo-router/react-navigation';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, FlatList, Pressable, useWindowDimensions, View } from 'react-native';
import { KeyboardAvoidingView, useKeyboardState } from 'react-native-keyboard-controller';
import Animated, { FadeIn, FadeOut } from 'react-native-reanimated';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { dismissComposerSuggestions, InlineComposer } from './thread/composer-suggestions';
import { serializeMentions } from './thread/composer-tokens';

import { isApiError } from '@/core/api/errors';
import { errorMessage, i18n } from '@/core/i18n';
import { CopilotSheet } from '@/features/copilot/copilot-sheet';
import { cn } from '@/ui/cn';
import { ConnectionBanner } from '@/ui/connection-banner';
import { EmptyState } from '@/ui/empty-state';
import { HeaderButton } from '@/ui/header-button';
import { Icon } from '@/ui/icon';
import { QueryState } from '@/ui/query-state';
import { OptionSheet } from '@/ui/select';
import { Skeleton } from '@/ui/skeleton';
import { StatusPill, type StatusTone } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import {
  type ConversationDetail,
  type LocalFile,
  useConversation,
  useInboxLive,
  useInboxScope,
  useMarkRead,
  useMessages,
  useSendMessage,
  useUpdateConversation,
} from './api';
import { useViewers } from './conversation-api';
import { AssigneeMenu, ConversationMenu, ViewersBar } from './conversation-tools';
import { readDraft, writeDraft } from './drafts';
import { contactLabel, fallbackFileName } from './helpers';
import { ComposerAttachmentTray, useComposerAttachments } from './thread/composer-attachments';
import { SlaBadge } from './sla-badge';
import { dayKey, MessageRow } from './thread/message-row';
import { collapseEvents, timelineMarks } from './timeline';

const STATUS_TONE: Record<string, StatusTone> = {
  open: 'progress',
  pending: 'progress',
  snoozed: 'neutral',
  resolved: 'done',
  closed: 'done',
};

/**
 * One conversation: the timeline (newest at the bottom, older pages on
 * scroll up), a reply / internal note composer that rides on the keyboard,
 * and resolve / reopen in the header. New customer messages arrive over the
 * inbox channel and mark the conversation read while it is open.
 */
export function ConversationScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  return <ConversationPane uuid={uuid} />;
}

/**
 * The conversation itself. As a pushed screen its actions sit in the native
 * header; embedded in the tablet split view (`embedded`) it draws its own
 * header bar beside the list.
 */
export function ConversationPane({ uuid, embedded = false }: { uuid: string; embedded?: boolean }) {
  const { t } = useTranslation('conversation');
  const conversation = useConversation(uuid);
  const headerHeight = useHeaderHeight();
  const insets = useSafeAreaInsets();
  const markRead = useMarkRead(uuid);
  useInboxLive(uuid, markRead);

  useEffect(() => {
    if (conversation.data) markRead();
    // Once per open conversation; live inbound messages mark it again.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [conversation.data?.uuid]);

  const actions = conversation.data ? (
    <View className="flex-row items-center gap-1">
      <StatusAction conversation={conversation.data} />
      <ConversationMenu
        conversation={conversation.data}
        trigger={<HeaderButton icon="more" guide="conversation.menu" label={t('menu.title')} onPress={() => {}} />}
      />
    </View>
  ) : null;

  return (
    <View
      className="flex-1 bg-background"
      // The status strip and composer are siblings of the list: inset the entire pane once.
      style={{
        paddingTop: embedded ? 0 : headerHeight,
        paddingLeft: insets.left,
        paddingRight: insets.right,
      }}>
      {embedded ? (
        <View className="flex-row items-center gap-2 border-b border-border bg-background px-4 py-2.5">
          <View className="min-w-0 flex-1">
            <ConversationTitle conversation={conversation.data} />
          </View>
          {actions}
        </View>
      ) : (
        <Stack.Screen
          options={{
            title: conversation.data ? contactLabel(conversation.data) : '',
            headerTransparent: true,
            headerLargeTitle: false,
            headerBackButtonDisplayMode: 'minimal',
            headerTitle: () => (
              <View style={{ maxWidth: 180 }}>
                <ConversationTitle conversation={conversation.data} />
              </View>
            ),
            headerRight: () => <HeaderActions>{actions}</HeaderActions>,
          }}
        />
      )}
      {conversation.data ? (
        <>
          <Thread key={uuid} conversation={conversation.data} />
        </>
      ) : (
        <View className="flex-1 bg-background px-4 pt-4">
          <QueryState query={conversation}>{() => null}</QueryState>
        </View>
      )}
    </View>
  );
}

/** A compact two-line title leaves space for back, resolve and the menu. */
function ConversationTitle({ conversation: c }: { conversation?: ConversationDetail }) {
  const { t } = useTranslation('channels');
  if (!c) return null;
  return (
    <View>
      <Text variant="headline" numberOfLines={1} accessibilityRole="header">
        {contactLabel(c)}
      </Text>
      <View className="flex-row items-center gap-1">
        <Icon name={conversationChannelIcon(c.channel_type)} size={12} tone="muted-foreground" />
        <Text variant="caption" tone="muted" numberOfLines={1}>
          {t(`types.${c.channel_type}`, { defaultValue: c.channel_type })} · #{c.number}
        </Text>
      </View>
    </View>
  );
}

function StatusAction({ conversation: c }: { conversation: ConversationDetail }) {
  const { t } = useTranslation('inbox');
  const { canManage } = useInboxScope();
  const update = useUpdateConversation(c.uuid);
  if (!canManage) return null;
  const closed = c.status === 'resolved' || c.status === 'closed';
  return update.isPending ? (
    <ActivityIndicator />
  ) : (
    <HeaderButton
      guide="conversation.status"
      icon={closed ? 'history' : 'check'}
      label={closed ? t('header.reopen') : t('header.resolve')}
      disabled={update.isPending}
      onPress={() =>
        update.mutate(
          { status: closed ? 'open' : 'resolved' },
          { onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)) },
        )
      }
    />
  );
}

function Thread({ conversation: c }: { conversation: ConversationDetail }) {
  const { t } = useTranslation('inbox');
  const messages = useMessages(c.uuid);
  const rows = useMemo(() => collapseEvents(messages.items, dayKey), [messages.items]);
  const chronologicalRows = useMemo(() => [...rows].reverse(), [rows]);
  const list = useRef<FlatList<(typeof rows)[number]>>(null);
  const positioned = useRef(false);
  const { width: windowWidth } = useWindowDimensions();
  const [threadWidth, setThreadWidth] = useState(windowWidth);
  const bubbleMaxWidth = Math.min(560, Math.max(0, threadWidth - 32) * 0.78);
  const [away, setAway] = useState(false);
  const marks = useMemo(
    () =>
      timelineMarks(
        rows.map((r) => r.message),
        dayKey,
      ),
    [rows],
  );
  // A new message while the agent reads older ones lights the "latest" button, never a jump.
  const newest = messages.items[0]?.client_id ?? messages.items[0]?.uuid;
  const [newestWhenLeft, setNewestWhenLeft] = useState<string | undefined>(undefined);
  const unseen = away && newest !== newestWhenLeft;
  const [typing, setTyping] = useState(false);
  const viewers = useViewers(c.uuid, typing);
  const keyboardVisible = useKeyboardState((state) => state.isVisible);
  useEffect(() => {
    if (keyboardVisible && !away) list.current?.scrollToEnd({ animated: false });
  }, [keyboardVisible, away]);

  return (
    <KeyboardAvoidingView
      behavior="translate-with-padding"
      // Measure in window coordinates, including the native header and the embedded toolbar.
      automaticOffset
      className="flex-1 bg-background">
      <View className="flex-row items-center gap-2 border-b border-border px-4 py-1">
        <StatusPill
          icon={
            c.status === 'snoozed'
              ? 'snoozed'
              : c.status === 'resolved' || c.status === 'closed'
                ? 'accept'
                : 'statusOpen'
          }
          label={t(`status.${c.status}`)}
          tone={STATUS_TONE[c.status] ?? 'neutral'}
        />
        <AssigneeMenu conversation={c} />
        {c.priority === 'urgent' || c.priority === 'high' ? (
          <Text variant="caption" uppercase={false} tone={c.priority === 'urgent' ? 'danger' : 'muted'}>
            {t(`priority.${c.priority}`)}
          </Text>
        ) : null}
        <SlaBadge conversation={c} />
      </View>
      <ViewersBar viewers={viewers} />
      <ConnectionBanner offlineText={t('conversation:offline')} className="px-4" />
      {messages.isError && messages.data ? (
        <Pressable
          accessibilityRole="button"
          onPress={() => void messages.refetch()}
          className="flex-row items-center justify-between gap-2 bg-destructive/10 px-4 py-2">
          <Text variant="footnote" tone="danger" className="flex-1">
            {t('common:load_failed')}
          </Text>
          <Text variant="footnote" className="font-sans-semibold">
            {t('common:retry')}
          </Text>
        </Pressable>
      ) : null}
      {messages.isError && !messages.data ? (
        <View className="flex-1 justify-center">
          <EmptyState
            icon="error"
            title={t('common:load_failed')}
            body={errorMessage(isApiError(messages.error) ? messages.error.code : undefined)}
            action={{ title: t('common:retry'), icon: 'refresh', onPress: () => void messages.refetch() }}
          />
        </View>
      ) : (
        <GuideTarget id="conversation.timeline" className="flex-1">
          <View className="flex-1" onLayout={({ nativeEvent }) => setThreadWidth(nativeEvent.layout.width)}>
            <FlatList
              onTouchStart={dismissComposerSuggestions}
              ref={list}
              // A normal UIKit coordinate space lets native context-menu gestures and previews
              // hit the actual bubble; an inverted list transforms every hosting controller.
              data={chronologicalRows}
              keyExtractor={({ message: m }) => m.client_id ?? m.uuid}
              className="flex-1"
              contentContainerClassName="flex-grow justify-end px-4 py-3"
              contentInsetAdjustmentBehavior="never"
              automaticallyAdjustKeyboardInsets={false}
              maintainVisibleContentPosition={{ minIndexForVisible: 0 }}
              onContentSizeChange={() => {
                if (!away && messages.items.length) {
                  list.current?.scrollToEnd({ animated: false });
                  positioned.current = true;
                }
              }}
              keyboardDismissMode="interactive"
              keyboardShouldPersistTaps="handled"
              accessibilityLabel={t('timeline.label')}
              renderItem={({ item, index }) => (
                <MessageRow
                  message={item.message}
                  eventCount={item.count}
                  mark={marks[rows.length - 1 - index]}
                  conversationUuid={c.uuid}
                  maxWidth={bubbleMaxWidth}
                />
              )}
              onScroll={(e) => {
                if (!positioned.current) return;
                const { contentOffset, contentSize, layoutMeasurement } = e.nativeEvent;
                const next = contentSize.height - layoutMeasurement.height - contentOffset.y > 320;
                if (next && !away) setNewestWhenLeft(newest);
                if (next !== away) setAway(next);
              }}
              scrollEventThrottle={64}
              onStartReachedThreshold={0.3}
              onStartReached={() => {
                if (positioned.current && away && messages.hasNextPage && !messages.isFetchingNextPage)
                  void messages.fetchNextPage();
              }}
              ListEmptyComponent={
                messages.isPending ? (
                  <View className="gap-4 py-4">
                    <Skeleton className="h-16 w-3/4 rounded-3xl" />
                    <Skeleton className="h-12 w-2/3 self-end rounded-3xl" />
                    {c.last_message_preview ? (
                      <View
                        className={cn(
                          'max-w-[78%] rounded-[18px] border border-border px-3.5 py-2.5',
                          c.last_message_direction === 'in' ? 'self-start bg-card' : 'self-end bg-signal-soft',
                        )}>
                        <Text>{c.last_message_preview}</Text>
                      </View>
                    ) : (
                      <Skeleton className="h-20 w-3/4 rounded-3xl" />
                    )}
                  </View>
                ) : (
                  <View>
                    <EmptyState icon="inbox" title={t('conversation:timeline.empty')} />
                  </View>
                )
              }
              ListHeaderComponent={messages.isFetchingNextPage ? <ActivityIndicator className="py-4" /> : undefined}
            />
            {away ? (
              <Animated.View entering={FadeIn} exiting={FadeOut} className="absolute bottom-4 right-4">
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={
                    unseen ? t('timeline.new_messages', { count: 1 }) : t('conversation:timeline.latest')
                  }
                  onPress={() => list.current?.scrollToEnd({ animated: true })}
                  className={cn(
                    'h-10 flex-row items-center gap-1.5 rounded-full border border-border px-3 active:opacity-80',
                    unseen ? 'bg-signal-soft' : 'bg-card',
                  )}
                  style={{ boxShadow: '0 4px 14px rgba(2, 6, 24, 0.16)' }}>
                  <Icon name="down" size={16} tone="foreground" />
                  {unseen ? (
                    <Text variant="footnote" className="font-sans-semibold">
                      {t('conversation:timeline.new')}
                    </Text>
                  ) : null}
                </Pressable>
              </Animated.View>
            ) : null}
          </View>
        </GuideTarget>
      )}
      <Composer conversation={c} onTyping={setTyping} />
    </KeyboardAvoidingView>
  );
}

type AttachSource = 'camera' | 'library' | 'file';

/** Picks files from the camera, the photo library or Files; null when the user backs out or says no. */
async function pickFiles(source: AttachSource): Promise<LocalFile[] | null> {
  if (source === 'file') {
    const r = await DocumentPicker.getDocumentAsync({ multiple: true, copyToCacheDirectory: true });
    if (r.canceled) return null;
    return r.assets.map((a) => ({
      uri: a.uri,
      name: a.name,
      type: a.mimeType ?? 'application/octet-stream',
    }));
  }
  // The system photo picker runs out of process and needs no library permission;
  // asking for it would show an unnecessary "allow access to all photos" prompt.
  const perm = source === 'camera' ? await ImagePicker.requestCameraPermissionsAsync() : { granted: true };
  if (!perm.granted) {
    toast.error(
      i18n.t(source === 'camera' ? 'conversation:attach.camera_denied' : 'conversation:attach.library_denied'),
    );
    return null;
  }
  const options: ImagePicker.ImagePickerOptions = {
    mediaTypes: source === 'camera' ? ['images'] : ['images', 'videos'],
    quality: 0.8,
    allowsMultipleSelection: source === 'library',
    selectionLimit: 10,
  };
  const r =
    source === 'camera'
      ? await ImagePicker.launchCameraAsync(options)
      : await ImagePicker.launchImageLibraryAsync(options);
  if (r.canceled) return null;
  return r.assets.map((a) => {
    const type = a.mimeType ?? (a.type === 'video' ? 'video/mp4' : 'image/jpeg');
    return { uri: a.uri, name: a.fileName || fallbackFileName(type), type };
  });
}

type Tool = AttachSource;

function Composer({ conversation: c, onTyping }: { conversation: ConversationDetail; onTyping: (v: boolean) => void }) {
  const { t } = useTranslation('inbox');
  const insets = useSafeAreaInsets();
  const keyboardVisible = useKeyboardState((state) => state.isVisible);
  const { org, canReply, canNote, canUpload } = useInboxScope();
  const [draft] = useState(() => readDraft(org, c.uuid));
  const [mode, setMode] = useState<'reply' | 'note'>(
    draft?.mode === 'note' && canNote ? 'note' : canReply ? 'reply' : 'note',
  );
  const [text, setText] = useState(draft?.text ?? '');
  const attachments = useComposerAttachments();
  const [toolsOpen, setToolsOpen] = useState(false);
  const [mentions, setMentions] = useState<{ uuid: string; name: string }[]>([]);
  const [copilotOpen, setCopilotOpen] = useState(false);
  const send = useSendMessage(c.uuid);

  // The draft survives leaving the screen, a call or an app kill (saved shortly after typing stops).
  useEffect(() => {
    const id = setTimeout(() => writeDraft(org, c.uuid, { text, mode }), 400);
    return () => clearTimeout(id);
  }, [org, c.uuid, text, mode]);

  // Others see "typing" while text changed in the last few seconds (the viewers bar's collision warning).
  useEffect(() => {
    if (!text) return onTyping(false);
    onTyping(true);
    const id = setTimeout(() => onTyping(false), 4000);
    return () => clearTimeout(id);
  }, [text, onTyping]);

  const pickTool = (tool: Tool) => {
    setToolsOpen(false);
    void attach(tool);
  };

  if (!canReply && !canNote) return null;

  const windowClosed = mode === 'reply' && !c.window_open;
  const noChannel = mode === 'reply' && !c.inbox;
  const blocked = windowClosed || noChannel;
  const uploading = attachments.pending || attachments.failed;
  const ready = attachments.ready;
  const canSend =
    (mode === 'note' ? canNote : canReply) && !blocked && !uploading && (text.trim().length > 0 || ready.length > 0);

  const submit = () => {
    if (!canSend) return;
    void Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
    const content = text.trim();
    // Only teammates whose @name is still in the text are notified.
    const mentioned = mode === 'note' ? serializeMentions(content, mentions) : [];
    send({ content, private: mode === 'note', attachments: ready, mentions: mentioned });
    setText('');
    attachments.clear();
    setMentions([]);
    writeDraft(org, c.uuid, { text: '', mode });
  };

  const attach = async (source: AttachSource) => {
    let files: LocalFile[] | null = null;
    try {
      files = await pickFiles(source);
    } catch {
      toast.error(t('conversation:attach.failed'));
    }
    attachments.add(files ?? []);
  };

  return (
    <View
      className="border-t border-border bg-background px-3 pt-2"
      style={{ paddingBottom: keyboardVisible ? 8 : Math.max(insets.bottom, 8) }}>
      <View className="gap-1">
        {blocked ? (
          <Text variant="footnote" tone="muted">
            {noChannel ? t('composer.no_channel') : t('composer.window_closed')}
          </Text>
        ) : null}
        <ComposerAttachmentTray attachments={attachments} caption={text} />
        <GuideTarget id="conversation.composer">
          <InlineComposer
            onPasteImage={canUpload && !blocked ? (file) => attachments.add([file]) : undefined}
            conversation={c}
            mentions={mentions}
            allowMentions={canNote}
            onMention={(member) => setMentions((cur) => [...cur.filter((m) => m.uuid !== member.uuid), member])}
            onNote={
              canNote
                ? () => {
                    if (mode !== 'note') toast.success(t('conversation:inline.note_hint'));
                    setMode('note');
                  }
                : undefined
            }
            onReply={canReply ? () => setMode('reply') : undefined}
            onAttach={
              canUpload && !blocked
                ? () =>
                    nativeChoice(
                      t('conversation:composer.tools'),
                      (['camera', 'library', 'file'] as const).map((source) => ({
                        label: t(`conversation:attach.${source}`),
                        onPress: () => void attach(source),
                      })),
                      t('common:cancel'),
                    )
                : undefined
            }
            value={text}
            onChangeText={setText}
            placeholder={mode === 'note' ? t('composer.note') : t('conversation:composer.message')}
            label={mode === 'note' ? t('composer.note') : t('composer.reply')}
            sendLabel={mode === 'note' ? t('composer.add_note') : t('composer.send')}
            onSend={submit}
            disabled={!canSend}
            inputDisabled={blocked}
            note={mode === 'note'}
            before={
              canUpload && !blocked ? (
                <OptionSheet<Tool>
                  trigger={
                    <Pressable
                      accessibilityRole="button"
                      accessibilityLabel={t('conversation:composer.tools')}
                      disabled={false}
                      onPress={() => setToolsOpen(true)}
                      className="size-11 items-center justify-center rounded-full">
                      <Icon name="add" size={21} tone="secondary-foreground" />
                    </Pressable>
                  }
                  disabled={false}
                  visible={toolsOpen}
                  title={t('conversation:composer.tools')}
                  options={([...(canUpload && !blocked ? (['camera', 'library', 'file'] as const) : [])] as Tool[]).map(
                    (v) => ({
                      value: v,
                      label: t(`conversation:attach.${v}`),
                    }),
                  )}
                  onClose={() => setToolsOpen(false)}
                  onPick={pickTool}
                />
              ) : undefined
            }
            leading={
              <Pressable
                accessibilityRole="button"
                accessibilityLabel={t(`composer.${mode}`)}
                accessibilityHint={t(`composer.${mode === 'reply' ? 'note' : 'reply'}`)}
                disabled={!canReply || !canNote}
                onPress={() => {
                  void Haptics.selectionAsync();
                  setMode(mode === 'reply' ? 'note' : 'reply');
                }}
                hitSlop={6}
                className="h-11 w-9 items-center justify-center pl-1">
                <Icon
                  name={mode === 'note' ? 'lockClosed' : 'reply'}
                  size={16}
                  tone={mode === 'note' ? 'warning' : 'muted-foreground'}
                />
              </Pressable>
            }
            trailing={
              canReply ? (
                <Pressable
                  accessibilityRole="button"
                  accessibilityLabel={t('copilot:title')}
                  onPress={() => setCopilotOpen(true)}
                  hitSlop={6}
                  className="h-11 w-10 items-center justify-center pr-1">
                  <Icon name="sparkles" size={18} tone="muted-foreground" />
                </Pressable>
              ) : null
            }
          />
        </GuideTarget>
      </View>
      <CopilotSheet
        visible={copilotOpen}
        conversation={c.uuid}
        draft={text}
        canNote={canNote}
        onClose={() => setCopilotOpen(false)}
        onUseDraft={(v) => {
          if (canReply) setMode('reply');
          setText(v);
        }}
        onUseNote={(v) => {
          setMode('note');
          setText(v);
        }}
      />
    </View>
  );
}
