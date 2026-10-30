import { Keyboard } from 'react-native';
import { Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';

export default function PublicLayout() {
  const { t } = useTranslation();
  return (
    <Stack
      screenListeners={{ blur: Keyboard.dismiss, transitionStart: Keyboard.dismiss }}
      screenOptions={{ headerShown: false }}>
      <Stack.Screen name="index" />
      <Stack.Screen
        name="code"
        options={{
          headerShown: true,
          title: t('auth:email_code_title'),
          headerBackTitle: t('common:back'),
          headerShadowVisible: false,
        }}
      />
    </Stack>
  );
}
