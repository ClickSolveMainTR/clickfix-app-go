import { loader } from "fumadocs-core/source";

import { openapi } from "./openapi";

import { docs } from "../../../.source/server";

export const source = loader({
  baseUrl: "/docs",
  plugins: [openapi.loaderPlugin()],
  source: docs.toFumadocsSource(),
  i18n: {
    languages: ["tr", "en"],
    defaultLanguage: "tr",
    parser: "dir",
    hideLocale: "always",
  },
  url: (slugs, locale) =>
    `/docs${slugs.length ? `/${slugs.join("/")}` : ""}${locale === "en" ? "?lang=en" : ""}`,
});
