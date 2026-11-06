import * as Clipboard from 'expo-clipboard';
import { Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { Platform, RefreshControl, Share, View } from 'react-native';

import { EmptyState } from '@/ui/empty-state';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';
import { useManualRefresh } from '@/ui/use-manual-refresh';

import { useMcpGuide, useMcpScope } from './api';
import { type GuideTool, mcpClientConfigs, splitTools } from './core';

const MONO = Platform.select({ ios: 'Menlo', default: 'monospace' });

/**
 * "Connect with MCP" (CF-173), the phone version of the panel's guide
 * (`frontend/src/features/mcp/components/mcp-guide-page.tsx`): the address,
 * a ready configuration per client to copy or share to the computer where
 * the client runs, how the approval works and which tools this member gets.
 * Nothing secret is shown or stored: MCP clients get their access from the
 * approval in the browser.
 */
export function McpScreen() {
  const { t } = useTranslation('admin');
  const { canConnect } = useMcpScope();
  const guide = useMcpGuide();
  const refresh = useManualRefresh(guide);

  const copy = (value: string) => void Clipboard.setStringAsync(value).then(() => toast.success(t('mcp.copied')));
  const share = (value: string) => void Share.share({ message: value }).catch(() => undefined);

  if (!canConnect)
    return (
      <>
        <Stack.Screen options={{ title: t('mcp.title') }} />
        <Screen>
          <EmptyState icon="link" title={t('mcp.no_permission')} body={t('mcp.no_permission_hint')} />
        </Screen>
      </>
    );

  return (
    <>
      <Stack.Screen options={{ title: t('mcp.title') }} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <Text variant="callout" tone="muted" className="px-4">
          {t('mcp.description')}
        </Text>
        <QueryState query={guide}>
          {(data) => {
            const { available, unavailable } = splitTools(data.tools);
            return (
              <View className="gap-4">
                <View>
                  <SectionHeader title={t('mcp.step1')} className="pt-1" />
                  <List>
                    <ListRow
                      icon="link"
                      title={data.resource_url}
                      titleLines={3}
                      subtitle={t('mcp.tap_to_copy')}
                      onPress={() => copy(data.resource_url)}
                    />
                    <ListRow icon="share" title={t('mcp.share')} onPress={() => share(data.resource_url)} last />
                  </List>
                  <Text variant="footnote" tone="muted" className="px-4 pt-2">
                    {t('mcp.step1_hint')}
                  </Text>
                </View>

                <View>
                  <SectionHeader title={t('mcp.step2')} />
                  <List>
                    {mcpClientConfigs(data.resource_url).map((c, i, all) => (
                      <ListRow
                        key={c.id}
                        icon="copy"
                        title={c.id === 'other' ? t('mcp.client_other') : c.name}
                        subtitle={
                          <View className="gap-1">
                            <Text variant="footnote" tone="muted">
                              {[t(`mcp.client_steps.${c.id}`), c.file].filter(Boolean).join(' · ')}
                            </Text>
                            <Text variant="caption" tone="muted" selectable style={{ fontFamily: MONO }}>
                              {c.value}
                            </Text>
                          </View>
                        }
                        onPress={() => share(c.value)}
                        last={i === all.length - 1}
                      />
                    ))}
                  </List>
                  <Text variant="footnote" tone="muted" className="px-4 pt-2">
                    {t('mcp.step2_hint')}
                  </Text>
                </View>

                <View>
                  <SectionHeader title={t('mcp.step3')} />
                  <List>
                    <ListRow title={t('mcp.approve_sign_in')} titleLines={4} />
                    <ListRow title={t('mcp.approve_organization')} titleLines={4} />
                    <ListRow
                      title={t(data.can_write ? 'mcp.approve_write' : 'mcp.approve_read_only')}
                      titleLines={5}
                      last
                    />
                  </List>
                  <Text variant="footnote" tone="muted" className="px-4 pt-2">
                    {t('mcp.no_secret')}
                  </Text>
                </View>

                <View>
                  <SectionHeader title={t('mcp.tools')} />
                  {available.length ? (
                    <List>
                      {available.map((tool, i) => (
                        <ToolRow key={tool.name} tool={tool} last={i === available.length - 1} />
                      ))}
                    </List>
                  ) : (
                    <List>
                      <ListRow title={t('mcp.tools_none')} titleLines={3} last />
                    </List>
                  )}
                  <Text variant="footnote" tone="muted" className="px-4 pt-2">
                    {t(data.can_write ? 'mcp.tools_write_note' : 'mcp.tools_read_only_note')}
                  </Text>
                </View>

                {unavailable.length ? (
                  <View>
                    <SectionHeader title={t('mcp.tools_unavailable')} />
                    <List>
                      {unavailable.map((tool, i) => (
                        <ToolRow key={tool.name} tool={tool} last={i === unavailable.length - 1} />
                      ))}
                    </List>
                  </View>
                ) : null}

                <Text variant="footnote" tone="muted" className="px-4">
                  {t('mcp.apps_on_panel')}
                </Text>
              </View>
            );
          }}
        </QueryState>
      </Screen>
    </>
  );
}

function ToolRow({ tool, last }: { tool: GuideTool; last: boolean }) {
  const { t } = useTranslation('admin');
  return (
    <ListRow
      title={tool.name}
      // The API's own description of the tool (English, as MCP clients see it).
      subtitle={tool.description}
      trailing={
        <StatusPill
          label={t(tool.write ? 'mcp.tool_write' : 'mcp.tool_read')}
          tone={tool.write ? 'progress' : 'neutral'}
          icon={tool.write ? 'edit' : 'show'}
        />
      }
      last={last}
    />
  );
}
