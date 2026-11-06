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

const isSpace = (c: string | undefined) =>
  c === " " || c === "\t" || c === "\u00a0";
const isWord = (c: string | undefined) =>
  c !== undefined && /[\p{L}\p{N}]/u.test(c);

/** Index of the "***" closing bold italic opened at rest[0..3], or -1. */
function closeBoth(rest: string): number {
  for (let from = 4; from < rest.length;) {
    const e = rest.indexOf("***", from);
    if (e < 0) return -1;
    if (!isSpace(rest[e - 1])) return e;
    from = e + 1;
  }
  return -1;
}

/** Index of the "**" closing a strong opened at rest[0..2], or -1. */
function closeStrong(rest: string): number {
  for (let from = 3; from < rest.length;) {
    const e = rest.indexOf("**", from);
    if (e < 0) return -1;
    if (!isSpace(rest[e - 1])) return e;
    from = e + 1;
  }
  return -1;
}

/**
 * Index of the "*" or "_" closing an emphasis opened at rest[0], or -1: not
 * after a space, not part of a doubled marker and, for "_", not inside a
 * word.
 */
function closeEm(rest: string): number {
  const d = rest[0];
  for (let e = 2; e < rest.length; e++) {
    if (rest[e] !== d || rest[e - 1] === d || isSpace(rest[e - 1])) continue;
    if (rest[e + 1] === d) continue;
    if (d === "_" && isWord(rest[e + 1])) continue;
    return e;
  }
  return -1;
}

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
    // A marker opens only at a word boundary and before a non-space, so
    // 2*3*4, snake_case and a lone * stay text (the Go parser of the channel
    // translator applies the same rules; see markdown_cases.json).
    // A marker that follows its own kind is the tail of a run that was
    // already read as text ("kelime***x***", "****x****").
    const opens = !isWord(src[i - 1]) && src[i - 1] !== rest[0];
    // ***text*** is bold italic (common in AI answers).
    if (
      opens &&
      rest.startsWith("***") &&
      rest.length > 3 &&
      rest[3] !== "*" &&
      !isSpace(rest[3])
    ) {
      const end = closeBoth(rest);
      if (end > 0) {
        flush();
        out.push({
          t: "strong",
          c: [{ t: "em", c: inline(rest.slice(3, end)) }],
        });
        i += end + 3;
        continue;
      }
    }
    if (
      opens &&
      rest.startsWith("**") &&
      rest.length > 2 &&
      rest[2] !== "*" &&
      !isSpace(rest[2])
    ) {
      const end = closeStrong(rest);
      if (end > 0) {
        flush();
        out.push({ t: "strong", c: inline(rest.slice(2, end)) });
        i += end + 2;
        continue;
      }
    }
    if (
      opens &&
      (rest[0] === "*" || rest[0] === "_") &&
      rest.length > 1 &&
      rest[1] !== rest[0] &&
      !isSpace(rest[1])
    ) {
      const end = closeEm(rest);
      if (end > 0) {
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
    if (
      (i === 0 || isSpace(src[i - 1]) || src[i - 1] === "(") &&
      (m = AUTO.exec(rest))
    ) {
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
    const item = /^[ \t]*(?:([-*•])|(\d{1,3})[.)])[ \t]+(.*)$/.exec(line);
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
