package voice

// Ported from garanti-kulucka-ecosystem/packages/database/src/call-logs.ts.
import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels"
)

var santralScenarios = map[string]bool{"Inbound_call": true,
	"Outbound_call": true,
	"Local_call":    true,
	"Answer":        true,
	"Hangup":        true,
	"Queue":         true,
	"InboundtoPBX":  true,
	"Context":       true,
	"DTMF":          true,
	"QueueLeave":    true,
	"cdr":           true}
var extensionPattern = regexp.MustCompile(`^\d{1,6}$`)
var digits = regexp.MustCompile(`\D`)

// PhoneLast10 is the shared Turkish contact matching key, not an international phone formatter.
func PhoneLast10(s string) string {
	s = digits.ReplaceAllString(s, "")
	if len(s) > 10 {
		s = s[len(s)-10:]
	}
	return s
}
func value(p map[string]any, keys ...string) string {
	for _, k := range keys {
		switch v := p[k].(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		case json.Number:
			return v.String()
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64)
		}
	}
	return ""
}
func callTime(s string) time.Time {
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if len(s) == 13 {
			return time.UnixMilli(n).UTC()
		}
		if len(s) == 10 {
			return time.Unix(n, 0).UTC()
		}
	}
	for _, layout := range []string{time.RFC3339Nano,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04",
		"02.01.2006 15:04:05"} {
		if t, err := time.ParseInLocation(layout, s, time.FixedZone("Europe/Istanbul", 3*3600)); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
func decodePayload(b []byte) (map[string]any, error) {
	p := map[string]any{}
	if bytes.HasPrefix(bytes.TrimSpace(b), []byte("{")) {
		d := json.NewDecoder(bytes.NewReader(b))
		d.UseNumber()
		err := d.Decode(&p)
		return p, err
	}
	f, err := url.ParseQuery(string(b))
	if err != nil {
		return nil, err
	}
	for k, v := range f {
		if len(v) > 0 {
			p[k] = v[0]
		}
	}
	return p, nil
}
func (a *Adapter) netgsmEvent(ctx context.Context, b []byte, inst channels.Instance) (channels.WebhookResult, error) {
	p, err := decodePayload(b)
	if err != nil {
		return channels.WebhookResult{}, errors.New("invalid santral body")
	}
	return a.normalizeNetgsm(ctx, p, inst, false)
}
func (a *Adapter) normalizeNetgsm(ctx context.Context,
	p map[string]any,
	inst channels.Instance,
	report bool) (channels.WebhookResult,
	error) {
	scenario := value(p, "scenario")
	if scenario != "" && !santralScenarios[scenario] {
		return channels.WebhookResult{}, nil
	}
	id := value(p, "unique_id", "uniqueid", "kimlik")
	if id == "" || len(id) > 180 {
		return channels.WebhookResult{}, errors.New("netgsm call id required")
	}
	// Legacy flat CDR callbacks have no scenario and use report direction codes.
	report = report || scenario == "" || (scenario == "cdr" && value(p, "yon") == "" && value(p, "direction") != "")
	code := value(p, "yon")
	direction := ""
	status := ""
	if report {
		code = value(p, "direction")
		direction = map[string]string{"0": "outbound",
			"1": "inbound",
			"2": "inbound",
			"3": "outbound",
			"4": "internal",
			"5": "internal"}[code]
		status = "completed"
		if code == "2" || code == "3" || code == "5" {
			status = "no-answer"
		}
	} else {
		direction = map[string]string{"1": "inbound", "2": "outbound"}[code]
		if direction == "" {
			direction = map[string]string{"Local_call": "internal",
				"Outbound_call": "outbound",
				"Inbound_call":  "inbound",
				"InboundtoPBX":  "inbound",
				"Queue":         "inbound"}[scenario]
		}
		status = map[string]string{"1": "completed",
			"2": "no-answer",
			"3": "busy",
			"4": "no-answer",
			"5": "failed"}[value(p,
			"sondurum")]
		if status == "" {
			status = map[string]string{"Answer": "in-progress",
				"Hangup":     "completed",
				"QueueLeave": "no-answer",
				"Queue":      "queued"}[scenario]
		}
	}
	from, to := value(p, "arayan", "source"), value(p, "aranan", "destination")
	internal := value(p, "internal_num")
	phone := value(p, "customer_num")
	if a.lookup != nil && (direction == "" || from == "" || to == "") {
		if old, e := a.lookup(ctx, inst, id); e == nil {
			if direction == "" {
				direction = old.Direction
			}
			if from == "" {
				from = old.From
			}
			if to == "" {
				to = old.To
			}
		}
	}
	if direction == "" {
		direction = "inbound"
	}
	if status == "" {
		status = "ringing"
	}
	if direction == "outbound" || direction == "internal" {
		if from == "" {
			from = internal
		}
		if to == "" {
			to = phone
		}
		if internal == "" {
			internal = from
		}
		if phone == "" {
			phone = to
		}
	} else {
		if from == "" {
			from = phone
		}
		if to == "" {
			to = internal
		}
		if internal == "" {
			internal = to
		}
		if phone == "" {
			phone = from
		}
	}
	if !extensionPattern.MatchString(internal) {
		internal = ""
	}
	if internal == "" && scenario == "Inbound_call" && extensionPattern.MatchString(value(p, "pbx_num")) {
		internal = value(p, "pbx_num")
	}
	n, _ := strconv.ParseInt(value(p, "talktime", "sure", "duration"), 10, 32)
	if n < 0 {
		n = 0
	}
	at := callTime(value(p, "timestamp", "date", "bas"))
	if at.IsZero() {
		at = time.Now().UTC()
	}
	answered := callTime(value(p, "callConnectedTime"))
	ended := callTime(value(p, "callEndedTime"))
	if scenario == "Answer" && answered.IsZero() {
		answered = at
	}
	if scenario == "Hangup" && ended.IsZero() {
		ended = at
	}
	initiated := callTime(value(p, "callInitiatedTime", "bas", "date"))
	if initiated.IsZero() && (scenario == "Inbound_call" || scenario == "Outbound_call" || scenario == "Local_call" || scenario == "InboundtoPBX") {
		initiated = at
	}
	ev := channels.CallEvent{FromReport: report, InitiatedAt: initiated,
		ProviderFinal:   report || scenario == "cdr",
		ExternalID:      id,
		Provider:        "netgsm",
		Direction:       direction,
		Status:          status,
		From:            from,
		To:              to,
		InternalNumber:  internal,
		DurationSeconds: int32(n),
		At:              at,
		AnsweredAt:      answered,
		EndedAt:         ended,
		RecordingURL: value(p,
			"seskaydi",
			"recording")}
	out := channels.WebhookResult{Calls: []channels.CallEvent{ev}}
	if direction == "internal" || PhoneLast10(phone) == "" {
		return out, nil
	}
	phone = normalizeNetgsmPhone(phone)
	canonical, _ := json.Marshal(p)
	hash := sha256.Sum256(canonical)
	text := "Gelen arama"
	if direction == "outbound" {
		text = "Giden arama"
	}
	switch status {
	case "in-progress":
		text = "Cevaplandı – dahili " + internal
	case "no-answer", "busy", "failed":
		text = "Cevapsız arama"
	case "completed":
		text = fmt.Sprintf("Arama tamamlandı (%d sn)", n)
	}
	out.Messages = []channels.InboundMessage{{ExternalID: fmt.Sprintf("call:%s:%x",
		id,
		hash),
		From: channels.Sender{ExternalID: phone,
			Phone: phone},
		Kind:     "text",
		Text:     text,
		ThreadID: id,
		At:       at,
		Attributes: map[string]any{"call_id": id,
			"call_status":      status,
			"direction":        direction,
			"internal_number":  internal,
			"duration_seconds": n,
			"scenario":         scenario,
			"queued_at": callTime(value(p,
				"callQueuedTime")),
			"pbx_number": value(p,
				"pbx_num"),
			"context": value(p,
				"context_name",
				"Context"),
			"dtmf": value(p,
				"digit",
				"Digit")}}}
	return out, nil
}

// RecordingMessage is invoked by the archive worker, never by the HTTP webhook.
func (a *Adapter) RecordingMessage(call channels.CallEvent) (channels.WebhookResult, error) {
	if err := assertRecordingURL(call.RecordingURL); err != nil {
		return channels.WebhookResult{}, err
	}
	phone := call.From
	if call.Direction == "outbound" {
		phone = call.To
	}
	phone = normalizeNetgsmPhone(phone)
	id := "recording:" + call.ExternalID
	call.RecordingMessageExternalID = id
	return channels.WebhookResult{Calls: []channels.CallEvent{call},
			Messages: []channels.InboundMessage{{ExternalID: id,
				ThreadID: call.ExternalID,
				From: channels.Sender{ExternalID: phone,
					Phone: phone},
				Kind:       "media",
				Text:       "Görüşme kaydı",
				At:         call.At,
				Attributes: map[string]any{"recording_duration_seconds": call.DurationSeconds},
				Attachments: []channels.InboundAttachment{{Filename: "recording.wav",
					ContentType: "audio/wav",
					Open:        a.netgsmRecording(call.RecordingURL)}}}}},
		nil
}
func assertRecordingURL(s string) error {
	u, e := url.Parse(s)
	if e != nil {
		return errors.New("untrusted recording URL")
	}
	trustedHost := u.Hostname() == "netgsm.com.tr" || strings.HasSuffix(u.Hostname(), ".netgsm.com.tr")
	trustedTransport := u.Scheme == "https" && u.User == nil && u.Port() == ""
	if !trustedHost || !trustedTransport {
		return errors.New("untrusted recording URL")
	}

	return nil
}
func (a *Adapter) netgsmRecording(target string) func(context.Context) (io.ReadCloser, error) {
	return func(ctx context.Context) (io.ReadCloser, error) {
		if err := assertRecordingURL(target); err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			cancel()
			return nil, errors.New("invalid recording request")
		}
		client := &http.Client{Timeout: 60 * time.Second,
			CheckRedirect: func(*http.Request,
				[]*http.Request) error {
				return http.ErrUseLastResponse
			}}
		if a.cfg.RecordingClient != nil {
			copy := *a.cfg.RecordingClient
			copy.Timeout = 60 * time.Second
			copy.CheckRedirect = client.CheckRedirect
			client = &copy
		}
		res, err := client.Do(req)
		if err != nil {
			cancel()
			return nil, errors.New("recording download failed")
		}
		defer func() { _ = res.Body.Close() }()
		defer cancel()
		mime := strings.ToLower(res.Header.Get("Content-Type"))
		if mime == "" {
			mime = "audio/wav"
		}
		if res.StatusCode != 200 || res.ContentLength > 100<<20 || !strings.HasPrefix(mime, "audio/") && !strings.HasPrefix(mime, "application/octet-stream") {
			return nil, errors.New("recording unavailable or invalid")
		}
		b, err := io.ReadAll(io.LimitReader(res.Body, (100<<20)+1))
		if err != nil || len(b) == 0 || len(b) > 100<<20 {
			return nil, errors.New("recording empty or exceeds 100 MB")
		}
		return io.NopCloser(bytes.NewReader(b)), nil
	}
}

var cdrErrors = map[string]string{"30": "Geçersiz kullanıcı veya API erişimi",
	"40":  "Kayıt bulunamadı",
	"50":  "Geçersiz tarih formatı",
	"60":  "Rapor aralığı 24 saati aşamaz",
	"70":  "Parametre hatası",
	"80":  "Sorgu sınırı aşıldı",
	"100": "Sistem hatası",
	"101": "Dakikada en fazla iki sorgu yapılabilir"}

func (a *Adapter) netgsmRequest(ctx context.Context,
	inst channels.Instance,
	path string,
	p map[string]any) ([]byte,
	error) {
	if inst.Credentials["usercode"] == "" || inst.Credentials["password"] == "" {
		return nil, errors.New("netgsm credentials required")
	}
	p["usercode"] = inst.Credentials["usercode"]
	p["password"] = inst.Credentials["password"]
	b, _ := json.Marshal(p)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.NetgsmURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, errors.New("invalid netgsm request")
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := a.cfg.Client.Do(req)
	if err != nil {
		return nil, errors.New("netgsm request failed")
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("netgsm HTTP %d", res.StatusCode)
	}
	return io.ReadAll(io.LimitReader(res.Body, 16<<20))
}

type netgsmBalance struct {
	Code     string
	Amount   string
	Packages []string
}

// parseNetgsmBalanceResponse retains decimal text to avoid rounding money through float64.
func parseNetgsmBalanceResponse(body []byte) netgsmBalance {
	out := netgsmBalance{}
	var parsed any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if !json.Valid(body) || decoder.Decode(&parsed) != nil {
		fields := strings.Fields(string(body))
		if len(fields) > 0 {
			out.Code = fields[0]
		}
		if len(fields) > 1 {
			out.Amount = strings.Join(fields[1:], " ")
		}
		return out
	}
	if p, ok := parsed.(map[string]any); ok {
		out.Code = value(p, "code")
		out.Amount = value(p, "balance", "amount")
		for _, key := range []string{"balance", "balances", "balanceinfo"} {
			items, _ := p[key].([]any)
			for _, item := range items {
				if v, ok := item.(map[string]any); ok {
					label := value(v, "balance_name", "name")
					amount := value(v, "amount")
					if label != "" && amount != "" {
						out.Packages = append(out.Packages, amount+" "+label)
					}
				}
			}
		}
	} else {
		out.Code = fmt.Sprint(parsed)
	}
	return out
}
func validNetgsmAmount(raw string) bool {
	raw = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(raw, "TL"), "TRY"), "₺"))
	if strings.Contains(raw, ",") {
		raw = strings.ReplaceAll(raw, ".", "")
		raw = strings.ReplaceAll(raw, ",", ".")
	}
	_, err := strconv.ParseFloat(raw, 64)
	return err == nil && raw != "NaN" && raw != "Inf" && raw != "-Inf"
}
func (a *Adapter) netgsmBalance(ctx context.Context, inst channels.Instance) (string, error) {
	b, err := a.netgsmRequest(ctx, inst, "/balance", map[string]any{"stip": 2})
	if err != nil {
		return "", err
	}
	credit := parseNetgsmBalanceResponse(b)
	if credit.Code != "00" && credit.Code != "0" {
		return "", fmt.Errorf("netgsm balance: %s", cdrErrors[credit.Code])
	}
	if !validNetgsmAmount(credit.Amount) {
		return "", errors.New("invalid netgsm balance")
	}
	detail := "Bakiye: " + credit.Amount + " TL"
	// Package lookup is supplementary: valid TL credit still proves authentication.
	if body, e := a.netgsmRequest(ctx, inst, "/balance", map[string]any{"stip": 1}); e == nil {
		packages := parseNetgsmBalanceResponse(body)
		if packages.Code == "00" || packages.Code == "0" {
			if len(packages.Packages) > 0 {
				detail += " · " + strings.Join(packages.Packages, ", ")
			}
		}
	}
	return detail, nil
}
func (a *Adapter) ReconcileNetgsm(ctx context.Context,
	inst channels.Instance,
	day time.Time) (channels.WebhookResult,
	error) {
	day = day.In(time.FixedZone("Europe/Istanbul", 10800))
	start := day.Format("02012006") + "0000"
	stop := day.Format("02012006") + "2359"
	b, err := a.netgsmRequest(ctx, inst, "/netsantral/report", map[string]any{"startdate": start, "stopdate": stop})
	if err != nil {
		return channels.WebhookResult{}, err
	}
	return a.parseCDR(ctx, inst, b)
}
func (a *Adapter) parseCDR(ctx context.Context, inst channels.Instance, b []byte) (channels.WebhookResult, error) {
	var raw any
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if d.Decode(&raw) != nil {
		return channels.WebhookResult{}, errors.New("invalid CDR response")
	}
	if code, ok := raw.(json.Number); ok {
		return cdrCode(code.String())
	}
	if code, ok := raw.(string); ok {
		return cdrCode(code)
	}
	if obj, ok := raw.(map[string]any); ok {
		return cdrCode(value(obj, "code"))
	}
	groups, ok := raw.([]any)
	if !ok {
		return channels.WebhookResult{}, errors.New("invalid CDR list")
	}
	out := channels.WebhookResult{}
	for _, group := range groups {
		g, ok := group.(map[string]any)
		if !ok {
			continue
		}
		rows, _ := g["values"].([]any)
		for _, row := range rows {
			p, ok := row.(map[string]any)
			if !ok {
				continue
			}
			p["uniqueid"] = g["uniqueid"]
			r, e := a.normalizeNetgsm(ctx, p, inst, true)
			if e != nil {
				return out, e
			}
			out.Calls = append(out.Calls, r.Calls...)
			out.Messages = append(out.Messages, r.Messages...)
		}
	}
	return out, nil
}
func cdrCode(code string) (channels.WebhookResult, error) {
	if code == "40" {
		return channels.WebhookResult{}, nil
	}
	msg := cdrErrors[code]
	if msg == "" {
		msg = "Geçersiz CDR yanıtı"
	}
	return channels.WebhookResult{}, fmt.Errorf("netgsm CDR %s: %s", code, msg)
}

func multipartPayload(contentType string, b []byte) (map[string]any, error) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil || params["boundary"] == "" {
		return nil, errors.New("invalid multipart form")
	}
	reader := multipart.NewReader(bytes.NewReader(b), params["boundary"])
	p := map[string]any{}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("invalid multipart form")
		}
		if part.FileName() != "" {
			_ = part.Close()
			continue
		}
		data, err := io.ReadAll(io.LimitReader(part, 65537))
		if err != nil || len(data) > 65536 {
			return nil, errors.New("form field too large")
		}
		p[part.FormName()] = string(data)
		_ = part.Close()
	}
	return p, nil
}

// Last-ten matching is separate from display normalization so foreign numbers retain their country code.
func normalizeNetgsmPhone(phone string) string {
	n := digits.ReplaceAllString(phone, "")
	if len(n) == 10 {
		return "+90" + n
	}
	if len(n) == 11 && strings.HasPrefix(n, "0") {
		return "+9" + n
	}
	if len(n) >= 8 && len(n) <= 15 {
		return "+" + strings.TrimPrefix(n, "00")
	}
	return strings.TrimSpace(phone)
}
