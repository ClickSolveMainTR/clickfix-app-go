import { router, Stack, useLocalSearchParams } from 'expo-router';
import { RefreshControl, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { EmptyState } from '@/ui/empty-state';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { useManualRefresh } from '@/ui/use-manual-refresh';

import { useCallScope, useCallTranscript, useVoiceCalls, type VoiceCall } from './api';
import { callTime, MissingCall, useCallT } from './calls-screen';
import {
  callFailed,
  counterpart,
  directionLabel,
  duration,
  findCall,
  statusLabel,
  transcriptProblem,
  transcriptStatusLabel,
} from './core';

const PROVIDERS: Record<string, string> = { twilio: 'Twilio', netgsm: 'Netgsm' };

/**
 * One call of a voice channel (CF-169): direction, numbers, status, the
 * times, duration, whether a recording reached the conversation and the
 * transcript when one was kept. The API has no single-call endpoint, so the
 * call is read from the pages of the log loaded so far.
 */
export function CallScreen() {
  const { channel, uuid } = useLocalSearchParams<{ channel: string; uuid: string }>();
  const t = useCallT();
  const { canRead, canTranscript } = useCallScope();
  const calls = useVoiceCalls(channel);
  const call = findCall(calls.data?.pages, uuid);
  const transcript = useCallTranscript(call?.transcript_uuid);
  const refresh = useManualRefresh({
    isRefetching: calls.isRefetching || transcript.isRefetching,
    refetch: () => Promise.all([calls.refetch(), call?.transcript_uuid && canTranscript ? transcript.refetch() : null]),
  });

  if (!canRead)
    return (
      <>
        <Stack.Screen options={{ title: t('calls.call') }} />
        <Screen>
          <EmptyState icon="phone" title={t('calls.no_permission')} />
        </Screen>
      </>
    );

  const openLog = () => router.replace({ pathname: '/more/calls', params: { uuid: channel } });
  if (!channel || !uuid) return <MissingCall title={t('calls.call')} onOpen={channel ? openLog : undefined} />;

  return (
    <>
      <Stack.Screen options={{ title: t('calls.call') }} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <QueryState query={calls}>
          {() =>
            call ? (
              detail(call)
            ) : (
              <EmptyState
                icon="phone"
                title={t('calls.missing')}
                body={t('calls.missing_hint')}
                action={{
                  title: t('calls.open_log'),
                  icon: 'phone',
                  onPress: openLog,
                }}
              />
            )
          }
        </QueryState>
      </Screen>
    </>
  );

  function detail(c: VoiceCall) {
    const text = (transcript.data?.transcript || c.transcript || '').trim();
    const summary = transcript.data && 'summary' in transcript.data.summary ? transcript.data.summary : null;
    const problem = transcript.isError
      ? transcriptProblem(isApiError(transcript.error) ? transcript.error.status : null)
      : null;
    return (
      <View className="gap-4">
        <List>
          <ListRow
            icon={c.direction === 'inbound' ? 'income' : c.direction === 'outbound' ? 'expense' : 'phone'}
            title={counterpart(c)}
            titleLines={2}
            subtitle={directionLabel(t, c.direction)}
            trailing={
              <StatusPill label={statusLabel(t, c.status)} tone={callFailed(c.status) ? 'problem' : 'neutral'} />
            }
            last
          />
        </List>

        <View>
          <SectionHeader title={t('calls.details')} />
          <List>
            <ListRow title={t('calls.from')} subtitle={c.from_number || '—'} />
            <ListRow
              title={t('calls.to')}
              subtitle={[
                c.to_number || '—',
                c.internal_number ? t('calls.extension', { number: c.internal_number }) : '',
              ]
                .filter(Boolean)
                .join(' · ')}
            />
            <ListRow title={t('calls.duration')} subtitle={duration(c.duration_seconds)} />
            <ListRow title={t('calls.started')} subtitle={callTime(c.initiated_at ?? c.created_at)} />
            {c.answered_at ? <ListRow title={t('calls.answered')} subtitle={callTime(c.answered_at)} /> : null}
            {c.ended_at ? <ListRow title={t('calls.ended')} subtitle={callTime(c.ended_at)} /> : null}
            <ListRow title={t('calls.provider')} subtitle={PROVIDERS[c.provider] ?? c.provider} last />
          </List>
        </View>

        <View>
          <SectionHeader title={t('calls.recording')} />
          <List>
            <ListRow
              icon={c.recording_message_uuid ? 'mic' : 'micOff'}
              title={c.recording_message_uuid ? t('calls.recorded') : t('calls.not_recorded')}
              titleLines={2}
              last
            />
          </List>
          {c.recording_message_uuid ? (
            <Text variant="footnote" tone="muted" className="px-4 pt-2">
              {t('calls.recording_hint')}
            </Text>
          ) : null}
        </View>

        <View>
          <SectionHeader title={t('calls.transcript')} />
          <List>
            <ListRow
              title={t('calls.transcript_state')}
              subtitle={transcriptStatusLabel(t, transcript.data?.status ?? c.transcript_status)}
              last={!text && !summary && !c.transcript_uuid}
            />
            {summary?.summary ? (
              <ListRow title={t('calls.summary')} subtitle={<Body text={summary.summary} />} />
            ) : null}
            {text ? (
              <ListRow title={t('calls.transcript_text')} subtitle={<Body text={text} />} last />
            ) : c.transcript_uuid ? (
              <ListRow
                title={
                  !canTranscript
                    ? t('calls.transcript_no_permission')
                    : transcript.isLoading
                      ? '…'
                      : problem
                        ? t(`calls.transcript_${problem}`)
                        : t('calls.transcript_empty')
                }
                titleLines={3}
                onPress={problem === 'failed' ? () => void transcript.refetch() : undefined}
                last
              />
            ) : null}
          </List>
        </View>
      </View>
    );
  }
}

function Body({ text }: { text: string }) {
  return (
    <Text variant="footnote" tone="muted" selectable>
      {text}
    </Text>
  );
}
