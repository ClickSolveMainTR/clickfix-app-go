export type ProfileName = { name?: string | null; surname?: string | null };

/** Compare the payload we actually send, so whitespace alone cannot enable Save. */
export function profileEditState(original: ProfileName | undefined, draft: ProfileName) {
  const payload = { name: draft.name?.trim() ?? '', surname: draft.surname?.trim() ?? '' };
  const dirty = payload.name !== (original?.name?.trim() ?? '') || payload.surname !== (original?.surname?.trim() ?? '');
  return { payload, dirty, valid: payload.name.length > 0 };
}
