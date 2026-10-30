package usecase

import (
	"context"
	"strings"
	"testing"
)

func TestRobotsSpecificGroupsWildcardsAndAllowPrecedence(t *testing.T) {
	policy, err := parseRobots("User-agent: *\nDisallow: /\nUser-agent: ClickFixKnowledgeBot\nDisallow: /private/*\nAllow: /private/public$\nSitemap: https://docs.example.com/sitemap.xml\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path    string
		allowed bool
	}{{"/help", true}, {"/private/x", false}, {"/private/public", true}, {"/private/public/child", false}} {
		u, err := toolURL("https://docs.example.com" + tc.path)
		if err != nil {
			t.Fatal(err)
		}
		if got := policy.allowed(u); got != tc.allowed {
			t.Errorf("path %s allowed=%v", tc.path, got)
		}
	}
}
func TestCrawlerRobotsSitemapAndSameOrigin(t *testing.T) {
	fetched := map[string]int{}
	pages := map[string]string{
		"https://docs.example.com/robots.txt":  "User-agent: *\nDisallow: /private\nSitemap: https://docs.example.com/sitemap.xml",
		"https://docs.example.com/sitemap.xml": `<urlset><url><loc>https://docs.example.com/private/secret</loc></url><url><loc>https://docs.example.com/second</loc></url><url><loc>https://127.0.0.1/admin</loc></url></urlset>`,
		"https://docs.example.com/guide":       "<title>Guide</title><p>Support</p><script>hidden secret</script><a href='/private/x'>private</a><a href='https://other.example.org/x'>other</a>",
		"https://docs.example.com/second":      "<title>Second</title><p>Answer</p>",
	}
	fetch := func(_ context.Context, target string) ([]byte, int, error) {
		fetched[target]++
		if body, ok := pages[target]; ok {
			return []byte(body), 200, nil
		}
		t.Errorf("unexpected fetch %s", target)
		return nil, 404, nil
	}
	docs, err := CrawlKnowledge(context.Background(), "https://docs.example.com/guide", 1, fetch)
	if err != nil || len(docs) != 2 {
		t.Fatalf("crawl %+v %v", docs, err)
	}
	for _, doc := range docs {
		if strings.Contains(doc.Body, "hidden secret") {
			t.Fatal("script indexed")
		}
	}
	if len(fetched) != 4 {
		t.Fatalf("disallowed/cross-origin links fetched: %v", fetched)
	}
}
func TestCrawlerFailsClosedOnRobotsAndInvalidSource(t *testing.T) {
	for _, status := range []int{401, 403, 500, 302} {
		fetch := func(context.Context, string) ([]byte, int, error) { return nil, status, nil }
		if _, err := CrawlKnowledge(context.Background(), "https://docs.example.com/", 0, fetch); err == nil {
			t.Errorf("robots status %d allowed", status)
		}
	}
	for _, target := range []string{"http://docs.example.com/", "https://127.0.0.1/", "https://169.254.169.254/latest/", "https://example.com:8443/", "https://user:pass@example.com/"} {
		if _, err := CrawlKnowledge(context.Background(), target, 0, nil); err == nil {
			t.Errorf("unsafe target allowed %s", target)
		}
	}
}
