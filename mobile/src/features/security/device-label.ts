/**
 * A readable device name from a session's User-Agent: the app's own
 * (`ClickFixMobile/1.2.0 (ios)`), else the browser and OS. Free of app
 * imports so it runs under `node --test`.
 */
export function deviceLabel(ua: string | null | undefined): { kind: 'app' | 'browser' | 'unknown'; label: string } {
  const s = (ua ?? '').trim();
  if (!s) return { kind: 'unknown', label: '' };
  const app = /ClickFixMobile\/([\w.-]+)\s*\((ios|android)\)/i.exec(s);
  if (app) return { kind: 'app', label: `ClickFix ${app[2].toLowerCase() === 'ios' ? 'iOS' : 'Android'} ${app[1]}` };
  const browser = /Edg\//.test(s)
    ? 'Edge'
    : /OPR\//.test(s)
      ? 'Opera'
      : /Firefox\//.test(s)
        ? 'Firefox'
        : /Chrome\//.test(s)
          ? 'Chrome'
          : /Safari\//.test(s)
            ? 'Safari'
            : null;
  const os = /iPhone|iPad/.test(s)
    ? 'iOS'
    : /Android/.test(s)
      ? 'Android'
      : /Mac OS X|Macintosh/.test(s)
        ? 'macOS'
        : /Windows/.test(s)
          ? 'Windows'
          : /Linux/.test(s)
            ? 'Linux'
            : null;
  if (!browser && !os) return { kind: 'unknown', label: s.slice(0, 60) };
  return { kind: 'browser', label: [browser, os].filter(Boolean).join(' · ') };
}
