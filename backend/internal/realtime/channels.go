package realtime

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

const ChannelSystemNotifications = "system.notifications"

// ChannelAuthorizer decides whether a principal may subscribe to a channel.
type ChannelAuthorizer interface {
	CanSubscribeChannel(ctx context.Context, userUUID uuid.UUID, isSuperAdmin bool, channel string) (bool, error)
}

// ParseUserChannel returns the user UUID from "user:{uuid}".
func ParseUserChannel(channel string) (uuid.UUID, bool) {
	raw, ok := stripPrefix(channel, "user:")
	if !ok {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// ParseWorkspaceChannel returns the workspace UUID from "workspace:{uuid}".
func ParseWorkspaceChannel(channel string) (uuid.UUID, bool) {
	raw, ok := stripPrefix(channel, "workspace:")
	if !ok {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// ParseConversationChannel returns the conversation UUID from "conversation:{uuid}".
func ParseConversationChannel(channel string) (uuid.UUID, bool) {
	raw, ok := stripPrefix(channel, "conversation:")
	if !ok {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

func stripPrefix(channel, prefix string) (string, bool) {
	channel = strings.TrimSpace(channel)
	if !strings.HasPrefix(channel, prefix) {
		return "", false
	}
	raw := strings.TrimPrefix(channel, prefix)
	if raw == "" {
		return "", false
	}
	return raw, true
}

// QRLoginChannelPrefix is the Centrifugo namespace for QR sign-in sessions.
// One channel per QR session / browser tab; only the backend publishes.
const QRLoginChannelPrefix = "qrlogin:"

// QRLoginChannel returns the channel for an opaque QR channel id.
func QRLoginChannel(channelID string) string {
	return QRLoginChannelPrefix + channelID
}

// InboxChannelPrefix is the namespace of an organization's inbox events
// (conversations and messages); only the backend publishes.
const InboxChannelPrefix = "inbox:"

// InboxChannel returns the inbox channel of an organization.
func InboxChannel(orgUUID uuid.UUID) string {
	return InboxChannelPrefix + orgUUID.String()
}

// ParseInboxChannel returns the organization UUID from "inbox:{uuid}".
func ParseInboxChannel(channel string) (uuid.UUID, bool) {
	raw, ok := stripPrefix(channel, InboxChannelPrefix)
	if !ok {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// InboxUserChannel is the inbox channel of one member without
// tenant.conversations.read_all (CF-44): the backend fans each inbox event
// out to the members allowed to see that conversation. Only that member may
// subscribe; members with read_all subscribe to InboxChannel instead.
func InboxUserChannel(orgUUID, userUUID uuid.UUID) string {
	return InboxChannelPrefix + orgUUID.String() + ":" + userUUID.String()
}

// ParseInboxUserChannel returns the organization and user UUIDs from
// "inbox:{org}:{user}".
func ParseInboxUserChannel(channel string) (orgUUID, userUUID uuid.UUID, ok bool) {
	raw, ok := stripPrefix(channel, InboxChannelPrefix)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	orgRaw, userRaw, found := strings.Cut(raw, ":")
	if !found {
		return uuid.Nil, uuid.Nil, false
	}
	o, err := uuid.Parse(orgRaw)
	if err != nil {
		return uuid.Nil, uuid.Nil, false
	}
	u, err := uuid.Parse(userRaw)
	if err != nil {
		return uuid.Nil, uuid.Nil, false
	}
	return o, u, true
}

// Organization-wide namespace (CF-10): agent presence and team events of
// one organization; only the backend publishes. Members may subscribe.
const OrgChannelPrefix = "org:"

// OrgChannel returns the channel of an organization.
func OrgChannel(orgUUID uuid.UUID) string {
	return OrgChannelPrefix + orgUUID.String()
}

// ParseOrgChannel returns the organization UUID from "org:{uuid}".
func ParseOrgChannel(channel string) (uuid.UUID, bool) {
	raw, ok := stripPrefix(channel, OrgChannelPrefix)
	if !ok {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// ConversationChannel returns the channel of one conversation: viewers
// (Centrifugo presence) and typing indicators.
func ConversationChannel(conversationUUID uuid.UUID) string {
	return "conversation:" + conversationUUID.String()
}

// Realtime message types published by CF-10.
const (
	EventPresenceChanged    = "presence.changed"
	EventConversationTyping = "conversation.typing"
)

// Message is the envelope every backend publication uses
// ({type, data}; see frontend lib/realtime parseRealtimeMessage).
type Message struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}
