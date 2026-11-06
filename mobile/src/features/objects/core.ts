/**
 * Custom object records, read-only (CF-172): how stored field values are
 * shown, with the panel's rules (`frontend/src/features/custom-fields/
 * components/custom-field-value.tsx`, `lib/values.ts`, `lib/conditions.ts`).
 * Free of app imports so it runs under `node --test`.
 */

type Option = { value: string; label: string; archived?: boolean };
export type Definition = {
  key: string;
  label: string;
  type: string;
  position: number;
  archived: boolean;
  options?: Option[] | null;
  config?: { precision?: number } | null;
};

/** Fields of a type as the panel lists them: archived ones out, by position. */
export function activeDefinitions<T extends Definition>(definitions: T[]): T[] {
  return definitions.filter((d) => !d.archived).sort((a, b) => a.position - b.position);
}

/** Nothing stored: `null`, `undefined`, an empty string or an empty list. */
export function isEmptyValue(value: unknown): boolean {
  if (value === null || value === undefined) return true;
  if (typeof value === 'string') return value.trim() === '';
  if (Array.isArray(value)) return value.length === 0;
  return false;
}

export function isMoneyValue(value: unknown): value is { amount_minor: number; currency: string } {
  if (!value || typeof value !== 'object') return false;
  const v = value as { amount_minor?: unknown; currency?: unknown };
  return typeof v.amount_minor === 'number' && Number.isFinite(v.amount_minor) && typeof v.currency === 'string';
}

/** Any stored value as plain text; an object is listed as `key: value`, never shown as `[object Object]`. */
export function plainText(value: unknown): string {
  if (isEmptyValue(value)) return '';
  if (Array.isArray(value)) return value.map(plainText).filter(Boolean).join(', ');
  if (typeof value === 'object')
    return Object.entries(value as Record<string, unknown>)
      .map(([k, v]) => `${k}: ${plainText(v)}`)
      .join(', ');
  return String(value);
}

export type OptionView = { label: string; archived: boolean };

/** An option no longer in the definition is shown by its stored value, marked archived. */
function optionOf(options: Option[] | null | undefined, value: string): OptionView {
  const found = (options ?? []).find((o) => o.value === value);
  return found ? { label: found.label, archived: !!found.archived } : { label: value, archived: true };
}

/** `https:` and `http:` links only; anything else (`javascript:`, `file:`…) is shown as text, never opened. */
export function safeUrl(value: string): string | null {
  const text = value.trim();
  return /^https?:\/\/[^\s]+$/i.test(text) ? text : null;
}

export type ValueView =
  | { kind: 'empty' }
  | { kind: 'text'; text: string; multiline?: boolean }
  | { kind: 'options'; options: OptionView[] }
  | { kind: 'boolean'; value: boolean }
  | { kind: 'money'; amount: number; currency: string }
  | { kind: 'number'; value: number; precision: number }
  | { kind: 'date'; iso: string }
  | { kind: 'link'; text: string; href: string }
  | { kind: 'user'; uuid: string };

/**
 * What to show for a stored value of a field. A field type the app does not
 * know, or a value that does not fit its type, falls back to plain text.
 */
export function valueView(definition: Pick<Definition, 'type' | 'options' | 'config'>, value: unknown): ValueView {
  if (isEmptyValue(value)) return { kind: 'empty' };
  const text = plainText(value);
  switch (definition.type) {
    case 'select':
      return { kind: 'options', options: [optionOf(definition.options, String(value))] };
    case 'multiselect':
      return {
        kind: 'options',
        options: (Array.isArray(value) ? value : [value]).map((v) => optionOf(definition.options, String(v))),
      };
    case 'boolean':
      return typeof value === 'boolean' ? { kind: 'boolean', value } : { kind: 'text', text };
    case 'money':
      return isMoneyValue(value)
        ? { kind: 'money', amount: value.amount_minor / 100, currency: value.currency }
        : { kind: 'text', text };
    case 'number': {
      const n = typeof value === 'number' ? value : Number(value);
      return Number.isFinite(n)
        ? { kind: 'number', value: n, precision: definition.config?.precision ?? 6 }
        : { kind: 'text', text };
    }
    case 'date':
      return Number.isNaN(new Date(String(value)).getTime())
        ? { kind: 'text', text }
        : { kind: 'date', iso: String(value) };
    case 'url': {
      const href = safeUrl(String(value));
      return href ? { kind: 'link', text: href.replace(/^https?:\/\//i, ''), href } : { kind: 'text', text };
    }
    case 'email':
      return /^[^\s@]+@[^\s@]+$/.test(text) ? { kind: 'link', text, href: `mailto:${text}` } : { kind: 'text', text };
    case 'phone':
      return /^[+\d][\d\s().-]{2,}$/.test(text)
        ? { kind: 'link', text, href: `tel:${text.replace(/[^\d+]/g, '')}` }
        : { kind: 'text', text };
    case 'user':
      return { kind: 'user', uuid: String(value) };
    case 'textarea':
      return { kind: 'text', text, multiline: true };
    default:
      return { kind: 'text', text };
  }
}
