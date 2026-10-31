import type { CreateOrganizationResult } from "@/features/platform-organizations/types";

export type DemoNotice = NonNullable<CreateOrganizationResult["demo"]>;

/**
 * What the create wizard could not finish after the organization itself was
 * created: profile fields the API rejected (`profile_errors`) and opening
 * credits that were not posted, plus the demo logins when demo data was
 * requested (shown once). Kept for the detail page in sessionStorage (per
 * tab, cleared once read); a blocked storage only loses the banner, the
 * wizard also shows a toast.
 */
export type CreationNotice = {
  profileFields: string[];
  failedCredits: string[];
  demo?: DemoNotice | null;
};

const key = (uuid: string) => `clickfix.org-created.${uuid}`;

export function saveCreationNotice(uuid: string, notice: CreationNotice) {
  if (
    !notice.profileFields.length &&
    !notice.failedCredits.length &&
    !notice.demo
  )
    return;
  try {
    window.sessionStorage.setItem(key(uuid), JSON.stringify(notice));
  } catch {
    // Storage blocked: the toast already told the operator.
  }
}

export function readCreationNotice(uuid: string): CreationNotice | null {
  try {
    const raw = window.sessionStorage.getItem(key(uuid));
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Partial<CreationNotice>;
    return {
      profileFields: Array.isArray(parsed.profileFields)
        ? parsed.profileFields.map(String)
        : [],
      failedCredits: Array.isArray(parsed.failedCredits)
        ? parsed.failedCredits.map(String)
        : [],
      demo:
        parsed.demo && Array.isArray(parsed.demo.accounts) ? parsed.demo : null,
    };
  } catch {
    return null;
  }
}

export function clearCreationNotice(uuid: string) {
  try {
    window.sessionStorage.removeItem(key(uuid));
  } catch {
    // Nothing to clear.
  }
}
