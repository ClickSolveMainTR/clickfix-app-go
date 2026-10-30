import { InlineComposer, dismissComposerSuggestions } from '@/features/inbox/thread/composer-suggestions';
import { serializeMentions, type Mention } from '@/features/inbox/thread/composer-tokens';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, FlatList, Pressable, View } from 'react-native';
import { KeyboardAvoidingView, useKeyboardState } from 'react-native-keyboard-controller';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { isApiError } from '@/core/api/errors';
import { useSession } from '@/core/auth/session';
import { formatShort } from '@/core/format';
import { errorMessage } from '@/core/i18n';
import { cn } from '@/ui/cn';
import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import { QueryState } from '@/ui/query-state';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import { useAskAi, useMarkThreadRead, useSendThreadMessage, useThreadMessages } from './api';
import { useMemberNames } from './names';

/** One team chat thread: messages (polled), reply, and "Ask AI" with the organization's knowledge. */
export function ThreadScreen() {
  const { t } = useTranslation('team');
  const { uuid, title } = useLocalSearchParams<{ uuid: string; title?: string }>();
  const me = useSession((s) => s.me?.user?.uuid);
  const messages = useThreadMessages(uuid);
  const read = useMarkThreadRead(uuid);
  const nameOf = useMemberNames();
  const items = [...(messages.data ?? [])].reverse();
  const newest = items[0]?.uuid;

  useEffect(() => {
    if (newest) read.mutate();
    // Mark read when a new message shows up while the thread is open.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [newest]);

  return (
    <>
      <Stack.Screen options={{ headerLargeTitle: false, title: title || t('chat.title') }} />
      <KeyboardAvoidingView behavior="translate-with-padding" automaticOffset className="flex-1 bg-background">
        {messages.data === undefined ? (
          <View className="flex-1 p-4">
            <QueryState query={messages}>{() => null}</QueryState>
          </View>
        ) : items.length === 0 ? (
          <View className="flex-1 justify-center">
            <EmptyState icon="chat" title={t('chat.no_messages')} />
          </View>
        ) : (
          <FlatList
            onTouchStart={dismissComposerSuggestions}
            inverted
            data={items}
            keyExtractor={(m) => m.uuid}
            keyboardDismissMode="interactive"
            keyboardShouldPersistTaps="handled"
            automaticallyAdjustKeyboardInsets={false}
            maintainVisibleContentPosition={{ minIndexForVisible: 0, autoscrollToTopThreshold: 80 }}
            className="flex-1"
            contentContainerClassName="gap-2 p-3"
            renderItem={({ item: m }) => {
              const mine = m.sender_uuid === me && !m.is_ai;
              return (
                <View className={cn('max-w-[85%] gap-1', mine ? 'self-end' : 'self-start')} style={{ maxWidth: 560 }}>
                  <View
                    className={cn(
                      'rounded-2xl px-3.5 py-2.5',
                      mine
                        ? 'bg-primary'
                        : m.is_ai
                          ? 'border border-signal/50 bg-signal-soft'
                          : 'border border-border bg-card',
                    )}
                    style={{ borderCurve: 'continuous' }}>
                    <Text variant="callout" selectable tone={mine ? 'onPrimary' : 'default'}>
                      {m.body}
                    </Text>
                  </View>
                  <Text variant="caption" tone="muted" className={mine ? 'self-end' : 'self-start'}>
                    {[mine ? null : m.is_ai ? t('chat.ai') : nameOf(m.sender_uuid), formatShort(m.created_at)]
                      .filter(Boolean)
                      .join(' · ')}
                  </Text>
                </View>
              );
            }}
          />
        )}
        <ThreadComposer uuid={uuid} />
      </KeyboardAvoidingView>
    </>
  );
}

function ThreadComposer({ uuid }: { uuid: string }) {
  const { t } = useTranslation('team');
  const insets = useSafeAreaInsets();
  const keyboardVisible = useKeyboardState((state) => state.isVisible);
  const send = useSendThreadMessage(uuid);
  const ask = useAskAi(uuid);
  const [text, setText] = useState('');
  const [mentions, setMentions] = useState<Mention[]>([]);
  const busy = send.isPending || ask.isPending;
  const ok = !!text.trim() && !busy;
  const fail = (e: unknown) => toast.error(errorMessage(isApiError(e) ? e.code : undefined));
  return (
    <View
      className="border-t border-border px-3 pt-2"
      style={{ paddingBottom: keyboardVisible ? 8 : Math.max(insets.bottom, 8) }}>
      <InlineComposer
        value={text}
        onChangeText={setText}
        label={t('chat.placeholder')}
        placeholder={t('conversation:composer.message')}
        sendLabel={t('chat.send')}
        disabled={!ok}
        onSend={() =>
          send.mutate(
            { body: text.trim(), mentions: serializeMentions(text, mentions) },
            {
              onSuccess: () => {
                setText('');
                setMentions([]);
              },
              onError: fail,
            },
          )
        }
        allowMentions={true}
        mentions={mentions}
        onMention={(member) => setMentions((cur) => [...cur, member])}
        onAi={() => ask.mutate(text.trim(), { onSuccess: () => setText(''), onError: fail })}
        before={
          <Pressable
            accessibilityRole="button"
            accessibilityLabel={t('chat.ask_ai')}
            disabled={!ok}
            onPress={() => ask.mutate(text.trim(), { onSuccess: () => setText(''), onError: fail })}
            className={cn('size-11 items-center justify-center rounded-full bg-secondary', !ok && 'opacity-40')}>
            {ask.isPending ? <ActivityIndicator /> : <Icon name="sparkles" size={19} tone="secondary-foreground" />}
          </Pressable>
        }
      />
    </View>
  );
}
