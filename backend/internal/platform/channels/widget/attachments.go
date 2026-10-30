package widget

import (
	"path"
	"slices"
	"strings"
)

// attachmentKinds maps the editor's file groups to MIME prefixes and
// extensions (the SDK builds its file picker "accept" list from the same
// table in frontend/widget-sdk/src/shared/schema.ts).
var attachmentKinds = map[string]struct {
	mimes []string
	exts  []string
}{
	"image":       {[]string{"image/"}, []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".heic", ".bmp"}},
	"pdf":         {[]string{"application/pdf"}, []string{".pdf"}},
	"document":    {[]string{"application/msword", "application/vnd.openxmlformats-officedocument.wordprocessingml", "application/vnd.oasis.opendocument.text", "application/rtf"}, []string{".doc", ".docx", ".odt", ".rtf"}},
	"spreadsheet": {[]string{"application/vnd.ms-excel", "application/vnd.openxmlformats-officedocument.spreadsheetml", "application/vnd.oasis.opendocument.spreadsheet", "text/csv"}, []string{".xls", ".xlsx", ".ods", ".csv"}},
	"video":       {[]string{"video/"}, []string{".mp4", ".mov", ".webm"}},
	"audio":       {[]string{"audio/"}, []string{".mp3", ".m4a", ".wav", ".ogg"}},
	"archive":     {[]string{"application/zip", "application/x-zip-compressed", "application/x-7z-compressed", "application/x-rar-compressed", "application/gzip"}, []string{".zip", ".7z", ".rar", ".gz"}},
	"text":        {[]string{"text/plain", "application/json"}, []string{".txt", ".log", ".json"}},
}

// AttachmentKind classifies an upload by extension, else MIME type; "" when
// it fits no group.
func AttachmentKind(contentType, filename string) string {
	ext := strings.ToLower(path.Ext(filename))
	ct := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	for _, kind := range AttachmentTypes {
		if ext != "" && slices.Contains(attachmentKinds[kind].exts, ext) {
			return kind
		}
	}
	for _, kind := range AttachmentTypes {
		for _, m := range attachmentKinds[kind].mimes {
			if strings.HasPrefix(ct, m) {
				return kind
			}
		}
	}
	return ""
}

// AttachmentAllowed reports whether the settings accept the upload.
func (s Settings) AttachmentAllowed(contentType, filename string) bool {
	kind := AttachmentKind(contentType, filename)
	return kind != "" && slices.Contains(s.Attachments.Types, kind)
}
