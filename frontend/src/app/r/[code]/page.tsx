import type { Metadata } from "next";

import { GuestJoinPage } from "@/features/video/components/guest-join-page";

type Props = { params: Promise<{ code: string }> };

export const metadata: Metadata = {
  title: "Video",
  robots: { index: false, follow: false },
  referrer: "no-referrer",
};

/** Public video room join page (CF-139): no login, signed link code. */
export default async function VideoJoinRoute({ params }: Props) {
  const { code } = await params;
  return <GuestJoinPage code={code} />;
}
