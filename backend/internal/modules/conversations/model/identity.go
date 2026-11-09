package model

import "time"

// Identity states of a contact (CF-195). The panel shows them as a badge and
// the inbox filters by them; SQL filters use the same rule
// (queries/contacts.sql and conversations.sql, identity_state).
const (
	// IdentityVerified: the contact proved the customer's user id with
	// user_hash (widget identify).
	IdentityVerified = "verified"
	// IdentityAnonymous: nothing is known (no name, e-mail or phone).
	IdentityAnonymous = "anonymous"
	// IdentitySelfNamed: the contact or its channel told us a name, e-mail
	// or phone, but nothing proved it.
	IdentitySelfNamed = "self_named"
)

// IdentityStates lists the accepted filter values.
var IdentityStates = []string{IdentityVerified, IdentityAnonymous, IdentitySelfNamed}

// IdentityState classifies a contact. A verified contact stays verified
// even without a name; an unverified one is never verified because of
// what it typed.
func IdentityState(verifiedAt *time.Time, name string, email, phone *string) string {
	switch {
	case verifiedAt != nil:
		return IdentityVerified
	case name == "" && (email == nil || *email == "") && (phone == nil || *phone == ""):
		return IdentityAnonymous
	default:
		return IdentitySelfNamed
	}
}

// ValidIdentityState reports whether s is a known identity state.
func ValidIdentityState(s string) bool {
	for _, v := range IdentityStates {
		if v == s {
			return true
		}
	}
	return false
}
