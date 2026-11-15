package storage

import "fmt"

// AppLogoObjectKey builds app/branding/logo.{ext} for system letterhead.
func AppLogoObjectKey(ext string) string {
	return fmt.Sprintf("app/branding/logo.%s", trimExt(ext))
}

// OrganizationLogoObjectKey builds
// organizations/{orgUUID}/branding/logo-{versionUUID}.{ext}. Every upload
// gets a new version so the API logo URL changes with it (cache busting).
func OrganizationLogoObjectKey(orgUUID, versionUUID, ext string) string {
	return fmt.Sprintf("organizations/%s/branding/logo-%s.%s", orgUUID, versionUUID, trimExt(ext))
}

// ExportObjectKey builds exports/{jobUUID}/export.{ext}.
func ExportObjectKey(jobUUID, ext string) string {
	return fmt.Sprintf("exports/%s/export.%s", jobUUID, trimExt(ext))
}

// ImportSourceObjectKey builds imports/{jobUUID}/source.{ext}.
func ImportSourceObjectKey(jobUUID, ext string) string {
	return fmt.Sprintf("imports/%s/source.%s", jobUUID, trimExt(ext))
}

// HistoryRawObjectKey builds orgs/{org}/history/{run}/raw.{ext}: the uploaded
// history export (CF-217). It is private and deleted when the run ends or its
// retention passes, whichever comes first.
func HistoryRawObjectKey(org int64, runUUID, ext string) string {
	return fmt.Sprintf("orgs/%d/history/%s/raw.%s", org, runUUID, trimExt(ext))
}

// AttachmentObjectKey builds orgs/{org}/attachments/{attachment}/original.
// The object is private; the API streams it after an organization check. The
// original filename lives in the database, never in the key.
func AttachmentObjectKey(orgUUID, attachmentUUID string) string {
	return fmt.Sprintf("orgs/%s/attachments/%s/original", orgUUID, attachmentUUID)
}

// AttachmentThumbKey builds orgs/{org}/attachments/{attachment}/thumb.jpg.
func AttachmentThumbKey(orgUUID, attachmentUUID string) string {
	return fmt.Sprintf("orgs/%s/attachments/%s/thumb.jpg", orgUUID, attachmentUUID)
}

func trimExt(ext string) string {
	for len(ext) > 0 && ext[0] == '.' {
		ext = ext[1:]
	}
	return ext
}

// ReplayChunkKey keeps replay objects private and organization scoped.
func ReplayChunkKey(org, session, chunk string) string {
	return fmt.Sprintf("orgs/%s/replays/%s/%s.json.gz", org, session, chunk)
}
