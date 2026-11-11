import { openapi } from "@/lib/docs/openapi";

/**
 * Same-origin proxy of the API reference playground (CF-208). The browser
 * never calls the API host directly; this route forwards only to
 * api.getclickfix.com, with the headers the reader entered (their API key),
 * and never with this site's cookies.
 */
const proxy = openapi.createProxy({
  allowedOrigins: ["https://api.getclickfix.com"],
});

export const { GET, POST, PUT, PATCH, DELETE, HEAD } = proxy;
