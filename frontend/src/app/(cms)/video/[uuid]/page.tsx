"use client";

import { use } from "react";

import { VideoRoomPage } from "@/features/video/components/video-room-page";

export default function VideoRoomRoute({
  params,
}: {
  params: Promise<{ uuid: string }>;
}) {
  const { uuid } = use(params);
  return <VideoRoomPage uuid={uuid} />;
}
