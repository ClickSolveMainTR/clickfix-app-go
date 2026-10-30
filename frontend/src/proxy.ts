import {
  NextResponse,
  type NextRequest,
  type NextFetchEvent,
} from "next/server";

import { auth } from "@/auth";

/** `auth` is lazily configured (`NextAuth(async () => ...)`), so wrapping a
 * callback returns a Promise instead of a handler. Call it as inline
 * middleware: it runs the `authorized` callback and its redirects. */
const protectedProxy = auth as unknown as (
  request: NextRequest,
  event: NextFetchEvent,
) => Promise<Response>;

/** Locale queries select a statically rendered English marketing page.
 * Public requests never invoke the auth adapter or dispatch to the panel. */
export default function proxy(request: NextRequest, event: NextFetchEvent) {
  const { pathname, searchParams } = request.nextUrl;
  const publicPage =
    /^\/(?:docs(?:\/.*)?|features(?:\/[^/]+)?|compare|security|contact|account-deletion|legal\/[^/]+)?$/.test(
      pathname,
    );
  if (publicPage) {
    if (pathname === "/docs/search" || pathname.startsWith("/docs/og/"))
      return NextResponse.next();
    if (searchParams.get("lang") === "en") {
      const url = request.nextUrl.clone();
      url.pathname = `/en${pathname === "/" ? "" : pathname}`;
      return NextResponse.rewrite(url);
    }
    return NextResponse.next();
  }
  return protectedProxy(request, event);
}

export const config = {
  matcher: [
    "/",
    "/docs/:path*",
    "/features/:path*",
    "/compare",
    "/security",
    "/contact",
    "/account-deletion",
    "/legal/:path*",
    "/dashboard/:path*",
    "/profile/:path*",
    "/platform/:path*",
    "/settings/:path*",
    "/objects/:path*",
    "/billing",
    "/teams/:path*",
    "/video/:path*",
  ],
};
