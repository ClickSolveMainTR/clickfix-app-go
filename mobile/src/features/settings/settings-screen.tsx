import { HelpSettingsCard } from '@/features/guides/help-settings-card';
import { router, Stack } from 'expo-router';
import * as Application from 'expo-application';
import { useTranslation } from 'react-i18next';
import { Alert, Pressable, View } from 'react-native';
import { useSession } from '@/core/auth/session';
import { APP_VERSION } from '@/core/config';
import { LANGUAGES, setLanguage } from '@/core/i18n';
import { openLegalPage } from '@/core/legal';
import { usePreferences } from '@/core/preferences';
import { SecurityCard } from '@/features/security/security-card';
import { WidgetsSettingsCard } from '@/features/widgets/widgets-settings-card';
import { AccountRow } from '@/ui/settings-rows';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { ActionMenu } from '@/ui/menu';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

export function SettingsScreen() {
  const { t, i18n } = useTranslation('settings');
  const user = useSession((s) => s.me?.user);
  const theme = usePreferences((s) => s.theme);
  const themes = [
    { value: 'system' as const, label: t('theme_system') },
    { value: 'light' as const, label: t('theme_light') },
    { value: 'dark' as const, label: t('theme_dark') },
  ];
  const signOut = () =>
    Alert.alert(t('sign_out_title'), t('sign_out_body'), [
      { text: t('common:cancel'), style: 'cancel' },
      { text: t('auth:sign_out'), style: 'destructive', onPress: () => void useSession.getState().signOut() },
    ]);
  return (
    <>
      <Stack.Screen options={{ title: t('title') }} />
      <Screen>
        <AccountRow
          name={[user?.name, user?.surname].filter(Boolean).join(' ') || user?.email || t('profile')}
          email={user?.email}
          onPress={() => router.push('/more/profile')}
        />
        <List>
          <ActionMenu
            title={t('appearance')}
            fullWidth
            value={theme}
            options={themes}
            onPick={(value) => usePreferences.getState().setTheme(value)}
            trigger={
              <Pressable accessibilityRole="button">
                <ListRow
                  iconTile
                  icon="theme"
                  title={t('appearance')}
                  trailing={<Text tone="muted">{themes.find((item) => item.value === theme)?.label}</Text>}
                />
              </Pressable>
            }
          />
          <ActionMenu
            title={t('language')}
            fullWidth
            value={i18n.language}
            options={LANGUAGES.map((l) => ({ value: l.code, label: l.name }))}
            onPick={(value) => void setLanguage(value as 'tr' | 'en')}
            trigger={
              <Pressable accessibilityRole="button">
                <ListRow
                  iconTile
                  icon="language"
                  title={t('language')}
                  trailing={<Text tone="muted">{LANGUAGES.find((l) => l.code === i18n.language)?.name}</Text>}
                />
              </Pressable>
            }
          />
          <ListRow
            iconTile
            icon="bell"
            title={t('notification_prefs')}
            onPress={() => router.push('/more/notifications')}
            last
          />
        </List>
        <WidgetsSettingsCard />
        <SecurityCard />
        <HelpSettingsCard />
        <View>
          <SectionHeader title={t('legal')} />
          <List>
            <ListRow iconTile icon="security" title={t('privacy')} onPress={() => void openLegalPage('privacy')} />
            <ListRow iconTile icon="receipt" title={t('terms')} onPress={() => void openLegalPage('terms')} />
            <ListRow iconTile icon="lockClosed" title={t('kvkk')} onPress={() => void openLegalPage('kvkk')} last />
          </List>
        </View>
        <List>
          <Pressable
            accessibilityRole="button"
            onPress={signOut}
            className="min-h-12 items-center justify-center px-4 py-3 active:bg-muted">
            <Text tone="danger">{t('auth:sign_out')}</Text>
          </Pressable>
        </List>
        <List>
          <Pressable
            accessibilityRole="button"
            onPress={() => router.push('/delete-account')}
            className="min-h-12 items-center justify-center px-4 py-3 active:bg-muted">
            <Text tone="danger">{t('delete_account')}</Text>
          </Pressable>
        </List>
        <Text variant="footnote" tone="muted" className="text-center">
          {t('version', { version: APP_VERSION, build: Application.nativeBuildVersion ?? '—' })}
        </Text>
      </Screen>
    </>
  );
}
