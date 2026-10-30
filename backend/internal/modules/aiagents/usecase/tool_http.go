package usecase

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/aiagents/model"
	"github.com/google/uuid"
)

func toolURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || len(raw) > 2000 {
		return nil, model.ErrInvalid
	}
	if u.Port() != "" && u.Port() != "443" {
		return nil, model.ErrInvalid
	}
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil && !publicToolIP(ip) {
		return nil, model.ErrInvalid
	}
	return u, nil
}
func publicToolIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, prefix := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "2001::/32", "2002::/16", "64:ff9b::/96", "64:ff9b:1::/48"} {
		if netip.MustParsePrefix(prefix).Contains(ip) {
			return false
		}
	}
	return true
}

// invokeHTTP pins a validated public address per dial, preserves TLS hostname
// verification, ignores proxy environment variables and never follows redirects.
// A failed or ambiguous remote action is recorded and never replayed silently.
func invokeHTTP(ctx context.Context, target string, secret toolAuth, invocation uuid.UUID, payload any) (string, error) {
	u, err := toolURL(target)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(payload)
	if err != nil || len(body) > 32<<10 {
		return "", model.ErrInvalid
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, ResponseHeaderTimeout: 10 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil || !strings.EqualFold(host, u.Hostname()) || port != "443" {
				return nil, model.ErrInvalid
			}
			ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil || len(ips) == 0 {
				return nil, errors.New("tool: DNS unavailable")
			}
			for _, ip := range ips {
				if !publicToolIP(ip) {
					return nil, model.ErrInvalid
				}
			}
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	timestamp := time.Now().UTC().Format(time.RFC3339)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Idempotency-Key", invocation.String())
	request.Header.Set("X-Webhook-Event", "ai.custom_tool.invoke")
	request.Header.Set("X-Webhook-Timestamp", timestamp)
	if secret.Bearer != "" {
		request.Header.Set("Authorization", "Bearer "+secret.Bearer)
	}
	if secret.HMAC != "" {
		mac := hmac.New(sha256.New, []byte(secret.HMAC))
		_, _ = mac.Write([]byte(timestamp + "." + string(body)))
		request.Header.Set("X-Webhook-Signature", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	}
	response, err := client.Do(request)
	if err != nil {
		return "", errors.New("tool: remote outcome unknown")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", errors.New("tool: HTTP rejected")
	}
	result, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	if err != nil || len(result) > 4096 {
		return "", errors.New("tool: invalid or oversized response")
	}
	return string(result), nil
}
