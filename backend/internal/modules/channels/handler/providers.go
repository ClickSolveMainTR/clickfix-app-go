package handler

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/channels/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/authctx"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/email"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/googlechat"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/instagram"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/messenger"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/metagraph"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/slack"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/whatsapp"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/furkanmeclis/clickfix-app-go/backend/pkg/response"
	"github.com/google/uuid"
)

// Error codes of the provider endpoints.
const (
	CodeChannelUnavailable = "CHANNEL_NOT_AVAILABLE"
	CodeProviderError      = "CHANNEL_PROVIDER_ERROR"
	CodeOAuthState         = "CHANNEL_OAUTH_STATE_INVALID"
	CodeDomainTaken        = "EMAIL_DOMAIN_TAKEN"
)

// Providers are the typed adapters behind the connect wizards. A nil
// adapter answers 404 CHANNEL_NOT_AVAILABLE (not configured, or a Meta
// channel while META_CHANNELS_ENABLED is off).
type Providers struct {
	Slack      *slack.Adapter
	GoogleChat *googlechat.Adapter
	Email      *email.Adapter
	WhatsApp   *whatsapp.Adapter
	Instagram  *instagram.Adapter
	Messenger  *messenger.Adapter
	// Box seals the Messenger page selection (APP_ENCRYPTION_KEY).
	Box *crypto.SecretBox
}

// WithProviders enables the connect wizards.
func (h *Handler) WithProviders(p Providers) *Handler {
	h.providers = p
	return h
}

func unavailable(w http.ResponseWriter, r *http.Request) {
	response.Error(w, r, http.StatusNotFound, CodeChannelUnavailable, "This channel is not available")
}

// providerError answers 502 with the provider's own message when it is a
// typed provider error (useful to the user: "number already registered");
// anything else (network, decoding) is logged and answered generically.
func (h *Handler) providerError(w http.ResponseWriter, r *http.Request, err error) {
	h.log.Warn("channel_provider_error", "path", r.URL.Path, "error", err)
	msg := "The provider could not be reached"
	var (
		we *whatsapp.Error
		ge *metagraph.Error
		re *email.APIError
	)
	switch {
	case errors.As(err, &we):
		msg = we.Message
	case errors.As(err, &ge):
		msg = ge.Message
	case errors.As(err, &re):
		msg = re.Message
	}
	response.ErrorWithDetails(w, r, http.StatusBadGateway, CodeProviderError, "The provider rejected the request",
		[]response.Detail{{Message: msg}})
}

// ---------------------------------------------------------------------------
// E-mail: sender domain wizard (SPF/DKIM through Resend, DMARC by DNS).

var domainRE = regexp.MustCompile(`^(?i)[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`)

// EmailDomainView is the wizard state of a sender domain.
type EmailDomainView struct {
	Domain           string            `json:"domain"`
	Status           string            `json:"status"`
	Records          []email.DNSRecord `json:"records"`
	DMARC            string            `json:"dmarc"`
	DMARCRecommended string            `json:"dmarc_recommended"`
	InboundDomain    string            `json:"inbound_domain"`
	ProviderID       string            `json:"-"`
}

func (h *Handler) domainError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, usecase.ErrDomainTaken) {
		response.Conflict(w, r, CodeDomainTaken, "This domain is registered by another organization")
		return
	}
	response.InternalErr(w, r, err, "")
}

// ownedDomain answers 404 unless the organization registered the domain.
func (h *Handler) ownedDomain(w http.ResponseWriter, r *http.Request) (string, bool) {
	name, ok := domainParam(w, r, r.PathValue("domain"))
	if !ok {
		return "", false
	}
	scope, ok := scopeOf(w, r)
	if !ok {
		return "", false
	}
	owns, err := h.svc.OwnsEmailDomain(r.Context(), scope.ID, name)
	if err != nil {
		response.InternalErr(w, r, err, "")
		return "", false
	}
	if !owns {
		response.NotFound(w, r, "Domain not registered")
		return "", false
	}
	return name, true
}

func (h *Handler) emailDomain(ctx context.Context, name string, create bool) (EmailDomainView, error) {
	rs := h.providers.Email.Resend()
	d, found, err := rs.FindDomain(ctx, name)
	if err != nil {
		return EmailDomainView{}, err
	}
	switch {
	case !found && create:
		if d, err = rs.CreateDomain(ctx, name); err != nil {
			return EmailDomainView{}, err
		}
	case !found:
		return EmailDomainView{}, usecase.ErrNotFound
	default:
		// The list has no records: read the domain itself.
		if d, err = rs.GetDomain(ctx, d.ID); err != nil {
			return EmailDomainView{}, err
		}
	}
	dmarc, _ := email.LookupDMARC(ctx, nil, name)
	return EmailDomainView{
		ProviderID: d.ID, Domain: name, Status: d.Status, Records: d.Records, DMARC: dmarc,
		DMARCRecommended: email.RecommendedDMARC, InboundDomain: h.providers.Email.InboundDomain(),
	}, nil
}

func domainParam(w http.ResponseWriter, r *http.Request, v string) (string, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if !domainRE.MatchString(v) || len(v) > 253 {
		response.ValidationError(w, r, []response.Detail{{Field: "domain", Message: "invalid domain"}})
		return "", false
	}
	return v, true
}

// EmailDomainCreate registers a sender domain and returns its DNS records.
func (h *Handler) EmailDomainCreate(w http.ResponseWriter, r *http.Request) {
	if h.providers.Email == nil {
		unavailable(w, r)
		return
	}
	var req struct {
		Domain string `json:"domain"`
	}
	if !decode(w, r, &req) {
		return
	}
	name, ok := domainParam(w, r, req.Domain)
	if !ok {
		return
	}
	if name == h.providers.Email.InboundDomain() || strings.HasSuffix(name, "."+h.providers.Email.InboundDomain()) {
		response.ValidationError(w, r, []response.Detail{{Field: "domain", Message: "the receiving domain cannot be a sender domain"}})
		return
	}
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	if err := h.svc.ClaimEmailDomain(r.Context(), scope.ID, actor, name, ""); err != nil {
		h.domainError(w, r, err)
		return
	}
	v, err := h.emailDomain(r.Context(), name, true)
	if err == nil {
		err = h.svc.ClaimEmailDomain(r.Context(), scope.ID, actor, name, v.ProviderID)
	}
	if err != nil {
		h.providerError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// EmailDomainGet returns the wizard state of a sender domain.
func (h *Handler) EmailDomainGet(w http.ResponseWriter, r *http.Request) {
	if h.providers.Email == nil {
		unavailable(w, r)
		return
	}
	name, ok := h.ownedDomain(w, r)
	if !ok {
		return
	}
	v, err := h.emailDomain(r.Context(), name, false)
	if errors.Is(err, usecase.ErrNotFound) {
		response.NotFound(w, r, "Domain not registered")
		return
	}
	if err != nil {
		h.providerError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// EmailDomainVerify asks for a new DNS check and returns the state.
func (h *Handler) EmailDomainVerify(w http.ResponseWriter, r *http.Request) {
	if h.providers.Email == nil {
		unavailable(w, r)
		return
	}
	name, ok := h.ownedDomain(w, r)
	if !ok {
		return
	}
	d, found, err := h.providers.Email.Resend().FindDomain(r.Context(), name)
	if err == nil && !found {
		response.NotFound(w, r, "Domain not registered")
		return
	}
	if err == nil {
		err = h.providers.Email.Resend().VerifyDomain(r.Context(), d.ID)
	}
	if err != nil {
		h.providerError(w, r, err)
		return
	}
	v, err := h.emailDomain(r.Context(), name, false)
	if err != nil {
		h.providerError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, v)
}

// ---------------------------------------------------------------------------
// WhatsApp: Embedded Signup and templates.

// WhatsAppSignupConfig returns the public FB.login settings.
func (h *Handler) WhatsAppSignupConfig(w http.ResponseWriter, r *http.Request) {
	if h.providers.WhatsApp == nil {
		unavailable(w, r)
		return
	}
	response.JSON(w, r, http.StatusOK, h.providers.WhatsApp.SignupConfig())
}

// WhatsAppSignup completes Embedded Signup: exchanges the code for the
// business token, subscribes the app to the WABA, registers each chosen
// number and connects one instance per number.
func (h *Handler) WhatsAppSignup(w http.ResponseWriter, r *http.Request) {
	wa := h.providers.WhatsApp
	if wa == nil {
		unavailable(w, r)
		return
	}
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	var req struct {
		Code          string `json:"code"`
		WABAID        string `json:"waba_id"`
		PhoneNumberID string `json:"phone_number_id"`
		Name          string `json:"name"`
	}
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Code) == "" || !whatsapp.ValidID(req.WABAID) || (req.PhoneNumberID != "" && !whatsapp.ValidID(req.PhoneNumberID)) {
		response.ValidationError(w, r, []response.Detail{{Message: "code and waba_id are required"}})
		return
	}
	ctx := r.Context()
	token, err := wa.ExchangeCode(ctx, req.Code)
	if err != nil {
		h.providerError(w, r, err)
		return
	}
	metaID, err := wa.SignupUserID(ctx, token)
	if err != nil {
		h.providerError(w, r, err)
		return
	}
	if err := wa.SubscribeApp(ctx, token, req.WABAID); err != nil {
		h.providerError(w, r, err)
		return
	}
	numbers, err := wa.PhoneNumbers(ctx, token, req.WABAID)
	if err != nil {
		h.providerError(w, r, err)
		return
	}
	actor := authctx.MustPrincipal(ctx).UserInternal
	out := []usecase.InstanceView{}
	for _, n := range numbers {
		if req.PhoneNumberID != "" && n.ID != req.PhoneNumberID {
			continue
		}
		// Cloud API numbers must be registered with a two-step PIN; an
		// already registered number answers an error we can ignore.
		if err := wa.RegisterNumber(ctx, token, n.ID, randomPIN()); err != nil {
			h.log.Info("whatsapp_register_number", "phone_number_id", n.ID, "error", err)
		}
		name := strings.TrimSpace(req.Name)
		if name == "" || len(numbers) > 1 {
			name = strings.TrimSpace(n.VerifiedName + " " + n.DisplayPhoneNumber)
		}
		v, err := h.svc.Create(ctx, usecase.CreateInput{
			OrganizationID: scope.ID, ActorID: actor, Type: channels.TypeWhatsApp, Name: name,
			Credentials: channels.Credentials{
				whatsapp.FieldPhoneNumberID: n.ID, whatsapp.FieldWABAID: req.WABAID, whatsapp.FieldAccessToken: token,
			},
			Settings: map[string]any{"display_phone_number": n.DisplayPhoneNumber, "connection": "embedded_signup", "meta_user_id": metaID},
		})
		if errors.Is(err, usecase.ErrAlreadyLinked) && len(numbers) > 1 {
			continue
		}
		if err != nil {
			writeError(w, r, err)
			return
		}
		h.record(r, scope, "channels.connected", v)
		out = append(out, v)
	}
	if len(out) == 0 {
		response.ValidationError(w, r, []response.Detail{{Field: "phone_number_id", Message: "no new phone number to connect in this WhatsApp Business Account"}})
		return
	}
	response.JSON(w, r, http.StatusCreated, map[string]any{"items": out})
}

func randomPIN() string {
	var b [3]byte
	_, _ = rand.Read(b[:])
	n := (int(b[0])<<16 | int(b[1])<<8 | int(b[2])) % 1000000
	return strings.Repeat("0", 6-len(strconv.Itoa(n))) + strconv.Itoa(n)
}

func (h *Handler) whatsappInstance(w http.ResponseWriter, r *http.Request) (channels.Instance, bool) {
	if h.providers.WhatsApp == nil {
		unavailable(w, r)
		return channels.Instance{}, false
	}
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return channels.Instance{}, false
	}
	inst, _, err := h.svc.Open(r.Context(), scope.ID, id)
	if err == nil && inst.Type != channels.TypeWhatsApp {
		err = usecase.ErrNotFound
	}
	if err != nil {
		writeError(w, r, err)
		return channels.Instance{}, false
	}
	return inst, true
}

// WhatsAppTemplates lists the message templates of the instance's WABA.
func (h *Handler) WhatsAppTemplates(w http.ResponseWriter, r *http.Request) {
	inst, ok := h.whatsappInstance(w, r)
	if !ok {
		return
	}
	items, err := h.providers.WhatsApp.ListTemplates(r.Context(), inst)
	if err != nil {
		h.providerError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// WhatsAppTemplateCreate submits a template for Meta's approval.
func (h *Handler) WhatsAppTemplateCreate(w http.ResponseWriter, r *http.Request) {
	inst, ok := h.whatsappInstance(w, r)
	if !ok {
		return
	}
	var req whatsapp.CreateTemplateInput
	if !decode(w, r, &req) {
		return
	}
	t, err := h.providers.WhatsApp.CreateTemplate(r.Context(), inst, req)
	if err != nil {
		var we *whatsapp.Error
		if errors.As(err, &we) {
			h.providerError(w, r, err)
			return
		}
		response.ValidationError(w, r, []response.Detail{{Message: err.Error()}})
		return
	}
	response.JSON(w, r, http.StatusCreated, t)
}

// WhatsAppTemplateDelete deletes a template (every language of the name).
func (h *Handler) WhatsAppTemplateDelete(w http.ResponseWriter, r *http.Request) {
	inst, ok := h.whatsappInstance(w, r)
	if !ok {
		return
	}
	if err := h.providers.WhatsApp.DeleteTemplate(r.Context(), inst, r.PathValue("name")); err != nil {
		h.providerError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"deleted": true})
}

// ---------------------------------------------------------------------------
// Instagram and Messenger: OAuth connect.

// oauthTTL bounds how long a login dialog may stay open.
const oauthTTL = 15 * time.Minute

// state is the signed OAuth state: organization, user, channel, expiry.
func (h *Handler) state(orgID, userID int64, channelType string) (string, error) {
	var nonce [8]byte
	_, _ = rand.Read(nonce[:])
	payload := strings.Join([]string{
		strconv.FormatInt(orgID, 10), strconv.FormatInt(userID, 10), channelType,
		strconv.FormatInt(time.Now().Add(oauthTTL).Unix(), 10), hex.EncodeToString(nonce[:]),
	}, ".")
	mac, err := h.mac(payload)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + mac, nil
}

func (h *Handler) checkState(v string, orgID, userID int64, channelType string) bool {
	enc, mac, ok := strings.Cut(v, ".")
	if !ok {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return false
	}
	want, err := h.mac(string(raw))
	if err != nil || !hmac.Equal([]byte(mac), []byte(want)) {
		return false
	}
	parts := strings.Split(string(raw), ".")
	if len(parts) != 5 || parts[0] != strconv.FormatInt(orgID, 10) || parts[1] != strconv.FormatInt(userID, 10) || parts[2] != channelType {
		return false
	}
	exp, err := strconv.ParseInt(parts[3], 10, 64)
	return err == nil && time.Now().Unix() <= exp
}

// mac signs OAuth state with the key set by WithOAuthKey.
func (h *Handler) mac(payload string) (string, error) {
	if len(h.oauthKey) == 0 {
		return "", errors.New("channels: oauth state key missing")
	}
	m := hmac.New(sha256.New, h.oauthKey)
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil)), nil
}

// WithOAuthKey sets the key OAuth state is signed with (derived from
// APP_ENCRYPTION_KEY by the caller).
func (h *Handler) WithOAuthKey(key []byte) *Handler {
	h.oauthKey = key
	return h
}

// OAuthStart returns the login dialog URL of Instagram or Messenger.
func (h *Handler) OAuthStart(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	channelType := r.PathValue("type")
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	st, err := h.state(scope.ID, actor, channelType)
	if err != nil {
		response.InternalErr(w, r, err, "")
		return
	}
	var u string
	switch {
	case channelType == channels.TypeSlack && h.providers.Slack != nil && h.providers.Slack.OAuthEnabled():
		u = h.providers.Slack.LoginURL(st)
	case channelType == channels.TypeInstagram && h.providers.Instagram != nil:
		u = h.providers.Instagram.LoginURL(st)
	case channelType == channels.TypeMessenger && h.providers.Messenger != nil:
		u = h.providers.Messenger.LoginURL(st)
	default:
		unavailable(w, r)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"url": u, "state": st})
}

type oauthCompleteRequest struct {
	Code  string `json:"code"`
	State string `json:"state"`
	Name  string `json:"name"`
}

func (h *Handler) oauthRequest(w http.ResponseWriter, r *http.Request, channelType string) (oauthCompleteRequest, int64, int64, bool) {
	var req oauthCompleteRequest
	scope, ok := scopeOf(w, r)
	if !ok || !decode(w, r, &req) {
		return req, 0, 0, false
	}
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	if strings.TrimSpace(req.Code) == "" || !h.checkState(req.State, scope.ID, actor, channelType) {
		response.BadRequest(w, r, CodeOAuthState, "The login expired or belongs to another session; start again")
		return req, 0, 0, false
	}
	return req, scope.ID, actor, true
}

// InstagramComplete finishes Instagram Login: a long-lived token, the app
// subscribed to the account's webhooks, and the instance connected.
func (h *Handler) InstagramComplete(w http.ResponseWriter, r *http.Request) {
	ig := h.providers.Instagram
	if ig == nil {
		unavailable(w, r)
		return
	}
	req, orgID, actor, ok := h.oauthRequest(w, r, channels.TypeInstagram)
	if !ok {
		return
	}
	ctx := r.Context()
	token, userID, expiresAt, err := ig.ExchangeCode(ctx, req.Code)
	if err != nil {
		h.providerError(w, r, err)
		return
	}
	acc, err := ig.Me(ctx, token)
	if err != nil {
		h.providerError(w, r, err)
		return
	}
	if acc.UserID != "" {
		userID = acc.UserID
	}
	if err := ig.SubscribeApp(ctx, token); err != nil {
		h.providerError(w, r, err)
		return
	}
	creds := channels.Credentials{instagram.FieldUserID: userID, instagram.FieldAccessToken: token, instagram.FieldUsername: acc.Username}
	if !expiresAt.IsZero() {
		creds[instagram.FieldExpiresAt] = expiresAt.UTC().Format(time.RFC3339)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "@" + acc.Username
	}
	h.connect(w, r, orgID, actor, channels.TypeInstagram, name, creds)
}

// selectionPurpose binds a sealed page list to this use of the box (the
// same key seals channel credentials).
const selectionPurpose = "channels|messenger-pages\n"

// pageSelection is the sealed list of pages a Messenger login returned;
// the page tokens never reach the browser.
type pageSelection struct {
	Org        int64            `json:"o"`
	User       int64            `json:"u"`
	Exp        int64            `json:"e"`
	Pages      []messenger.Page `json:"p"`
	MetaUserID string           `json:"m"`
}

// MessengerComplete finishes Facebook Login and returns the pages the user
// manages with a sealed selection token to pick one.
func (h *Handler) MessengerComplete(w http.ResponseWriter, r *http.Request) {
	ms := h.providers.Messenger
	if ms == nil || h.providers.Box == nil {
		unavailable(w, r)
		return
	}
	req, orgID, actor, ok := h.oauthRequest(w, r, channels.TypeMessenger)
	if !ok {
		return
	}
	token, err := ms.ExchangeCode(r.Context(), req.Code)
	if err != nil {
		h.providerError(w, r, err)
		return
	}
	metaID, err := ms.UserID(r.Context(), token)
	if err != nil {
		h.providerError(w, r, err)
		return
	}
	pages, err := ms.Pages(r.Context(), token)
	if err != nil {
		h.providerError(w, r, err)
		return
	}
	raw, _ := json.Marshal(pageSelection{Org: orgID, User: actor, Exp: time.Now().Add(oauthTTL).Unix(), Pages: pages, MetaUserID: metaID})
	sealed, err := h.providers.Box.Encrypt(selectionPurpose + string(raw))
	if err != nil {
		response.InternalErr(w, r, err, "")
		return
	}
	type page struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	out := make([]page, 0, len(pages))
	for _, p := range pages {
		out = append(out, page{ID: p.ID, Name: p.Name})
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"selection": sealed, "pages": out})
}

// MessengerSelectPage connects the chosen page of a Messenger login.
func (h *Handler) MessengerSelectPage(w http.ResponseWriter, r *http.Request) {
	ms := h.providers.Messenger
	if ms == nil || h.providers.Box == nil {
		unavailable(w, r)
		return
	}
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	var req struct {
		Selection string `json:"selection"`
		PageID    string `json:"page_id"`
		Name      string `json:"name"`
	}
	if !decode(w, r, &req) {
		return
	}
	actor := authctx.MustPrincipal(r.Context()).UserInternal
	var sel pageSelection
	plain, err := h.providers.Box.Decrypt(req.Selection)
	if err != nil || !strings.HasPrefix(plain, selectionPurpose) || json.Unmarshal([]byte(strings.TrimPrefix(plain, selectionPurpose)), &sel) != nil || sel.Org != scope.ID || sel.User != actor || time.Now().Unix() > sel.Exp {
		response.BadRequest(w, r, CodeOAuthState, "The login expired or belongs to another session; start again")
		return
	}
	for _, p := range sel.Pages {
		if p.ID != req.PageID {
			continue
		}
		if err := ms.SubscribePage(r.Context(), p.ID, p.AccessToken); err != nil {
			h.providerError(w, r, err)
			return
		}
		name := strings.TrimSpace(req.Name)
		if name == "" {
			name = p.Name
		}
		h.connect(w, r, scope.ID, actor, channels.TypeMessenger, name, channels.Credentials{
			messenger.FieldPageID: p.ID, messenger.FieldPageAccessToken: p.AccessToken,
		}, sel.MetaUserID)
		return
	}
	response.ValidationError(w, r, []response.Detail{{Field: "page_id", Message: "page not in this login"}})
}

func (h *Handler) connect(w http.ResponseWriter, r *http.Request, orgID, actor int64, channelType, name string, creds channels.Credentials, metaID ...string) {
	scope, _ := scopeOf(w, r)
	settings := map[string]any{"connection": "oauth"}
	if channelType == channels.TypeInstagram {
		settings["meta_user_id"] = creds[instagram.FieldUserID]
	}
	if len(metaID) > 0 {
		settings["meta_user_id"] = metaID[0]
	}
	v, err := h.svc.Create(r.Context(), usecase.CreateInput{
		OrganizationID: orgID, ActorID: actor, Type: channelType, Name: name, Credentials: creds,
		Settings: settings,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, scope, "channels.connected", v)
	response.JSON(w, r, http.StatusCreated, v)
}

// SlackComplete exchanges a session/org-bound OAuth code and seals the workspace bot token.
func (h *Handler) SlackComplete(w http.ResponseWriter, r *http.Request) {
	a := h.providers.Slack
	if a == nil || !a.OAuthEnabled() {
		unavailable(w, r)
		return
	}
	req, orgID, actor, ok := h.oauthRequest(w, r, channels.TypeSlack)
	if !ok {
		return
	}
	creds, err := a.ExchangeCode(r.Context(), req.Code)
	if err != nil {
		h.providerError(w, r, err)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "Slack " + creds["team_id"]
	}
	h.connect(w, r, orgID, actor, channels.TypeSlack, name, creds)
}

// SlackConversations lists the channels the workspace bot was invited to, so
// operators can pick the settings.channel_ids allowlist.
func (h *Handler) SlackConversations(w http.ResponseWriter, r *http.Request) {
	a := h.providers.Slack
	if a == nil {
		unavailable(w, r)
		return
	}
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	inst, _, err := h.svc.Open(r.Context(), scope.ID, id)
	if err == nil && inst.Type != channels.TypeSlack {
		err = usecase.ErrNotFound
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, err := a.Conversations(r.Context(), inst)
	if err != nil {
		h.providerError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// GoogleChatSpaces lists the rooms the Google Chat app was added to, so
// operators can pick the settings.space_ids allowlist.
func (h *Handler) GoogleChatSpaces(w http.ResponseWriter, r *http.Request) {
	a := h.providers.GoogleChat
	if a == nil {
		unavailable(w, r)
		return
	}
	scope, id, ok := scopeAndID(w, r)
	if !ok {
		return
	}
	inst, _, err := h.svc.Open(r.Context(), scope.ID, id)
	if err == nil && inst.Type != channels.TypeGoogleChat {
		err = usecase.ErrNotFound
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, err := a.Spaces(r.Context(), inst)
	if err != nil {
		h.providerError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// EmailTest tests the supplied mailbox/sender without creating an instance.
func (h *Handler) EmailTest(w http.ResponseWriter, r *http.Request) {
	scope, ok := scopeOf(w, r)
	if !ok {
		return
	}
	if h.providers.Email == nil {
		unavailable(w, r)
		return
	}
	var req struct {
		Credentials channels.Credentials `json:"credentials"`
		Settings    map[string]any       `json:"settings"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Credentials[email.FieldProvider] != email.ProviderIMAP && req.Credentials[email.FieldProvider] != email.ProviderSMTP {
		response.ValidationError(w, r, []response.Detail{{Field: "credentials.provider", Message: "provider imap veya smtp olmalıdır"}})
		return
	}
	creds, _ := channels.ValidateCredentials(h.providers.Email, req.Credentials)
	_, err := h.providers.Email.Connect(r.Context(), channels.Instance{UUID: uuid.New(), OrganizationID: scope.ID, Credentials: creds, Settings: req.Settings})
	if err != nil {
		var restricted *email.SMTPPortRestrictionError
		if errors.As(err, &restricted) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(response.Envelope{Success: false, Data: map[string]any{"ok": false, "smtp_port_hint": email.SMTPPortHint}, Error: &response.ErrorBody{Code: response.CodeValidationError, Message: email.SMTPPortHint, Details: []response.Detail{{Field: "credentials.smtp_port", Message: email.SMTPPortHint, Code: "smtp_port_restricted"}}}, Meta: response.Meta{RequestID: response.RequestIDFunc(r.Context())}})
			return
		}
		response.ValidationError(w, r, []response.Detail{{Field: "credentials", Message: err.Error()}})
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"ok": true, "smtp_port_hint": ""})
}
