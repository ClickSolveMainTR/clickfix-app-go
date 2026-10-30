package usecase_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/xml"
	"math/big"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/crewjam/saml"
	"github.com/crewjam/saml/testsaml"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sso/repository"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/sso/usecase"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/crypto"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/jwt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSAMLSignatureAudienceAndRequestBinding(t *testing.T) {
	rawDB := os.Getenv("TEST_DATABASE_URL")
	if rawDB == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	pool, e := pgxpool.New(ctx, rawDB)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	var org int64
	var orgUUID uuid.UUID
	if e = pool.QueryRow(ctx, "INSERT INTO organizations(slug,name) VALUES($1,'SAML') RETURNING id,uuid", "saml-"+uuid.NewString()[:12]).Scan(&org, &orgUUID); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "DELETE FROM organizations WHERE id=$1", org) })
	box, e := crypto.NewSecretBox(strings.Repeat("x", 32))
	if e != nil {
		t.Fatal(e)
	}
	tokens, e := jwt.NewManager("saml-test-secret", time.Minute, time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	s := &usecase.Service{Repo: repository.New(pool, db.New(pool)), Box: box, Tokens: tokens, Entitlements: entitled{}, BaseURL: "https://support.example.test/api"}
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Test IdP"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
	der, e := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	cert, e := x509.ParseCertificate(der)
	if e != nil {
		t.Fatal(e)
	}
	metadataURL, _ := url.Parse("https://idp.example.test/metadata")
	ssoURL, _ := url.Parse("https://idp.example.test/sso")
	idp := &saml.IdentityProvider{Key: key, Certificate: cert, MetadataURL: *metadataURL, SSOURL: *ssoURL}
	metadata, e := xml.Marshal(idp.Metadata())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Save(ctx, org, usecase.Config{Protocol: "saml", Enabled: true, SAMLMetadata: string(metadata)}); e != nil {
		t.Fatal(e)
	}
	in := usecase.SCIMUser{ExternalID: "saml-subject", UserName: uuid.NewString() + "@example.test"}
	if _, e = s.Provision(ctx, org, in); e != nil {
		t.Fatal(e)
	}
	spRaw, e := s.Metadata(orgUUID)
	if e != nil {
		t.Fatal(e)
	}
	var sp saml.EntityDescriptor
	if e = xml.Unmarshal(spRaw, &sp); e != nil {
		t.Fatal(e)
	}
	makeResponse := func(mutate bool) (*url.URL, string) {
		start, e := s.Start(ctx, orgUUID, "192.0.2.1:1")
		if e != nil {
			t.Fatal(e)
		}
		target, e := url.Parse(start.AuthorizationURL)
		if e != nil {
			t.Fatal(e)
		}
		raw, e := testsaml.ParseRedirectRequest(target)
		if e != nil {
			t.Fatal(e)
		}
		var auth saml.AuthnRequest
		if e = xml.Unmarshal(raw, &auth); e != nil {
			t.Fatal(e)
		}
		req := &saml.IdpAuthnRequest{IDP: idp, HTTPRequest: httptest.NewRequest("GET", "https://idp.example.test/sso", nil), Request: auth, ServiceProviderMetadata: &sp, SPSSODescriptor: &sp.SPSSODescriptors[0], ACSEndpoint: &sp.SPSSODescriptors[0].AssertionConsumerServices[0], Now: time.Now()}
		if e = (saml.DefaultAssertionMaker{}).MakeAssertion(req, &saml.Session{ID: uuid.NewString(), CreateTime: time.Now(), ExpireTime: time.Now().Add(time.Minute), NameID: "saml-subject", NameIDFormat: string(saml.PersistentNameIDFormat)}); e != nil {
			t.Fatal(e)
		}
		if mutate {
			req.Assertion.Conditions.AudienceRestrictions[0].Audience.Value = "https://another-service.example.test"
		}
		form, e := req.PostBinding()
		if e != nil {
			t.Fatal(e)
		}
		u, e := url.Parse(form.URL)
		if e != nil {
			t.Fatal(e)
		}
		u.RawQuery = url.Values{"response": {form.SAMLResponse}, "state": {start.State}}.Encode()
		return u, start.State
	}
	for _, tc := range []struct {
		name          string
		wrongAudience bool
	}{{"valid signed assertion", false}, {"signed wrong audience", true}} {
		t.Run(tc.name, func(t *testing.T) {
			u, state := makeResponse(tc.wrongAudience)
			values := url.Values{"SAMLResponse": {u.Query().Get("response")}, "RelayState": {state}}
			r := httptest.NewRequest("POST", u.Scheme+"://"+u.Host+u.Path, strings.NewReader(values.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if e = r.ParseForm(); e != nil {
				t.Fatal(e)
			}
			out, e := s.Complete(ctx, orgUUID, state, "", "saml", "192.0.2.1:1", r)
			if tc.wrongAudience {
				if e == nil {
					t.Fatal("wrong audience accepted")
				}
				return
			}
			if e != nil || out.AccessToken == "" {
				t.Fatalf("signed callback: %v", e)
			}
			if _, e = s.Complete(ctx, orgUUID, state, "", "saml", "192.0.2.1:1", r); e == nil {
				t.Fatal("SAML replay accepted")
			}
		})
	}
}
