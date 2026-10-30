/**
 * A small, safe markdown subset for chat messages: **bold**, *italic*,
 * `code`, ```code blocks```, [links](https://…), bare links, "- " and
 * "1. " lists and line breaks. It returns a token tree; the runtime builds
 * DOM nodes from it with textContent only (never innerHTML).
 */

export type Inline =
  | { t: "text"; v: string }
  | { t: "strong" | "em"; c: Inline[] }
  | { t: "code"; v: string }
  | { t: "link"; href: string; c: Inline[] }
  | { t: "br" };

export type Block =
  | { t: "p"; c: Inline[] }
  | { t: "list"; ordered: boolean; items: Inline[][] }
  | { t: "pre"; v: string };

const LINK = /^\[([^\]\n]{1,300})\]\((https?:\/\/[^\s)]+|mailto:[^\s)]+)\)/;
const AUTO = /^https?:\/\/[^\s<>"']+[^\s<>"'.,;:!?)]/;

/** Inline tokens of one line. */
export function inline(src: string): Inline[] {
  const out: Inline[] = [];
  let text = "";
  const flush = () => {
    if (text) out.push({ t: "text", v: text });
    text = "";
  };
  let i = 0;
  while (i < src.length) {
    const rest = src.slice(i);
    let m: RegExpExecArray | null;
    if (rest.startsWith("`")) {
      const end = rest.indexOf("`", 1);
      if (end > 1) {
        flush();
        out.push({ t: "code", v: rest.slice(1, end) });
        i += end + 1;
        continue;
      }
    }
    if (rest.startsWith("**")) {
      const end = rest.indexOf("**", 2);
      if (end > 2) {
        flush();
        out.push({ t: "strong", c: inline(rest.slice(2, end)) });
        i += end + 2;
        continue;
      }
    }
    if ((rest[0] === "*" || rest[0] === "_") && rest[1] !== " ") {
      const end = rest.indexOf(rest[0], 1);
      if (end > 1 && rest[end - 1] !== " ") {
        flush();
        out.push({ t: "em", c: inline(rest.slice(1, end)) });
        i += end + 1;
        continue;
      }
    }
    if ((m = LINK.exec(rest))) {
      flush();
      out.push({ t: "link", href: m[2]!, c: inline(m[1]!) });
      i += m[0].length;
      continue;
    }
    if ((m = AUTO.exec(rest)) && (i === 0 || /\s|\(/.test(src[i - 1]!))) {
      flush();
      out.push({ t: "link", href: m[0], c: [{ t: "text", v: m[0] }] });
      i += m[0].length;
      continue;
    }
    text += src[i];
    i++;
  }
  flush();
  return out;
}

/** Block tokens of a message. With markdown off, only links are kept. */
export function parse(src: string, markdown = true): Block[] {
  const lines = src.replace(/\r\n?/g, "\n").split("\n");
  if (!markdown) {
    return [
      {
        t: "p",
        c: lines.flatMap((l, n) => {
          const parts: Inline[] = [];
          if (n > 0) parts.push({ t: "br" });
          let rest = l;
          while (rest) {
            const i = rest.search(/https?:\/\//);
            if (i < 0) {
              parts.push({ t: "text", v: rest });
              break;
            }
            if (i > 0) parts.push({ t: "text", v: rest.slice(0, i) });
            const m = AUTO.exec(rest.slice(i));
            const url = m ? m[0] : rest.slice(i).split(/\s/)[0]!;
            parts.push(
              m
                ? { t: "link", href: url, c: [{ t: "text", v: url }] }
                : { t: "text", v: url },
            );
            rest = rest.slice(i + url.length);
          }
          return parts;
        }),
      },
    ];
  }
  const blocks: Block[] = [];
  let para: Inline[] = [];
  const endPara = () => {
    if (para.length) blocks.push({ t: "p", c: para });
    para = [];
  };
  for (let n = 0; n < lines.length; n++) {
    const line = lines[n]!;
    if (line.startsWith("```")) {
      endPara();
      const body: string[] = [];
      n++;
      while (n < lines.length && !lines[n]!.startsWith("```"))
        body.push(lines[n++]!);
      blocks.push({ t: "pre", v: body.join("\n") });
      continue;
    }
    const item = /^\s*(?:([-*•])|(\d{1,3})[.)])\s+(.*)$/.exec(line);
    if (item) {
      endPara();
      const ordered = Boolean(item[2]);
      const last = blocks[blocks.length - 1];
      const content = inline(item[3]!);
      if (last && last.t === "list" && last.ordered === ordered)
        last.items.push(content);
      else blocks.push({ t: "list", ordered, items: [content] });
      continue;
    }
    if (!line.trim()) {
      endPara();
      continue;
    }
    if (para.length) para.push({ t: "br" });
    para.push(...inline(line));
  }
  endPara();
  return blocks;
}

/** Plain text of a message (previews, notifications). */
export function plain(src: string): string {
  return src
    .replace(/```[\s\S]*?```/g, " ")
    .replace(/\[([^\]]+)\]\([^)]+\)/g, "$1")
    .replace(/[*_`]/g, "")
    .replace(/\s+/g, " ")
    .trim();
}
