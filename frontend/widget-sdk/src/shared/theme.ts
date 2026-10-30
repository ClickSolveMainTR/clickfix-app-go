/** Color helpers shared by the runtime and the editor's contrast checks. */

export function hexToRgb(hex: string): [number, number, number] {
  const n = parseInt(hex.slice(1), 16);
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}

/** WCAG relative luminance. */
export function luminance(hex: string): number {
  const lin = hexToRgb(hex).map((c) => {
    const s = c / 255;
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * lin[0]! + 0.7152 * lin[1]! + 0.0722 * lin[2]!;
}

/** WCAG contrast ratio of two colors (1–21). */
export function contrast(a: string, b: string): number {
  const [x, y] = [luminance(a), luminance(b)].sort((p, q) => q - p);
  return (x! + 0.05) / (y! + 0.05);
}

/** Readable text color on a background. */
export function onColor(hex: string): string {
  return contrast(hex, "#ffffff") >= contrast(hex, "#111827")
    ? "#ffffff"
    : "#111827";
}

export const FONT_STACKS: Record<string, string> = {
  system:
    'ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif',
  inherit: "inherit",
  rounded:
    'ui-rounded, "SF Pro Rounded", "Nunito", "Varela Round", ui-sans-serif, system-ui, sans-serif',
  serif: 'ui-serif, Georgia, Cambria, "Times New Roman", serif',
  mono: 'ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace',
};

/** Background images of the panel patterns (inline SVG, no requests). */
export function patternCSS(pattern: string, accent: string): string {
  const c = encodeURIComponent(accent);
  switch (pattern) {
    case "dots":
      return `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='16' height='16'%3E%3Ccircle cx='2' cy='2' r='1.2' fill='${c}' fill-opacity='.18'/%3E%3C/svg%3E")`;
    case "grid":
      return `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='20' height='20'%3E%3Cpath d='M20 0H0v20' fill='none' stroke='${c}' stroke-opacity='.12'/%3E%3C/svg%3E")`;
    case "waves":
      return `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='40' height='12'%3E%3Cpath d='M0 6c10-8 10 8 20 0s10 8 20 0' fill='none' stroke='${c}' stroke-opacity='.14'/%3E%3C/svg%3E")`;
    case "diagonal":
      return `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='10'%3E%3Cpath d='M0 10L10 0' stroke='${c}' stroke-opacity='.12'/%3E%3C/svg%3E")`;
    default:
      return "none";
  }
}
