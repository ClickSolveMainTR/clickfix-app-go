package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// Embedded Signup (Tech Provider): a business signs in with Facebook in the
// panel (FB.login with SignupConfig), picks or creates its WhatsApp
// Business Account and number, and the browser hands ClickFix a short-lived
// code with the WABA and phone number ids. The code is exchanged for a
// business integration token, ClickFix's app is subscribed to the WABA's
// webhooks and the number is registered for the Cloud API.

// SignupSettings is what the frontend needs to launch Embedded Signup.
type SignupSettings struct {
	AppID      string `json:"app_id"`
	ConfigID   string `json:"config_id"`
	APIVersion string `json:"api_version"`
}

// SignupConfig answers the public Embedded Signup settings.
func (a *Adapter) SignupConfig() SignupSettings {
	return SignupSettings{AppID: a.cfg.AppID, ConfigID: a.cfg.ConfigID, APIVersion: a.cfg.APIVersion}
}

// ExchangeCode turns an Embedded Signup code into the business integration
// token.
func (a *Adapter) ExchangeCode(ctx context.Context, code string) (string, error) {
	if a.cfg.AppID == "" || a.cfg.AppSecret == "" {
		return "", errors.New("whatsapp: Meta app is not configured")
	}
	if strings.TrimSpace(code) == "" {
		return "", errors.New("whatsapp: code is required")
	}
	q := url.Values{"client_id": {a.cfg.AppID}, "client_secret": {a.cfg.AppSecret}, "code": {code}}
	body, err := a.call(ctx, http.MethodGet, "oauth/access_token", q, nil, "")
	if err != nil {
		return "", err
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &out); err != nil || strings.TrimSpace(out.AccessToken) == "" {
		return "", &Error{Status: http.StatusOK, Message: "answer without an access token"}
	}
	return out.AccessToken, nil
}

// PhoneNumbers lists the numbers of a WABA the token can see.
func (a *Adapter) PhoneNumbers(ctx context.Context, token, wabaID string) ([]PhoneNumber, error) {
	if !ValidID(wabaID) {
		return nil, errors.New("whatsapp: invalid waba_id")
	}
	q := url.Values{"fields": {"id,display_phone_number,verified_name,quality_rating"}}
	body, err := a.call(ctx, http.MethodGet, wabaID+"/phone_numbers", q, nil, token)
	if err != nil {
		return nil, err
	}
	var out struct {
		Data []PhoneNumber `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, &Error{Status: http.StatusOK, Message: "unreadable phone number list"}
	}
	return out.Data, nil
}

// SubscribeApp subscribes ClickFix's app to the WABA's webhooks.
func (a *Adapter) SubscribeApp(ctx context.Context, token, wabaID string) error {
	if !ValidID(wabaID) {
		return errors.New("whatsapp: invalid waba_id")
	}
	_, err := a.call(ctx, http.MethodPost, wabaID+"/subscribed_apps", nil, nil, token)
	return err
}

// RegisterNumber registers the number for the Cloud API with its two-step
// verification pin (6 digits). A number shared with the WhatsApp Business
// app (coexistence) is not registered.
func (a *Adapter) RegisterNumber(ctx context.Context, token, phoneNumberID, pin string) error {
	if !ValidID(phoneNumberID) {
		return errors.New("whatsapp: invalid phone_number_id")
	}
	if len(pin) != 6 || digits(pin) != pin {
		return errors.New("whatsapp: pin must be 6 digits")
	}
	_, err := a.call(ctx, http.MethodPost, phoneNumberID+"/register", nil,
		map[string]any{"messaging_product": "whatsapp", "pin": pin}, token)
	return err
}

// SignupUserID resolves the subject bound to a business integration token.
// Store this mapping so an app-scoped deauthorization can find its instances.
func (a *Adapter) SignupUserID(ctx context.Context, token string) (string, error) {
	body, err := a.call(ctx, http.MethodGet, "debug_token", url.Values{"input_token": {token}}, nil, a.cfg.AppID+"|"+a.cfg.AppSecret)
	if err != nil {
		return "", err
	}
	var out struct {
		Data struct {
			UserID string `json:"user_id"`
			AppID  string `json:"app_id"`
			Valid  bool   `json:"is_valid"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &out) != nil || !out.Data.Valid || out.Data.AppID != a.cfg.AppID || !ValidID(out.Data.UserID) {
		return "", errors.New("whatsapp: invalid signup token subject")
	}
	return out.Data.UserID, nil
}
