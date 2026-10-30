"use client";

import { usePathname } from "next/navigation";
import { useEffect } from "react";

import aliases from "../heading-aliases.json";

export function DocsAnchorAliases() {
  const pathname = usePathname();
  useEffect(() => {
    const last = pathname.split("/").filter(Boolean).at(-1);
    const page = !last || last === "docs" ? "index" : last;
    const map = (aliases as Record<string, Record<string, string>>)[page];
    if (!map) return;
    const resolve = () => {
      const target = map[window.location.hash.slice(1)];
      if (!target) return;
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
  }, [pathname]);
  return null;
}
