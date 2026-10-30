import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useWindowDimensions, View } from 'react-native';

import { useOrgSlug } from '@/core/api/org';
import { EmptyState } from '@/ui/empty-state';

import { ConversationPane } from './conversation-screen';
import { InboxScreen } from './inbox-screen';

/** Width from which the inbox shows list and conversation side by side (iPad, large Android tablets). */
export const SPLIT_MIN_WIDTH = 768;

/**
 * Inbox tab: a list on phones; on a tablet the list pane plus the open
 * conversation beside it (Mail / Front / Help Scout on iPad), so an agent
 * works through the queue without going back and forth.
 */
export function InboxTab() {
  const org = useOrgSlug();
  // A new organization starts with nothing open (the open conversation belonged to the previous one).
  return <Split key={org} />;
}

function Split() {
  const { t } = useTranslation('conversation');
  const { width } = useWindowDimensions();
  const [open, setOpen] = useState<string | null>(null);
  if (width < SPLIT_MIN_WIDTH) return <InboxScreen />;
  const listWidth = Math.min(420, Math.round(width * 0.4));
  return (
    <View className="flex-1 flex-row bg-background">
      <View style={{ width: listWidth }} className="border-r border-border">
        <InboxScreen onOpen={setOpen} activeUuid={open ?? undefined} />
      </View>
      <View className="flex-1">
        {open ? (
          <ConversationPane key={open} uuid={open} embedded />
        ) : (
          <View className="flex-1 justify-center">
            <EmptyState icon="inbox" title={t('split.empty')} body={t('split.empty_hint')} />
          </View>
        )}
      </View>
    </View>
  );
}
