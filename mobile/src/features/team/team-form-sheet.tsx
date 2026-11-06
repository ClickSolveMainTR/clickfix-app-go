import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { type FormField, type FormSection, FormSheet } from '@/ui/form-sheet';
import { toast } from '@/ui/toast';

import { type Team, useSkillCatalog, useTeamMutations } from './api';
import {
  addSkills,
  type DayRow,
  type FormErrorKey,
  formErrors,
  formOf,
  sameForm,
  serverErrors,
  type TeamForm,
  timeZones,
  toRequest,
} from './teams-core';

/**
 * Team details form (CF-166), create and edit in one sheet with the fields
 * of the panel's form (`frontend/src/features/teams/components/team-form.tsx`):
 * name, description, color, skills and working hours (own, or the
 * organization's). Editing also offers "delete team", confirmed first.
 */
export function TeamFormSheet({
  visible,
  team,
  onClose,
  onSaved,
  onDeleted,
}: {
  visible: boolean;
  /** `null` creates a team. */
  team: Team | null;
  onClose: () => void;
  onSaved?: (team: Team) => void;
  onDeleted?: () => void;
}) {
  const { t } = useTranslation('team');
  const m = useTeamMutations();
  const catalog = useSkillCatalog(visible);
  const [form, setForm] = useState<TeamForm>(() => formOf(team));
  const [draft, setDraft] = useState('');
  const [skillError, setSkillError] = useState<'skill' | 'too_many_skills' | null>(null);
  // API field errors by code (`serverErrors`), shown translated until the next edit.
  const [server, setServer] = useState<Record<string, FormErrorKey>>({});

  // The form is filled each time the sheet opens (state adjusted while rendering, no effect).
  const opened = visible ? `${team?.uuid ?? 'new'}:${team?.updated_at ?? ''}` : '';
  const [seen, setSeen] = useState('');
  if (opened !== seen) {
    setSeen(opened);
    if (opened) {
      setForm(formOf(team));
      setDraft('');
      setSkillError(null);
      setServer({});
    }
  }

  const set = (patch: Partial<TeamForm>) => {
    setForm((f) => ({ ...f, ...patch }));
    setServer({});
  };
  const setDay = (index: number, patch: Partial<DayRow>) =>
    set({ days: form.days.map((d, i) => (i === index ? { ...d, ...patch } : d)) });

  const errors = formErrors(form);
  const error = (key: string) => (errors[key] ? t(`manage.validation.${errors[key]}`) : undefined);
  const busy = m.create.isPending || m.update.isPending || m.remove.isPending;
  const serverError = (key: string) => (server[key] ? t(`manage.validation.${server[key]}`) : undefined);
  const fail = (e: unknown) => {
    // A taken name comes as a field error (400 `name` / `taken`, or 409 in a race); the code picks the text.
    const fields = isApiError(e) ? serverErrors(e.details) : {};
    setServer(fields);
    toast.error(
      fields.name === 'name_taken'
        ? t('manage.validation.name_taken')
        : errorMessage(isApiError(e) ? e.code : undefined),
    );
  };

  const addSkill = () => {
    const next = addSkills(form.skills, draft);
    set({ skills: next.skills });
    setSkillError(next.error ?? null);
    if (!next.error) setDraft('');
  };

  const save = () => {
    const done = {
      onSuccess: (saved: Team) => {
        toast.success(t(team ? 'manage.updated' : 'manage.created'));
        onClose();
        onSaved?.(saved);
      },
      onError: fail,
    };
    if (team) m.update.mutate({ uuid: team.uuid, body: toRequest(form) }, done);
    else m.create.mutate(toRequest(form), done);
  };

  const remove = (target: Team) =>
    Alert.alert(t('manage.delete_title', { name: target.name }), t('manage.delete_body'), [
      { text: t('common:cancel'), style: 'cancel' },
      {
        text: t('manage.delete'),
        style: 'destructive',
        onPress: () =>
          m.remove.mutate(target.uuid, {
            onSuccess: () => {
              toast.success(t('manage.deleted'));
              onClose();
              onDeleted?.();
            },
            onError: fail,
          }),
      },
    ]);

  // Tags of the team first, then the ones other teams and members use.
  const skillOptions = [...new Set([...form.skills, ...(catalog.data?.items ?? [])])];

  const days: FormField[] = form.days.flatMap((d, i): FormField[] => [
    {
      key: `open-${d.day}`,
      kind: 'toggle',
      label: t(`manage.day_${d.day}`),
      value: d.open,
      onChange: (open) => setDay(i, { open }),
    },
    ...(d.open
      ? ([
          {
            key: `start-${d.day}`,
            kind: 'text',
            label: t('manage.day_start', { day: t(`manage.day_${d.day}`) }),
            placeholder: '09:00',
            value: d.start,
            onChange: (start) => setDay(i, { start: start.trim() }),
            error: error(`days.${i}.start`) ?? serverError(`days.${i}.start`),
          },
          {
            key: `end-${d.day}`,
            kind: 'text',
            label: t('manage.day_end', { day: t(`manage.day_${d.day}`) }),
            placeholder: '18:00',
            value: d.end,
            onChange: (end) => setDay(i, { end: end.trim() }),
            error: error(`days.${i}.end`) ?? serverError(`days.${i}.end`),
          },
        ] satisfies FormField[])
      : []),
  ]);

  const sections: FormSection[] = [
    {
      title: t('manage.section_general'),
      fields: [
        {
          key: 'name',
          kind: 'text',
          label: t('manage.field_name'),
          value: form.name,
          onChange: (name) => set({ name }),
          // An untouched new form does not open with an error under the name.
          error: (form.name ? error('name') : undefined) ?? serverError('name'),
        },
        {
          key: 'description',
          kind: 'text',
          label: t('manage.field_description'),
          value: form.description,
          onChange: (description) => set({ description }),
          multiline: true,
          error: error('description') ?? serverError('description'),
        },
        {
          key: 'color',
          kind: 'text',
          label: t('manage.field_color'),
          placeholder: '#2563EB',
          value: form.color,
          onChange: (color) => set({ color: color.trim().slice(0, 7) }),
          error: error('color') ?? serverError('color'),
        },
      ],
      footer: t('manage.color_hint'),
    },
    {
      title: t('manage.field_skills'),
      fields: [
        ...(skillOptions.length
          ? [
              {
                key: 'skills',
                kind: 'multi' as const,
                label: t('manage.field_skills'),
                values: form.skills,
                options: skillOptions.map((s) => ({ value: s, label: s })),
                onChange: (skills: string[]) => {
                  set({ skills: [...skills].sort((a, b) => a.localeCompare(b)) });
                  setSkillError(null);
                },
                error: error('skills') ?? serverError('skills'),
              },
            ]
          : []),
        {
          key: 'skill-draft',
          kind: 'text' as const,
          label: t('manage.skill_new'),
          value: draft,
          onChange: (value: string) => {
            setDraft(value);
            setSkillError(null);
          },
          error: skillError ? t(`manage.validation.${skillError}`) : undefined,
        },
        {
          key: 'skill-add',
          kind: 'button' as const,
          label: t('manage.skill_add'),
          disabled: !draft.trim(),
          onPress: addSkill,
        },
      ],
      footer: t('manage.skills_hint'),
    },
    {
      title: t('manage.section_hours'),
      fields: [
        {
          key: 'inherit',
          kind: 'toggle',
          label: t('manage.inherit_hours'),
          value: form.inherit_hours,
          onChange: (inherit_hours) => set({ inherit_hours }),
        },
        ...(form.inherit_hours
          ? []
          : [
              {
                key: 'timezone',
                kind: 'pick' as const,
                label: t('manage.field_timezone'),
                value: form.timezone,
                options: [
                  { value: '', label: t('manage.timezone_org') },
                  ...timeZones(form.timezone).map((zone) => ({ value: zone, label: zone })),
                ],
                onChange: (timezone: string) => set({ timezone }),
                error: serverError('timezone'),
              },
            ]),
      ],
      footer: form.inherit_hours ? t('manage.inherit_hours_hint') : t('manage.hours_hint'),
    },
    ...(form.inherit_hours ? [] : [{ title: t('manage.section_days'), fields: days, footer: t('manage.days_hint') }]),
    ...(team
      ? [
          {
            fields: [
              {
                key: 'delete',
                kind: 'button' as const,
                label: t('manage.delete'),
                disabled: busy,
                onPress: () => remove(team),
              },
            ],
            footer: t('manage.delete_hint'),
          },
        ]
      : []),
  ];

  const dirty = !sameForm(form, formOf(team)) || !!draft.trim();
  return (
    <FormSheet
      visible={visible}
      title={team ? team.name : t('manage.new')}
      onCancel={onClose}
      dirty={dirty}
      detents={['large']}
      primary={{
        label: team ? t('common:save') : t('manage.create'),
        // Like the panel: nothing to save until something changed.
        disabled: busy || Object.keys(errors).length > 0 || (!!team && sameForm(form, formOf(team))),
        loading: m.create.isPending || m.update.isPending,
        onPress: save,
      }}
      sections={sections}
    />
  );
}
