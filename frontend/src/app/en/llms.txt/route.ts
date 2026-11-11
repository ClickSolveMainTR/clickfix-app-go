import { llmsIndex } from "@/lib/docs/llms";

export const dynamic = "force-static";

/** `/en/llms.txt`: the English docs index for LLMs (CF-208). */
export function GET() {
  return new Response(llmsIndex("en"), {
    headers: { "content-type": "text/markdown; charset=utf-8" },
  });
}
