// Package usecase serves the public, unauthenticated help center (CF-74) and
// the widget help search (CF-55). It only ever reads the PUBLISHED snapshot
// of PUBLIC knowledge articles: current drafts, internal articles and
// deleted articles never leave this package.
package usecase

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/furkanmeclis/clickfix-app-go/backend/internal/platform/safeconv"
	"hash/fnv"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/clickfix-app-go/backend/internal/database/db"
	kbmodel "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/model"
	kbusecase "github.com/furkanmeclis/clickfix-app-go/backend/internal/modules/knowledge/usecase"
)

var (
	// ErrNotFound: unknown/suspended organization, an organization without a
	// published public article, or an unknown article/collection.
	ErrNotFound = errors.New("helpcenter: not found")
	// ErrInvalid: bad locale, empty or oversized query.
	ErrInvalid = errors.New("helpcenter: invalid input")
)

const (
	excerptRunes    = 220
	maxQueryRunes   = 200
	searchLimit     = 10
	recentLimit     = 6
	relatedLimit    = 5
	collectionLimit = 50
)

// Querier is the subset of sqlc queries the help center reads.
type Querier interface {
	HelpOrganization(ctx context.Context, slug string) (db.HelpOrganizationRow, error)
	HelpOrganizationByID(ctx context.Context, id int64) (db.HelpOrganizationByIDRow, error)
	HelpPublicLocales(ctx context.Context, organizationID int64) ([]db.HelpPublicLocalesRow, error)
	HelpAccentColor(ctx context.Context, organizationID int64) (string, error)
	HelpCollections(ctx context.Context, arg db.HelpCollectionsParams) ([]db.HelpCollectionsRow, error)
	HelpArticles(ctx context.Context, arg db.HelpArticlesParams) ([]db.HelpArticlesRow, error)
	HelpArticleCount(ctx context.Context, arg db.HelpArticleCountParams) (int64, error)
	HelpArticleBySlug(ctx context.Context, arg db.HelpArticleBySlugParams) (db.HelpArticleBySlugRow, error)
	HelpSearchArticles(ctx context.Context, arg db.HelpSearchArticlesParams) ([]db.HelpSearchArticlesRow, error)
	HelpArticlesByUUIDs(ctx context.Context, arg db.HelpArticlesByUUIDsParams) ([]db.HelpArticlesByUUIDsRow, error)
	HelpSitemap(ctx context.Context, organizationID int64) ([]db.HelpSitemapRow, error)
	HelpUpsertFeedback(ctx context.Context, arg db.HelpUpsertFeedbackParams) error
}

// Searcher is the knowledge hybrid search (published-only revalidation in
// Postgres). The help center passes a lexical-only RAG: anonymous visitors
// must not spend the organization's metered embedding budget.
type Searcher interface {
	Search(ctx context.Context, org int64, query, locale string, publicOnly bool) ([]kbmodel.Citation, error)
}

// Storage streams the organization logo.
type Storage interface {
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
}

// Service is the help center read model.
type Service struct {
	q       Querier
	search  Searcher
	storage Storage
	secret  []byte
	now     func() time.Time
}

// New builds the service. search and storage may be nil.
func New(q Querier, search Searcher, storage Storage, secret string) *Service {
	return &Service{q: q, search: search, storage: storage, secret: []byte(secret), now: time.Now}
}

// LexicalOnly copies a RAG without its embedder and reranker (both call paid
// AI providers), keeping the published/public revalidation of hybrid search.
func LexicalOnly(rag *kbusecase.RAG) Searcher {
	if rag == nil {
		return nil
	}
	return &kbusecase.RAG{KB: rag.KB, Lexical: rag.Lexical}
}

// Organization is the public branding of a help center.
type Organization struct {
	ID      int64         `json:"-"`
	Slug    string        `json:"slug"`
	Name    string        `json:"name"`
	LogoURL *string       `json:"logo_url"`
	Accent  *string       `json:"accent_color"`
	Locales []LocaleCount `json:"locales"`
	logoKey pgtype.Text
}

// LocaleCount is a locale with at least one public article.
type LocaleCount struct {
	Locale       string `json:"locale"`
	ArticleCount int64  `json:"article_count"`
}

// Collection is a public collection or category.
type Collection struct {
	UUID         uuid.UUID `json:"uuid"`
	Name         string    `json:"name"`
	Slug         string    `json:"slug"`
	Kind         string    `json:"kind"`
	ParentSlug   *string   `json:"parent_slug"`
	ArticleCount int64     `json:"article_count"`
	id           int64
}

// ArticleSummary is a published public article in lists and search.
type ArticleSummary struct {
	UUID           uuid.UUID `json:"uuid"`
	Slug           string    `json:"slug"`
	Locale         string    `json:"locale"`
	Title          string    `json:"title"`
	Excerpt        string    `json:"excerpt"`
	UpdatedAt      time.Time `json:"updated_at"`
	CollectionSlug *string   `json:"collection_slug"`
}

// Article is the published content of one public article.
type Article struct {
	ArticleSummary
	Body       string           `json:"body"`
	Tags       []string         `json:"tags"`
	CreatedAt  time.Time        `json:"created_at"`
	Collection *Collection      `json:"collection"`
	Parent     *Collection      `json:"parent_collection"`
	Related    []ArticleSummary `json:"related"`
	id         int64
	version    int32
	collection pgtype.Int8
}

// Home is the help center landing payload.
type Home struct {
	Organization Organization     `json:"organization"`
	Locale       string           `json:"locale"`
	Collections  []Collection     `json:"collections"`
	Recent       []ArticleSummary `json:"recent"`
}

// CollectionPage is one collection with its categories and articles.
type CollectionPage struct {
	Organization Organization     `json:"organization"`
	Locale       string           `json:"locale"`
	Collection   Collection       `json:"collection"`
	Parent       *Collection      `json:"parent_collection"`
	Children     []Collection     `json:"children"`
	Articles     []ArticleSummary `json:"articles"`
	Total        int64            `json:"total"`
}

// ArticlePage is an article with the help center branding.
type ArticlePage struct {
	Organization Organization `json:"organization"`
	Article      Article      `json:"article"`
}

// SitemapEntry is one indexable article.
type SitemapEntry struct {
	Slug      string    `json:"slug"`
	Locale    string    `json:"locale"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Sitemap lists the indexable help center pages.
type Sitemap struct {
	Organization Organization   `json:"organization"`
	Articles     []SitemapEntry `json:"articles"`
	Collections  []SitemapEntry `json:"collections"`
}

// NormalizeLocale defaults an empty locale to tr; anything else but tr/en is invalid.
func NormalizeLocale(locale string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(locale)) {
	case "", "tr":
		return "tr", nil
	case "en":
		return "en", nil
	}
	return "", ErrInvalid
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// organization resolves a live organization that has a published public
// article; other organizations have no help center (and are not enumerable).
func (s *Service) organization(ctx context.Context, slug string) (Organization, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" || len(slug) > 64 {
		return Organization{}, ErrNotFound
	}
	row, err := s.q.HelpOrganization(ctx, slug)
	if err != nil {
		return Organization{}, notFound(err)
	}
	return s.branding(ctx, row.ID, row.Slug, row.Name, row.LogoObjectKey)
}

func (s *Service) branding(ctx context.Context, id int64, slug, name string, logo pgtype.Text) (Organization, error) {
	locales, err := s.q.HelpPublicLocales(ctx, id)
	if err != nil {
		return Organization{}, err
	}
	if len(locales) == 0 {
		return Organization{}, ErrNotFound
	}
	out := Organization{ID: id, Slug: slug, Name: name, Locales: make([]LocaleCount, 0, len(locales)), logoKey: logo}
	for _, l := range locales {
		out.Locales = append(out.Locales, LocaleCount{Locale: l.Locale, ArticleCount: l.ArticleCount})
	}
	if logo.Valid && strings.TrimSpace(logo.String) != "" {
		h := fnv.New32a()
		_, _ = h.Write([]byte(logo.String))
		u := fmt.Sprintf("/v1/public/help/%s/logo?v=%x", slug, h.Sum32())
		out.LogoURL = &u
	}
	if color, err := s.q.HelpAccentColor(ctx, id); err == nil && hexColor.MatchString(color) {
		c := strings.ToLower(color)
		out.Accent = &c
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Organization{}, err
	}
	return out, nil
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func (s *Service) collections(ctx context.Context, org int64, locale string) ([]Collection, error) {
	rows, err := s.q.HelpCollections(ctx, db.HelpCollectionsParams{OrganizationID: org, Locale: locale})
	if err != nil {
		return nil, err
	}
	out := make([]Collection, 0, len(rows))
	for _, r := range rows {
		c := Collection{UUID: r.Uuid, Name: r.Name, Slug: r.Slug, Kind: r.Kind, ArticleCount: r.ArticleCount, id: r.ID}
		if r.ParentSlug != "" {
			p := r.ParentSlug
			c.ParentSlug = &p
		}
		out = append(out, c)
	}
	// A collection counts its categories' articles too.
	totals := map[string]int64{}
	for _, c := range out {
		if c.ParentSlug != nil {
			totals[*c.ParentSlug] += c.ArticleCount
		}
	}
	visible := out[:0]
	for _, c := range out {
		c.ArticleCount += totals[c.Slug]
		if c.ArticleCount > 0 {
			visible = append(visible, c)
		}
	}
	return visible, nil
}

// Home returns branding, non-empty collections and the latest articles.
func (s *Service) Home(ctx context.Context, slug, locale string) (Home, error) {
	locale, err := NormalizeLocale(locale)
	if err != nil {
		return Home{}, err
	}
	org, err := s.organization(ctx, slug)
	if err != nil {
		return Home{}, err
	}
	cols, err := s.collections(ctx, org.ID, locale)
	if err != nil {
		return Home{}, err
	}
	rows, err := s.q.HelpArticles(ctx, db.HelpArticlesParams{OrganizationID: org.ID, Locale: locale, CollectionIds: []int64{}, Limit: recentLimit})
	if err != nil {
		return Home{}, err
	}
	return Home{Organization: org, Locale: locale, Collections: cols, Recent: summaries(rows)}, nil
}

// Collection returns one collection page (articles of the collection and its categories).
func (s *Service) Collection(ctx context.Context, slug, collectionSlug, locale string, limit, offset int32) (CollectionPage, error) {
	locale, err := NormalizeLocale(locale)
	if err != nil {
		return CollectionPage{}, err
	}
	org, err := s.organization(ctx, slug)
	if err != nil {
		return CollectionPage{}, err
	}
	cols, err := s.collections(ctx, org.ID, locale)
	if err != nil {
		return CollectionPage{}, err
	}
	page := CollectionPage{Organization: org, Locale: locale, Children: []Collection{}}
	found := false
	for _, c := range cols {
		if c.Slug == collectionSlug {
			page.Collection, found = c, true
		}
	}
	if !found {
		return CollectionPage{}, ErrNotFound
	}
	ids := []int64{page.Collection.id}
	for _, c := range cols {
		if c.ParentSlug != nil && *c.ParentSlug == page.Collection.Slug {
			page.Children = append(page.Children, c)
			ids = append(ids, c.id)
		}
		if page.Collection.ParentSlug != nil && c.Slug == *page.Collection.ParentSlug {
			parent := c
			page.Parent = &parent
		}
	}
	if limit <= 0 || limit > collectionLimit {
		limit = collectionLimit
	}
	rows, err := s.q.HelpArticles(ctx, db.HelpArticlesParams{OrganizationID: org.ID, Locale: locale, CollectionIds: ids, Limit: limit, Offset: max(offset, 0)})
	if err != nil {
		return CollectionPage{}, err
	}
	page.Articles = summaries(rows)
	page.Total, err = s.q.HelpArticleCount(ctx, db.HelpArticleCountParams{OrganizationID: org.ID, Locale: locale, CollectionIds: ids})
	return page, err
}

// Article returns the published content of a public article.
func (s *Service) Article(ctx context.Context, slug, articleSlug, locale string) (ArticlePage, error) {
	locale, err := NormalizeLocale(locale)
	if err != nil {
		return ArticlePage{}, err
	}
	org, err := s.organization(ctx, slug)
	if err != nil {
		return ArticlePage{}, err
	}
	a, err := s.article(ctx, org.ID, articleSlug, locale)
	if err != nil {
		return ArticlePage{}, err
	}
	if a.collection.Valid {
		cols, err := s.collections(ctx, org.ID, locale)
		if err != nil {
			return ArticlePage{}, err
		}
		for _, c := range cols {
			if c.id == a.collection.Int64 {
				col := c
				a.Collection = &col
				a.CollectionSlug = &col.Slug
			}
		}
		if a.Collection != nil && a.Collection.ParentSlug != nil {
			for _, c := range cols {
				if c.Slug == *a.Collection.ParentSlug {
					parent := c
					a.Parent = &parent
				}
			}
		}
	}
	if a.Collection != nil {
		rows, err := s.q.HelpArticles(ctx, db.HelpArticlesParams{OrganizationID: org.ID, Locale: locale, CollectionIds: []int64{a.Collection.id}, ExcludeID: a.id, Limit: relatedLimit})
		if err != nil {
			return ArticlePage{}, err
		}
		a.Related = summaries(rows)
	}
	return ArticlePage{Organization: org, Article: a}, nil
}

func (s *Service) article(ctx context.Context, org int64, articleSlug, locale string) (Article, error) {
	articleSlug = strings.TrimSpace(articleSlug)
	if articleSlug == "" || len(articleSlug) > 200 {
		return Article{}, ErrNotFound
	}
	row, err := s.q.HelpArticleBySlug(ctx, db.HelpArticleBySlugParams{OrganizationID: org, Locale: locale, Slug: articleSlug})
	if err != nil {
		return Article{}, notFound(err)
	}
	a := Article{
		ArticleSummary: ArticleSummary{UUID: row.Uuid, Slug: row.Slug, Locale: row.Locale, Title: row.Title, Excerpt: Excerpt(row.Body, excerptRunes), UpdatedAt: row.UpdatedAt.Time.UTC()},
		Body:           row.Body, Tags: row.Tags, CreatedAt: row.CreatedAt.Time.UTC(), Related: []ArticleSummary{},
		id: row.ID, version: row.PublishedVersion.Int32, collection: row.CollectionID,
	}
	if a.Tags == nil {
		a.Tags = []string{}
	}
	return a, nil
}

// Search finds published public articles: lexical hybrid search hits first
// (chunk index, revalidated in Postgres), then title/body matches.
func (s *Service) Search(ctx context.Context, slug, query, locale string) (Organization, []ArticleSummary, error) {
	locale, err := NormalizeLocale(locale)
	if err != nil {
		return Organization{}, nil, err
	}
	org, err := s.organization(ctx, slug)
	if err != nil {
		return Organization{}, nil, err
	}
	items, err := s.SearchOrganization(ctx, org.ID, query, locale, searchLimit)
	return org, items, err
}

// SearchOrganization searches the public articles of an organization by id
// (widget help search, which resolves the organization from the widget).
func (s *Service) SearchOrganization(ctx context.Context, org int64, query, locale string, limit int) ([]ArticleSummary, error) {
	query = strings.Join(strings.Fields(query), " ")
	if query == "" || utf8.RuneCountInString(query) > maxQueryRunes {
		return nil, ErrInvalid
	}
	if limit <= 0 || limit > searchLimit {
		limit = searchLimit
	}
	out := []ArticleSummary{}
	seen := map[uuid.UUID]bool{}
	if s.search != nil {
		// Best effort: a guard rejection or index outage falls back to SQL.
		if hits, err := s.search.Search(ctx, org, query, locale, true); err == nil && len(hits) > 0 {
			ids := []uuid.UUID{}
			for _, h := range hits {
				if !seen[h.ArticleUUID] {
					seen[h.ArticleUUID] = true
					ids = append(ids, h.ArticleUUID)
				}
			}
			rows, err := s.q.HelpArticlesByUUIDs(ctx, db.HelpArticlesByUUIDsParams{OrganizationID: org, Ids: ids, Locale: locale})
			if err != nil {
				return nil, err
			}
			byID := map[uuid.UUID]ArticleSummary{}
			for _, r := range rows {
				byID[r.Uuid] = summary(r.Uuid, r.Slug, r.Locale, r.Title, r.Excerpt, r.UpdatedAt, r.CollectionSlug)
			}
			seen = map[uuid.UUID]bool{}
			for _, id := range ids {
				if a, ok := byID[id]; ok {
					out = append(out, a)
					seen[id] = true
				}
			}
		}
	}
	if len(out) < limit {
		rows, err := s.q.HelpSearchArticles(ctx, db.HelpSearchArticlesParams{OrganizationID: org, Locale: locale, Q: query, Pattern: "%" + escapeLike(query) + "%", Limit: safeconv.ToInt32(limit)})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			if !seen[r.Uuid] && len(out) < limit {
				seen[r.Uuid] = true
				out = append(out, summary(r.Uuid, r.Slug, r.Locale, r.Title, r.Excerpt, r.UpdatedAt, r.CollectionSlug))
			}
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// RecentOrganization lists the latest public articles of an organization by id.
func (s *Service) RecentOrganization(ctx context.Context, org int64, locale string, limit int) ([]ArticleSummary, error) {
	if limit <= 0 || limit > searchLimit {
		limit = searchLimit
	}
	rows, err := s.q.HelpArticles(ctx, db.HelpArticlesParams{OrganizationID: org, Locale: locale, CollectionIds: []int64{}, Limit: safeconv.ToInt32(limit)})
	if err != nil {
		return nil, err
	}
	return summaries(rows), nil
}

// HasHelpCenter reports the organization slug when it has a public help center.
func (s *Service) HasHelpCenter(ctx context.Context, org int64) (string, bool) {
	row, err := s.q.HelpOrganizationByID(ctx, org)
	if err != nil {
		return "", false
	}
	locales, err := s.q.HelpPublicLocales(ctx, org)
	if err != nil || len(locales) == 0 {
		return "", false
	}
	return row.Slug, true
}

// Sitemap lists the indexable articles and collections of a help center.
func (s *Service) Sitemap(ctx context.Context, slug string) (Sitemap, error) {
	org, err := s.organization(ctx, slug)
	if err != nil {
		return Sitemap{}, err
	}
	rows, err := s.q.HelpSitemap(ctx, org.ID)
	if err != nil {
		return Sitemap{}, err
	}
	out := Sitemap{Organization: org, Articles: make([]SitemapEntry, 0, len(rows)), Collections: []SitemapEntry{}}
	latest := map[string]time.Time{}
	for _, r := range rows {
		t := r.UpdatedAt.Time.UTC()
		out.Articles = append(out.Articles, SitemapEntry{Slug: r.Slug, Locale: r.Locale, UpdatedAt: t})
		if t.After(latest[r.Locale]) {
			latest[r.Locale] = t
		}
	}
	for _, l := range org.Locales {
		cols, err := s.collections(ctx, org.ID, l.Locale)
		if err != nil {
			return Sitemap{}, err
		}
		for _, c := range cols {
			out.Collections = append(out.Collections, SitemapEntry{Slug: c.Slug, Locale: l.Locale, UpdatedAt: latest[l.Locale]})
		}
	}
	return out, nil
}

// Feedback records a helpful / not helpful vote. The voter key is an HMAC of
// the client IP, article and UTC day, so no IP is stored and a repeated vote
// from the same visitor on the same day replaces the earlier one.
func (s *Service) Feedback(ctx context.Context, slug, articleSlug, locale, clientIP string, helpful bool) error {
	locale, err := NormalizeLocale(locale)
	if err != nil {
		return err
	}
	org, err := s.organization(ctx, slug)
	if err != nil {
		return err
	}
	a, err := s.article(ctx, org.ID, articleSlug, locale)
	if err != nil {
		return err
	}
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte("help-feedback\x00" + clientIP + "\x00" + strconv.FormatInt(a.id, 10) + "\x00" + s.now().UTC().Format("2006-01-02")))
	return s.q.HelpUpsertFeedback(ctx, db.HelpUpsertFeedbackParams{OrganizationID: org.ID, ArticleID: a.id, Version: a.version, Helpful: helpful, VoterHash: hex.EncodeToString(mac.Sum(nil))})
}

// ArticleKey is the rate limit subject of an article (organization + slug).
func ArticleKey(slug, articleSlug, locale string) string {
	return slug + "/" + locale + "/" + articleSlug
}

// Logo streams the organization logo of a help center.
func (s *Service) Logo(ctx context.Context, slug string) (io.ReadCloser, string, error) {
	org, err := s.organization(ctx, slug)
	if err != nil {
		return nil, "", err
	}
	if s.storage == nil || !org.logoKey.Valid || strings.TrimSpace(org.logoKey.String) == "" {
		return nil, "", ErrNotFound
	}
	rc, _, err := s.storage.Download(ctx, org.logoKey.String)
	if err != nil {
		return nil, "", ErrNotFound
	}
	return rc, org.logoKey.String, nil
}

func summaries(rows []db.HelpArticlesRow) []ArticleSummary {
	out := make([]ArticleSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, summary(r.Uuid, r.Slug, r.Locale, r.Title, r.Excerpt, r.UpdatedAt, r.CollectionSlug))
	}
	return out
}

func summary(id uuid.UUID, slug, locale, title, body string, updated pgtype.Timestamptz, collection string) ArticleSummary {
	a := ArticleSummary{UUID: id, Slug: slug, Locale: locale, Title: title, Excerpt: Excerpt(body, excerptRunes), UpdatedAt: updated.Time.UTC()}
	if collection != "" {
		a.CollectionSlug = &collection
	}
	return a
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

var (
	mdFence    = regexp.MustCompile("(?s)```.*?```")
	mdImage    = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	mdLink     = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	mdTag      = regexp.MustCompile(`<[^>]+>`)
	mdLineMark = regexp.MustCompile(`(?m)^\s{0,3}(#{1,6}\s+|>\s?|[-*+]\s+(\[[ xX]\]\s+)?|\d+[.)]\s+|\|)`)
	mdInline   = regexp.MustCompile("[*_~`|]+")
	mdRule     = regexp.MustCompile(`(?m)^\s*([-*_=]\s*){3,}$`)
)

// Excerpt turns markdown into one plain-text line of at most n runes.
func Excerpt(markdown string, n int) string {
	s := mdFence.ReplaceAllString(markdown, " ")
	s = mdImage.ReplaceAllString(s, " ")
	s = mdLink.ReplaceAllString(s, "$1")
	s = mdTag.ReplaceAllString(s, " ")
	s = mdRule.ReplaceAllString(s, " ")
	s = mdLineMark.ReplaceAllString(s, "")
	s = mdInline.ReplaceAllString(s, "")
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)[:n]
	cut := string(runes)
	if i := strings.LastIndex(cut, " "); i > n/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,.;:") + "…"
}
