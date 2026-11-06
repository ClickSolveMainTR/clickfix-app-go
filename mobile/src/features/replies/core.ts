/**
 * Canned response form rules, the same ones the API enforces
 * (`backend/internal/modules/macros/usecase/service.go`, `normalize`) and the
 * panel's form applies (`frontend/src/features/macros/components/replies-page.tsx`).
 */

export type CannedForm = {
  title: string;
  shortcut: string;
  category: string;
  content: string;
  scope: 'personal' | 'organization';
};

export const EMPTY_FORM: CannedForm = { title: '', shortcut: '', category: '', content: '', scope: 'personal' };

/** Variables the API fills in when a canned response is rendered for a conversation. */
export const CANNED_VARIABLES = [
  '{{contact.first_name}}',
  '{{contact.name}}',
  '{{contact.email}}',
  '{{company.name}}',
  '{{agent.first_name}}',
  '{{agent.name}}',
  '{{conversation.number}}',
] as const;

const SHORTCUT = /^[a-z0-9][a-z0-9_-]{0,39}$/;
const length = (s: string) => [...s].length;

/** What the member types, reduced to a valid shortcut: lowercase letters, digits, `-` and `_`. */
export function cleanShortcut(value: string) {
  return value
    .toLowerCase()
    .replace(/[^a-z0-9_-]/g, '')
    .replace(/^[_-]+/, '')
    .slice(0, 40);
}

export function formOf(item: {
  title: string;
  shortcut: string;
  category: string;
  content: string;
  scope: string;
}): CannedForm {
  return {
    title: item.title,
    shortcut: item.shortcut,
    category: item.category,
    content: item.content,
    scope: item.scope === 'organization' ? 'organization' : 'personal',
  };
}

export function sameForm(a: CannedForm, b: CannedForm) {
  return (
    a.title === b.title &&
    a.shortcut === b.shortcut &&
    a.category === b.category &&
    a.content === b.content &&
    a.scope === b.scope
  );
}

/** True when the API would accept the form. */
export function validForm(form: CannedForm) {
  const title = form.title.trim();
  const content = form.content.trim();
  return (
    length(title) >= 1 &&
    length(title) <= 120 &&
    SHORTCUT.test(form.shortcut) &&
    length(content) >= 1 &&
    length(content) <= 10000 &&
    length(form.category.trim()) <= 60
  );
}

/** Personal ones belong to their owner; shared ones need `tenant.macros.write`. */
export function canEdit(item: { mine: boolean }, canWrite: boolean) {
  return item.mine || canWrite;
}
