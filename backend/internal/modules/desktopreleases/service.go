// Package desktopreleases serves the desktop apps' releases (CF-240): the
// Tauri updater manifest, the download page list and the package files, and
// the platform endpoints that publish and withdraw a release.
//
// Releases are platform scoped (no organization). Package bytes live in
// object storage under desktop/releases/...; clients only ever see API URLs.
package desktopreleases

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Errors mapped to HTTP statuses by the routes.
var (
	ErrInvalid   = errors.New("desktopreleases: invalid input")
	ErrNotFound  = errors.New("desktopreleases: not found")
	ErrConflict  = errors.New("desktopreleases: release already exists")
	ErrWithdrawn = errors.New("desktopreleases: release already withdrawn")
)

// Apps, channels and file kinds.
const (
	AppAgent   = "agent"
	AppSupport = "support"

	ChannelStable = "stable"
	ChannelBeta   = "beta"

	KindUpdater   = "updater"
	KindInstaller = "installer"
)

var targetRe = regexp.MustCompile(`^(darwin|windows|linux)-(x86_64|aarch64|i686|armv7)$`)

// ValidApp reports whether app names one of the two desktop apps.
func ValidApp(app string) bool { return app == AppAgent || app == AppSupport }

// ValidTarget reports whether target is a Tauri updater platform key
// ({{target}}-{{arch}}, e.g. darwin-aarch64, windows-x86_64).
func ValidTarget(target string) bool { return targetRe.MatchString(target) }

// ParseChannel maps "" to stable and rejects unknown channels.
func ParseChannel(s string) (string, error) {
	switch s {
	case "", ChannelStable:
		return ChannelStable, nil
	case ChannelBeta:
		return ChannelBeta, nil
	}
	return "", ErrInvalid
}

// channelsFor: a beta client also takes stable releases (the newest wins), a
// stable client never sees beta.
func channelsFor(channel string) []string {
	if channel == ChannelBeta {
		return []string{ChannelStable, ChannelBeta}
	}
	return []string{ChannelStable}
}

// Queries is the part of db.Queries the service uses.
type Queries interface {
	CreateDesktopRelease(ctx context.Context, arg db.CreateDesktopReleaseParams) (db.DesktopRelease, error)
	GetDesktopReleaseByUUID(ctx context.Context, id uuid.UUID) (db.DesktopRelease, error)
	ListPublishedDesktopReleases(ctx context.Context, arg db.ListPublishedDesktopReleasesParams) ([]db.DesktopRelease, error)
	ListAllPublishedDesktopReleases(ctx context.Context, channels []string) ([]db.DesktopRelease, error)
	ListDesktopReleases(ctx context.Context, arg db.ListDesktopReleasesParams) ([]db.DesktopRelease, error)
	CountDesktopReleases(ctx context.Context, arg db.CountDesktopReleasesParams) (int64, error)
	WithdrawDesktopRelease(ctx context.Context, arg db.WithdrawDesktopReleaseParams) (db.DesktopRelease, error)
}

// Store is the part of storage.Driver the service uses.
type Store interface {
	Upload(ctx context.Context, file storage.File, path string) error
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
	Delete(ctx context.Context, path string) error
}

// Service implements the releases API.
type Service struct {
	Q     Queries
	Store Store
	// BaseURL is the public web origin (PUBLIC_FRONTEND_URL). Package URLs
	// in the updater manifest are absolute under BaseURL + /api/v1; the
	// download page gets same-origin /api/v1 paths.
	BaseURL string
}

// Manifest is the Tauri updater dynamic server response.
type Manifest struct {
	Version   string `json:"version"`
	Notes     string `json:"notes"`
	PubDate   string `json:"pub_date"`
	URL       string `json:"url"`
	Signature string `json:"signature"`
}

// File describes a package file.
type File struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	URL    string `json:"url"`
}

// Release is the platform view of a release.
type Release struct {
	UUID            uuid.UUID  `json:"uuid"`
	App             string     `json:"app"`
	Channel         string     `json:"channel"`
	Version         string     `json:"version"`
	Target          string     `json:"target"`
	Status          string     `json:"status"`
	Notes           string     `json:"notes"`
	OSSigned        bool       `json:"os_signed"`
	Updater         File       `json:"updater"`
	Installer       *File      `json:"installer"`
	PublishedAt     time.Time  `json:"published_at"`
	WithdrawnAt     *time.Time `json:"withdrawn_at"`
	WithdrawnReason string     `json:"withdrawn_reason"`
}

// Download is one entry of the download page: the newest published release
// of an app for a target. The file is the installer (macOS .dmg) or, when the
// release has none, the updater package (Windows NSIS installer).
type Download struct {
	App         string    `json:"app"`
	Target      string    `json:"target"`
	OS          string    `json:"os"`
	Arch        string    `json:"arch"`
	Version     string    `json:"version"`
	Channel     string    `json:"channel"`
	OSSigned    bool      `json:"os_signed"`
	File        File      `json:"file"`
	PublishedAt time.Time `json:"published_at"`
}

func filePath(id uuid.UUID, kind, name string) string {
	return "/api/v1/public/desktop/files/" + id.String() + "/" + kind + "/" + name
}

func toRelease(row db.DesktopRelease) Release {
	out := Release{
		UUID: row.Uuid, App: row.App, Channel: row.Channel, Version: row.Version, Target: row.Target,
		Status: row.Status, Notes: row.Notes, OSSigned: row.OsSigned, PublishedAt: row.PublishedAt.Time,
		WithdrawnReason: row.WithdrawnReason,
		Updater: File{Name: row.UpdaterName, Size: row.UpdaterSize, SHA256: row.UpdaterSha256,
			URL: filePath(row.Uuid, KindUpdater, row.UpdaterName)},
	}
	if row.InstallerKey.Valid {
		out.Installer = &File{Name: row.InstallerName.String, Size: row.InstallerSize.Int64, SHA256: row.InstallerSha256.String,
			URL: filePath(row.Uuid, KindInstaller, row.InstallerName.String)}
	}
	if row.WithdrawnAt.Valid {
		t := row.WithdrawnAt.Time
		out.WithdrawnAt = &t
	}
	return out
}

// newest returns the published row with the highest version, or false.
// Rows whose version does not parse are skipped (the publish endpoint
// rejects them, so this is only a guard).
func newest(rows []db.DesktopRelease) (db.DesktopRelease, Version, bool) {
	var best db.DesktopRelease
	var bestV Version
	found := false
	for _, row := range rows {
		v, err := ParseVersion(row.Version)
		if err != nil {
			continue
		}
		if !found || Compare(v, bestV) > 0 {
			best, bestV, found = row, v, true
		}
	}
	return best, bestV, found
}

// Update answers the Tauri updater: the newest published release of app for
// target that is newer than current, or nil (204) when the client is up to
// date. No organization and no login: the path names only public data.
func (s *Service) Update(ctx context.Context, app, target, current, channel string) (*Manifest, error) {
	if !ValidApp(app) || !ValidTarget(target) {
		return nil, ErrInvalid
	}
	cur, err := ParseVersion(current)
	if err != nil {
		return nil, ErrInvalid
	}
	ch, err := ParseChannel(channel)
	if err != nil {
		return nil, err
	}
	rows, err := s.Q.ListPublishedDesktopReleases(ctx, db.ListPublishedDesktopReleasesParams{App: app, Target: target, Channels: channelsFor(ch)})
	if err != nil {
		return nil, err
	}
	row, v, ok := newest(rows)
	if !ok || Compare(v, cur) <= 0 {
		return nil, nil
	}
	return &Manifest{
		Version:   row.Version,
		Notes:     row.Notes,
		PubDate:   row.PublishedAt.Time.UTC().Format(time.RFC3339),
		URL:       strings.TrimRight(s.BaseURL, "/") + filePath(row.Uuid, KindUpdater, row.UpdaterName),
		Signature: row.Signature,
	}, nil
}

// Downloads lists the newest published release per app and target.
func (s *Service) Downloads(ctx context.Context, channel string) ([]Download, error) {
	ch, err := ParseChannel(channel)
	if err != nil {
		return nil, err
	}
	rows, err := s.Q.ListAllPublishedDesktopReleases(ctx, channelsFor(ch))
	if err != nil {
		return nil, err
	}
	groups := map[string][]db.DesktopRelease{}
	for _, row := range rows {
		k := row.App + "/" + row.Target
		groups[k] = append(groups[k], row)
	}
	out := make([]Download, 0, len(groups))
	for _, g := range groups {
		row, _, ok := newest(g)
		if !ok {
			continue
		}
		rel := toRelease(row)
		f := rel.Updater
		if rel.Installer != nil {
			f = *rel.Installer
		}
		osName, arch, _ := strings.Cut(row.Target, "-")
		out = append(out, Download{App: row.App, Target: row.Target, OS: osName, Arch: arch, Version: row.Version,
			Channel: row.Channel, OSSigned: row.OsSigned, File: f, PublishedAt: row.PublishedAt.Time})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].App != out[j].App {
			return out[i].App < out[j].App
		}
		return out[i].Target < out[j].Target
	})
	return out, nil
}

// Open streams a package file of a published release.
func (s *Service) Open(ctx context.Context, id uuid.UUID, kind, name string) (io.ReadCloser, int64, string, error) {
	row, err := s.Q.GetDesktopReleaseByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && row.Status != "published" {
		return nil, 0, "", ErrNotFound
	}
	if err != nil {
		return nil, 0, "", err
	}
	var key, file string
	switch kind {
	case KindUpdater:
		key, file = row.UpdaterKey, row.UpdaterName
	case KindInstaller:
		if !row.InstallerKey.Valid {
			return nil, 0, "", ErrNotFound
		}
		key, file = row.InstallerKey.String, row.InstallerName.String
	default:
		return nil, 0, "", ErrNotFound
	}
	if name != file {
		return nil, 0, "", ErrNotFound
	}
	rc, size, err := s.Store.Download(ctx, key)
	if err != nil {
		return nil, 0, "", err
	}
	return rc, size, file, nil
}

// Upload is one package file of a publish request.
type Upload struct {
	Name string
	Size int64
	Body io.ReadSeeker
}

// PublishInput is a publish request.
type PublishInput struct {
	App, Channel, Version, Target, Notes, Signature string
	OSSigned                                        bool
	Updater                                         Upload
	Installer                                       *Upload
}

var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,199}$`)

var (
	updaterExts   = []string{".app.tar.gz", ".exe", ".msi", ".AppImage"}
	installerExts = []string{".dmg", ".exe", ".msi", ".AppImage", ".deb"}
)

func validName(name string, exts []string) bool {
	if !nameRe.MatchString(name) || strings.Contains(name, "..") {
		return false
	}
	for _, e := range exts {
		if strings.HasSuffix(name, e) {
			return true
		}
	}
	return false
}

// validSignature: a Tauri .sig file is base64 of a minisign signature.
func validSignature(sig string) bool {
	if sig == "" || len(sig) > 4096 {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(sig)
	return err == nil && strings.HasPrefix(string(raw), "untrusted comment:")
}

func (in *PublishInput) validate() error {
	in.Signature = strings.TrimSpace(in.Signature)
	in.Version = strings.TrimSpace(in.Version)
	v, err := ParseVersion(in.Version)
	switch {
	case err != nil, v.String() != in.Version: // canonical form only (no v prefix, no build metadata)
		return ErrInvalid
	case !ValidApp(in.App), !ValidTarget(in.Target):
		return ErrInvalid
	case !validSignature(in.Signature), len(in.Notes) > 10000:
		return ErrInvalid
	case in.Updater.Body == nil || in.Updater.Size <= 0 || !validName(in.Updater.Name, updaterExts):
		return ErrInvalid
	case in.Installer != nil && (in.Installer.Body == nil || in.Installer.Size <= 0 || !validName(in.Installer.Name, installerExts)):
		return ErrInvalid
	case in.Installer != nil && in.Installer.Name == in.Updater.Name:
		return ErrInvalid
	}
	ch, err := ParseChannel(in.Channel)
	if err != nil {
		return err
	}
	// Beta builds carry a pre-release version; stable builds never do.
	if (ch == ChannelBeta) != (len(v.Pre) > 0) {
		return ErrInvalid
	}
	in.Channel = ch
	return nil
}

func hashOf(r io.ReadSeeker) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func contentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".tar.gz"):
		return "application/gzip"
	case strings.HasSuffix(name, ".dmg"):
		return "application/x-apple-diskimage"
	case strings.HasSuffix(name, ".exe"):
		return "application/vnd.microsoft.portable-executable"
	case strings.HasSuffix(name, ".msi"):
		return "application/x-msi"
	case strings.HasSuffix(name, ".deb"):
		return "application/vnd.debian.binary-package"
	}
	return "application/octet-stream"
}

// Publish stores the package files and records the release. The objects
// are written before the row, so a published row always has its files; a
// failed insert removes them again.
func (s *Service) Publish(ctx context.Context, actor int64, in PublishInput) (Release, error) {
	if err := in.validate(); err != nil {
		return Release{}, err
	}
	folder := uuid.NewString()
	type stored struct {
		key, sha string
	}
	put := func(u Upload) (stored, error) {
		sha, err := hashOf(u.Body)
		if err != nil {
			return stored{}, err
		}
		key := storage.DesktopReleaseObjectKey(in.App, in.Channel, in.Version, in.Target, folder, u.Name)
		err = s.Store.Upload(ctx, storage.File{Body: u.Body, Size: u.Size, ContentType: contentType(u.Name), Filename: u.Name,
			Metadata: map[string]string{"sha256": sha}}, key)
		return stored{key: key, sha: sha}, err
	}
	var keys []string
	cleanup := func() {
		for _, k := range keys {
			_ = s.Store.Delete(context.WithoutCancel(ctx), k)
		}
	}
	up, err := put(in.Updater)
	if err != nil {
		return Release{}, fmt.Errorf("desktopreleases: upload updater: %w", err)
	}
	keys = append(keys, up.key)
	params := db.CreateDesktopReleaseParams{
		App: in.App, Channel: in.Channel, Version: in.Version, Target: in.Target, Notes: in.Notes,
		Signature: in.Signature, OsSigned: in.OSSigned,
		UpdaterKey: up.key, UpdaterName: in.Updater.Name, UpdaterSize: in.Updater.Size, UpdaterSha256: up.sha,
		PublishedByUserID: pgtype.Int8{Int64: actor, Valid: actor > 0},
	}
	if in.Installer != nil {
		inst, err := put(*in.Installer)
		if err != nil {
			cleanup()
			return Release{}, fmt.Errorf("desktopreleases: upload installer: %w", err)
		}
		keys = append(keys, inst.key)
		params.InstallerKey = pgtype.Text{String: inst.key, Valid: true}
		params.InstallerName = pgtype.Text{String: in.Installer.Name, Valid: true}
		params.InstallerSize = pgtype.Int8{Int64: in.Installer.Size, Valid: true}
		params.InstallerSha256 = pgtype.Text{String: inst.sha, Valid: true}
	}
	row, err := s.Q.CreateDesktopRelease(ctx, params)
	if err != nil {
		cleanup()
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23505" {
			return Release{}, ErrConflict
		}
		return Release{}, err
	}
	return toRelease(row), nil
}

// Withdraw takes a release out of the updater and the download page. Its
// files stay in storage (audit, forensics); clients already on it are moved
// off by publishing a higher version.
func (s *Service) Withdraw(ctx context.Context, actor int64, id uuid.UUID, reason string) (Release, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 1000 {
		return Release{}, ErrInvalid
	}
	row, err := s.Q.WithdrawDesktopRelease(ctx, db.WithdrawDesktopReleaseParams{Uuid: id, Reason: reason,
		WithdrawnByUserID: pgtype.Int8{Int64: actor, Valid: actor > 0}})
	if errors.Is(err, pgx.ErrNoRows) {
		if _, gerr := s.Q.GetDesktopReleaseByUUID(ctx, id); gerr == nil {
			return Release{}, ErrWithdrawn
		}
		return Release{}, ErrNotFound
	}
	if err != nil {
		return Release{}, err
	}
	return toRelease(row), nil
}

// ListFilter filters the platform list.
type ListFilter struct {
	App, Channel, Status string
	Limit, Offset        int32
}

func optText(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

// List returns releases, newest first, and the total.
func (s *Service) List(ctx context.Context, f ListFilter) ([]Release, int64, error) {
	if f.App != "" && !ValidApp(f.App) ||
		f.Channel != "" && f.Channel != ChannelStable && f.Channel != ChannelBeta ||
		f.Status != "" && f.Status != "published" && f.Status != "withdrawn" {
		return nil, 0, ErrInvalid
	}
	rows, err := s.Q.ListDesktopReleases(ctx, db.ListDesktopReleasesParams{App: optText(f.App), Channel: optText(f.Channel),
		Status: optText(f.Status), Lim: f.Limit, Off: f.Offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.Q.CountDesktopReleases(ctx, db.CountDesktopReleasesParams{App: optText(f.App), Channel: optText(f.Channel),
		Status: optText(f.Status)})
	if err != nil {
		return nil, 0, err
	}
	out := make([]Release, len(rows))
	for i, row := range rows {
		out[i] = toRelease(row)
	}
	return out, total, nil
}
