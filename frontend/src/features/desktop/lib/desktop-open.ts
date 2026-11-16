/**
 * "Masaüstünde aç" links (CF-236). Notification e-mails link to
 * `/desktop/open?path=<in-app path>` (an https URL, because many mail
 * clients drop custom-scheme links). The page hands the path to ClickFix
 * Masaüstü through `clickfix://` and offers the browser as a fallback.
 * Free of imports (`node --test`).
 */

export const DESKTOP_OPEN_PATH = "/desktop/open";

const UUID =
  /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;

/** Same rules as the app (`desktop/apps/agent/src-tauri/src/links.rs`). */
export function desktopOpenPath(raw: string | null | undefined): string | null {
  const p = (raw ?? "").trim();
  if (
    !p.startsWith("/") ||
    p.startsWith("//") ||
    p.includes("\\") ||
    p.length > 2048 ||
    /[\u0000-\u001f\u007f]/.test(p)
  ) {
    return null;
  }
  return p;
}

/**
 * The app link for an in-app path: a conversation (`/inbox?c=<uuid>`) uses
 * the short `clickfix://conversation/<uuid>` form, anything else
 * `clickfix://open?path=...`.
 */
export function desktopAppLink(path: string): string {
  const q = path.startsWith("/inbox?") ? path.slice("/inbox?".length) : null;
  if (q !== null) {
    const params = new URLSearchParams(q);
    const id = params.get("c") ?? "";
    if ([...params.keys()].length === 1 && UUID.test(id)) {
      return `clickfix://conversation/${id.toLowerCase()}`;
    }
  }
  return `clickfix://open?path=${encodeURIComponent(path)}`;
}
