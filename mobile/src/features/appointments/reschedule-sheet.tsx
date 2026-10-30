import { isApiError } from '@/core/api/errors';
import { formatTime } from '@/core/format';
import { errorMessage } from '@/core/i18n';
import { FormSheet } from '@/ui/form-sheet';
import { toast } from '@/ui/toast';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useAppointmentSlots, useRescheduleAppointment, type Appointment } from './api';

export function RescheduleSheet({ appointment, onClose }: { appointment: Appointment; onClose: () => void }) {
  const { t } = useTranslation('hub');
  const [starts, setStarts] = useState(() => new Date(appointment.starts_at));
  const duration = Math.round((Date.parse(appointment.ends_at) - Date.parse(appointment.starts_at)) / 60000);
  const slots = useAppointmentSlots(appointment.type.uuid, starts, duration);
  const mutation = useRescheduleAppointment(appointment.uuid);
  const valid = slots.data?.items.some((slot) => Date.parse(slot.starts_at) === starts.getTime());
  return (
    <FormSheet
      visible
      title={t('agenda.reschedule')}
      onCancel={onClose}
      dirty={starts.getTime() !== Date.parse(appointment.starts_at)}
      primary={{
        label: t('common:save'),
        disabled: !valid,
        loading: mutation.isPending,
        onPress: () =>
          mutation.mutate(
            { starts_at: starts.toISOString(), duration },
            {
              onSuccess: () => {
                toast.success(t('agenda.rescheduled'));
                onClose();
              },
              onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)),
            },
          ),
      }}
      sections={[
        {
          fields: [
            { key: 'date', kind: 'date', label: t('agenda.date'), value: starts, onChange: setStarts },
            {
              key: 'slot',
              kind: 'pick',
              label: t('agenda.slots'),
              value: valid ? starts.toISOString() : null,
              options: (slots.data?.items ?? []).map((slot) => ({
                value: new Date(slot.starts_at).toISOString(),
                label: formatTime(slot.starts_at),
              })),
              onChange: (value) => setStarts(new Date(value)),
            },
          ],
          footer: slots.isFetching
            ? t('common:loading')
            : slots.isError
              ? errorMessage(isApiError(slots.error) ? slots.error.code : undefined)
              : !slots.data?.items.length
                ? t('agenda.no_slots')
                : undefined,
        },
      ]}
    />
  );
}
