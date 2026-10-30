import { ImageResponse } from "next/og";

import {
  TILE_CURSOR_POINTS,
  TILE_VIEWBOX,
} from "@/components/brand/clickfix-paths";
import { brand } from "@/config/brand";

export const OG_SIZE = { width: 1200, height: 630 };

/**
 * Open Graph card of the marketing site: ink with a signal glow, the green
 * tile mark, an eyebrow and the page title.
 */
export function ogImage({
  eyebrow,
  title,
}: {
  eyebrow: string;
  title: string;
}) {
  return new ImageResponse(
    <div
      style={{
        width: "100%",
        height: "100%",
        display: "flex",
        flexDirection: "column",
        justifyContent: "space-between",
        padding: 72,
        background: `radial-gradient(circle at 85% 15%, #164D1E 0%, ${brand.colors.surface} 55%)`,
        color: "#fff",
      }}
    >
      <div style={{ display: "flex", alignItems: "center", gap: 20 }}>
        <svg width="64" height="64" viewBox={TILE_VIEWBOX}>
          <rect
            width="2050"
            height="2050"
            rx="420"
            fill={brand.colors.signal}
          />
          <polygon fill={brand.colors.ink} points={TILE_CURSOR_POINTS} />
        </svg>
        <span style={{ fontSize: 40, fontWeight: 700, letterSpacing: -1 }}>
          {brand.wordmark}
        </span>
      </div>
      <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
        <span
          style={{
            fontSize: 24,
            letterSpacing: 6,
            textTransform: "uppercase",
            color: "#7DFF86",
          }}
        >
          {eyebrow}
        </span>
        <span
          style={{
            fontSize: 72,
            fontWeight: 700,
            lineHeight: 1.04,
            letterSpacing: -2,
            maxWidth: 1000,
          }}
        >
          {title}
        </span>
      </div>
      <span style={{ fontSize: 24, color: "rgba(255,255,255,0.55)" }}>
        {brand.siteUrl.replace(/^https?:\/\//, "")}
      </span>
    </div>,
    OG_SIZE,
  );
}
