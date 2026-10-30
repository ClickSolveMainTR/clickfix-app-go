package clientip

import (
	"net/http/httptest"
	"testing"
)

func TestFromRequest(t *testing.T) {
	cases := []struct {
		remote, xff, want string
	}{
		{"10.0.1.5:41234", "", "10.0.1.5"},
		{"10.0.1.5:41234", "203.0.113.7", "203.0.113.7"},
		{"10.0.1.5:41234", " 203.0.113.7 , 10.0.1.5", "203.0.113.7"},
		{"10.0.1.5:41234", " , ", "10.0.1.5"},
		{"bad-remote", "", "bad-remote"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.remote
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := FromRequest(r); got != c.want {
			t.Errorf("FromRequest(remote=%q, xff=%q) = %q, want %q", c.remote, c.xff, got, c.want)
		}
	}
}

// TestDistinctClientsBehindBFF: two browsers proxied by the same BFF get
// separate rate-limit subjects (CF-153: survey, status and booking limits
// used to share the BFF's address, i.e. one global bucket).
func TestDistinctClientsBehindBFF(t *testing.T) {
	a := httptest.NewRequest("POST", "/v1/public/booking/acme/demo", nil)
	b := httptest.NewRequest("POST", "/v1/public/booking/acme/demo", nil)
	a.RemoteAddr, b.RemoteAddr = "172.18.0.4:5000", "172.18.0.4:5001"
	a.Header.Set("X-Forwarded-For", "198.51.100.10")
	b.Header.Set("X-Forwarded-For", "198.51.100.11")
	if FromRequest(a) == FromRequest(b) {
		t.Fatal("clients behind the BFF share one rate-limit subject")
	}
}
