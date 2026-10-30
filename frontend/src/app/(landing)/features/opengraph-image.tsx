import { ogImage, OG_SIZE } from "@/features/landing/lib/og";
import { translate } from "@/lib/i18n/messages";

export const size = OG_SIZE;
export const contentType = "image/png";
export const alt = "ClickFix";

/** Share card of the features index (Turkish, the primary language). */
export default function OpenGraphImage() {
  return ogImage({
    eyebrow: translate("tr", "landing.features.eyebrow"),
    title: translate("tr", "landing.features.title"),
  });
}
