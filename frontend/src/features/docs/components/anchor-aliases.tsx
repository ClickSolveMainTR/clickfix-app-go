"use client";

import { usePathname, useRouter } from "next/navigation";
import { useEffect } from "react";

import aliases from "../heading-aliases.json";

/**
 * Old anchors keep working: a value starting with `#`-less text is the
 * current heading id on the same page; a value starting with `/` is a page
 * that took the section over (the guide was split into step pages, CF-208)
 * and the reader is sent there, keeping `?lang=en`.
 */
export function DocsAnchorAliases() {
  const pathname = usePathname();
  const router = useRouter();
  useEffect(() => {
    const last = pathname.split("/").filter(Boolean).at(-1);
    const page = !last || last === "docs" ? "index" : last;
    const map = (aliases as Record<string, Record<string, string>>)[page];
    if (!map) return;
    const resolve = () => {
      const target = map[window.location.hash.slice(1)];
      if (!target) return;
      if (target.startsWith("/")) {
        const [path, hash] = target.split("#", 2);
        router.replace(
          `${path}${window.location.search}${hash ? `#${hash}` : ""}`,
        );
        return;
      }
      window.history.replaceState(
        null,
        "",
        `${window.location.pathname}${window.location.search}#${target}`,
      );
      document.getElementById(target)?.scrollIntoView();
    };
    resolve();
    window.addEventListener("hashchange", resolve);
    return () => window.removeEventListener("hashchange", resolve);
  }, [pathname, router]);
  return null;
}
