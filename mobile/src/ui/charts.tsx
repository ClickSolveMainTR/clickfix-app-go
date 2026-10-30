import { barFractions } from './chart-core';
import { useState } from 'react';
import { type LayoutChangeEvent, Pressable, View } from 'react-native';
import Svg, { Path } from 'react-native-svg';
import { useCSSVariable } from 'uniwind';

import { Text } from './text';

function useColor(token: string, fallback: string) {
  const v = useCSSVariable(`--color-${token}`);
  return typeof v === 'string' ? v : fallback;
}

/**
 * One-series volume chart: proportional bars, rounded tops, separate touch targets
 * and bucket labels. RN views avoid SVG hit rectangles covering the data.
 * A tap shows a bar's value (the phone's hover); screen readers get the
 * series as text through `summary`.
 */
export function BarChart({
  data,
  height = 140,
  format = (v) => String(v),
  summary,
}: {
  data: { label: string; value: number | null }[];
  height?: number;
  format?: (v: number) => string;
  /** Spoken description, e.g. "Volume per day: Mon 12, Tue 9 …". */
  summary: string;
}) {
  const [active, setActive] = useState<number | null>(null);
  const bar = useColor('signal', '#03FF13');
  const dim = useColor('muted-foreground', '#737373');
  const fractions = barFractions(data.map((d) => d.value));
  const gap = data.length > 12 ? 3 : 10;
  const shown = active != null ? data[active] : null;

  return (
    <View className="gap-1.5">
      <View accessible accessibilityRole="image" accessibilityLabel={summary} />
      <View className="h-5 flex-row items-center justify-between">
        <Text variant="footnote" tone="muted">
          {shown ? shown.label : ''}
        </Text>
        <Text variant="footnote" className="font-sans-semibold tabular-nums">
          {shown && shown.value != null ? format(shown.value) : ''}
        </Text>
      </View>
      <View style={{ height, flexDirection: 'row', alignItems: 'flex-end', gap }}>
        {data.map((d, i) => (
          <Pressable
            key={i}
            accessibilityRole="button"
            accessibilityLabel={`${d.label}: ${format(d.value ?? 0)}`}
            onPress={() => setActive(i)}
            style={{ flex: 1, height: '100%', justifyContent: 'flex-end', gap: 5 }}>
            <View
              style={{
                height: fractions[i] * (height - 24),
                backgroundColor: active == null || active === i ? bar : dim,
                borderTopLeftRadius: 5,
                borderTopRightRadius: 5,
                minHeight: d.value ? 2 : 0,
              }}
            />
            <Text variant="caption" tone="muted" numberOfLines={1} className="text-center">
              {data.length <= 10 || i % Math.ceil(data.length / 5) === 0 || i === data.length - 1
                ? d.label
                : ' '}
            </Text>
          </Pressable>
        ))}
      </View>
    </View>
  );
}

/** A small trend line (health score history); 2 px stroke, no axes. */
export function Sparkline({
  values,
  height = 36,
  summary,
}: {
  values: number[];
  height?: number;
  summary: string;
}) {
  const [width, setWidth] = useState(0);
  const stroke = useColor('primary', '#0a0a0a');
  if (values.length < 2) return null;
  const min = Math.min(...values);
  const max = Math.max(...values);
  const span = max - min || 1;
  const step = width / (values.length - 1);
  const d = values
    .map(
      (v, i) =>
        `${i ? 'L' : 'M'}${(i * step).toFixed(1)},${(height - 2 - ((v - min) / span) * (height - 4)).toFixed(1)}`,
    )
    .join(' ');
  return (
    <View
      accessible
      accessibilityRole="image"
      accessibilityLabel={summary}
      style={{ height }}
      onLayout={(e: LayoutChangeEvent) => setWidth(e.nativeEvent.layout.width)}>
      {width > 0 ? (
        <Svg width={width} height={height}>
          <Path
            d={d}
            stroke={stroke}
            strokeWidth={2}
            fill="none"
            strokeLinejoin="round"
            strokeLinecap="round"
          />
        </Svg>
      ) : null}
    </View>
  );
}
