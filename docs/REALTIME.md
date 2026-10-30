# Realtime channels and presence (CF-10)

Centrifugo carries every live update. Clients never subscribe on their own
(`allow_subscribe_for_client: false`): the BFF asks
`POST /v1/realtime/subscription-token` per channel and the API checks access.
Only the backend publishes (`allow_publish_for_client: false`).

| Channel | Who may subscribe | What is published |
|---|---|---|
| `user:{user_uuid}` | that user | notifications (in-app mirror) |
| `org:{org_uuid}` | members of the organization | `presence.changed` (`user_uuid`, `status`, `last_seen_at`); Centrifugo presence + join/leave |
| `inbox:{org_uuid}` | members with `tenant.conversations.read` | inbox list changes (CF-43) |
| `conversation:{uuid}` | members of the owning organization with `tenant.conversations.read` | `conversation.typing`; Centrifugo presence = who has it open |
| `widget:{instance}.{visitor hash}` | the widget visitor (anonymous connection; token issued by the widget module, `widget.VisitorChannel`, CF-55) | replies to the visitor |

Authorization is a chain of `realtime.ChannelAuthorizer`s built in
`httpserver`: `presence.Authorizer` (org, conversation) → `inbox.Authorizer`
→ auth use case (`user:`, `system.notifications`). Unknown channels fail closed.

The connection token carries `info.name`, so presence lists and join/leave
events show names without another request.

## Presence

`agent_presence` holds `status` (what teammates see) and `manual_status`
(what the agent picked) per organization member.

- `PUT /v1/tenant/presence/me` sets the status (`online`, `away`, `busy`, `offline`).
- `POST /v1/tenant/presence/heartbeat` every 30 s while the panel is open. The
  first heartbeat (or the first after an idle sweep) restores the picked status.
- The worker's `app:presence:sweep` (every minute) turns agents without a
  heartbeat for 3 minutes offline. A dropped socket alone never does.
- Every real change publishes `presence.changed` on `org:{uuid}` and writes the
  `agent.presence_changed` domain event (routing can react to it).

## Typing

`POST /v1/tenant/realtime/typing {conversation_uuid, state: start|stop}`: the
client asks, the API checks the conversation belongs to the organization,
rate limits (`start` once per 2 s per user and conversation) and publishes
`conversation.typing`. Clients never publish to Centrifugo directly.

## Frontend

`@/features/presence`: `useTeamPresence()`, `useMyPresence()` (heartbeat +
status menu, mounted by the sidebar user card), `useConversationViewers(uuid)`,
`useTyping(uuid)` (`typing`, `notifyTyping()`). Generic `useChannel` and
`usePresence` live in `@/hooks/use-realtime`; channel names come from
`RealtimeChannels`.
