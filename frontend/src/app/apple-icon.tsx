import { ImageResponse } from "next/og";

import {
  TILE_CURSOR_POINTS,
  TILE_VIEWBOX,
} from "@/components/brand/clickfix-paths";
import { brand } from "@/config/brand";

export const size = { width: 180, height: 180 };
export const contentType = "image/png";

/**
 * Apple touch / PWA icon: the kit's green tile (black cursor on signal green,
 * Asset 1). iOS rounds the corners itself, so the tile is square and full
 * bleed.
 */
export default function AppleIcon() {
  return new ImageResponse(
    <svg width="180" height="180" viewBox={TILE_VIEWBOX}>
      <rect width="2050" height="2050" fill={brand.colors.signal} />
      <polygon fill={brand.colors.ink} points={TILE_CURSOR_POINTS} />
    </svg>,
    { ...size },
  );
}
