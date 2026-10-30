import { parse, type Block, type Inline } from "../shared/markdown";

type Attr = string | boolean | number | EventListener | undefined | null;
export type Attrs = Record<string, Attr>;
export type Child = Node | string | null | undefined | false;

/** Builds an element; "text" sets textContent, "on…" adds listeners. */
export function el<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  attrs: Attrs = {},
  children: Child[] = [],
): HTMLElementTagNameMap[K] {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v === undefined || v === null || v === false) continue;
    if (typeof v === "function") node.addEventListener(k.slice(2), v);
    else if (k === "class") node.className = String(v);
    else if (k === "text") node.textContent = String(v);
    else node.setAttribute(k, v === true ? "" : String(v));
  }
  for (const c of children) {
    if (c === null || c === undefined || c === false) continue;
    node.append(typeof c === "string" ? document.createTextNode(c) : c);
  }
  return node;
}

const SVG = (body: string, fill = false) =>
  `<svg viewBox="0 0 24 24" fill="${fill ? "currentColor" : "none"}" stroke="${fill ? "none" : "currentColor"}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false">${body}</svg>`;

export const ICONS = {
  chat: SVG(
    '<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>',
  ),
  help: SVG(
    '<circle cx="12" cy="12" r="10"/><path d="M9.1 9a3 3 0 0 1 5.8 1c0 2-3 3-3 3"/><path d="M12 17h.01"/>',
  ),
  headset: SVG(
    '<path d="M3 14v-3a9 9 0 0 1 18 0v3"/><path d="M21 16a2 2 0 0 1-2 2h-1v-6h1a2 2 0 0 1 2 2z"/><path d="M3 16a2 2 0 0 0 2 2h1v-6H5a2 2 0 0 0-2 2z"/>',
  ),
  sparkles: SVG(
    '<path d="m12 3-1.9 5.8L4 10.7l6.1 1.9L12 18.5l1.9-5.9L20 10.7l-6.1-1.9z"/><path d="M5 3v4M19 17v4M3 5h4M17 19h4"/>',
  ),
  wave: SVG(
    '<path d="M18 11V6a2 2 0 0 0-4 0v5"/><path d="M14 10V4a2 2 0 0 0-4 0v2"/><path d="M10 10.5V6a2 2 0 0 0-4 0v8"/><path d="M18 8a2 2 0 1 1 4 0v6a8 8 0 0 1-8 8h-2c-2.8 0-4.5-.9-6-2.4l-3.6-3.6a2 2 0 0 1 2.8-2.8L7 15"/>',
  ),
  close: SVG('<path d="M18 6 6 18M6 6l12 12"/>'),
  back: SVG('<path d="m15 18-6-6 6-6"/>'),
  send: SVG('<path d="m22 2-7 20-4-9-9-4z"/><path d="M22 2 11 13"/>'),
  clip: SVG(
    '<path d="m21.44 11.05-9.19 9.19a6 6 0 0 1-8.49-8.49l8.57-8.57A4 4 0 1 1 18 8.84l-8.59 8.57a2 2 0 0 1-2.83-2.83l8.49-8.48"/>',
  ),
  smile: SVG(
    '<circle cx="12" cy="12" r="10"/><path d="M8 14s1.5 2 4 2 4-2 4-2"/><path d="M9 9h.01M15 9h.01"/>',
  ),
  home: SVG(
    '<path d="m3 9 9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><path d="M9 22V12h6v10"/>',
  ),
  messages: SVG(
    '<path d="M14 9a2 2 0 0 1-2 2H6l-4 4V4a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2z"/><path d="M18 9h2a2 2 0 0 1 2 2v11l-4-4h-6a2 2 0 0 1-2-2v-1"/>',
  ),
  search: SVG('<circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/>'),
  arrow: SVG('<path d="M5 12h14M12 5l7 7-7 7"/>'),
  external: SVG(
    '<path d="M15 3h6v6M10 14 21 3M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>',
  ),
  phone: SVG(
    '<path d="M22 16.9v3a2 2 0 0 1-2.2 2 19.8 19.8 0 0 1-8.6-3.1 19.5 19.5 0 0 1-6-6A19.8 19.8 0 0 1 2.1 4.2 2 2 0 0 1 4.1 2h3a2 2 0 0 1 2 1.7c.1 1 .4 1.9.7 2.8a2 2 0 0 1-.5 2.1L8 9.9a16 16 0 0 0 6 6l1.3-1.3a2 2 0 0 1 2.1-.4c.9.3 1.8.6 2.8.7a2 2 0 0 1 1.7 2z"/>',
  ),
  mail: SVG(
    '<rect x="2" y="4" width="20" height="16" rx="2"/><path d="m22 7-10 6L2 7"/>',
  ),
  video: SVG(
    '<path d="m22 8-6 4 6 4z"/><rect x="2" y="6" width="14" height="12" rx="2"/>',
  ),
  status: SVG('<path d="M22 12h-4l-3 9L9 3l-3 9H2"/>'),
  calendar: SVG(
    '<rect x="3" y="4" width="18" height="18" rx="2"/><path d="M16 2v4M8 2v4M3 10h18"/>',
  ),
  link: SVG(
    '<path d="M10 13a5 5 0 0 0 7.5.5l3-3a5 5 0 0 0-7-7l-1.7 1.7"/><path d="M14 11a5 5 0 0 0-7.5-.5l-3 3a5 5 0 0 0 7 7l1.7-1.7"/>',
  ),
  user: SVG('<circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/>'),
  whatsapp: SVG(
    '<path d="M12 2a10 10 0 0 0-8.6 15.1L2 22l5-1.3A10 10 0 1 0 12 2zm5.3 14.1c-.2.6-1.3 1.2-1.8 1.2-.5.1-1 .1-3.3-.8-2.8-1.1-4.6-4-4.7-4.2-.1-.2-1.1-1.5-1.1-2.9s.7-2.1 1-2.4c.3-.3.6-.3.8-.3h.6c.2 0 .4 0 .6.5l.9 2.1c.1.2.1.3 0 .5l-.4.6-.4.4c-.1.2-.3.3-.1.6.2.3.8 1.3 1.7 2.1 1.2 1 2.1 1.3 2.4 1.5.3.1.5.1.6-.1l.9-1.1c.2-.3.4-.2.6-.1l2 1c.3.1.5.2.5.3.1.2.1.7-.1 1.3z"/>',
    true,
  ),
  instagram: SVG(
    '<rect x="2" y="2" width="20" height="20" rx="5"/><circle cx="12" cy="12" r="4"/><path d="M17.5 6.5h.01"/>',
  ),
  telegram: SVG('<path d="m22 3-20 8 7 2 2 7 4-5 5 4z"/><path d="m9 13 9-7"/>'),
};
export type IconName = keyof typeof ICONS;

/** Direction-aware icons mirror in right-to-left layouts. */
const FLIP: IconName[] = ["back", "arrow", "send"];

export function icon(name: IconName): HTMLSpanElement {
  const s = el("span", { class: FLIP.includes(name) ? "icon flip" : "icon" });
  s.innerHTML = ICONS[name]; // static, trusted markup
  return s;
}

function inlineNodes(list: Inline[]): Node[] {
  return list.map((t) => {
    switch (t.t) {
      case "text":
        return document.createTextNode(t.v);
      case "br":
        return el("br");
      case "code":
        return el("code", { text: t.v });
      case "strong":
        return el("strong", {}, inlineNodes(t.c));
      case "em":
        return el("em", {}, inlineNodes(t.c));
      case "link":
        return el(
          "a",
          {
            href: t.href,
            target: "_blank",
            rel: "noopener noreferrer nofollow",
          },
          inlineNodes(t.c),
        );
    }
  });
}

function blockNode(b: Block): Node {
  switch (b.t) {
    case "pre":
      return el("pre", {}, [el("code", { text: b.v })]);
    case "list":
      return el(
        b.ordered ? "ol" : "ul",
        {},
        b.items.map((i) => el("li", {}, inlineNodes(i))),
      );
    default:
      return el("p", {}, inlineNodes(b.c));
  }
}

/** Message text as DOM (safe markdown subset; links only when off). */
export function renderText(text: string, markdown: boolean): DocumentFragment {
  const frag = document.createDocumentFragment();
  for (const b of parse(text, markdown)) frag.append(blockNode(b));
  return frag;
}
