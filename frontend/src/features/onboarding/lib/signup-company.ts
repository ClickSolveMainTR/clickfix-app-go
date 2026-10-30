/**
 * Company name typed on `/register` (CF-143), kept in this browser until the
 * first organization is opened. When the sign-up is interrupted (the
 * visitor verifies the e-mail later, or signs in on another visit) the
 * organization form on `/setup` starts with it.
 */
const STORAGE_KEY = "clickfix.signup.company";

export function readSignupCompany(): string {
  try {
    return window.localStorage.getItem(STORAGE_KEY) ?? "";
  } catch {
    return "";
  }
}

export function writeSignupCompany(name: string | null) {
  try {
    if (name?.trim()) window.localStorage.setItem(STORAGE_KEY, name.trim());
    else window.localStorage.removeItem(STORAGE_KEY);
  } catch {
    // Private mode / blocked storage: the form simply starts empty.
  }
}
