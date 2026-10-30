import { Text as RNText, View } from 'react-native';
import Svg, { Circle, Polygon, Rect } from 'react-native-svg';

/**
 * ClickFix logo kit geometry (docs/brand.md, `frontend/src/assets/brand/`),
 * the same as `frontend/src/components/brand/clickfix-paths.ts`. The polygon
 * is translated between artboards, never redrawn or recoloured.
 */
const VIEWBOX = '0 0 1832 1832';
const CURSOR = '411 1319 347 575 1627 743 635 751 411 1319';
const TILE_VIEWBOX = '0 0 2050 2050';
const TILE_CURSOR = '482.33 1391 418.33 647 1698.33 815 706.33 823 482.33 1391';
const INK = '#000000';
const SIGNAL = '#03FF13';

/**
 * The in-product mark (`clickfix-circle-ink.svg`): black disc, green cursor. On dark screens it gets a hairline white
 * ring so the disc does not disappear (`ring`).
 */
export function BrandMark({ size = 64, ring = false }: { size?: number; ring?: boolean }) {
  return (
    <Svg width={size} height={size} viewBox={VIEWBOX} accessibilityLabel="ClickFix">
      <Circle
        cx={916}
        cy={916}
        r={ring ? 906 : 916}
        fill={INK}
        stroke={ring ? '#FFFFFF' : undefined}
        strokeOpacity={ring ? 0.18 : undefined}
        strokeWidth={ring ? 20 : undefined}
      />
      <Polygon points={CURSOR} fill={SIGNAL} />
    </Svg>
  );
}

/** App icon (`clickfix-tile-green.svg`): black cursor on the green tile, iOS corner radius. */
export function BrandTile({ size = 64 }: { size?: number }) {
  return (
    <Svg width={size} height={size} viewBox={TILE_VIEWBOX} accessibilityLabel="ClickFix">
      <Rect width={2050} height={2050} rx={461} fill={SIGNAL} />
      <Polygon points={TILE_CURSOR} fill={INK} />
    </Svg>
  );
}

/**
 * Mark + typeset name: lowercase `clickfix` in Space Grotesk 600 with tight
 * tracking, the mark at 1.2× the cap size (docs/brand.md, "Brand kit").
 */
export function BrandLogo({ size = 28 }: { size?: number }) {
  return (
    <View className="flex-row items-center gap-2" accessible accessibilityLabel="ClickFix">
      <BrandMark size={Math.round(size * 1.2)} />
      <RNText
        className="font-display text-brand-glyph"
        style={{ fontSize: size, lineHeight: size * 1.15, letterSpacing: -0.04 * size }}>
        clickfix
      </RNText>
    </View>
  );
}
