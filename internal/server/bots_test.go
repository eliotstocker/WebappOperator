package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	webappv1alpha1 "github.com/eliotstocker/webapp-operator/api/v1alpha1"
	"github.com/eliotstocker/webapp-operator/internal/cache"
)

func boolPtr(b bool) *bool {
	return &b
}

func TestCompileBotPage(t *testing.T) {
	if got := CompileBotPage(nil); got != nil {
		t.Fatalf("expected nil for nil spec, got %+v", got)
	}

	if got := CompileBotPage(&webappv1alpha1.BotPageSpec{Enabled: boolPtr(false)}); got != nil {
		t.Fatalf("expected nil when explicitly disabled, got %+v", got)
	}

	if got := CompileBotPage(&webappv1alpha1.BotPageSpec{OverrideDefaults: true}); got != nil {
		t.Fatalf("expected nil when overrideDefaults=true and no userAgents provided, got %+v", got)
	}

	// Default configuration enables built-in crawlers and defaults file to bot.html
	cb := CompileBotPage(&webappv1alpha1.BotPageSpec{})
	if cb == nil {
		t.Fatal("expected non-nil CompiledBotPage for empty non-nil spec")
	}
	if cb.File() != "bot.html" {
		t.Errorf("expected default file bot.html, got %q", cb.File())
	}
	if !cb.IsBot("Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)") {
		t.Error("expected Googlebot to match defaults")
	}
	if !cb.IsBot("Slackbot-LinkExpanding 1.0 (+https://api.slack.com/robots)") {
		t.Error("expected Slackbot to match defaults")
	}
	if !cb.IsBot("Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko); compatible; GPTBot/1.0") {
		t.Error("expected GPTBot to match defaults")
	}
	if cb.IsBot("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36") {
		t.Error("expected normal Chrome browser not to match")
	}
	if cb.IsBot("") {
		t.Error("expected empty User-Agent not to match")
	}

	// Custom userAgents extend defaults by default
	cbExtend := CompileBotPage(&webappv1alpha1.BotPageSpec{
		File:       "seo/crawler.html",
		UserAgents: []string{"MyInternalScraper", "  "},
	})
	if cbExtend.File() != "seo/crawler.html" {
		t.Errorf("expected file seo/crawler.html, got %q", cbExtend.File())
	}
	if !cbExtend.IsBot("myinternalscraper/2.0") {
		t.Error("expected custom userAgent to match case-insensitively")
	}
	if !cbExtend.IsBot("Twitterbot/1.0") {
		t.Error("expected built-in Twitterbot to still match when overrideDefaults is false")
	}

	// OverrideDefaults restricts matching exclusively to custom userAgents
	cbOverride := CompileBotPage(&webappv1alpha1.BotPageSpec{
		UserAgents:       []string{"OnlyThisBot"},
		OverrideDefaults: true,
	})
	if !cbOverride.IsBot("OnlyThisBot/1.0") {
		t.Error("expected OnlyThisBot to match")
	}
	if cbOverride.IsBot("Googlebot/2.1") {
		t.Error("expected Googlebot not to match when overrideDefaults is true")
	}

	// Path traversal sanitization
	cbTraversal := CompileBotPage(&webappv1alpha1.BotPageSpec{
		File: "../../etc/passwd",
	})
	if cbTraversal.File() != "etc/passwd" {
		t.Errorf("expected traversal stripped to etc/passwd, got %q", cbTraversal.File())
	}
}

func TestRouterBotPageServing(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "webapp-bot-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	mgr, err := cache.NewDiskManager(tempDir)
	if err != nil {
		t.Fatalf("failed to init disk manager: %v", err)
	}

	router := NewRouter(mgr)

	ns, web, rev := "default", "my-app", "rev-1"
	revDir := mgr.RevisionDir(ns, web, rev)
	if err := os.MkdirAll(filepath.Join(revDir, "assets"), 0755); err != nil {
		t.Fatalf("failed to create dirs: %v", err)
	}
	_ = os.WriteFile(filepath.Join(revDir, "index.html"), []byte("<html><head></head><body>HUMAN SPA</body></html>"), 0644)
	_ = os.WriteFile(filepath.Join(revDir, "bot.html"), []byte("<html><head><title>SEO</title></head><body>BOT PAGE</body></html>"), 0644)
	_ = os.WriteFile(filepath.Join(revDir, "robots.txt"), []byte("User-agent: *\nAllow: /\n"), 0644)
	_ = os.WriteFile(filepath.Join(revDir, "assets", "app.js"), []byte("console.log('ok');"), 0644)

	router.SetWebsiteRoutes([]string{"app.example.com"}, RouteTarget{
		Namespace:     ns,
		WebsiteName:   web,
		RevisionName:  rev,
		Image:         "ghcr.io/example/app:v1",
		Env:           map[string]string{"ENV": "prod"},
		InjectionMode: "inline",
		BotPage: &webappv1alpha1.BotPageSpec{
			File: "bot.html",
		},
	})

	// 1. Bot requesting / receives bot.html without inline __ENV__ script, with Vary header
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "app.example.com"
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for bot /, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "BOT PAGE") {
		t.Errorf("expected bot.html body, got: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "__ENV__") {
		t.Errorf("did not expect inline env script injected into bot.html")
	}
	if rec.Header().Get("Vary") != "User-Agent" {
		t.Errorf("expected Vary: User-Agent, got %q", rec.Header().Get("Vary"))
	}

	// 2. Bot requesting SPA subroute (/products/123) also receives bot.html
	req = httptest.NewRequest(http.MethodGet, "/products/123", nil)
	req.Host = "app.example.com"
	req.Header.Set("User-Agent", "Discordbot/2.0")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "BOT PAGE") {
		t.Fatalf("expected bot.html on SPA route, got %d: %s", rec.Code, rec.Body.String())
	}

	// 3. Bot requesting static files (robots.txt and assets/app.js) still gets actual files
	for _, path := range []string{"/robots.txt", "/assets/app.js"} {
		req = httptest.NewRequest(http.MethodGet, path, nil)
		req.Host = "app.example.com"
		req.Header.Set("User-Agent", "Googlebot/2.1")
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", path, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "BOT PAGE") {
			t.Errorf("expected actual static file for %s, got bot.html", path)
		}
	}

	// 4. Normal browser receives index.html with inline injection and Vary: User-Agent
	req = httptest.NewRequest(http.MethodGet, "/products/123", nil)
	req.Host = "app.example.com"
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Chrome/120.0.0.0")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for human request, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "HUMAN SPA") {
		t.Errorf("expected index.html body for human, got: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "__ENV__") {
		t.Errorf("expected inline env script in index.html for human")
	}
	if rec.Header().Get("Vary") != "User-Agent" {
		t.Errorf("expected Vary: User-Agent on human HTML response when botPage is enabled, got %q", rec.Header().Get("Vary"))
	}

	// 5. Missing bot file falls back gracefully to index.html
	router.SetWebsiteRoutes([]string{"missing.example.com"}, RouteTarget{
		Namespace:    ns,
		WebsiteName:  web,
		RevisionName: rev,
		BotPage: &webappv1alpha1.BotPageSpec{
			File: "does-not-exist.html",
		},
	})
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "missing.example.com"
	req.Header.Set("User-Agent", "Googlebot/2.1")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "HUMAN SPA") {
		t.Fatalf("expected fallback to index.html when bot file is missing, got %d: %s", rec.Code, rec.Body.String())
	}

	// 6. Disabled botPage serves index.html and does not emit Vary: User-Agent
	router.SetWebsiteRoutes([]string{"off.example.com"}, RouteTarget{
		Namespace:    ns,
		WebsiteName:  web,
		RevisionName: rev,
		BotPage: &webappv1alpha1.BotPageSpec{
			Enabled: boolPtr(false),
			File:    "bot.html",
		},
	})
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "off.example.com"
	req.Header.Set("User-Agent", "Googlebot/2.1")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "HUMAN SPA") {
		t.Fatalf("expected index.html when botPage disabled, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Vary") != "" {
		t.Errorf("expected no Vary header when botPage disabled, got %q", rec.Header().Get("Vary"))
	}
}
