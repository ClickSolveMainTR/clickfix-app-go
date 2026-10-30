// A tiny site that embeds the ClickFix widget (no dependencies).
// GET /            the demo page (widget id and app origin filled in)
// POST /user-hash  { user_id } -> { user_hash }: hex(HMAC-SHA256(secret, user_id)).
//                  Real sites sign the id of the signed-in user, never one sent by the browser.
import { createHmac } from "node:crypto";
import { readFile } from "node:fs/promises";
import { createServer } from "node:http";

const APP = (process.env.CLICKFIX_APP ?? "http://127.0.0.1:3000").replace(/\/$/, "");
const WIDGET = process.env.CLICKFIX_WIDGET_ID ?? "";
const SECRET = process.env.CLICKFIX_IDENTITY_SECRET ?? "";
const PORT = Number(process.env.PORT ?? 8788);

export function userHash(secret, userId) {
  return createHmac("sha256", secret).update(String(userId)).digest("hex");
}

const escape = (s) => s.replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`);

async function page() {
  const html = await readFile(new URL("./public/index.html", import.meta.url), "utf8");
  return html
    .replaceAll("{{APP}}", escape(APP))
    .replaceAll("{{WIDGET}}", escape(WIDGET))
    .replaceAll("{{IDENTITY}}", SECRET ? "true" : "false");
}

export const server = createServer(async (req, res) => {
  try {
    if (req.method === "GET" && (req.url === "/" || req.url?.startsWith("/app/"))) {
      res.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
      res.end(await page());
      return;
    }
    if (req.method === "POST" && req.url === "/user-hash") {
      if (!SECRET) {
        res.writeHead(404).end();
        return;
      }
      let body = "";
      for await (const chunk of req) body += chunk;
      const { user_id: userId } = JSON.parse(body || "{}");
      if (typeof userId !== "string" || !userId || userId.length > 200) {
        res.writeHead(400).end();
        return;
      }
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ user_hash: userHash(SECRET, userId) }));
      return;
    }
    res.writeHead(404).end();
  } catch {
    res.writeHead(500).end();
  }
});

if (import.meta.url === `file://${process.argv[1]}`) {
  if (!WIDGET) console.warn("CLICKFIX_WIDGET_ID is not set");
  server.listen(PORT, () => console.log(`http://localhost:${PORT}`));
}
