package diagnostics

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTechnicalContextMasking(t *testing.T) {
	in := Context{URL: "https://person:pass@shop.test/orders/ada@example.com?token=secret#email", ConsoleErrors: []string{"Ada ada@example.com +90 555 123 45 67 token=secret Authorization: Bearer abcde12345 TR330006100519786457841326 10000000146 https://shop.test/private/id?key=abc"}, NetworkFailures: []NetworkFailure{{URL: "https://api.test/users/123?key=secret", Method: "GET", Status: 500, Error: "Bearer abcdef"}}, Metadata: map[string]any{"password": "visible", "nested": map[string]any{"email": "ada@example.com", "release": "v1"}}}
	out, err := in.Mask()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	for _, secret := range []string{"ada@example.com", "555", "abcdef", "token=secret", "/orders", "/users", "person:pass", "visible", "abcde12345", "TR330006100519786457841326", "10000000146"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("leaked %s in %s", secret, raw)
		}
	}
	if out.URL != "https://shop.test" {
		t.Fatalf("url=%s", out.URL)
	}
}
func TestContextBounds(t *testing.T) {
	if _, err := (Context{ConsoleErrors: make([]string, 51)}).Mask(); err == nil {
		t.Fatal("unbounded logs")
	}
	if _, err := (Context{NetworkFailures: []NetworkFailure{{Method: "FETCH"}}}).Mask(); err == nil {
		t.Fatal("invalid method")
	}
}
func element(id []byte, payload []byte) []byte {
	return append(append(append([]byte{}, id...), byte(0x80|len(payload))), payload...)
}
func recording(timestamp uint16) []byte {
	header := element([]byte{0x1a, 0x45, 0xdf, 0xa3}, element([]byte{0x42, 0x82}, []byte("webm")))
	tracks := element([]byte{0x16, 0x54, 0xae, 0x6b}, element([]byte{0xae}, element([]byte{0x83}, []byte{1})))
	cluster := element([]byte{0x1f, 0x43, 0xb6, 0x75}, append(element([]byte{0xe7}, []byte{byte(timestamp >> 8), byte(timestamp)}), element([]byte{0xa3}, []byte{0x81, 0, 0, 0, 1})...))
	return append(header, element([]byte{0x18, 0x53, 0x80, 0x67}, append(tracks, cluster...))...)
}
func TestRecordingCapsUseContainerTimestamps(t *testing.T) {
	if duration, err := WebMDuration(recording(30000)); err != nil || duration != 30000 {
		t.Fatalf("duration=%d err=%v", duration, err)
	}
	if _, err := WebMDuration(recording(60001)); err == nil {
		t.Fatal("accepted >60s")
	}
	for _, v := range [][]byte{nil, []byte("not video"), recording(30000)[:10]} {
		if _, err := WebMDuration(v); err == nil {
			t.Fatal("accepted malformed recording")
		}
	}
}

func TestRecordingRejectsLongBlockDuration(t *testing.T) {
	header := element([]byte{0x1a, 0x45, 0xdf, 0xa3}, element([]byte{0x42, 0x82}, []byte("webm")))
	tracks := element([]byte{0x16, 0x54, 0xae, 0x6b}, element([]byte{0xae}, element([]byte{0x83}, []byte{1})))
	group := element([]byte{0xa0}, append(element([]byte{0xa1}, []byte{0x81, 0, 0, 0, 1}), element([]byte{0x9b}, []byte{0xea, 0x61})...))
	cluster := element([]byte{0x1f, 0x43, 0xb6, 0x75}, append(element([]byte{0xe7}, []byte{0}), group...))
	data := append(header, element([]byte{0x18, 0x53, 0x80, 0x67}, append(tracks, cluster...))...)
	if _, err := WebMDuration(data); err == nil {
		t.Fatal("accepted block duration over 60 seconds")
	}
}

func TestQuotedCredentialsAndMetadataIdentifiersAreRedacted(t *testing.T) {
	in := Context{ConsoleErrors: []string{`{"password":"never-persist-this","name":"Ada Lovelace","access_token":"private-token"}`}, Metadata: map[string]any{"user_id": 12345.0, "customer_id": "customer-reference", "value": 12345678901.0}}
	out, err := in.Mask()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	for _, value := range []string{"never-persist-this", "Ada Lovelace", "private-token", "12345", "customer-reference", "12345678901"} {
		if strings.Contains(string(raw), value) {
			t.Fatalf("PII escaped: %s", raw)
		}
	}
}

func TestDurationlessMediaRecorderClusters(t *testing.T) {
	header := element([]byte{0x1a, 0x45, 0xdf, 0xa3}, element([]byte{0x42, 0x82}, []byte("webm")))
	tracks := element([]byte{0x16, 0x54, 0xae, 0x6b}, element([]byte{0xae}, element([]byte{0x83}, []byte{1})))
	data := append(header, []byte{0x18, 0x53, 0x80, 0x67, 0xff}...)
	data = append(data, tracks...)
	for i := 0; i < 10; i++ {
		timestamp := i * 5000
		data = append(data, []byte{0x1f, 0x43, 0xb6, 0x75, 0xff}...)
		data = append(data, element([]byte{0xe7}, []byte{byte(timestamp >> 8), byte(timestamp)})...)
		data = append(data, element([]byte{0xa3}, []byte{0x81, 0, 0, 0, 1})...)
	}
	if duration, err := WebMDuration(data); err != nil || duration != 45000 {
		t.Fatalf("duration=%d err=%v", duration, err)
	}
}
