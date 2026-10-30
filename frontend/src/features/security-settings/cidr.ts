/** IPv4 (4 bytes) or IPv6 (16 bytes) address, or null when invalid. */
function parseIP(raw: string): number[] | null {
  const ip = raw.trim();
  if (/^\d{1,3}(\.\d{1,3}){3}$/.test(ip)) {
    const bytes = ip.split(".").map(Number);
    return bytes.some((b) => b > 255) ? null : bytes;
  }
  if (!ip.includes(":") || !/^[0-9a-fA-F:.]+$/.test(ip)) return null;
  let head = ip;
  let tail: number[] = [];
  // Embedded IPv4 suffix (::ffff:192.0.2.1).
  const v4 = ip.match(/:(\d{1,3}(\.\d{1,3}){3})$/);
  if (v4) {
    const bytes = parseIP(v4[1]!);
    if (!bytes) return null;
    tail = bytes;
    head = ip.slice(0, -v4[1]!.length) + "0:0";
  }
  const halves = head.split("::");
  if (halves.length > 2) return null;
  const left = halves[0] ? halves[0].split(":") : [];
  const right = halves.length === 2 && halves[1] ? halves[1].split(":") : [];
  if ([...left, ...right].some((g) => !/^[0-9a-fA-F]{1,4}$/.test(g))) {
    return null;
  }
  const missing = 8 - left.length - right.length;
  if (halves.length === 1 ? missing !== 0 : missing < 1) return null;
  const groups = [
    ...left,
    ...Array<string>(halves.length === 2 ? missing : 0).fill("0"),
    ...right,
  ].map((g) => parseInt(g, 16));
  const bytes = groups.flatMap((g) => [g >> 8, g & 0xff]);
  if (tail.length) bytes.splice(12, 4, ...tail);
  return bytes;
}

type Prefix = { bytes: number[]; length: number };

/**
 * Parses `a.b.c.d/n` or `x::y/n` like Go's netip.ParsePrefix. A bare
 * address is not a prefix: callers normalize it with `normalizeCIDR`.
 */
export function parseCIDR(raw: string): Prefix | null {
  const [addr, len, extra] = raw.trim().split("/");
  if (extra !== undefined || !addr || !len || !/^\d{1,3}$/.test(len)) {
    return null;
  }
  const bytes = parseIP(addr);
  const length = Number(len);
  if (!bytes || length > bytes.length * 8) return null;
  return { bytes, length };
}

/** Accepts a CIDR or a single address (→ /32 or /128); null when invalid. */
export function normalizeCIDR(raw: string): string | null {
  const v = raw.trim();
  if (v.includes("/")) return parseCIDR(v) ? v : null;
  const bytes = parseIP(v);
  return bytes ? `${v}/${bytes.length * 8}` : null;
}

const MAPPED_V4 = [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff];

export function cidrContains(cidr: string, address: string): boolean {
  const p = parseCIDR(cidr);
  let ip = parseIP(address);
  if (!p || !ip) return false;
  // Go unmaps ::ffff:a.b.c.d before matching IPv4 prefixes.
  if (ip.length === 16 && MAPPED_V4.every((b, i) => ip![i] === b)) {
    ip = ip.slice(12);
  }
  if (p.bytes.length !== ip.length) return false;
  for (let bit = 0; bit < p.length; bit++) {
    const mask = 0x80 >> (bit % 8);
    const i = Math.floor(bit / 8);
    if ((p.bytes[i]! & mask) !== (ip[i]! & mask)) return false;
  }
  return true;
}

export function allowedBy(list: string[], address: string): boolean {
  return list.length === 0 || list.some((c) => cidrContains(c, address));
}
