package sms

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/channelhttp"
)

// IYSConsentCheck ports takwim platform/iys.Status's MESAJ/ONAY check. The
// account is the connected channel's sealed credentials, scoped to its org.
func IYSConsentCheck(client channelhttp.Client, baseURL string) ConsentCheck {
	if client == nil {
		client = channelhttp.DefaultClient()
	}
	if baseURL == "" {
		baseURL = "https://api.iys.org.tr"
	}
	return func(ctx context.Context, inst channels.Instance, phone string) (bool, error) {
		if err := channelhttp.Required(inst, "iys_username", "iys_password", "iys_code", "iys_brand_code"); err != nil {
			return false, channels.Permanent("iys_configuration", err)
		}
		b,
			_ := json.Marshal(map[string]string{"username": inst.Credentials["iys_username"],
			"password":   inst.Credentials["iys_password"],
			"grant_type": "password"})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/oauth2/token", bytes.NewReader(b))
		if err != nil {
			return false, err
		}
		req.Header.Set("Content-Type", "application/json")
		var token struct {
			Value string `json:"access_token"`
		}
		if err := channelhttp.Do(client, req, &token); err != nil {
			return false, err
		}
		if token.Value == "" {
			return false, errors.New("İYS access token missing")
		}
		b, _ = json.Marshal(map[string]string{"recipient": phone, "recipientType": "BIREYSEL", "type": "MESAJ"})
		path := baseURL + "/sps/" + url.PathEscape(inst.Credentials["iys_code"]) +
			"/brands/" + url.PathEscape(inst.Credentials["iys_brand_code"]) + "/consents/status"
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, path, bytes.NewReader(b))
		if err != nil {
			return false, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token.Value)
		var consent struct {
			Status string `json:"status"`
		}
		if err := channelhttp.Do(client, req, &consent); err != nil {
			return false, err
		}
		return consent.Status == "ONAY", nil
	}
}
