import { ogImage, OG_SIZE } from "@/features/landing/lib/og";
import { translate } from "@/lib/i18n/messages";

export const size = OG_SIZE;
export const contentType = "image/png";
export const alt = "ClickFix";

export default function OpenGraphImage() {
  return ogImage({
    eyebrow: translate("tr", "landing.compare.eyebrow"),
    title: translate("tr", "landing.compare.title"),
  });
}
