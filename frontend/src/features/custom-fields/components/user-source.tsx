"use client";

import { createContext, useCallback, useContext, type ReactNode } from "react";

import type { ComboboxOption } from "@/components/ui/async-combobox";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

/**
 * Where `user` fields find organization members. There is no tenant members
 * endpoint yet (CF-5), so the default source offers the signed-in user and
 * accepts a pasted user id. A screen that already loads members wraps its
 * forms in `CustomFieldUserSourceProvider` to offer them instead.
 */
export type CustomFieldUserSource = {
  loadOptions: (query: string) => Promise<ComboboxOption[]>;
  /** Display label of a stored user uuid; `undefined` when unknown. */
  resolveLabel: (uuid: string) => string | undefined;
};

const UserSourceContext = createContext<CustomFieldUserSource | null>(null);

export function CustomFieldUserSourceProvider({
  value,
  children,
}: {
  value: CustomFieldUserSource;
  children: ReactNode;
}) {
  return (
    <UserSourceContext.Provider value={value}>
      {children}
    </UserSourceContext.Provider>
  );
}

const UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function useDefaultUserSource(): CustomFieldUserSource {
  const { user } = useAuth();
  const { t } = useLocale();
  const selfLabel = user
    ? t("custom_fields.user.self", { name: user.fullName || user.email })
    : undefined;

  const loadOptions = useCallback(
    async (query: string): Promise<ComboboxOption[]> => {
      const q = query.trim();
      const out: ComboboxOption[] = [];
      if (
        user &&
        selfLabel &&
        (!q ||
          selfLabel.toLowerCase().includes(q.toLowerCase()) ||
          user.email.toLowerCase().includes(q.toLowerCase()))
      ) {
        out.push({
          value: user.uuid,
          label: selfLabel,
          description: user.email,
        });
      }
      if (UUID_RE.test(q) && q.toLowerCase() !== user?.uuid) {
        out.push({
          value: q.toLowerCase(),
          label: q.toLowerCase(),
          description: t("custom_fields.user.by_id"),
        });
      }
      return out;
    },
    [selfLabel, t, user],
  );

  const resolveLabel = useCallback(
    (uuid: string) => (user && uuid === user.uuid ? selfLabel : undefined),
    [selfLabel, user],
  );

  return { loadOptions, resolveLabel };
}

export function useCustomFieldUserSource(): CustomFieldUserSource {
  const provided = useContext(UserSourceContext);
  const fallback = useDefaultUserSource();
  return provided ?? fallback;
}
