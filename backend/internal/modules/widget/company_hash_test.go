package widget_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/channels/widget"
)

// companyOf returns the contact's company (external id, verified) by the
// contact's external id; ok is false when the contact has no company.
func companyOf(t *testing.T, e *env, userID string) (extID string, verified, ok bool) {
	t.Helper()
	var ext *string
	err := e.pool.QueryRow(context.Background(), `SELECT co.external_id, co.verified_at IS NOT NULL
		FROM contacts c JOIN companies co ON co.id = c.company_id
		WHERE c.organization_id = $1 AND c.external_id = $2 AND c.deleted_at IS NULL`, e.org, userID).Scan(&ext, &verified)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return "", false, false
		}
		t.Fatal(err)
	}
	if ext != nil {
		extID = *ext
	}
	return extID, verified, true
}

func companyCount(t *testing.T, e *env, where string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), "SELECT count(*) FROM companies WHERE organization_id = $1 AND deleted_at IS NULL AND "+where, append([]any{e.org}, args...)...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func identify(e *env, body map[string]any) reply {
	token := str(e.call(http.MethodPost, "/bootstrap", "", siteOrigin, nil).Data, "visitor", "token")
	return e.call(http.MethodPost, "/identify", token, siteOrigin, body)
}

// CF-206: user_hash signs user_id and company.id together; the proved id
// creates or matches the company, marks it verified and binds the contact
// to it, even away from a company it was attached to without proof.
func TestIdentifySignedCompanyBinds(t *testing.T) {
	e := setup(t)
	hint := identify(e, map[string]any{
		"user_id": "u-10", "user_hash": widget.UserHash(identitySecret, "u-10"),
		"company": map[string]any{"name": "Eski Firma", "domain": "old.test"},
	})
	if hint.Code != http.StatusOK || hint.Data["company_verified"] != false {
		t.Fatalf("identify with a company hint = %d %v %v", hint.Code, hint.Err, hint.Data["company_verified"])
	}
	if ext, verified, ok := companyOf(t, e, "u-10"); !ok || ext != "" || verified {
		t.Fatalf("company by hint = %q verified=%v ok=%v", ext, verified, ok)
	}
	signed := identify(e, map[string]any{
		"user_id": "u-10", "user_hash": widget.UserCompanyHash(identitySecret, "u-10", "F-10"),
		"company": map[string]any{"id": "F-10", "name": "Acme A.Ş.", "domain": "acme.test"},
	})
	if signed.Code != http.StatusOK || signed.Data["verified"] != true || signed.Data["company_verified"] != true {
		t.Fatalf("signed identify = %d %v %v", signed.Code, signed.Err, signed.Data)
	}
	if ext, verified, ok := companyOf(t, e, "u-10"); !ok || ext != "F-10" || !verified {
		t.Fatalf("signed company = %q verified=%v ok=%v", ext, verified, ok)
	}
	if n := companyCount(t, e, "external_id = 'F-10' AND name = 'Acme A.Ş.' AND domain = 'acme.test'"); n != 1 {
		t.Fatalf("companies with the proved id = %d", n)
	}
	// The next bootstrap of the user reports the proved company too.
	boot := e.call(http.MethodPost, "/bootstrap", str(signed.Data, "token"), siteOrigin, nil)
	if boot.Code != http.StatusOK || str(boot.Data, "visitor", "id") == "" {
		t.Fatalf("bootstrap = %d %v", boot.Code, boot.Err)
	}
	if v, _ := boot.Data["visitor"].(map[string]any); v["company_verified"] != true {
		t.Fatalf("bootstrap visitor = %v", boot.Data["visitor"])
	}
}

// CF-206: a company that exists by domain without an external id takes the
// proved id instead of a duplicate being created.
func TestIdentifySignedCompanyAdoptsDomainMatch(t *testing.T) {
	e := setup(t)
	if _, err := e.pool.Exec(context.Background(), "INSERT INTO companies (organization_id, name, domain) VALUES ($1, 'Acme', 'acme.test')", e.org); err != nil {
		t.Fatal(err)
	}
	r := identify(e, map[string]any{
		"user_id": "u-11", "user_hash": widget.UserCompanyHash(identitySecret, "u-11", "F-11"),
		"company": map[string]any{"id": "F-11", "domain": "acme.test"},
	})
	if r.Code != http.StatusOK || r.Data["company_verified"] != true {
		t.Fatalf("identify = %d %v", r.Code, r.Err)
	}
	if n := companyCount(t, e, "domain = 'acme.test'"); n != 1 {
		t.Fatalf("companies for the domain = %d (duplicate created)", n)
	}
	if ext, verified, ok := companyOf(t, e, "u-11"); !ok || ext != "F-11" || !verified {
		t.Fatalf("company = %q verified=%v ok=%v", ext, verified, ok)
	}
}

// CF-206: a hash that signs another company id is rejected like any wrong
// hash; nothing is created or linked.
func TestIdentifyWrongCompanyRejected(t *testing.T) {
	e := setup(t)
	r := identify(e, map[string]any{
		"user_id": "u-12", "user_hash": widget.UserCompanyHash(identitySecret, "u-12", "F-12"),
		"company": map[string]any{"id": "F-99", "name": "Başkası"},
	})
	if r.Code != http.StatusUnauthorized || str(r.Err, "code") != "WIDGET_IDENTITY_INVALID" {
		t.Fatalf("other company id = %d %v", r.Code, r.Err)
	}
	// A company-signed hash is not a user-only hash either.
	if r := identify(e, map[string]any{"user_id": "u-12", "user_hash": widget.UserCompanyHash(identitySecret, "u-12", "F-12")}); r.Code != http.StatusUnauthorized {
		t.Fatalf("company hash without company = %d", r.Code)
	}
	if n := companyCount(t, e, "true"); n != 0 {
		t.Fatalf("companies after rejected calls = %d", n)
	}
	if _, _, ok := companyOf(t, e, "u-12"); ok {
		t.Fatal("rejected user must not get a contact with a company")
	}
	// Line breaks cannot be signed: the ids are separated by one.
	if r := identify(e, map[string]any{"user_id": "u-12", "user_hash": widget.UserHash(identitySecret, "u-12"), "company": map[string]any{"id": "F-12\nF-13"}}); r.Code != http.StatusBadRequest {
		t.Fatalf("newline in company id = %d", r.Code)
	}
}

// CF-206: the original formula keeps working. Without a company id the
// user is verified as before; with an unsigned company id the user is
// still verified but the id is a claim: it binds no company by external
// id and is not stored, while name and domain stay hints.
func TestIdentifyLegacyHashAndUnsignedCompanyClaim(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if _, err := e.pool.Exec(ctx, "INSERT INTO companies (organization_id, name, external_id) VALUES ($1, 'Gizli Firma', 'F-13')", e.org); err != nil {
		t.Fatal(err)
	}
	plain := identify(e, map[string]any{"user_id": "u-13", "user_hash": widget.UserHash(identitySecret, "u-13"), "name": "Ada"})
	if plain.Code != http.StatusOK || plain.Data["verified"] != true || plain.Data["company_verified"] != false {
		t.Fatalf("legacy identify = %d %v %v", plain.Code, plain.Err, plain.Data)
	}
	// Claiming the hidden company's id with a user-only hash: verified user, no company.
	claim := identify(e, map[string]any{
		"user_id": "u-13", "user_hash": widget.UserHash(identitySecret, "u-13"),
		"company": map[string]any{"id": "F-13"},
	})
	if claim.Code != http.StatusOK || claim.Data["verified"] != true || claim.Data["company_verified"] != false {
		t.Fatalf("unsigned claim = %d %v %v", claim.Code, claim.Err, claim.Data)
	}
	if _, _, ok := companyOf(t, e, "u-13"); ok {
		t.Fatal("an unsigned company id must not bind the contact to the company")
	}
	if n := companyCount(t, e, "external_id = 'F-13' AND verified_at IS NOT NULL"); n != 0 {
		t.Fatal("an unsigned claim must not verify the company")
	}
	// With name and domain the hint still creates/matches a company, without the id.
	hint := identify(e, map[string]any{
		"user_id": "u-13", "user_hash": widget.UserHash(identitySecret, "u-13"),
		"company": map[string]any{"id": "F-13", "name": "Beta Ltd.", "domain": "beta.test"},
	})
	if hint.Code != http.StatusOK || hint.Data["company_verified"] != false {
		t.Fatalf("hint identify = %d %v", hint.Code, hint.Err)
	}
	if ext, verified, ok := companyOf(t, e, "u-13"); !ok || ext != "" || verified {
		t.Fatalf("hint company = %q verified=%v ok=%v (the claimed id must not be stored)", ext, verified, ok)
	}
	if n := companyCount(t, e, "external_id = 'F-13'"); n != 1 {
		t.Fatalf("companies with the claimed id = %d", n)
	}
}

// CF-206: company.domain is not signed. A proved id must not land on a
// company that carries another external id just because the call names
// its domain; the proved id gets its own company and the other one stays
// as it was (not verified, no new contact).
func TestIdentifySignedCompanyDoesNotTakeOverAnotherCompanyByDomain(t *testing.T) {
	e := setup(t)
	if _, err := e.pool.Exec(context.Background(), "INSERT INTO companies (organization_id, name, domain, external_id) VALUES ($1, 'Rakip', 'rakip.test', 'F-20')", e.org); err != nil {
		t.Fatal(err)
	}
	r := identify(e, map[string]any{
		"user_id": "u-21", "user_hash": widget.UserCompanyHash(identitySecret, "u-21", "F-21"),
		"company": map[string]any{"id": "F-21", "name": "Yeni", "domain": "rakip.test"},
	})
	if r.Code != http.StatusOK || r.Data["company_verified"] != true {
		t.Fatalf("identify = %d %v %v", r.Code, r.Err, r.Data)
	}
	if ext, verified, ok := companyOf(t, e, "u-21"); !ok || ext != "F-21" || !verified {
		t.Fatalf("company = %q verified=%v ok=%v", ext, verified, ok)
	}
	if n := companyCount(t, e, "external_id = 'F-20' AND verified_at IS NULL AND domain = 'rakip.test' AND (SELECT count(*) FROM contacts c WHERE c.company_id = companies.id) = 0"); n != 1 {
		t.Fatal("the other company must stay unverified, keep its domain and get no contact")
	}
	if n := companyCount(t, e, "external_id = 'F-21' AND domain IS NULL"); n != 1 {
		t.Fatal("the proved id must get its own company without the other company's domain")
	}
}
