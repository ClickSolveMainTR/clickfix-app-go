/** Formats Turkish numbers without changing the value used for calls/copy. */
export function formatPhone(value: string): string {
  const digits = value.replace(/\D/g, '');
  const national = digits.length === 10
    ? digits
    : digits.length === 11 && digits.startsWith('0')
      ? digits.slice(1)
      : digits.length === 12 && digits.startsWith('90')
        ? digits.slice(2)
        : null;
  return national ? `+90 ${national.slice(0, 3)} ${national.slice(3, 6)} ${national.slice(6, 8)} ${national.slice(8)}` : value;
}

export function identityLabel(identity: { display_name: string; username: string; channel_type: string }, fallback: string): string {
  const name = identity.display_name?.trim();
  const username = identity.username?.trim().replace(/^@+/, '');
  if (identity.channel_type === 'whatsapp' || identity.channel_type === 'sms') {
    return [name, username && formatPhone(username)].filter(Boolean).join(' · ') || fallback;
  }
  return [name, username && `@${username}`].filter(Boolean).join(' · ') || fallback;
}
