import { isApiError } from '@/core/api/errors';
import { P, useCan } from '@/core/auth/permissions';
import { formatTime } from '@/core/format';
import { errorMessage } from '@/core/i18n';
import { useDebounced } from '@/core/use-debounced';
import { useContactList, type ContactRecord } from '@/features/contacts/api';
import { FormSheet } from '@/ui/form-sheet';
import { OptionSheet } from '@/ui/select';
import { toast } from '@/ui/toast';
import { router } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useAppointmentSlots, useAppointmentTypes, useBookAppointment } from './api';

export function NewAppointmentSheet({ visible, onClose }: { visible: boolean; onClose: () => void }) {
  // Unmount between openings so another customer's draft is never reused.
  return visible ? <BookingForm onClose={onClose} /> : null;
}

function BookingForm({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation('hub');
  const can = useCan();
  const canSearchCustomers = can(P.contactsRead);
  const [guestName, setGuestName] = useState('');
  const [guestEmail, setGuestEmail] = useState('');
  const types = useAppointmentTypes();
  const book = useBookAppointment();
  const [typeUuid, setTypeUuid] = useState<string | null>(null);
  const [video, setVideo] = useState(false);
  const [duration, setDuration] = useState(0);
  const [starts, setStarts] = useState(() => new Date(Math.ceil((Date.now() + 3600000) / 900000) * 900000));
  const [customer, setCustomer] = useState<ContactRecord | null>(null);
  const [pickingCustomer, setPickingCustomer] = useState(false);
  const [q, setQ] = useState('');
  const contacts = useContactList(useDebounced(q, 250));
  const [notes, setNotes] = useState('');
  const active = (types.data?.items ?? []).filter((item) => item.is_active);
  const available = active.filter((item) => (item.location_kind === 'video') === video);
  const type = available.find((item) => item.uuid === typeUuid);
  const slots = useAppointmentSlots(typeUuid, starts, duration || type?.default_duration || 30);
  const slotValid = slots.data?.items.some((slot) => Date.parse(slot.starts_at) === starts.getTime());
  const [openedAt] = useState(() => new Date());
  const valid =
    !!type &&
    !!slotValid &&
    (canSearchCustomers ? !!customer : !!guestName.trim()) &&
    starts.getTime() > openedAt.getTime();
  const submit = () => {
    if (!type || !valid) return;
    book.mutate(
      {
        type_uuid: type.uuid,
        starts_at: starts.toISOString(),
        duration: duration || type.default_duration,
        timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
        contact_uuid: customer?.uuid ?? null,
        invitee_name: customer?.name ?? guestName.trim(),
        invitee_email: customer?.email ?? guestEmail.trim(),
        invitee_phone: customer?.phone ?? '',
        notes: notes.trim(),
        notify: true,
      },
      {
        onSuccess: (appointment) => {
          onClose();
          toast.success(t('agenda.created'));
          if (appointment.video_warning) toast.error(appointment.video_warning);
          router.push(`/appointment/${appointment.uuid}`);
        },
        onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)),
      },
    );
  };
  return (
    <>
      <FormSheet
        visible
        title={t('agenda.create')}
        onCancel={onClose}
        dirty={!!customer || !!guestName || !!notes || !!typeUuid}
        detents={['large']}
        primary={{ label: t('agenda.create'), disabled: !valid, loading: book.isPending, onPress: submit }}
        sections={[
          {
            fields: canSearchCustomers
              ? [
                  {
                    key: 'customer',
                    kind: 'button',
                    label: customer?.name ?? t('agenda.customer'),
                    onPress: () => setPickingCustomer(true),
                  },
                ]
              : [
                  { key: 'guest', kind: 'text', label: t('agenda.customer'), value: guestName, onChange: setGuestName },
                  {
                    key: 'email',
                    kind: 'text',
                    label: t('admin:members.email'),
                    value: guestEmail,
                    onChange: setGuestEmail,
                    keyboard: 'email',
                  },
                ],
          },
          {
            fields: [
              {
                key: 'video',
                kind: 'toggle',
                label: t('agenda.video'),
                hint: t('agenda.location_hint'),
                value: video,
                onChange: (v) => {
                  setVideo(v);
                  setTypeUuid(null);
                  setDuration(0);
                },
                disabled: !active.some((item) => (item.location_kind === 'video') !== video),
              },
              {
                key: 'type',
                kind: 'pick',
                label: t('agenda.type'),
                value: typeUuid,
                options: available.map((item) => ({ value: item.uuid, label: item.name })),
                onChange: (v) => {
                  setTypeUuid(v);
                  setDuration(0);
                },
              },
              {
                key: 'starts',
                kind: 'date',
                label: t('agenda.date'),
                value: starts,
                min: openedAt,
                onChange: setStarts,
              },
              {
                key: 'slot',
                kind: 'pick',
                label: t('agenda.slots'),
                value: slotValid ? starts.toISOString() : null,
                options: (slots.data?.items ?? []).map((slot) => ({
                  value: new Date(slot.starts_at).toISOString(),
                  label: formatTime(slot.starts_at),
                })),
                onChange: (v) => setStarts(new Date(v)),
                hint: slots.isFetching
                  ? t('common:loading')
                  : typeUuid && !slots.data?.items.length
                    ? t('agenda.no_slots')
                    : undefined,
              },
              ...(type
                ? [
                    {
                      key: 'duration',
                      kind: 'pick' as const,
                      label: t('agenda.duration'),
                      value: String(duration || type.default_duration),
                      options: type.duration_options.map((minutes) => ({
                        value: String(minutes),
                        label: String(minutes),
                      })),
                      onChange: (v: string) => setDuration(Number(v)),
                    },
                    {
                      key: 'location',
                      kind: 'info' as const,
                      label: t('agenda.location'),
                      value: type.location_value || t(`agenda.location_${type.location_kind}`),
                    },
                  ]
                : []),
              {
                key: 'notes',
                kind: 'text',
                label: t('conversation:issue.description'),
                value: notes,
                onChange: setNotes,
                multiline: true,
              },
            ],
            footer: types.isFetching ? t('common:loading') : !available.length ? t('agenda.no_types') : undefined,
          },
        ]}
      />
      <OptionSheet
        visible={pickingCustomer}
        title={t('agenda.customer')}
        onClose={() => setPickingCustomer(false)}
        onSearch={setQ}
        loading={contacts.isFetching}
        options={(contacts.data?.pages.flatMap((p) => p.items) ?? []).map((c) => ({
          value: c.uuid,
          label: c.name,
          description: c.email ?? c.phone ?? undefined,
        }))}
        onPick={(id) => {
          const c = contacts.data?.pages.flatMap((p) => p.items).find((c) => c.uuid === id);
          if (c) setCustomer(c);
          setPickingCustomer(false);
        }}
      />
    </>
  );
}
