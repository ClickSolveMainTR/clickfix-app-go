package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRequiresMode(t *testing.T) {
	for _, args := range [][]string{{}, {"--mode=other"}} {
		var out, errb bytes.Buffer
		if err := run(context.Background(), args, strings.NewReader(""), &out, &errb); err == nil {
			t.Fatalf("%v: hata beklenirdi", args)
		}
	}
}

func TestRunVersionFlag(t *testing.T) {
	var out, errb bytes.Buffer
	if err := run(context.Background(), []string{"--version"}, strings.NewReader(""), &out, &errb); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != version {
		t.Fatalf("sürüm %q, beklenen %q", out.String(), version)
	}
}

func TestRunServesUntilEOF(t *testing.T) {
	var out, errb bytes.Buffer
	in := strings.NewReader(`{"id":1,"method":"ping"}` + "\n" + `{"id":2,"method":"version"}` + "\n")
	if err := run(context.Background(), []string{"--mode=agent", "--log-dir=" + t.TempDir()}, in, &out, &errb); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("3 satır beklenirdi (ready + 2 yanıt), gelen %d: %s", len(lines), out.String())
	}
	var ready struct {
		Event string `json:"event"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &ready); err != nil || ready.Event != "ready" {
		t.Fatalf("ilk satır ready olayı olmalı: %s", lines[0])
	}
	if !strings.Contains(out.String(), `"pong":true`) || !strings.Contains(out.String(), `"mode":"agent"`) {
		t.Fatalf("ping/version yanıtları eksik: %s", out.String())
	}
}

func TestRunSupportMode(t *testing.T) {
	t.Setenv("CLICKFIX_APP_URL", "http://localhost:1")
	dir := t.TempDir()
	var out, errb bytes.Buffer
	in := strings.NewReader(strings.Join([]string{
		`{"id":1,"method":"permissions.status"}`,
		`{"id":2,"method":"session.resolve","params":{"code":"12"}}`,
		`{"id":3,"method":"session.stop"}`,
		`{"id":4,"method":"sysinfo.get"}`,
	}, "\n") + "\n")
	if err := run(context.Background(), []string{"--mode=support", "--log-dir=" + dir}, in, &out, &errb); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{`"screen_recording":`, `"code":-32602`, `"kind":"INVALID_PARAMS"`, `"phase":"idle"`, `"app_version":"dev"`} {
		if !strings.Contains(got, want) {
			t.Errorf("%s yok:\n%s", want, got)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "sidecar.log")); err != nil {
		t.Errorf("günlük dosyası yok: %v", err)
	}
}

func TestRunSupportRejectsInsecureURL(t *testing.T) {
	t.Setenv("CLICKFIX_APP_URL", "http://example.com")
	var out, errb bytes.Buffer
	if err := run(context.Background(), []string{"--mode=support", "--log-dir="}, strings.NewReader(""), &out, &errb); err == nil {
		t.Fatal("http kök kabul edildi")
	}
}

func TestAgentModeHasNoSupportMethods(t *testing.T) {
	var out, errb bytes.Buffer
	in := strings.NewReader(`{"id":1,"method":"session.resolve","params":{"code":"123456789"}}` + "\n")
	if err := run(context.Background(), []string{"--mode=agent", "--log-dir="}, in, &out, &errb); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"code":-32601`) {
		t.Fatalf("agent modunda support yöntemi var: %s", out.String())
	}
}
