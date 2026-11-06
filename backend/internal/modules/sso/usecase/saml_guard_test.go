package usecase

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"testing"

	"github.com/crewjam/saml"
)

func samlRequest(body string) *http.Request {
	return &http.Request{PostForm: url.Values{"SAMLResponse": {base64.StdEncoding.EncodeToString([]byte(body))}}}
}

func TestCheckSAMLResponse(t *testing.T) {
	const head = `<samlp:Response xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol" xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion">`
	const tail = `</samlp:Response>`
	cases := []struct {
		name string
		body string
		ok   bool
	}{
		{"valid", head + `<saml:Assertion><saml:Issuer>idp</saml:Issuer><saml:Subject><saml:NameID>u</saml:NameID></saml:Subject></saml:Assertion>` + tail, true},
		{"default namespace", `<Response xmlns="urn:oasis:names:tc:SAML:2.0:protocol"><Assertion xmlns="urn:oasis:names:tc:SAML:2.0:assertion"><Issuer>idp</Issuer><Subject><NameID>u</NameID></Subject></Assertion></Response>`, true},
		{"missing issuer", head + `<saml:Assertion><saml:Subject><saml:NameID>u</saml:NameID></saml:Subject></saml:Assertion>` + tail, false},
		{"foreign issuer", head + `<saml:Assertion><x:Issuer xmlns:x="urn:evil">idp</x:Issuer><saml:Subject><saml:NameID>u</saml:NameID></saml:Subject></saml:Assertion>` + tail, false},
		{"foreign name id", head + `<saml:Assertion><saml:Issuer>idp</saml:Issuer><saml:Subject><x:NameID xmlns:x="urn:evil">admin</x:NameID></saml:Subject></saml:Assertion>` + tail, false},
		{"unqualified name id", head + `<saml:Assertion><saml:Issuer>idp</saml:Issuer><saml:Subject><NameID>admin</NameID></saml:Subject></saml:Assertion>` + tail, false},
		{"foreign and real name id", head + `<saml:Assertion><saml:Issuer>idp</saml:Issuer><saml:Subject><x:NameID xmlns:x="urn:evil">admin</x:NameID><saml:NameID>u</saml:NameID></saml:Subject></saml:Assertion>` + tail, false},
		{"foreign subject", head + `<saml:Assertion><saml:Issuer>idp</saml:Issuer><x:Subject xmlns:x="urn:evil"><saml:NameID>u</saml:NameID></x:Subject></saml:Assertion>` + tail, false},
		{"foreign assertion", head + `<x:Assertion xmlns:x="urn:evil"><saml:Issuer>idp</saml:Issuer></x:Assertion>` + tail, false},
		{"second assertion without issuer", head + `<saml:Assertion><saml:Issuer>idp</saml:Issuer></saml:Assertion><saml:Assertion><saml:Subject><saml:NameID>u</saml:NameID></saml:Subject></saml:Assertion>` + tail, false},
		{"encrypted assertion", head + `<saml:EncryptedAssertion/>` + tail, false},
		{"no assertion", head + tail, false},
		{"not xml", "nope", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if e := checkSAMLResponse(samlRequest(tc.body)); (e == nil) != tc.ok {
				t.Fatalf("ok=%v, got %v", tc.ok, e)
			}
		})
	}
	if checkSAMLResponse(&http.Request{PostForm: url.Values{"SAMLResponse": {"%%%"}}}) == nil {
		t.Fatal("invalid base64 accepted")
	}
	if checkSAMLResponse(&http.Request{}) == nil {
		t.Fatal("empty response accepted")
	}
}

func TestParseSAMLResponseRecoversPanic(t *testing.T) {
	// A nil provider makes the library call panic; the login must be denied, not crash.
	var sp *saml.ServiceProvider
	r := samlRequest(`<samlp:Response xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol" xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion"><saml:Assertion><saml:Issuer>idp</saml:Issuer></saml:Assertion></samlp:Response>`)
	if a, e := parseSAMLResponse(sp, r, nil); e == nil || a != nil {
		t.Fatalf("got %v, %v", a, e)
	}
}
