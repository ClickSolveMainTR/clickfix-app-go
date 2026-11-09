/** Value at a dotted path ("customer.name", "lines.0.amount"). */
export function getPath(value: unknown, path: string): unknown {
  if (!path) return value;
  let cur: unknown = value;
  for (const part of path.split(".")) {
    if (Array.isArray(cur)) {
      const i = Number(part);
      if (!Number.isInteger(i)) return undefined;
      cur = cur[i];
    } else if (cur && typeof cur === "object") {
      cur = (cur as Record<string, unknown>)[part];
    } else {
      return undefined;
    }
  }
  return cur;
}

/** Records under the items path: an array's elements or one object. */
export function itemsAt(body: unknown, itemsPath: string): unknown[] {
  const v = getPath(body, itemsPath);
  if (Array.isArray(v)) return v.slice(0, 20);
  if (v && typeof v === "object") return [v];
  return [];
}

export function display(value: unknown): string {
  if (value === null || value === undefined) return "—";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}
