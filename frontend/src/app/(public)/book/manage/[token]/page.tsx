"use client";

import { use } from "react";

import { BookingManagePage } from "@/features/booking";

export default function Page({
  params,
}: {
  params: Promise<{ token: string }>;
}) {
  const { token } = use(params);
  return <BookingManagePage token={token} />;
}
