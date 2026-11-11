import { llmsIndex } from "@/lib/docs/llms";

export const dynamic = "force-static";

/** `/llms.txt`: the Turkish docs index for LLMs (CF-208); English is `/en/llms.txt`. */
export function GET() {
  return new Response(llmsIndex("tr"), {
    headers: { "content-type": "text/markdown; charset=utf-8" },
  });
}
