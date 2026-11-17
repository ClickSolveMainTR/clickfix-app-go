// Package logging, sidecar günlüklerini kurar: döndürmeli dosya ve stderr,
// ikisinde de sırlar (token, client_secret, kod, katılma bağlantısı)
// maskelenmiş olarak.
//
// Maskeleme iki katmandır: anahtarı hassas olan öznitelikler (token, secret,
// code, ...) değerine bakılmadan gizlenir; geri kalan bütün metinler
// (mesaj dahil) desenlerle taranır: JWT, Bearer başlığı, 9 haneli kod,
// t= / access_token= sorgu değerleri.
package logging

import (
	"log/slog"
	"regexp"
	"strings"
)

// Mask, maskelenen değerin yerine yazılan metindir.
const Mask = "[gizli]"

// sensitiveKeys, değeri her zaman gizlenen öznitelik adlarıdır (küçük harf,
// tam eşleşme ya da sonek). Yeni bir sır alanı eklenince buraya da girer.
var sensitiveKeys = []string{
	"token", "secret", "client_secret", "password", "authorization",
	"code", "join_token", "link", "access_token", "t",
}

func sensitiveKey(k string) bool {
	k = strings.ToLower(k)
	for _, s := range sensitiveKeys {
		if k == s || strings.HasSuffix(k, "_"+s) || strings.HasSuffix(k, "."+s) {
			return true
		}
	}
	return false
}

var (
	// JWT: üç base64url parçası, ilki "eyJ" ile başlar (LiveKit token'ı).
	reJWT = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}`)
	// Authorization: Bearer <x>
	reBearer = regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]+`)
	// Sorgu ve JSON içinde sır taşıyan alanlar: t=, token=, access_token=,
	// client_secret=, "client_secret":"..." vb.
	reQuery = regexp.MustCompile(`(?i)([?&](?:t|token|access_token|client_secret|code)=)[^&\s"']+`)
	reJSON  = regexp.MustCompile(`(?i)("(?:token|client_secret|access_token|code|join_token|secret)"\s*:\s*")[^"]*"`)
	// 9 haneli destek kodu, boşluk/tire ile ya da bitişik; daha uzun bir
	// sayının parçası değilse.
	reCode = regexp.MustCompile(`(^|[^0-9])([0-9]{3}[ -]?[0-9]{3}[ -]?[0-9]{3})([^0-9]|$)`)
)

// Scrub, serbest metindeki sırları maskeler.
func Scrub(s string) string {
	if s == "" {
		return s
	}
	s = reJWT.ReplaceAllString(s, Mask)
	s = reBearer.ReplaceAllString(s, "${1}"+Mask)
	s = reQuery.ReplaceAllString(s, "${1}"+Mask)
	s = reJSON.ReplaceAllString(s, `${1}`+Mask+`"`)
	// Örtüşen eşleşmeler için iki geçiş (ör. "111222333 444555666").
	for range 2 {
		s = reCode.ReplaceAllString(s, "${1}"+Mask+"${3}")
	}
	return s
}

// ReplaceAttr, slog.HandlerOptions.ReplaceAttr olarak maskeleme yapar.
func ReplaceAttr(groups []string, a slog.Attr) slog.Attr {
	if a.Key == slog.MessageKey {
		return slog.String(a.Key, Scrub(a.Value.String()))
	}
	if a.Key == slog.TimeKey || a.Key == slog.LevelKey || a.Key == slog.SourceKey {
		return a
	}
	if sensitiveKey(a.Key) {
		return slog.String(a.Key, Mask)
	}
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindString:
		return slog.String(a.Key, Scrub(v.String()))
	case slog.KindAny:
		if err, ok := v.Any().(error); ok {
			return slog.String(a.Key, Scrub(err.Error()))
		}
		// Yapılar ve haritalar metne çevrilip taranır; böylece içlerindeki
		// sırlar da kaçmaz.
		return slog.String(a.Key, Scrub(v.String()))
	case slog.KindGroup:
		return a
	default:
		return slog.Attr{Key: a.Key, Value: v}
	}
}
