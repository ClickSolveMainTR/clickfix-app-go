import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { useSession } from '@/core/auth/session';
import { errorMessage } from '@/core/i18n';
import { FormScreen } from '@/ui/screen';
import { AccountRow, SettingsSection, SettingsTextRow } from '@/ui/settings-rows';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';
import { useMutation } from '@tanstack/react-query';
import { Stack, router } from 'expo-router';
import { usePreventRemove } from 'expo-router/react-navigation';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, Alert, Pressable } from 'react-native';
import { profileEditState } from './profile-core';

export function ProfileScreen() {
  const { t } = useTranslation('settings');
  const user = useSession((s) => s.me?.user);
  const [name, setName] = useState(user?.name ?? '');
  const [surname, setSurname] = useState(user?.surname ?? '');
  const [discarded, setDiscarded] = useState(false);
  const state = profileEditState(user, { name, surname });
  const save = useMutation({
    mutationFn: async () => {
      await unwrap(api.PATCH('/v1/auth/profile', { body: state.payload }));
      await useSession.getState().refreshMe();
    },
    onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)),
  });
  usePreventRemove(state.dirty && !discarded && !save.isSuccess, ({ repeat }) => {
    if (save.isPending) return;
    Alert.alert(t('conversation:form.discard'), undefined, [
      { text: t('common:cancel'), style: 'cancel' },
      {
        text: t('conversation:form.discard_action'),
        style: 'destructive',
        onPress: () => {
          setDiscarded(true);
          repeat();
        },
      },
    ]);
  });
  useEffect(() => {
    if (save.isSuccess) router.back();
  }, [save.isSuccess]);

  const disabled = !state.dirty || !state.valid || save.isPending;
  return (
    <>
      <Stack.Screen
        options={{
          title: t('profile'),
          headerRight: () => save.isPending ? <ActivityIndicator /> : (
            <Pressable
              accessibilityRole="button"
              accessibilityState={{ disabled }}
              disabled={disabled}
              onPress={() => save.mutate()}
              className="min-h-11 justify-center px-3"
              style={{ opacity: disabled ? 0.4 : 1 }}>
              <Text tone="primary" className="font-sans-semibold">{t('common:save')}</Text>
            </Pressable>
          ),
        }}
      />
      <FormScreen>
        <AccountRow
          name={[user?.name, user?.surname].filter(Boolean).join(' ') || user?.email || t('profile')}
          email={user?.email}
        />
        <SettingsSection title={t('account')}>
          <SettingsTextRow
            label={t('name')}
            value={name}
            onChangeText={setName}
            autoCapitalize="words"
            autoComplete="given-name"
            textContentType="givenName"
            editable={!save.isPending}
          />
          <SettingsTextRow
            label={t('surname')}
            value={surname}
            onChangeText={setSurname}
            autoCapitalize="words"
            autoComplete="family-name"
            textContentType="familyName"
            editable={!save.isPending}
            last
          />
        </SettingsSection>
      </FormScreen>
    </>
  );
}
