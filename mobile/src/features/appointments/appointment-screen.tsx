import { Stack, useLocalSearchParams, useRouter } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Linking, View } from 'react-native';
import { RescheduleSheet } from './reschedule-sheet';

import { isApiError } from '@/core/api/errors';
import { formatDate, formatTime } from '@/core/format';
import { errorMessage } from '@/core/i18n';
import { Button } from '@/ui/button';
import { Card } from '@/ui/card';
import { List, ListRow } from '@/ui/list';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import {
  type Appointment,
  type AppointmentStatus,
  statusTone,
  useAppointment,
  useAppointmentScope,
  useAppointmentsLive,
  useSetAppointmentStatus,
} from './api';

/** One appointment: when, who, where, links and lifecycle actions (CF-138). */
export function AppointmentScreen() {
  const { t } = useTranslation('appointments');
  const { uuid, reschedule } = useLocalSearchParams<{ uuid: string; reschedule?: string }>();
  const [rescheduling, setRescheduling] = useState(reschedule === '1');
  const { canWrite } = useAppointmentScope();
  const appointment = useAppointment(uuid);
  useAppointmentsLive();
  return (
    <>
      <Stack.Screen options={{ title: t('mobile.detail_title') }} />
      <Screen>
        <QueryState query={appointment}>
          {(a) => (
            <>
              <Body appointment={a} />
              {canWrite && ['scheduled', 'confirmed'].includes(a.status) ? (
                <Button
                  title={t('hub:agenda.reschedule')}
                  variant="outline"
                  icon="calendar"
                  onPress={() => setRescheduling(true)}
                />
              ) : null}
              {canWrite && rescheduling ? (
                <RescheduleSheet appointment={a} onClose={() => setRescheduling(false)} />
              ) : null}
            </>
          )}
        </QueryState>
      </Screen>
    </>
  );
}

function Body({ appointment: a }: { appointment: Appointment }) {
  const { t } = useTranslation('appointments');
  const router = useRouter();
  const { canWrite } = useAppointmentScope();
  const setStatus = useSetAppointmentStatus(a.uuid);
  const active = a.status === 'scheduled' || a.status === 'confirmed';
  // Screen-open time is precise enough to offer complete / no-show.
  const [openedAt] = useState(() => Date.now());
  const started = new Date(a.starts_at).getTime() <= openedAt;

  const change = (status: AppointmentStatus) =>
    setStatus.mutate(
      { status },
      {
        onSuccess: () => toast.success(t('detail.status_changed')),
        onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)),
      },
    );

  return (
    <>
      <Card>
        <View className="flex-row flex-wrap items-center gap-2">
          <StatusPill label={t(`calendar:status.${a.status}`)} tone={statusTone(a.status)} />
          <Text variant="footnote" tone="muted">
            {a.type.name}
          </Text>
        </View>
        <Text variant="headline">{a.title}</Text>
        <Text variant="body">
          {formatDate(a.starts_at)} · {formatTime(a.starts_at)} – {formatTime(a.ends_at)}
        </Text>
      </Card>

      <List>
        <ListRow icon="user" title={a.invitee.name || '—'} subtitle={t('detail.invitee')} />
        {a.invitee.email ? (
          <ListRow
            icon="mail"
            title={a.invitee.email}
            onPress={() => void Linking.openURL(`mailto:${a.invitee.email}`)}
          />
        ) : null}
        {a.invitee.phone ? (
          <ListRow
            icon="phone"
            title={a.invitee.phone}
            onPress={() => void Linking.openURL(`tel:${a.invitee.phone}`)}
          />
        ) : null}
        <ListRow icon="staff" title={a.host?.name || '—'} subtitle={t('detail.host')} />
        <ListRow
          icon="link"
          title={t(`location.${a.location_kind}`)}
          subtitle={a.location_value || undefined}
          onPress={a.location_value.startsWith('https://') ? () => void Linking.openURL(a.location_value) : undefined}
          last={!a.conversation}
        />
        {a.conversation ? (
          <ListRow
            icon="inbox"
            title={`#${a.conversation.number}`}
            subtitle={t('mobile.open_conversation')}
            onPress={() => router.push(`/conversation/${a.conversation!.uuid}`)}
            last
          />
        ) : null}
      </List>

      {a.notes ? (
        <Card title={t('book.notes')}>
          <Text variant="body">{a.notes}</Text>
        </Card>
      ) : null}

      {a.status !== 'cancelled' ? (
        <List>
          <ListRow icon="calendarAdd" title={t('detail.ics')} onPress={() => void Linking.openURL(a.links.ics)} last />
        </List>
      ) : null}

      {canWrite && active ? (
        <View className="gap-2">
          {started ? (
            <>
              <Button
                title={t('detail.complete')}
                icon="complete"
                loading={setStatus.isPending}
                onPress={() => change('completed')}
              />
              <Button
                title={t('detail.no_show')}
                icon="noShow"
                variant="outline"
                disabled={setStatus.isPending}
                onPress={() => change('no_show')}
              />
            </>
          ) : a.status === 'scheduled' ? (
            <Button
              title={t('detail.confirm')}
              icon="confirm"
              loading={setStatus.isPending}
              onPress={() => change('confirmed')}
            />
          ) : null}
          <Button
            title={t('detail.cancel')}
            icon="cancel"
            variant="ghost"
            disabled={setStatus.isPending}
            onPress={() => change('cancelled')}
          />
        </View>
      ) : null}
    </>
  );
}
