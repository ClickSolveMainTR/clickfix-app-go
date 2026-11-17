package logging

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	jwt    = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJyczpjdXN0b21lciJ9.c2lnbmF0dXJlLXNpZ24"
	secret = "Zm9vYmFyYmF6cXV4LXNlY3JldC0yNTYtYml0LXZhbHVl"
)

func TestScrub(t *testing.T) {
	cases := []struct{ in, mustNot string }{
		{"token " + jwt + " alındı", jwt},
		{"wss://lk.example/rtc?access_token=" + jwt + "&auto_subscribe=1", jwt},
		{"Authorization: Bearer " + secret, secret},
		{"clickfix-support://join?t=" + secret, secret},
		{`{"client_secret":"` + secret + `","view":true}`, secret},
		{"kod 123456789 girildi", "123456789"},
		{"kod 123 456 789 girildi", "123 456 789"},
		{"kod 123-456-789", "123-456-789"},
		{"123456789", "123456789"},
	}
	for _, c := range cases {
		got := Scrub(c.in)
		if strings.Contains(got, c.mustNot) {
			t.Errorf("Scrub(%q) = %q, sır görünüyor", c.in, got)
		}
		if !strings.Contains(got, Mask) {
			t.Errorf("Scrub(%q) = %q, maske yok", c.in, got)
		}
	}
	// Sır olmayan sayılar ve metin dokunulmaz.
	for _, s := range []string{"1920x1080 ekran", "süre 12345 ms", "oturum 1234567890123 ms", "rs_6f1c"} {
		if got := Scrub(s); got != s {
			t.Errorf("Scrub(%q) = %q, değişmemeliydi", s, got)
		}
	}
}

func TestLoggerMasksEverywhere(t *testing.T) {
	dir := t.TempDir()
	var stderr bytes.Buffer
	log, closer, err := New(Options{Dir: dir, Stderr: &stderr})
	if err != nil {
		t.Fatal(err)
	}
	type grant struct {
		URL   string
		Token string
	}
	log.Info("çözüldü kod=987654321", "code", "987654321", "client_secret", secret)
	log.Info("katılma", "link", "clickfix-support://join?t="+secret, "session", "6f1c")
	log.With("token", jwt).Info("bağlandı", slog.Group("lk", "access_token", jwt, "room", "rs_1"))
	log.Warn("istek başarısız", "url", "https://x/rtc?access_token="+jwt, "err", errors.New("401 Bearer "+secret))
	log.Info("yapı", "grant", grant{URL: "wss://lk", Token: jwt})
	log.Info(fmt.Sprintf("ham yanıt {\"client_secret\":\"%s\"}", secret))
	_ = closer.Close()

	file, err := os.ReadFile(filepath.Join(dir, "sidecar.log"))
	if err != nil {
		t.Fatal(err)
	}
	for name, out := range map[string]string{"stderr": stderr.String(), "dosya": string(file)} {
		for _, s := range []string{jwt, secret, "987654321"} {
			if strings.Contains(out, s) {
				t.Errorf("%s günlüğünde sır var (%s):\n%s", name, s, out)
			}
		}
		if !strings.Contains(out, "session=6f1c") || !strings.Contains(out, "room=rs_1") {
			t.Errorf("%s günlüğünde hassas olmayan alanlar kayboldu:\n%s", name, out)
		}
	}
}

func TestRotatingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.log")
	r, err := OpenRotating(path, 100, 2)
	if err != nil {
		t.Fatal(err)
	}
	line := bytes.Repeat([]byte("x"), 39)
	line = append(line, '\n')
	for range 10 {
		if _, err := r.Write(line); err != nil {
			t.Fatal(err)
		}
	}
	_ = r.Close()
	for _, p := range []string{path, path + ".1", path + ".2"} {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatalf("%s yok: %v", p, err)
		}
		if st.Size() > 100 {
			t.Errorf("%s boyutu %d, sınır 100", p, st.Size())
		}
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Errorf("en çok 2 yedek tutulmalı")
	}
}
