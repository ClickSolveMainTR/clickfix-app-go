/** Visitor API payloads (backend/docs/openapi.yaml Widget* schemas). */
import type { WidgetSettings } from "../shared/schema";

export type PublicSettings = Omit<
  WidgetSettings,
  | "allowed_domains"
  | "team_member_uuids"
  | "attribute_mapping"
  | "rate_limit"
  | "ai"
> & {
  ai: { enabled: boolean; human_button: boolean };
  attribute_keys: { key: string; anonymous: boolean }[];
};

export type HelpArticle = { title: string; excerpt: string; url: string };
export type HelpResult = {
  available: boolean;
  help_center_url: string | null;
  items: HelpArticle[];
};
export type BookingLink = { available: boolean; url: string | null };
export type TeamMember = {
  name: string;
  initials: string;
  avatar_url: string | null;
};
export type VisitorContact = {
  name: string;
  email: string | null;
  phone: string | null;
};
export type Visitor = {
  id: string;
  token: string;
  expires_at: string;
  verified: boolean;
  contact: VisitorContact | null;
};
export type Conversation = {
  uuid: string;
  number: number;
  status: string;
  open: boolean;
  last_message_preview: string;
  last_message_direction: string;
  last_message_at: string;
  unread_count: number;
  can_rate: boolean;
  rating: number | null;
};
export type Attachment = {
  uuid: string;
  filename: string;
  content_type: string;
  size_bytes: number;
  url: string;
};
export type Message = {
  uuid: string;
  direction: "in" | "out";
  sender_type?: string;
  sender_name: string;
  kind: string;
  content: string;
  status: string;
  client_id: string | null;
  attachments: Attachment[];
  created_at: string;
  read_at: string | null;
  pending?: boolean;
  failed?: boolean;
  queued?: boolean;
  /** Local-only (proactive message shown before the conversation). */
  local?: boolean;
};
export type FormCondition = {
  field: string;
  operator: string;
  value?: unknown;
};
export type FormField = {
  key: string;
  source: string;
  type: string;
  label: string;
  help: string;
  required: boolean;
  options: { value: string; label: string }[];
  placeholder: string;
  condition: FormCondition | null;
};
export type RequestForm = {
  uuid: string;
  name: string;
  description: string;
  fields: FormField[];
};
export type BootstrapContext = {
  visible: boolean;
  hidden_reason: string;
  device: "desktop" | "mobile";
  country: string;
  returning: boolean;
  eligible_triggers: string[];
};
export type Bootstrap = {
  widget: {
    id: string;
    name: string;
    settings: PublicSettings;
    online: boolean;
    team: TeamMember[];
    reply?: { value: string; next_open_at: string | null };
  };
  visitor: Visitor;
  realtime: { enabled: boolean };
  limits: { max_file_bytes: number; max_text_length: number };
  conversations: Conversation[];
  request_form?: RequestForm | null;
  context?: BootstrapContext;
};
export type Envelope<T> = {
  success: boolean;
  data: T;
  error?: { code?: string; message?: string };
};

export type IdentifyInput = {
  user_id: string;
  user_hash: string;
  name?: string;
  email?: string;
  phone?: string;
  company?: { id?: string; name?: string; domain?: string };
  attributes?: Record<string, string | number | boolean>;
};

export type View = "home" | "messages" | "help" | "chat" | "callback";

export type InitOptions = {
  widgetId?: string;
  locale?: string;
  position?: "left" | "right";
  color?: string;
  theme?: "light" | "dark" | "auto";
  dir?: "ltr" | "rtl";
  hideLauncher?: boolean;
  /** CSS selector of the host page's own launcher button(s). */
  customLauncherSelector?: string;
  zIndex?: number;
  user?: IdentifyInput;
  attributes?: Record<string, string | number | boolean>;
  /** Pre-granted consent from the site's own consent manager. */
  consent?: boolean;
  debug?: boolean;
};

export type TrackedEvent = {
  name: string;
  properties?: Record<string, unknown>;
  at: string;
};

export type Handler = (payload: unknown) => void;
