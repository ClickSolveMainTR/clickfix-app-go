import type { PublicSettings } from "./types";
import { FONT_STACKS, onColor, patternCSS } from "../shared/theme";

/**
 * Theme variables on the widget root. Colors left empty in the settings
 * fall back to the primary color; text colors are picked for contrast.
 */
export function themeVars(
  s: PublicSettings,
  zIndex?: number,
): Record<string, string> {
  const a = s.appearance;
  const accent = s.color;
  const header = a.header_color || accent;
  const launcher = a.launcher_color || accent;
  const bubble = a.bubble_color || accent;
  return {
    "--cf-accent": accent,
    "--cf-on-accent": onColor(accent),
    "--cf-header": header,
    "--cf-on-header": onColor(header),
    "--cf-launcher": launcher,
    "--cf-on-launcher": onColor(launcher),
    "--cf-bubble": bubble,
    "--cf-on-bubble": onColor(bubble),
    "--cf-radius": `${a.radius}px`,
    "--cf-radius-sm": `${Math.max(4, Math.round(a.radius * 0.6))}px`,
    "--cf-font": FONT_STACKS[a.font] ?? FONT_STACKS.system!,
    "--cf-pattern": patternCSS(a.pattern, header),
    "--cf-launcher-size": `${a.launcher.size}px`,
    "--cf-offset-x": `${a.launcher.offset_x}px`,
    "--cf-offset-y": `${a.launcher.offset_y}px`,
    "--cf-z": String(zIndex ?? a.z_index),
  };
}

const LIGHT = `--cf-bg:#ffffff;--cf-surface:#f7f7f9;--cf-card:#ffffff;--cf-text:#111827;--cf-muted:#6b7280;--cf-border:#e5e7eb;--cf-hover:#f3f4f6;--cf-input:#ffffff;--cf-in-bubble:#f1f2f6;--cf-on-in-bubble:#111827;--cf-shadow:0 16px 48px rgba(17,24,39,.22);--cf-notice:#fff7ed;--cf-on-notice:#9a3412;--cf-danger:#b91c1c;`;
const DARK = `--cf-bg:#111318;--cf-surface:#0b0d12;--cf-card:#1a1d24;--cf-text:#f3f4f6;--cf-muted:#9ca3af;--cf-border:#2a2f3a;--cf-hover:#232733;--cf-input:#151820;--cf-in-bubble:#232733;--cf-on-in-bubble:#f3f4f6;--cf-shadow:0 16px 48px rgba(0,0,0,.55);--cf-notice:#3b2412;--cf-on-notice:#fdba74;--cf-danger:#f87171;`;

/** Static stylesheet of the widget (scoped by the Shadow DOM). */
export const CSS = `
:host { all: initial; }
* { box-sizing: border-box; }
.root { ${LIGHT} position: fixed; bottom: var(--cf-offset-y); z-index: var(--cf-z); color: var(--cf-text); font-family: var(--cf-font); font-size: 14px; line-height: 1.45; -webkit-font-smoothing: antialiased; }
.root.theme-dark { ${DARK} }
@media (prefers-color-scheme: dark) { .root.theme-auto { ${DARK} } }
.root.right { right: var(--cf-offset-x); } .root.left { left: var(--cf-offset-x); }
.root[hidden] { display: none; }
button, input, textarea, select { font: inherit; color: inherit; }
a { color: inherit; }
.icon { display: inline-grid; place-items: center; }
.icon svg { width: 20px; height: 20px; }
.root[dir="rtl"] .flip svg { transform: scaleX(-1); }
.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
:focus-visible { outline: 3px solid color-mix(in srgb, var(--cf-accent) 55%, transparent); outline-offset: 2px; }

/* Launcher */
.launcher { height: var(--cf-launcher-size); min-width: var(--cf-launcher-size); border: 0; cursor: pointer; background: var(--cf-launcher); color: var(--cf-on-launcher);
  box-shadow: 0 8px 24px rgba(17,24,39,.24); display: inline-flex; align-items: center; justify-content: center; gap: 8px; position: relative; transition: transform .15s ease; padding: 0; }
.launcher.circle { border-radius: 50%; width: var(--cf-launcher-size); }
.launcher.rounded { border-radius: calc(var(--cf-launcher-size) * .3); width: var(--cf-launcher-size); }
.launcher.pill, .launcher.labelled { border-radius: 999px; padding: 0 20px 0 16px; width: auto; }
.launcher:hover { transform: scale(1.04); }
.launcher .icon svg { width: calc(var(--cf-launcher-size) * .46); height: calc(var(--cf-launcher-size) * .46); }
.launcher img { width: calc(var(--cf-launcher-size) * .6); height: calc(var(--cf-launcher-size) * .6); object-fit: contain; border-radius: 6px; }
.launcher .label { font-weight: 600; font-size: 15px; white-space: nowrap; }
.launcher[hidden] { display: none; }
.badge { position: absolute; top: -4px; right: -4px; min-width: 20px; height: 20px; border-radius: 10px; padding: 0 6px; background: #dc2626; color: #fff; font-size: 12px; font-weight: 700; display: grid; place-items: center; border: 2px solid #fff; }

/* Proactive bubble */
.proactive { position: absolute; bottom: calc(var(--cf-launcher-size) + 14px); width: 300px; max-width: calc(100vw - 32px); background: var(--cf-card); color: var(--cf-text);
  border-radius: var(--cf-radius); box-shadow: var(--cf-shadow); border: 1px solid var(--cf-border); padding: 14px 14px 12px; cursor: pointer; animation: cf-pop .25s ease; }
.right .proactive { right: 0; } .left .proactive { left: 0; }
.proactive .who { display: flex; align-items: center; gap: 8px; font-size: 12px; color: var(--cf-muted); margin-bottom: 6px; }
.proactive .text { overflow-wrap: anywhere; }
.proactive .text p { margin: 0 0 4px; }
.proactive .x { position: absolute; top: 6px; inset-inline-end: 6px; border: 0; background: transparent; cursor: pointer; color: var(--cf-muted); border-radius: 8px; padding: 2px; }
.proactive .x .icon svg { width: 16px; height: 16px; }

/* Panel */
.panel { position: absolute; bottom: calc(var(--cf-launcher-size) + 16px); width: 384px; height: min(680px, calc(100vh - var(--cf-launcher-size) - var(--cf-offset-y) - 40px)); background: var(--cf-bg); border-radius: var(--cf-radius);
  box-shadow: var(--cf-shadow); display: flex; flex-direction: column; overflow: hidden; border: 1px solid var(--cf-border); animation: cf-pop .2s ease; }
.right .panel { right: 0; } .left .panel { left: 0; }
.panel[hidden] { display: none; }
.root.no-launcher .panel { bottom: 0; }
.root.no-launcher .proactive { bottom: 0; }
@keyframes cf-pop { from { opacity: 0; transform: translateY(8px) scale(.98); } to { opacity: 1; transform: none; } }
@media (prefers-reduced-motion: reduce) { .panel, .proactive { animation: none; } .launcher { transition: none; } }
@media (max-width: 480px) { .root.fullscreen .panel { position: fixed; inset: 0; width: 100vw; height: 100%; border-radius: 0; border: 0; }
  .root.fullscreen.open .launcher { display: none; } }
.preview-frame .panel { animation: none; }

.header { background: var(--cf-header); color: var(--cf-on-header); padding: 14px 16px; display: flex; gap: 10px; align-items: center; position: relative; }
.header::before { content: ""; position: absolute; inset: 0; background-image: var(--cf-pattern); pointer-events: none; }
.header > * { position: relative; }
.header .grow { flex: 1; min-width: 0; }
.header h2 { margin: 0; font-size: 16px; font-weight: 650; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.header .sub { margin: 2px 0 0; opacity: .88; font-size: 12px; display: flex; align-items: center; gap: 6px; }
.header .logo { height: 28px; max-width: 120px; object-fit: contain; }
.icon-btn { background: transparent; border: 0; color: inherit; cursor: pointer; padding: 6px; border-radius: 10px; display: grid; place-items: center; }
.icon-btn:hover { background: color-mix(in srgb, currentColor 14%, transparent); }
.status-dot { width: 8px; height: 8px; border-radius: 50%; background: #22c55e; display: inline-block; flex: none; }
.status-dot.off { background: #9ca3af; }
.avatars { display: flex; }
.avatar { width: 30px; height: 30px; border-radius: 50%; background: color-mix(in srgb, var(--cf-on-header) 22%, transparent); display: grid; place-items: center; font-size: 11px; font-weight: 700;
  border: 2px solid var(--cf-header); margin-inline-start: -8px; overflow: hidden; flex: none; }
.avatar img { width: 100%; height: 100%; object-fit: cover; }
.avatars .avatar:first-child { margin-inline-start: 0; }

/* Home */
.hero { background: var(--cf-header); color: var(--cf-on-header); padding: 22px 20px 56px; position: relative; }
.hero::before { content: ""; position: absolute; inset: 0; background-image: var(--cf-pattern); pointer-events: none; }
.hero > * { position: relative; }
.hero .top { display: flex; justify-content: space-between; align-items: center; margin-bottom: 18px; min-height: 30px; }
.hero h2 { margin: 0; font-size: 24px; font-weight: 700; line-height: 1.2; }
.hero p { margin: 6px 0 0; font-size: 15px; opacity: .9; }
.body { flex: 1; overflow-y: auto; background: var(--cf-surface); }
.home .cards { margin-top: -40px; padding: 0 14px 14px; position: relative; }
.pad { padding: 14px; }
.card { background: var(--cf-card); border: 1px solid var(--cf-border); border-radius: var(--cf-radius-sm); padding: 14px; margin-bottom: 10px; box-shadow: 0 1px 2px rgba(17,24,39,.04); }
.card h3 { margin: 0 0 4px; font-size: 14px; font-weight: 650; }
.card .muted { margin: 0; }
.card-btn { display: flex; width: 100%; text-align: start; gap: 12px; align-items: center; border: 0; background: transparent; padding: 0; cursor: pointer; text-decoration: none; }
.card-btn .grow { flex: 1; min-width: 0; }
.card-btn .go { color: var(--cf-accent); }
.muted { color: var(--cf-muted); font-size: 12px; }
.primary { width: 100%; border: 0; border-radius: var(--cf-radius-sm); padding: 11px 14px; background: var(--cf-accent); color: var(--cf-on-accent); font-weight: 600; cursor: pointer; display: inline-flex; gap: 8px; align-items: center; justify-content: center; }
.primary:disabled { opacity: .55; cursor: default; }
.secondary { border: 1px solid var(--cf-border); background: var(--cf-card); border-radius: var(--cf-radius-sm); padding: 9px 12px; cursor: pointer; font-weight: 600; display: inline-flex; gap: 6px; align-items: center; }
.list-item { display: flex; width: 100%; text-align: start; gap: 10px; align-items: center; padding: 10px; border-radius: var(--cf-radius-sm); border: 0; background: transparent; cursor: pointer; text-decoration: none; }
.list-item:hover, .list-item:focus-visible { background: var(--cf-hover); }
.list-item .grow { flex: 1; min-width: 0; }
.ellipsis { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.dot { width: 9px; height: 9px; border-radius: 50%; background: var(--cf-accent); flex: none; }
.chips { display: flex; flex-wrap: wrap; gap: 6px; }
.chip { border: 1px solid color-mix(in srgb, var(--cf-accent) 45%, var(--cf-border)); color: var(--cf-text); background: var(--cf-card); border-radius: 999px; padding: 6px 12px; cursor: pointer; font-size: 13px; }
.chip:hover { background: color-mix(in srgb, var(--cf-accent) 10%, var(--cf-card)); }
.channels { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 8px; }
.channel { display: inline-flex; align-items: center; gap: 6px; border: 1px solid var(--cf-border); border-radius: 999px; padding: 6px 12px; text-decoration: none; background: var(--cf-card); cursor: pointer; font-size: 13px; }
.channel .icon svg { width: 16px; height: 16px; }
.search { display: flex; align-items: center; gap: 8px; border: 1px solid var(--cf-border); border-radius: var(--cf-radius-sm); padding: 0 10px; background: var(--cf-input); }
.search input { flex: 1; border: 0; background: transparent; padding: 10px 0; outline: none; min-width: 0; }
.search .icon svg { width: 16px; height: 16px; color: var(--cf-muted); }
.article strong { display: block; font-size: 13px; font-weight: 600; }
.article span { display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; font-size: 12px; color: var(--cf-muted); margin-top: 2px; }
.link-strong { display: inline-flex; align-items: center; gap: 4px; margin-top: 8px; font-size: 13px; font-weight: 600; color: var(--cf-accent); text-decoration: none; }
.link-strong .icon svg { width: 14px; height: 14px; }

/* Tabs */
.tabs { display: flex; border-top: 1px solid var(--cf-border); background: var(--cf-bg); }
.tab { flex: 1; border: 0; background: transparent; padding: 8px 4px 10px; cursor: pointer; display: grid; justify-items: center; gap: 2px; font-size: 12px; color: var(--cf-muted); position: relative; }
.tab[aria-selected="true"] { color: var(--cf-accent); font-weight: 600; }
.tab .count { position: absolute; top: 4px; inset-inline-start: calc(50% + 6px); background: #dc2626; color: #fff; border-radius: 9px; font-size: 10px; min-width: 16px; height: 16px; display: grid; place-items: center; padding: 0 4px; }

/* Chat */
.msgs { display: flex; flex-direction: column; gap: 4px; padding: 16px 14px; }
.day { text-align: center; font-size: 11px; color: var(--cf-muted); margin: 8px 0; }
.row { display: flex; gap: 8px; align-items: flex-end; }
.row.in { justify-content: flex-end; }
.row .avatar { width: 26px; height: 26px; border-color: transparent; background: var(--cf-in-bubble); color: var(--cf-text); margin: 0; }
.msg { max-width: 78%; padding: 9px 12px; border-radius: var(--cf-radius); overflow-wrap: anywhere; }
.msg p { margin: 0; } .msg p + p, .msg ul, .msg ol, .msg pre { margin-top: 6px; } .msg ul, .msg ol { padding-inline-start: 18px; margin-bottom: 0; }
.msg code { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; background: color-mix(in srgb, currentColor 12%, transparent); padding: 1px 4px; border-radius: 4px; }
.msg pre { white-space: pre-wrap; background: color-mix(in srgb, currentColor 10%, transparent); padding: 8px; border-radius: 8px; margin-bottom: 0; }
.msg pre code { background: none; padding: 0; }
.msg a { text-decoration: underline; }
.row.in .msg { background: var(--cf-bubble); color: var(--cf-on-bubble); border-end-end-radius: 4px; }
.row.out .msg { background: var(--cf-in-bubble); color: var(--cf-on-in-bubble); border-end-start-radius: 4px; }
.msg.pending { opacity: .65; } .msg.failed { outline: 2px solid var(--cf-danger); cursor: pointer; }
.meta { font-size: 11px; color: var(--cf-muted); margin: 2px 4px 8px; }
.meta.in { text-align: end; } .meta.out { margin-inline-start: 38px; }
.file { display: block; margin-top: 6px; padding: 6px 8px; border-radius: 8px; background: color-mix(in srgb, currentColor 10%, transparent); text-decoration: none; font-size: 12px; }
.file img { max-width: 100%; border-radius: 6px; display: block; margin-bottom: 4px; }
.typing { display: inline-flex; gap: 3px; padding: 12px; background: var(--cf-in-bubble); border-radius: var(--cf-radius); }
.typing i { width: 6px; height: 6px; border-radius: 50%; background: var(--cf-muted); animation: cf-blink 1.2s infinite both; }
.typing i:nth-child(2) { animation-delay: .2s; } .typing i:nth-child(3) { animation-delay: .4s; }
@keyframes cf-blink { 0%, 80%, 100% { opacity: .3; } 40% { opacity: 1; } }
.footer { border-top: 1px solid var(--cf-border); padding: 10px; background: var(--cf-bg); }
.composer { display: flex; gap: 4px; align-items: flex-end; border: 1px solid var(--cf-border); border-radius: var(--cf-radius-sm); padding: 4px; background: var(--cf-input); }
.composer:focus-within { border-color: var(--cf-accent); }
.composer textarea { flex: 1; resize: none; border: 0; padding: 8px; max-height: 120px; min-height: 36px; background: transparent; outline: none; }
.composer .icon-btn { color: var(--cf-muted); } .composer .icon-btn:hover { color: var(--cf-text); }
.composer .send { color: var(--cf-on-accent); background: var(--cf-accent); }
.composer .send:hover { color: var(--cf-on-accent); opacity: .9; }
.emoji-grid { display: grid; grid-template-columns: repeat(8, 1fr); gap: 2px; padding: 6px; border: 1px solid var(--cf-border); border-radius: var(--cf-radius-sm); margin-bottom: 8px; background: var(--cf-card); }
.emoji-grid button { border: 0; background: transparent; font-size: 20px; cursor: pointer; border-radius: 6px; padding: 2px; }
.emoji-grid button:hover { background: var(--cf-hover); }
.field { display: block; margin-bottom: 8px; } .field > span { display: block; font-size: 12px; color: var(--cf-muted); margin-bottom: 3px; }
.field input, .field select, .field textarea { width: 100%; border: 1px solid var(--cf-border); border-radius: var(--cf-radius-sm); padding: 8px 10px; background: var(--cf-input); }
.field input:focus, .field select:focus, .field textarea:focus { outline: 2px solid var(--cf-accent); outline-offset: -1px; }
.field small { display: block; font-size: 11px; color: var(--cf-muted); margin-top: 2px; }
.check { display: flex; align-items: flex-start; gap: 8px; font-size: 13px; } .check input { width: auto; margin-top: 3px; }
.choices { display: flex; flex-direction: column; gap: 4px; }
.error { color: var(--cf-danger); font-size: 12px; margin: 6px 0 0; }
.notice { background: var(--cf-notice); color: var(--cf-on-notice); border-radius: var(--cf-radius-sm); padding: 10px 12px; font-size: 13px; margin-bottom: 10px; }
.rating { text-align: center; }
.scale { display: flex; justify-content: center; gap: 6px; margin: 8px 0; }
.scale button { background: none; border: 0; font-size: 28px; cursor: pointer; line-height: 1; padding: 2px; filter: grayscale(1); opacity: .6; }
.scale button.on, .scale button:hover { filter: none; opacity: 1; }
.scale.stars button { color: #f59e0b; }
.rating textarea { width: 100%; border: 1px solid var(--cf-border); border-radius: 8px; padding: 8px; margin-bottom: 8px; resize: vertical; min-height: 56px; background: var(--cf-input); }
.powered { text-align: center; font-size: 11px; color: var(--cf-muted); padding: 8px 0 2px; }
.powered a { text-decoration: none; }
.human { margin: 0 0 8px; }
.consent { padding: 20px; }
.consent h3 { margin: 0 0 8px; font-size: 16px; }
.consent p { margin: 0 0 14px; color: var(--cf-muted); }
`;
