import { InlineComposer, dismissComposerSuggestions } from '@/features/inbox/thread/composer-suggestions';
import { type Mention } from '@/features/inbox/thread/composer-tokens';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FlatList, View } from 'react-native';
import { KeyboardAvoidingView, useKeyboardState } from 'react-native-keyboard-controller';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { isApiError } from '@/core/api/errors';
import { formatShort } from '@/core/format';
import { errorMessage } from '@/core/i18n';
import { HeaderActions } from '@/features/guides/help-menu';
import { cn } from '@/ui/cn';
import { EmptyState } from '@/ui/empty-state';
import { HeaderButton } from '@/ui/header-button';
import { QueryState } from '@/ui/query-state';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import { useInboxScope } from './api';
import { useReplySide, useSetSideStatus, useSideConversation } from './conversation-api';

/**
 * A side conversation (CF-79): the e-mail thread with a third party (a
 * supplier, another department) next to the customer conversation. Read,
 * reply, close / reopen.
 */
export function SideConversationScreen() {
  const { t } = useTranslation('conversation');
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const side = useSideConversation(uuid);
  const status = useSetSideStatus(uuid);
  const s = side.data;
  return (
    <>
      <Stack.Screen
        options={{
          headerLargeTitle: false,
          title: s?.subject ?? t('side.title'),
          headerRight: () => (
            <HeaderActions>
              {s ? (
                <HeaderButton
                  icon={s.status === 'open' ? 'check' : 'history'}
                  label={s.status === 'open' ? t('side.close') : t('side.reopen')}
                  disabled={status.isPending}
                  onPress={() =>
                    status.mutate(s.status === 'open' ? 'closed' : 'open', {
                      onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)),
                    })
                  }
                />
              ) : null}
            </HeaderActions>
          ),
        }}
      />
      <KeyboardAvoidingView behavior="translate-with-padding" automaticOffset className="flex-1 bg-background">
        <QueryState query={side}>
          {(x) => (
            <>
              <View className="flex-row flex-wrap items-center gap-2 border-b border-border px-4 py-2">
                <StatusPill label={t(`side.status_${x.status}`)} tone={x.status === 'open' ? 'progress' : 'done'} />
                <Text variant="footnote" tone="muted" numberOfLines={1} className="flex-1">
                  {[x.inbox?.name, x.participants.join(', ')].filter(Boolean).join(' · ')}
                </Text>
              </View>
              <FlatList
                onTouchStart={dismissComposerSuggestions}
                inverted
                data={[...(x.messages ?? [])].reverse()}
                ListEmptyComponent={
                  <View style={{ transform: [{ scaleY: -1 }] }}>
                    <EmptyState icon="mail" title={t('timeline.empty')} />
                  </View>
                }
                keyExtractor={(m) => m.uuid}
                keyboardDismissMode="interactive"
                keyboardShouldPersistTaps="handled"
                automaticallyAdjustKeyboardInsets={false}
                maintainVisibleContentPosition={{ minIndexForVisible: 0, autoscrollToTopThreshold: 80 }}
                className="flex-1"
                contentContainerClassName="gap-3 p-3"
                renderItem={({ item: m }) => {
                  const out = m.direction === 'out';
                  return (
                    <View
                      className={cn('max-w-[88%] gap-1', out ? 'self-end' : 'self-start')}
                      style={{ maxWidth: 560 }}>
                      <View
                        className={cn('rounded-2xl px-3.5 py-2.5', out ? 'bg-primary' : 'border border-border bg-card')}
                        style={{ borderCurve: 'continuous' }}>
                        <Text variant="callout" selectable tone={out ? 'onPrimary' : 'default'}>
                          {m.content}
                        </Text>
                      </View>
                      <Text
                        variant="caption"
                        tone={m.status === 'failed' ? 'danger' : 'muted'}
                        className={out ? 'self-end' : 'self-start'}>
                        {[
                          out ? m.sender?.name : m.from_name || m.from_address,
                          formatShort(m.created_at),
                          m.status === 'failed' ? m.error || t('side.failed') : null,
                        ]
                          .filter(Boolean)
                          .join(' · ')}
                      </Text>
                    </View>
                  );
                }}
              />
              {x.status === 'open' ? <SideComposer uuid={x.uuid} conversation={x.conversation_uuid} /> : null}
            </>
          )}
        </QueryState>
      </KeyboardAvoidingView>
    </>
  );
}

function SideComposer({ uuid, conversation }: { uuid: string; conversation: string }) {
  const { t } = useTranslation('conversation');
  const insets = useSafeAreaInsets();
  const keyboardVisible = useKeyboardState((state) => state.isVisible);
  const { canReply } = useInboxScope();
  const reply = useReplySide(uuid);
  const [text, setText] = useState('');
  const [mentions, setMentions] = useState<Mention[]>([]);
  if (!canReply) return null;
  const ok = !!text.trim() && !reply.isPending;
  return (
    <View
      className="border-t border-border px-3 pt-2"
      style={{ paddingBottom: keyboardVisible ? 8 : Math.max(insets.bottom, 8) }}>
      <InlineComposer
        renderContext={conversation}
        value={text}
        onChangeText={setText}
        label={t('side.reply_placeholder')}
        placeholder={t('conversation:composer.message')}
        sendLabel={t('side.send')}
        disabled={!ok}
        onSend={() =>
          reply.mutate(text.trim(), {
            onSuccess: () => setText(''),
            onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)),
          })
        }
        allowMentions={true}
        mentions={mentions}
        onMention={(member) => setMentions((cur) => [...cur, member])}
      />
    </View>
  );
}
