package activity

import (
	"strings"
)

// Personal data never lands in audit payloads (CF-12): the audit log is read
// by organization admins and exported, and it must survive account deletion
// requests (KVKK / GDPR) without holding the person's data. Events reference
// people by uuid; the actor is joined from users at read time.
//
// Scrub returns a copy of payload where
//   - e-mail addresses are masked (a***@example.com), so admins still see
//     which invitation an event is about,
//   - other personal fields (names, phone, address, tax and identity numbers)
//     and secrets (password, token) are removed,
//
// recursively through nested maps and slices.
func Scrub(payload map[string]any) map[string]any {
	if payload == nil {
		return nil
	}
	out := make(map[string]any, len(payload))
	for k, v := range payload {
		key := strings.ToLower(k)
		switch {
		case droppedKey(key):
			continue
		case isEmailKey(key):
			out[k] = maskValue(v)
		default:
			out[k] = scrubValue(v)
		}
	}
	return out
}

func scrubValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return Scrub(t)
	case []any:
		cp := make([]any, len(t))
		for i, x := range t {
			cp[i] = scrubValue(x)
		}
		return cp
	default:
		return v
	}
}

// droppedKeys match a key exactly or as a suffix ("customer_phone").
var droppedKeys = []string{
	"surname", "full_name", "first_name", "last_name", "phone", "mobile", "address",
	"tax_number", "tax_no", "tckn", "vkn", "identity_number", "national_id", "birth_date",
	"password", "token", "secret", "otp",
}

// personNameKeys are names of people; a plain "name" (team, role,
// organization) is kept.
var personNameKeys = []string{"owner_name", "user_name", "inviter_name", "customer_name", "contact_name", "actor_name"}

func droppedKey(key string) bool {
	for _, d := range droppedKeys {
		if key == d || strings.HasSuffix(key, "_"+d) {
			return true
		}
	}
	for _, d := range personNameKeys {
		if key == d {
			return true
		}
	}
	return false
}

func isEmailKey(key string) bool {
	return key == "email" || strings.HasSuffix(key, "_email") || key == "emails"
}

func maskValue(v any) any {
	switch t := v.(type) {
	case string:
		return MaskEmail(t)
	case []any:
		cp := make([]any, len(t))
		for i, x := range t {
			cp[i] = maskValue(x)
		}
		return cp
	case []string:
		cp := make([]string, len(t))
		for i, x := range t {
			cp[i] = MaskEmail(x)
		}
		return cp
	default:
		return nil
	}
}

// MaskEmail keeps the first character of the local part and the domain:
// "ayse.yilmaz@example.com" -> "a***@example.com".
func MaskEmail(email string) string {
	email = strings.TrimSpace(email)
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		if email == "" {
			return ""
		}
		return "***"
	}
	r := []rune(email[:at])
	return string(r[0]) + "***" + email[at:]
}
