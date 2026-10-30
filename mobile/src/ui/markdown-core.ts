/**
 * A small Markdown reader for knowledge articles (the panel's editor stores
 * Markdown): headings, paragraphs, bullet / numbered lists, quotes, code
 * blocks, and inline bold, italic, code and links. Pure, so it runs under
 * `node --test`; `markdown.tsx` draws the result.
 */

export type Inline =
  | { kind: 'text'; text: string }
  | { kind: 'bold'; text: string }
  | { kind: 'italic'; text: string }
  | { kind: 'code'; text: string }
  | { kind: 'link'; text: string; href: string };

export type Block =
  | { kind: 'heading'; level: 1 | 2 | 3; inline: Inline[] }
  | { kind: 'paragraph'; inline: Inline[] }
  | { kind: 'list'; ordered: boolean; items: Inline[][] }
  | { kind: 'quote'; inline: Inline[] }
  | { kind: 'code'; text: string }
  | { kind: 'table'; header: Inline[][]; rows: Inline[][][] }
  | { kind: 'rule' };

const INLINE = /(\*\*[^*]+\*\*|__[^_]+__|`[^`]+`|\[[^\]]+\]\([^)\s]+\)|\*[^*\s][^*]*\*|_[^_\s][^_]*_)/g;

export function parseInline(src: string): Inline[] {
  const out: Inline[] = [];
  let at = 0;
  for (const m of src.matchAll(INLINE)) {
    const i = m.index ?? 0;
    if (i > at) out.push({ kind: 'text', text: src.slice(at, i) });
    const tok = m[0];
    if (tok.startsWith('**') || tok.startsWith('__')) out.push({ kind: 'bold', text: tok.slice(2, -2) });
    else if (tok.startsWith('`')) out.push({ kind: 'code', text: tok.slice(1, -1) });
    else if (tok.startsWith('[')) {
      const close = tok.indexOf('](');
      out.push({ kind: 'link', text: tok.slice(1, close), href: tok.slice(close + 2, -1) });
    } else out.push({ kind: 'italic', text: tok.slice(1, -1) });
    at = i + tok.length;
  }
  if (at < src.length) out.push({ kind: 'text', text: src.slice(at) });
  return out;
}

const BULLET = /^\s*[-*+]\s+(.*)$/;
const NUMBER = /^\s*\d+[.)]\s+(.*)$/;

export function parseMarkdown(md: string): Block[] {
  const lines = md.replace(/\r\n?/g, '\n').split('\n');
  const blocks: Block[] = [];
  let para: string[] = [];
  const flush = () => {
    if (para.length) blocks.push({ kind: 'paragraph', inline: parseInline(para.join(' ').trim()) });
    para = [];
  };
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (line.trim().startsWith('```')) {
      flush();
      const code: string[] = [];
      for (i++; i < lines.length && !lines[i].trim().startsWith('```'); i++) code.push(lines[i]);
      blocks.push({ kind: 'code', text: code.join('\n') });
      continue;
    }
    if (line.includes('|') && i + 1 < lines.length && isTableDivider(lines[i + 1])) {
      flush();
      const header = tableCells(line).map(parseInline);
      const rows: Inline[][][] = [];
      i++;
      while (i + 1 < lines.length && lines[i + 1].trim() && lines[i + 1].includes('|')) {
        const cells = tableCells(lines[++i]);
        rows.push(header.map((_, column) => parseInline(cells[column] ?? '')));
      }
      blocks.push({ kind: 'table', header, rows });
      continue;
    }
    if (!line.trim()) {
      flush();
      continue;
    }
    const h = /^(#{1,6})\s+(.*)$/.exec(line);
    if (h) {
      flush();
      blocks.push({ kind: 'heading', level: Math.min(h[1].length, 3) as 1 | 2 | 3, inline: parseInline(h[2].trim()) });
      continue;
    }
    if (/^\s*([-*_])\s*\1\s*\1[\s\-*_]*$/.test(line)) {
      flush();
      blocks.push({ kind: 'rule' });
      continue;
    }
    if (line.startsWith('>')) {
      flush();
      blocks.push({ kind: 'quote', inline: parseInline(line.replace(/^>\s?/, '')) });
      continue;
    }
    const b = BULLET.exec(line) ?? NUMBER.exec(line);
    if (b) {
      flush();
      const ordered = !BULLET.test(line);
      const prev = blocks[blocks.length - 1];
      const item = parseInline(b[1]);
      if (prev?.kind === 'list' && prev.ordered === ordered) prev.items.push(item);
      else blocks.push({ kind: 'list', ordered, items: [item] });
      continue;
    }
    para.push(line.trim());
  }
  flush();
  return blocks;
}

/** Plain text of Markdown (previews, share text). */
export function markdownToText(md: string): string {
  return parseMarkdown(md)
    .map((b) =>
      b.kind === 'code'
        ? b.text
        : b.kind === 'rule'
          ? ''
          : b.kind === 'table'
            ? [b.header, ...b.rows]
                .map((row) => row.map((cell) => cell.map((x) => x.text).join('')).join(' | '))
                .join('\n')
            : b.kind === 'list'
              ? b.items.map((i) => `• ${i.map((x) => x.text).join('')}`).join('\n')
              : b.inline.map((x) => x.text).join(''),
    )
    .filter(Boolean)
    .join('\n\n');
}

function tableCells(line: string): string[] {
  return line
    .trim()
    .replace(/^\|/, '')
    .replace(/(?<!\\)\|$/, '')
    .split(/(?<!\\)\|/)
    .map((cell) => cell.trim().replace(/\\\|/g, '|'));
}
function isTableDivider(line: string): boolean {
  const cells = tableCells(line);
  return line.includes('|') && cells.length > 0 && cells.every((cell) => /^:?-{3,}:?$/.test(cell));
}
