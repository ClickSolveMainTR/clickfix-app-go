/**
 * Download page helpers (CF-240, `/indir`): operating system detection and
 * grouping of the API's download entries. Pure functions; the page and
 * downloads.test.mjs use them.
 */

export type DesktopApp = "agent" | "support";
export type DesktopOS = "darwin" | "windows" | "linux";

/** One entry of GET /v1/public/desktop/downloads. */
export type DesktopDownload = {
  app: DesktopApp;
  target: string;
  os: DesktopOS;
  arch: string;
  version: string;
  channel: "stable" | "beta";
  os_signed: boolean;
  file: { name: string; size: number; sha256: string; url: string };
  published_at: string;
};

export const DESKTOP_APPS: readonly DesktopApp[] = ["support", "agent"];

/** Order of operating systems and architectures on the page. */
const OS_ORDER: DesktopOS[] = ["darwin", "windows", "linux"];
const ARCH_ORDER = ["aarch64", "x86_64", "i686", "armv7"];

/**
 * The visitor's operating system from the user agent (and, where the
 * browser offers it, `navigator.userAgentData.platform`). Phones and
 * tablets answer null: the desktop apps do not run there.
 */
export function detectOS(userAgent: string, platform = ""): DesktopOS | null {
  const ua = `${platform} ${userAgent}`.toLowerCase();
  if (/android|iphone|ipad|ipod|mobile/.test(ua)) return null;
  if (/windows|win32|win64/.test(ua)) return "windows";
  if (/mac ?os|macintosh|macintel/.test(ua)) return "darwin";
  if (/linux|x11|cros/.test(ua)) return "linux";
  return null;
}

/**
 * The entry to recommend for an app on the detected system: on macOS the
 * Apple silicon build first (a browser cannot tell an Intel Mac apart
 * reliably; both are listed), elsewhere x86_64 first.
 */
export function recommended(
  items: DesktopDownload[],
  app: DesktopApp,
  os: DesktopOS | null,
): DesktopDownload | null {
  if (!os) return null;
  const candidates = sortDownloads(
    items.filter((item) => item.app === app && item.os === os),
  );
  return candidates[0] ?? null;
}

/** Stable order: macOS, Windows, Linux; Apple silicon before Intel. */
export function sortDownloads(items: DesktopDownload[]): DesktopDownload[] {
  const rank = (list: string[], value: string) => {
    const i = list.indexOf(value);
    return i === -1 ? list.length : i;
  };
  return [...items].sort(
    (a, b) =>
      rank(OS_ORDER, a.os) - rank(OS_ORDER, b.os) ||
      rank(ARCH_ORDER, a.arch) - rank(ARCH_ORDER, b.arch),
  );
}

/** `?app=support` (remote support message) puts that app first. */
export function appOrder(selected: string | null): DesktopApp[] {
  if (selected === "agent") return ["agent", "support"];
  return [...DESKTOP_APPS];
}

/** "84,2 MB" style size in the page's locale. */
export function formatSize(bytes: number, locale: string): string {
  const mb = bytes / (1024 * 1024);
  return `${new Intl.NumberFormat(locale, { maximumFractionDigits: 1 }).format(mb)} MB`;
}

/** Only API-owned same-origin paths are used as download links. */
export function isApiDownloadPath(url: string): boolean {
  return url.startsWith("/api/v1/public/desktop/files/");
}
