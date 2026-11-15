package mail

import "testing"

func TestIsReservedAddress(t *testing.T) {
	cases := map[string]bool{
		"user@example.com":         true,
		"User@EXAMPLE.COM":         true,
		"a@mail.example.org":       true,
		"a@example.net.":           true,
		"a@demo.test":              true,
		"a@foo.invalid":            true,
		"a@localhost":              true,
		"a@app.localhost":          true,
		"a@shop.example":           true,
		"Ad Soyad <a@example.com>": true,
		"a@example.com.tr":         false,
		"a@notexample.com":         false,
		"a@clickfix.app":           false,
		"a@testing.io":             false,
		"no-domain":                false,
		"a@":                       false,
		"":                         false,
	}
	for addr, want := range cases {
		if got := IsReservedAddress(addr); got != want {
			t.Errorf("IsReservedAddress(%q) = %v, want %v", addr, got, want)
		}
	}
}
