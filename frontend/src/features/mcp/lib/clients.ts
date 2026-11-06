/**
 * Copy-paste configurations for common MCP clients (CF-173), built from the
 * endpoint the API reports. None of them contains a key or a token: the
 * client registers itself and the member approves it in the browser (OAuth
 * 2.1 with PKCE), so there is nothing secret to paste.
 */

export type McpClientId =
  "claude" | "claude_code" | "cursor" | "vscode" | "other";

export type McpClientConfig = {
  id: McpClientId;
  /** Product name; not translated. */
  name: string;
  /** What to copy: a URL, a shell command or a JSON file body. */
  kind: "url" | "command" | "json";
  /** Where the JSON goes (file path), when `kind` is "json". */
  file?: string;
  value: string;
  /**
   * The flow was run against ClickFix with this client (or with the HTTP
   * steps it makes). Untested clients are listed with a note: a client whose
   * OAuth return address is a custom scheme (`cursor://`, `vscode://`) cannot
   * register, since only https and loopback http return addresses are
   * accepted.
   */
  tested: boolean;
};

/** Name the server is saved under in the client. */
export const MCP_SERVER_NAME = "clickfix";

const json = (value: unknown) => JSON.stringify(value, null, 2);

export function mcpClientConfigs(resourceUrl: string): McpClientConfig[] {
  return [
    // Claude (web and desktop): Settings → Connectors → Add custom connector.
    {
      id: "claude",
      tested: true,
      name: "Claude",
      kind: "url",
      value: resourceUrl,
    },
    {
      id: "claude_code",
      tested: true,
      name: "Claude Code",
      kind: "command",
      value: `claude mcp add --transport http ${MCP_SERVER_NAME} ${resourceUrl}`,
    },
    {
      id: "cursor",
      tested: false,
      name: "Cursor",
      kind: "json",
      file: "~/.cursor/mcp.json",
      value: json({ mcpServers: { [MCP_SERVER_NAME]: { url: resourceUrl } } }),
    },
    {
      id: "vscode",
      tested: false,
      name: "VS Code",
      kind: "json",
      file: ".vscode/mcp.json",
      value: json({
        servers: { [MCP_SERVER_NAME]: { type: "http", url: resourceUrl } },
      }),
    },
    // Clients that only start local (stdio) servers reach a remote one through mcp-remote.
    {
      id: "other",
      tested: true,
      name: "mcp-remote",
      kind: "json",
      value: json({
        mcpServers: {
          [MCP_SERVER_NAME]: {
            command: "npx",
            args: ["-y", "mcp-remote", resourceUrl],
          },
        },
      }),
    },
  ];
}

export type GuideTool = {
  name: string;
  description: string;
  write: boolean;
  available: boolean;
};

/** Tools the member gets now, read tools first; then the ones their permissions leave out. */
export function splitTools(tools: GuideTool[]) {
  const order = (a: GuideTool, b: GuideTool) =>
    Number(a.write) - Number(b.write) || a.name.localeCompare(b.name);
  return {
    available: tools.filter((t) => t.available).sort(order),
    unavailable: tools.filter((t) => !t.available).sort(order),
  };
}
