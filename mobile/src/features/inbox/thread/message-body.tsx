import * as WebBrowser from 'expo-web-browser';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';

import { cn } from '@/ui/cn';
import { parseInline } from '@/ui/markdown-core';
import { Text } from '@/ui/text';

/** Keep message line breaks, with the shared Markdown reader for inline emphasis and links. */
export function MessageBody({ body, preview = false }: { body: string; preview?: boolean }) {
  const { t } = useTranslation('conversation');
  const [expanded, setExpanded] = useState(false);
  const [overflow, setOverflow] = useState(false);
  const parts = useMemo(() => parseInline(body.replace(/\r\n?/g, '\n').replace(/^(\s*)[-*+] +/gm, '$1• ')), [body]);
  return (
    <View className="min-w-0 shrink gap-2" style={{ maxWidth: '100%' }}>
      <Text
        variant="callout"
        numberOfLines={preview ? 10 : expanded ? undefined : 12}
        onTextLayout={({ nativeEvent }) => {
          // iOS can report only the visible lines for a truncated text node.
          if (nativeEvent.lines.length >= 12) setOverflow(true);
        }}
        className="shrink leading-[22px]"
        style={{ maxWidth: '100%' }}>
        {parts.map((part, i) =>
          part.kind === 'text' ? (
            part.text
          ) : (
            <Text
              key={i}
              variant="callout"
              accessibilityRole={part.kind === 'link' ? 'link' : undefined}
              onPress={
                part.kind === 'link'
                  ? () => {
                      if (/^https?:\/\//i.test(part.href)) void WebBrowser.openBrowserAsync(part.href);
                    }
                  : undefined
              }
              className={cn(
                'leading-[22px]',
                part.kind === 'bold' && 'font-sans-semibold',
                part.kind === 'italic' && 'italic',
                part.kind === 'code' && 'bg-muted',
                part.kind === 'link' && 'text-signal-text underline',
              )}>
              {part.text}
            </Text>
          ),
        )}
      </Text>
      {overflow && !preview ? (
        <Pressable
          accessibilityRole="button"
          accessibilityState={{ expanded }}
          hitSlop={8}
          onPress={() => setExpanded((value) => !value)}
          className="min-h-8 justify-center self-start">
          <Text variant="footnote" className="font-sans-semibold underline">
            {t(expanded ? 'message.show_less' : 'message.show_more')}
          </Text>
        </Pressable>
      ) : null}
    </View>
  );
}
