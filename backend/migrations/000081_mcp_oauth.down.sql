-- Data-loss note: disconnects all MCP clients.
DROP TABLE mcp_oauth_tokens,mcp_oauth_codes,mcp_oauth_grants,mcp_oauth_auth_requests,mcp_oauth_clients;
