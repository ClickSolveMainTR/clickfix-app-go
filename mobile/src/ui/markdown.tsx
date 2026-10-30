import * as WebBrowser from 'expo-web-browser';
import { useMemo } from 'react';
import { ScrollView, View } from 'react-native';

import { type Inline, parseMarkdown } from './markdown-core';
import { Text } from './text';

function Spans({ inline }: { inline: Inline[] }) {
  return inline.map((s, i) => {
    switch (s.kind) {
      case 'bold':
        return (
          <Text key={i} className="font-sans-bold">
            {s.text}
          </Text>
        );
      case 'italic':
        return (
          <Text key={i} className="italic">
            {s.text}
          </Text>
        );
      case 'code':
        return (
          <Text key={i} className="bg-muted text-[14px]">
            {s.text}
          </Text>
        );
      case 'link':
        return (
          <Text
            key={i}
            tone="signal"
            accessibilityRole="link"
            className="underline"
            onPress={() => /^https?:\/\//i.test(s.href) && void WebBrowser.openBrowserAsync(s.href)}>
            {s.text}
          </Text>
        );
      default:
        return s.text;
    }
  });
}

/** Renders a knowledge article's Markdown with the app's type scale (selectable, Dynamic Type). */
export function Markdown({ source }: { source: string }) {
  const blocks = useMemo(() => parseMarkdown(source), [source]);
  return (
    <View className="gap-3">
      {blocks.map((b, i) => {
        switch (b.kind) {
          case 'heading':
            return (
              <Text key={i} accessibilityRole="header" variant={b.level === 1 ? 'title' : 'headline'} className="pt-1">
                <Spans inline={b.inline} />
              </Text>
            );
          case 'paragraph':
            return (
              <Text key={i} selectable>
                <Spans inline={b.inline} />
              </Text>
            );
          case 'quote':
            return (
              <View key={i} className="border-l-2 border-border pl-3">
                <Text tone="muted" selectable>
                  <Spans inline={b.inline} />
                </Text>
              </View>
            );
          case 'table':
            return (
              <ScrollView
                key={i}
                horizontal
                showsHorizontalScrollIndicator
                contentContainerClassName="rounded-xl border border-border">
                <View>
                  {[b.header, ...b.rows].map((row, rowIndex) => (
                    <View
                      key={rowIndex}
                      className={`flex-row ${rowIndex === 0 ? 'bg-secondary' : 'border-t border-border'}`}>
                      {row.map((cell, column) => (
                        <View key={column} className="w-44 border-r border-border px-3 py-2">
                          <Text
                            selectable
                            variant="footnote"
                            className={rowIndex === 0 ? 'font-sans-semibold' : undefined}>
                            <Spans inline={cell} />
                          </Text>
                        </View>
                      ))}
                    </View>
                  ))}
                </View>
              </ScrollView>
            );
          case 'code':
            return (
              <View key={i} className="rounded-md bg-muted p-3">
                <Text selectable className="text-[14px]">
                  {b.text}
                </Text>
              </View>
            );
          case 'rule':
            return <View key={i} className="h-px bg-border" />;
          case 'list':
            return (
              <View key={i} className="gap-1.5">
                {b.items.map((item, j) => (
                  <View key={j} className="flex-row gap-2">
                    <Text tone="muted" className="tabular-nums">
                      {b.ordered ? `${j + 1}.` : '•'}
                    </Text>
                    <Text selectable className="flex-1">
                      <Spans inline={item} />
                    </Text>
                  </View>
                ))}
              </View>
            );
        }
      })}
    </View>
  );
}
