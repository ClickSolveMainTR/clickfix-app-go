package main

import (
	"bytes"
	"context"
	"encoding/json"
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
	if err := run(context.Background(), []string{"--mode=agent"}, in, &out, &errb); err != nil {
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
