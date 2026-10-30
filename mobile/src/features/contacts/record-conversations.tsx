import { router } from 'expo-router';
import { useTranslation } from 'react-i18next';

import { formatRelative } from '@/core/format';
import { List, ListRow } from '@/ui/list';
import { EmptyState } from '@/ui/empty-state';

type Row = {
  uuid: string;
  number: number;
  subject: string;
  status: string;
  channel_type: string;
  last_message_at: string;
  last_message_preview: string;
};

/** A contact's or company's conversations; a row opens the conversation. */
export function ConversationHistory({ items, emptyText }: { items: Row[]; emptyText: string }) {
  const { t } = useTranslation('inbox');
  if (!items.length)
    return emptyText ? <EmptyState icon="inbox" title={emptyText} /> : null;
  return (
    <List>
      {items.map((c, i) => (
        <ListRow
          key={c.uuid}
          icon="inbox"
          title={c.subject || c.last_message_preview || `#${c.number}`}
          subtitle={[
            `#${c.number}`,
            t(`status.${c.status}`, { defaultValue: c.status }),
            t(`channels:types.${c.channel_type}`, { defaultValue: c.channel_type }),
            formatRelative(c.last_message_at),
          ].join(' · ')}
          onPress={() => router.push(`/conversation/${c.uuid}`)}
          last={i === items.length - 1}
        />
      ))}
    </List>
  );
}
