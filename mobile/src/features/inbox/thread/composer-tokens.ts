export type Trigger = { kind: '@' | '/'; query: string; start: number; end: number };
export type Mention = { uuid: string; name: string };

/** Only a token at the caret, starting at a word boundary, can open suggestions. */
export function parseTrigger(text: string, caret = text.length): Trigger | null {
  const end = Math.max(0, Math.min(caret, text.length));
  const match = /(?:^|\s)([@/])([^\s@/]*)$/.exec(text.slice(0, end));
  if (!match) return null;
  let tokenEnd = end;
  while (tokenEnd < text.length && !/[\s@/]/.test(text[tokenEnd])) tokenEnd++;
  return { kind: match[1] as '@' | '/', query: match[2], start: end - match[2].length - 1, end: tokenEnd };
}

export function insertToken(text: string, trigger: Trigger, replacement: string) {
  const suffix = text.slice(trigger.end);
  const inserted = replacement ? replacement + (suffix.startsWith(' ') ? '' : ' ') : '';
  return { text: text.slice(0, trigger.start) + inserted + suffix, caret: trigger.start + inserted.length };
}

/** API contract: readable @name in content, distinct UUIDs in mentions. */
export function serializeMentions(text: string, mentions: Mention[]): string[] {
  return [
    ...new Set(
      mentions
        .filter((m) => {
          const token = '@' + m.name;
          let at = text.indexOf(token);
          while (at >= 0) {
            const end = at + token.length;
            if ((at === 0 || /\s/.test(text[at - 1])) && (end === text.length || /[\s.,!?;:]/.test(text[end])))
              return true;
            at = text.indexOf(token, at + 1);
          }
          return false;
        })
        .map((m) => m.uuid),
    ),
  ];
}

/** Highlight only complete selected tokens, sharing the API serializer's boundaries. */
export function mentionParts(text: string, mentions: Mention[]): { text: string; mention: boolean }[] {
  const tokens = [...new Set(mentions.map((m) => '@' + m.name))].sort((a, b) => b.length - a.length);
  const parts: { text: string; mention: boolean }[] = [];
  let plain = '';
  let at = 0;
  while (at < text.length) {
    const token = tokens.find(
      (token) =>
        text.startsWith(token, at) &&
        (at === 0 || /\s/.test(text[at - 1])) &&
        (at + token.length === text.length || /[\s.,!?;:]/.test(text[at + token.length])),
    );
    if (token) {
      if (plain) parts.push({ text: plain, mention: false });
      plain = '';
      parts.push({ text: token, mention: true });
      at += token.length;
    } else plain += text[at++];
  }
  if (plain) parts.push({ text: plain, mention: false });
  return parts;
}
