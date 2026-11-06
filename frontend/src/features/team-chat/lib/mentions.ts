// Ported from mobile/src/features/inbox/thread/composer-tokens.ts (the parts
// the team chat composer uses), so web and mobile serialize mentions alike.

export type Mention = { uuid: string; name: string };

/** The `@query` token ending at the caret, if it starts at a word boundary. */
export function mentionQueryAt(text: string, caret: number): string | null {
  const match = /(?:^|\s)@([^\s@]{0,30})$/.exec(text.slice(0, caret));
  return match ? (match[1] ?? "") : null;
}

/** Replaces the `@query` before the caret with `@name `; returns the caret. */
export function insertMention(text: string, caret: number, name: string) {
  const before = text.slice(0, caret).replace(/@([^\s@]*)$/, `@${name} `);
  return { text: before + text.slice(caret), caret: before.length };
}

/** API contract: readable @name in the body, distinct UUIDs in mentions. */
export function serializeMentions(text: string, mentions: Mention[]): string[] {
  return [
    ...new Set(
      mentions
        .filter((m) => {
          const token = "@" + m.name;
          let at = text.indexOf(token);
          while (at >= 0) {
            const end = at + token.length;
            if (
              (at === 0 || /\s/.test(text[at - 1] ?? "")) &&
              (end === text.length || /[\s.,!?;:]/.test(text[end] ?? ""))
            )
              return true;
            at = text.indexOf(token, at + 1);
          }
          return false;
        })
        .map((m) => m.uuid),
    ),
  ];
}
