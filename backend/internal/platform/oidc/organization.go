package oidc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/netguard"
)

// OrganizationClient blocks private/link-local IdP endpoints and redirects,
// including DNS rebinding, when an organization supplies OIDC configuration.
func OrganizationClient() *http.Client {
	d := &net.Dialer{Timeout: 5 * time.Second}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			if !netguard.IsPublic(ip) {
				return nil, fmt.Errorf("oidc: private endpoint forbidden")
			}
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("oidc: no endpoint address")
		}
		return d.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}
	return &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// ExchangeOrganizationCode performs authorization-code + PKCE exchange. The
// returned id_token must still pass Verifier.Verify with the stored nonce.
func ExchangeOrganizationCode(ctx context.Context, client *http.Client, endpoint, clientID, secret, code, redirect, verifier string) (string, error) {
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirect}, "client_id": {clientID}, "code_verifier": {verifier}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if secret != "" {
		req.SetBasicAuth(clientID, secret)
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != 200 {
		return "", ErrInvalidToken
	}
	var out struct {
		IDToken string `json:"id_token"`
	}
	if err = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&out); err != nil || out.IDToken == "" {
		return "", ErrInvalidToken
	}
	return out.IDToken, nil
}
