/**
 * "Connect with MCP" guide (CF-173): the configurations a member copies into
 * an MCP client, built from the endpoint the API reports, the same ones the
 * panel shows (`frontend/src/features/mcp/lib/clients.ts`). None contains a
 * key or a token: the client registers itself and the member approves it in
 * the browser. Free of app imports so it runs under `node --test`.
 */

export type McpClientId = 'claude' | 'claude_code' | 'cursor' | 'vscode' | 'other';

export type McpClientConfig = {
  id: McpClientId;
  /** Product name; not translated. */
  name: string;
  /** Where the JSON goes (file path), when there is one. */
  file?: string;
  value: string;
  /** Run against ClickFix with this client (or the HTTP steps it makes); see the panel's `clients.ts`. */
  tested: boolean;
};

/** Name the server is saved under in the client. */
export const MCP_SERVER_NAME = 'clickfix';

const json = (value: unknown) => JSON.stringify(value, null, 2);

export function mcpClientConfigs(resourceUrl: string): McpClientConfig[] {
  return [
    { id: 'claude', tested: true, name: 'Claude', value: resourceUrl },
    {
      id: 'claude_code',
      tested: true,
      name: 'Claude Code',
      value: `claude mcp add --transport http ${MCP_SERVER_NAME} ${resourceUrl}`,
    },
    {
      id: 'cursor',
      tested: false,
      name: 'Cursor',
      file: '~/.cursor/mcp.json',
      value: json({ mcpServers: { [MCP_SERVER_NAME]: { url: resourceUrl } } }),
    },
    {
      id: 'vscode',
      tested: false,
      name: 'VS Code',
      file: '.vscode/mcp.json',
      value: json({ servers: { [MCP_SERVER_NAME]: { type: 'http', url: resourceUrl } } }),
    },
    {
      id: 'other',
      tested: true,
      name: 'mcp-remote',
      value: json({ mcpServers: { [MCP_SERVER_NAME]: { command: 'npx', args: ['-y', 'mcp-remote', resourceUrl] } } }),
    },
  ];
}

export type GuideTool = { name: string; description: string; write: boolean; available: boolean };

/** Tools the member gets now, read tools first; then the ones their permissions leave out. */
export function splitTools(tools: GuideTool[]) {
  const order = (a: GuideTool, b: GuideTool) => Number(a.write) - Number(b.write) || a.name.localeCompare(b.name);
  return {
    available: tools.filter((t) => t.available).sort(order),
    unavailable: tools.filter((t) => !t.available).sort(order),
  };
}
