import { isBrandSlug, type BrandSlug } from "@/components/brand-logos";

import type {
  SidebarAppInput,
  SidebarAppPlacement,
  SidebarAppScope,
} from "../services/sidebar-apps.service";

/**
 * One-click app templates. Every entry was checked against its vendor's
 * documentation to be embeddable in a plain iframe (no SDK, no popups) and
 * usable for free; the URL keeps `{placeholders}` the installer fills in.
 * Text lives under `apps.gallery.<id>.*`.
 */
export type GalleryApp = {
  id: string;
  icon: string;
  placement: SidebarAppPlacement;
  scopes: SidebarAppScope[];
  url: string;
  /** Where the installer learns how to get the URL (vendor docs). */
  docsUrl: string;
};

export const GALLERY: GalleryApp[] = [
  {
    // examples/apps/order-lookup (MIT); its full source is published on
    // the docs page below, so the guide never points at the repository.
    id: "order_lookup",
    icon: "package",
    placement: "conversation_sidebar",
    scopes: ["conversation.read", "conversation.note", "conversation.reply"],
    url: "https://{uygulamanizin-adresi}/",
    docsUrl: "/docs/order-lookup-sample",
  },
  {
    // Calendly documents a plain iframe of any scheduling link.
    id: "calendly",
    icon: "calendar",
    placement: "conversation_sidebar",
    scopes: ["conversation.read"],
    url: "https://calendly.com/{kullanici-adi}/{etkinlik}",
    docsUrl:
      "https://help.calendly.com/hc/en-us/articles/31645309248791-How-to-embed-Calendly-with-an-iframe",
  },
  {
    // Uptime Kuma (MIT) status pages; framing needs
    // UPTIME_KUMA_DISABLE_FRAME_SAMEORIGIN=true on the instance.
    id: "uptime_kuma",
    icon: "activity",
    placement: "conversation_sidebar",
    scopes: ["conversation.read"],
    url: "https://{durum-sayfaniz}/status/{sayfa}",
    docsUrl: "https://github.com/louislam/uptime-kuma/wiki/Status-Page",
  },
  {
    // Google Docs "Publish to the web" link with embedded=true.
    id: "google_docs",
    icon: "book",
    placement: "nav",
    scopes: [],
    url: "https://docs.google.com/document/d/e/{yayin-kimligi}/pub?embedded=true",
    docsUrl: "https://support.google.com/docs/answer/183965",
  },
  {
    // Metabase public dashboards are part of the open-source edition.
    id: "metabase",
    icon: "chart",
    placement: "nav",
    scopes: [],
    url: "https://{metabase-adresiniz}/public/dashboard/{pano-kimligi}",
    docsUrl: "https://www.metabase.com/docs/latest/embedding/public-links",
  },
  {
    // openstreetmap.org export embed (ODbL, attribution included).
    id: "openstreetmap",
    icon: "map",
    placement: "nav",
    scopes: [],
    url: "https://www.openstreetmap.org/export/embed.html?bbox=28.95,41.00,29.05,41.06&layer=mapnik",
    docsUrl: "https://wiki.openstreetmap.org/wiki/Export",
  },
];

/** True while the URL still holds a `{placeholder}` from a gallery template. */
export function hasPlaceholder(url: string): boolean {
  return /[{}]|%7B|%7D/i.test(url);
}

/** Form values a gallery template installs with. */
export function galleryInput(
  app: GalleryApp,
  name: string,
): Omit<SidebarAppInput, "revision"> {
  return {
    name,
    url: app.url,
    icon: app.icon,
    placement: app.placement,
    scopes: app.scopes,
    enabled: true,
  };
}

/** Hosts of the gallery vendors, so installed apps keep the vendor logo. */
const BRAND_HOSTS: [host: string, slug: BrandSlug][] = [
  ["calendly.com", "calendly"],
  ["docs.google.com", "google_docs"],
  ["openstreetmap.org", "openstreetmap"],
];

/** Logo of a gallery entry, when the vendor is an external company. */
export function galleryBrand(app: GalleryApp): BrandSlug | null {
  return isBrandSlug(app.id) ? app.id : null;
}

/** Vendor logo of an installed app, matched by its URL's host. */
export function appBrand(url: string): BrandSlug | null {
  let host: string;
  try {
    host = new URL(url).hostname;
  } catch {
    return null;
  }
  const hit = BRAND_HOSTS.find(([h]) => host === h || host.endsWith(`.${h}`));
  return hit ? hit[1] : null;
}
