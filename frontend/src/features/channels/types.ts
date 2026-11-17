import type { components } from "@/generated/api";

type Schemas = components["schemas"];

export type ChannelInstance = Schemas["ChannelInstance"];
export type ChannelHealth = Schemas["ChannelHealth"];
export type ChannelType = Schemas["ChannelType"];
export type ChannelInstanceCreate = Schemas["ChannelInstanceCreate"];
export type ChannelInstanceUpdate = Schemas["ChannelInstanceUpdate"];
export type ChannelCredentials = Schemas["ChannelCredentials"];
export type ChannelSettings = Schemas["ChannelSettings"];
export type EmailDomain = Schemas["EmailDomain"];
/** Body of `POST /v1/tenant/channels/email/test` (nothing is saved). */
export type EmailConnectionTest = {
  credentials: ChannelCredentials;
  settings?: ChannelSettings;
};
export type WhatsAppSignupConfig = Schemas["WhatsAppSignupConfig"];
export type WhatsAppSignupRequest = Schemas["WhatsAppSignupRequest"];
export type WhatsAppTemplate = Schemas["WhatsAppTemplate"];
export type WhatsAppTemplateCreate = Schemas["WhatsAppTemplateCreate"];
export type MessengerPages = Schemas["MessengerPages"];
export type SlackConversation = Schemas["SlackConversation"];
export type GoogleChatSpace = Schemas["GoogleChatSpace"];
export type VoiceCall = Schemas["VoiceCall"];

/** Channel types that connect through a provider login instead of a form. */
export type OAuthChannelType = "instagram" | "messenger" | "slack";

/**
 * Operational settings of an inbox (CF-61), kept in the instance's
 * `settings` next to channel specific keys (e.g. `inbound_alias`). The old
 * `ai_enabled` flag is no longer written: the AI agent's channel-type scope
 * decides (CF-243).
 */
export type ChannelOperationalSettings = {
  default_team_uuid?: string;
};
