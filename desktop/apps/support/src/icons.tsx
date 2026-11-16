// Marka kiti geometrisi (frontend/src/assets/brand/, docs/brand.md) ve
// tasarımdaki çizgi ikonlar.

const TILE_CURSOR = "482.33 1391 418.33 647 1698.33 815 706.33 823 482.33 1391";
const BARE_CURSOR = "64 744 0 0 1280 168 288 176 64 744";

/** Yeşil karo, siyah imleç (uygulama simgesi). */
export function Tile({ size = 34, label }: { size?: number; label?: string }) {
  return (
    <svg
      viewBox="0 0 2050 2050"
      width={size}
      height={size}
      role={label ? "img" : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
    >
      <rect width="2050" height="2050" rx="450" fill="#03ff13" />
      <polygon fill="#000000" points={TILE_CURSOR} />
    </svg>
  );
}

/** Çıplak imleç; `outline` ile siyah kontur (tasarımdaki uzak imleç). */
export function Cursor({
  width = 16,
  height = 10,
  outline = false,
  fill = "currentColor",
}: {
  width?: number;
  height?: number;
  outline?: boolean;
  fill?: string;
}) {
  return (
    <svg viewBox="0 0 1280 744" width={width} height={height} aria-hidden="true">
      <polygon
        fill={fill}
        stroke={outline ? "#000000" : undefined}
        strokeWidth={outline ? 40 : undefined}
        points={BARE_CURSOR}
      />
    </svg>
  );
}

type IconProps = { size?: number };

const base = {
  fill: "none",
  stroke: "currentColor",
  strokeLinecap: "round" as const,
  strokeLinejoin: "round" as const,
  "aria-hidden": true,
};

export const ArrowRight = ({ size = 16 }: IconProps) => (
  <svg width={size} height={size} viewBox="0 0 24 24" strokeWidth="2" {...base}>
    <path d="M5 12h14M13 6l6 6-6 6" />
  </svg>
);

export const Shield = ({ size = 20 }: IconProps) => (
  <svg width={size} height={size} viewBox="0 0 24 24" strokeWidth="2" {...base}>
    <path d="M12 3l8 3v6c0 4.5-3.4 8.3-8 9-4.6-.7-8-4.5-8-9V6l8-3z" />
    <path d="M12 8v5M12 16.5v.01" />
  </svg>
);

export const Badge = ({ size = 13 }: IconProps) => (
  <svg width={size} height={size} viewBox="0 0 24 24" strokeWidth="2.5" {...base}>
    <path d="M12 2l2.4 2.1 3.2-.3.9 3.1 2.8 1.6-1 3 1 3-2.8 1.6-.9 3.1-3.2-.3L12 22l-2.4-2.1-3.2.3-.9-3.1-2.8-1.6 1-3-1-3 2.8-1.6.9-3.1 3.2.3z" />
    <path d="M8.5 12l2.5 2.5 4.5-5" />
  </svg>
);

export const Eye = ({ size = 12 }: IconProps) => (
  <svg width={size} height={size} viewBox="0 0 24 24" strokeWidth="2" {...base}>
    <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z" />
    <circle cx="12" cy="12" r="3" />
  </svg>
);

export const Pointer = ({ size = 12 }: IconProps) => (
  <svg width={size} height={size} viewBox="0 0 24 24" strokeWidth="2" {...base}>
    <path d="M5 3l14 7-6 2-2 6z" />
  </svg>
);

export const Record = ({ size = 12 }: IconProps) => (
  <svg width={size} height={size} viewBox="0 0 24 24" strokeWidth="2" {...base}>
    <circle cx="12" cy="12" r="8" />
    <circle cx="12" cy="12" r="3" fill="currentColor" />
  </svg>
);

export const Flag = ({ size = 13 }: IconProps) => (
  <svg width={size} height={size} viewBox="0 0 24 24" strokeWidth="2" {...base}>
    <path d="M4 21V4h11l-1 4h6v9h-9l1-4H4" />
  </svg>
);

export const Monitor = ({ size = 16 }: IconProps) => (
  <svg width={size} height={size} viewBox="0 0 24 24" strokeWidth="2" {...base}>
    <rect x="3" y="4" width="18" height="12" rx="2" />
    <path d="M8 20h8M12 16v4" />
  </svg>
);

export const Clock = ({ size = 14 }: IconProps) => (
  <svg width={size} height={size} viewBox="0 0 24 24" strokeWidth="2" {...base}>
    <circle cx="12" cy="12" r="9" />
    <path d="M12 7v5l3 2" />
  </svg>
);

export const Check = ({ size = 30 }: IconProps) => (
  <svg width={size} height={size} viewBox="0 0 24 24" strokeWidth="2.4" {...base}>
    <path d="M5 12.5l4.5 4.5L19 7.5" />
  </svg>
);

export const StopSquare = () => (
  <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true">
    <rect width="10" height="10" rx="2" fill="currentColor" />
  </svg>
);

export const Grip = () => (
  <svg width="8" height="14" viewBox="0 0 8 14" aria-hidden="true">
    {[2, 7, 12].map((y) => (
      <g key={y}>
        <circle cx="2" cy={y} r="1.2" fill="currentColor" />
        <circle cx="6" cy={y} r="1.2" fill="currentColor" />
      </g>
    ))}
  </svg>
);
