import { roleLabel } from '@/core/roles';
import { router } from 'expo-router';
import * as WebBrowser from 'expo-web-browser';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { useSession } from '@/core/auth/session';
import { WEB_URL } from '@/core/config';
import { Button } from '@/ui/button';
import { EmptyState } from '@/ui/empty-state';
import { Icon } from '@/ui/icon';
import { List, ListRow } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';

/**
 * Picks the organization every `/v1/tenant/*` call is made for (ported from
 * takwim-app-go `organization-picker.tsx`). Shown on its own after sign-in
 * when the account has several memberships (or none), and as a sheet from
 * Settings to switch. A new organization is created on the web panel.
 */
export function OrganizationPicker() {
  const { t } = useTranslation('auth');
  const me = useSession((s) => s.me);
  const current = useSession((s) => s.organization);
  const orgs = me?.organizations ?? [];

  const pick = (slug: string) => {
    useSession.getState().selectOrganization(slug);
    if (router.canGoBack()) router.back();
  };

  return (
    <Screen className="pt-6">
      {orgs.length === 0 ? (
        <EmptyState
          icon="building"
          title={t('no_org_title')}
          body={t('no_org_body')}
          action={{
            title: t('open_panel'),
            icon: 'externalLink',
            // The full smart set-up (CF-140) runs on the web: open an
            // organization there and the panel continues with /getting-started.
            onPress: () => void WebBrowser.openBrowserAsync(`${WEB_URL}/setup?yeni=1`),
          }}
        />
      ) : (
        <>
          <View className="gap-1 px-1">
            <Text variant="title">{t('pick_org_title')}</Text>
            <Text variant="callout" tone="muted">
              {t('pick_org_body')}
            </Text>
          </View>
          <List>
            {orgs.map((o, i) => {
              const suspended = o.status !== 'active';
              return (
                <ListRow
                  key={o.uuid}
                  icon="building"
                  title={o.name}
                  subtitle={suspended ? `${roleLabel(o.role)} · ${t('org_suspended')}` : roleLabel(o.role)}
                  trailing={
                    current?.uuid === o.uuid ? <Icon name="check" tone="primary" size={18} /> : undefined
                  }
                  onPress={suspended ? undefined : () => pick(o.slug)}
                  last={i === orgs.length - 1}
                />
              );
            })}
          </List>
        </>
      )}
      {current ? null : (
        <Button
          icon="signOut"
          title={t('sign_out')}
          variant="ghost"
          onPress={() => void useSession.getState().signOut()}
        />
      )}
    </Screen>
  );
}
