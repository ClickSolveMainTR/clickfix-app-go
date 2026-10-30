import type {
  ChannelInstance,
  ChannelInstanceCreate,
  ChannelInstanceUpdate,
  ChannelType,
  EmailConnectionTest,
  EmailDomain,
  GoogleChatSpace,
  MessengerPages,
  OAuthChannelType,
  SlackConversation,
  VoiceCall,
  WhatsAppSignupConfig,
  WhatsAppSignupRequest,
  WhatsAppTemplate,
  WhatsAppTemplateCreate,
} from "@/features/channels/types";
import { tenantRequest } from "@/lib/api/tenant-request";

const BASE = "/v1/tenant/channels";

type Items<T> = { items: T[] };

/** Channel instances (inboxes) of one organization and their wizards. */
export const channelsService = {
  types(slug: string) {
    return tenantRequest<Items<ChannelType>>(slug, "GET", `${BASE}/types`);
  },
  list(slug: string) {
    return tenantRequest<Items<ChannelInstance>>(slug, "GET", BASE);
  },
  create(slug: string, body: ChannelInstanceCreate) {
    return tenantRequest<ChannelInstance>(slug, "POST", BASE, { body });
  },
  update(slug: string, uuid: string, body: ChannelInstanceUpdate) {
    return tenantRequest<ChannelInstance>(slug, "PATCH", `${BASE}/${uuid}`, {
      body,
    });
  },
  remove(slug: string, uuid: string) {
    return tenantRequest<ChannelInstance>(slug, "DELETE", `${BASE}/${uuid}`);
  },
  check(slug: string, uuid: string) {
    return tenantRequest<ChannelInstance>(
      slug,
      "POST",
      `${BASE}/${uuid}/check`,
    );
  },
  reconnect(slug: string, uuid: string) {
    return tenantRequest<ChannelInstance>(
      slug,
      "POST",
      `${BASE}/${uuid}/reconnect`,
    );
  },

  /** Logs in to IMAP/SMTP with the typed values; errors come back in Turkish. */
  emailTest(slug: string, body: EmailConnectionTest) {
    return tenantRequest<{ ok: boolean }>(slug, "POST", `${BASE}/email/test`, {
      body,
      silent: true,
    });
  },
  emailDomainCreate(slug: string, domain: string) {
    return tenantRequest<EmailDomain>(slug, "POST", `${BASE}/email/domains`, {
      body: { domain },
    });
  },
  emailDomainGet(slug: string, domain: string) {
    return tenantRequest<EmailDomain>(
      slug,
      "GET",
      `${BASE}/email/domains/${encodeURIComponent(domain)}`,
      { silent: true },
    );
  },
  emailDomainVerify(slug: string, domain: string) {
    return tenantRequest<EmailDomain>(
      slug,
      "POST",
      `${BASE}/email/domains/${encodeURIComponent(domain)}/verify`,
    );
  },

  whatsappSignupConfig(slug: string) {
    return tenantRequest<WhatsAppSignupConfig>(
      slug,
      "GET",
      `${BASE}/whatsapp/signup`,
      { silent: true },
    );
  },
  whatsappSignup(slug: string, body: WhatsAppSignupRequest) {
    return tenantRequest<Items<ChannelInstance>>(
      slug,
      "POST",
      `${BASE}/whatsapp/signup`,
      { body },
    );
  },
  whatsappTemplates(slug: string, uuid: string) {
    return tenantRequest<Items<WhatsAppTemplate>>(
      slug,
      "GET",
      `${BASE}/${uuid}/whatsapp/templates`,
    );
  },
  whatsappTemplateCreate(
    slug: string,
    uuid: string,
    body: WhatsAppTemplateCreate,
  ) {
    return tenantRequest<WhatsAppTemplate>(
      slug,
      "POST",
      `${BASE}/${uuid}/whatsapp/templates`,
      { body },
    );
  },
  whatsappTemplateDelete(slug: string, uuid: string, name: string) {
    return tenantRequest<{ deleted: boolean }>(
      slug,
      "DELETE",
      `${BASE}/${uuid}/whatsapp/templates/${encodeURIComponent(name)}`,
    );
  },

  oauthStart(slug: string, type: OAuthChannelType) {
    return tenantRequest<{ url: string; state: string }>(
      slug,
      "GET",
      `${BASE}/oauth/${type}/start`,
    );
  },
  instagramComplete(slug: string, code: string, state: string) {
    return tenantRequest<ChannelInstance>(
      slug,
      "POST",
      `${BASE}/oauth/instagram/complete`,
      { body: { code, state } },
    );
  },
  messengerComplete(slug: string, code: string, state: string) {
    return tenantRequest<MessengerPages>(
      slug,
      "POST",
      `${BASE}/oauth/messenger/complete`,
      { body: { code, state } },
    );
  },
  slackComplete(slug: string, code: string, state: string) {
    return tenantRequest<ChannelInstance>(
      slug,
      "POST",
      `${BASE}/oauth/slack/complete`,
      { body: { code, state } },
    );
  },
  slackConversations(slug: string, uuid: string) {
    return tenantRequest<Items<SlackConversation>>(
      slug,
      "GET",
      `${BASE}/${uuid}/slack/conversations`,
      { silent: true },
    );
  },
  googleChatSpaces(slug: string, uuid: string) {
    return tenantRequest<Items<GoogleChatSpace>>(
      slug,
      "GET",
      `${BASE}/${uuid}/google_chat/spaces`,
      { silent: true },
    );
  },
  voiceCalls(slug: string, uuid: string, limit: number, offset: number) {
    return tenantRequest<{
      items: VoiceCall[];
      total: number;
      limit: number;
      offset: number;
    }>(slug, "GET", `${BASE}/${uuid}/voice/calls`, {
      query: { limit, offset },
    });
  },
  messengerSelectPage(slug: string, selection: string, pageId: string) {
    return tenantRequest<ChannelInstance>(
      slug,
      "POST",
      `${BASE}/oauth/messenger/pages`,
      { body: { selection, page_id: pageId } },
    );
  },
};
