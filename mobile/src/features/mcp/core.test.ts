import assert from 'node:assert/strict';
import { test } from 'node:test';

import { mcpClientConfigs, splitTools } from './core.ts';

const URL_ = 'https://app.example.test/mcp/business';

test('every client configuration points at the reported endpoint', () => {
  const configs = mcpClientConfigs(URL_);
  assert.deepEqual(
    configs.map((c) => c.id),
    ['claude', 'claude_code', 'cursor', 'vscode', 'other'],
  );
  for (const c of configs) assert.ok(c.value.includes(URL_), c.id);
  assert.equal(configs[1].value, `claude mcp add --transport http clickfix ${URL_}`);
});

test('clients whose return address may be a custom scheme are marked untested', () => {
  assert.deepEqual(Object.fromEntries(mcpClientConfigs(URL_).map((c) => [c.id, c.tested])), {
    claude: true,
    claude_code: true,
    cursor: false,
    vscode: false,
    other: true,
  });
});

test('the JSON configurations parse and carry no credential', () => {
  const configs = mcpClientConfigs(URL_);
  assert.deepEqual(JSON.parse(configs[2].value), { mcpServers: { clickfix: { url: URL_ } } });
  assert.deepEqual(JSON.parse(configs[3].value), { servers: { clickfix: { type: 'http', url: URL_ } } });
  assert.deepEqual(JSON.parse(configs[4].value), {
    mcpServers: { clickfix: { command: 'npx', args: ['-y', 'mcp-remote', URL_] } },
  });
  for (const c of configs) assert.doesNotMatch(c.value, /token|secret|authorization|bearer|api[_-]?key/i);
});

test('splitTools separates what the member gets, read tools first', () => {
  const tool = (name: string, write: boolean, available: boolean) => ({ name, description: '', write, available });
  const { available, unavailable } = splitTools([
    tool('reply', true, true),
    tool('list_contacts', false, false),
    tool('search_conversations', false, true),
    tool('set_status', true, false),
  ]);
  assert.deepEqual(
    available.map((t) => t.name),
    ['search_conversations', 'reply'],
  );
  assert.deepEqual(
    unavailable.map((t) => t.name),
    ['list_contacts', 'set_status'],
  );
});
