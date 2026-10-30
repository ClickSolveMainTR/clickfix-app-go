package usecase

import (
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxFilenameLen is the longest stored filename (bytes).
const maxFilenameLen = 255

// blockedExtensions are never accepted, whatever the declared type is.
var blockedExtensions = map[string]bool{
	".exe": true, ".dll": true, ".bat": true, ".cmd": true, ".com": true,
	".scr": true, ".msi": true, ".ps1": true, ".vbs": true, ".jar": true,
	".sh": true, ".app": true, ".dmg": true, ".pif": true, ".lnk": true,
	".hta": true, ".cpl": true, ".reg": true, ".wsf": true,
}

// allowedExact are accepted non-prefix media types.
var allowedExact = map[string]bool{
	"application/pdf":               true,
	"application/zip":               true,
	"application/json":              true,
	"application/rtf":               true,
	"application/gzip":              true,
	"application/msword":            true,
	"application/vnd.ms-excel":      true,
	"application/vnd.ms-powerpoint": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
	"application/vnd.oasis.opendocument.text":                                   true,
	"application/vnd.oasis.opendocument.spreadsheet":                            true,
	"text/plain":    true,
	"text/csv":      true,
	"text/markdown": true,
}

// allowedPrefixes are accepted media type families.
var allowedPrefixes = []string{"image/", "audio/", "video/"}

// sanitizeFilename keeps only the base name, drops control characters and
// caps the length. It returns "" when nothing usable is left.
func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '"' {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "." || name == ".." || name == "/" {
		return ""
	}
	for len(name) > maxFilenameLen {
		_, size := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-size]
	}
	return name
}

// resolveContentType picks the stored media type. The sniffed type of the
// first bytes wins over the client's claim; a generic sniff (plain text,
// octet-stream) falls back to the declared type, then to the extension.
func resolveContentType(filename, declared string, head []byte) string {
	sniffed := baseMediaType(http.DetectContentType(head))
	if sniffed != "application/octet-stream" && sniffed != "text/plain" {
		// Office and OpenDocument files are zip containers; keep a more
		// specific declared zip-based type instead of "application/zip".
		if sniffed == "application/zip" {
			if d := baseMediaType(declared); strings.HasPrefix(d, "application/vnd.") && allowedExact[d] {
				return d
			}
			if t := baseMediaType(mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))); strings.HasPrefix(t, "application/vnd.") && allowedExact[t] {
				return t
			}
		}
		return sniffed
	}
	if d := baseMediaType(declared); d != "" && d != "application/octet-stream" {
		if sniffed == "text/plain" && !strings.HasPrefix(d, "text/") && d != "application/json" && d != "application/rtf" {
			return sniffed
		}
		return d
	}
	if t := baseMediaType(mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))); t != "" {
		return t
	}
	return sniffed
}

func baseMediaType(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if mt, _, err := mime.ParseMediaType(v); err == nil {
		return strings.ToLower(mt)
	}
	if i := strings.IndexByte(v, ';'); i >= 0 {
		v = v[:i]
	}
	return strings.ToLower(strings.TrimSpace(v))
}

func isAllowedType(contentType string) bool {
	if allowedExact[contentType] {
		return true
	}
	for _, p := range allowedPrefixes {
		if strings.HasPrefix(contentType, p) {
			return true
		}
	}
	return false
}

// looksExecutable reports native executables by magic bytes (PE, ELF,
// Mach-O) regardless of name or declared type.
func looksExecutable(head []byte) bool {
	switch {
	case len(head) >= 2 && head[0] == 'M' && head[1] == 'Z':
		return true
	case len(head) >= 4 && head[0] == 0x7f && head[1] == 'E' && head[2] == 'L' && head[3] == 'F':
		return true
	case len(head) >= 4 && (head[0] == 0xfe && head[1] == 0xed && head[2] == 0xfa || head[0] == 0xcf && head[1] == 0xfa && head[2] == 0xed && head[3] == 0xfe):
		return true
	}
	return false
}

// activeContent types can run script when rendered inline by a browser, so
// they are always served as a download.
func isActiveContent(contentType string) bool {
	switch baseMediaType(contentType) {
	case "text/html", "application/xhtml+xml", "image/svg+xml", "text/xml", "application/xml":
		return true
	}
	return false
}

// IsInlineSafe reports whether a stored type may be rendered inline.
func IsInlineSafe(contentType string) bool {
	ct := baseMediaType(contentType)
	if ct == "" || isActiveContent(ct) {
		return false
	}
	return strings.HasPrefix(ct, "image/") || strings.HasPrefix(ct, "video/") ||
		strings.HasPrefix(ct, "audio/") || ct == "application/pdf" || ct == "text/plain"
}
