package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eliotstocker/webapp-operator/internal/cache"
)

func TestServerRoutingAndConfig(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "webapp-server-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	mgr, err := cache.NewDiskManager(tempDir)
	if err != nil {
		t.Fatalf("failed to init disk manager: %v", err)
	}

	router := NewRouter(mgr)

	ns := "default"
	web := "my-spa"
	rev := "rev-123"

	// Create dummy files on disk
	revDir := mgr.RevisionDir(ns, web, rev)
	if err := os.MkdirAll(filepath.Join(revDir, "assets"), 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	_ = os.WriteFile(filepath.Join(revDir, "index.html"), []byte("<html><head></head><body>SPA</body></html>"), 0644)
	_ = os.WriteFile(filepath.Join(revDir, "assets", "main.js"), []byte("console.log('main');"), 0644)
	_ = os.WriteFile(filepath.Join(revDir, "favicon.png"), []byte("pngdata"), 0644)

	router.SetWebsiteRoutes([]string{"spa.example.com"}, RouteTarget{
		Namespace:     ns,
		WebsiteName:   web,
		RevisionName:  rev,
		Env:           map[string]string{"API_HOST": "https://api.test"},
		InjectionMode: "both",
		ConfigPath:    "/_config.js",
	})

	// 1. Test /_config.js dynamic endpoint
	req := httptest.NewRequest(http.MethodGet, "/_config.js", nil)
	req.Host = "spa.example.com:8080" // test with port in host
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /_config.js, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "API_HOST") {
		t.Fatalf("missing env var in config js: %s", rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-cache, no-store, must-revalidate" {
		t.Errorf("expected no-cache header, got %s", rec.Header().Get("Cache-Control"))
	}

	// 2. Test static asset /assets/main.js
	req = httptest.NewRequest(http.MethodGet, "/assets/main.js", nil)
	req.Host = "spa.example.com"
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /assets/main.js, got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("expected immutable cache control header, got %s", rec.Header().Get("Cache-Control"))
	}

	// 3. Test non-assets root immutable file (/favicon.png)
	req = httptest.NewRequest(http.MethodGet, "/favicon.png", nil)
	req.Host = "spa.example.com"
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /favicon.png, got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("expected immutable cache header for .png")
	}

	// 4. Test root / with inline injection (mode: both)
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "spa.example.com"
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<script id=\"__ENV__\">") {
		t.Errorf("expected inline script injected into index.html")
	}

	// 5. Test SPA fallback on unmatched route (e.g. /dashboard/settings)
	req = httptest.NewRequest(http.MethodGet, "/dashboard/settings", nil)
	req.Host = "spa.example.com"
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for SPA fallback, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<body>SPA</body>") {
		t.Fatalf("expected index.html body on fallback, got %s", rec.Body.String())
	}

	// 6. Test unknown host -> 404
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "unknown.example.com"
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown host, got %d", rec.Code)
	}

	// 7. Test uncached target -> 503
	router.SetWebsiteRoutes([]string{"uncached.example.com"}, RouteTarget{
		Namespace:    "ns",
		WebsiteName:  "web",
		RevisionName: "rev-uncached",
	})
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "uncached.example.com"
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 for uncached revision, got %d", rec.Code)
	}

	// 8. Test RemoveWebsiteRoutes
	router.RemoveWebsiteRoutes([]string{"spa.example.com"})
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "spa.example.com"
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 after RemoveWebsiteRoutes, got %d", rec.Code)
	}
}

func TestIsImmutableAsset(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"bundle.js", true},
		{"style.css", true},
		{"img.png", true},
		{"photo.jpg", true},
		{"icon.svg", true},
		{"font.woff2", true},
		{"data.json", false},
		{"index.html", false},
	}

	for _, c := range cases {
		got := isImmutableAsset(c.path)
		if got != c.want {
			t.Errorf("isImmutableAsset(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}
