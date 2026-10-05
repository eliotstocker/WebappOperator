package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/eliotstocker/webapp-operator/internal/cache"
)

// setupBenchmarkEnv creates an on-disk mock revision with typical SPA assets.
func setupBenchmarkEnv(b *testing.B) (cache.Manager, *Router, func()) {
	b.Helper()
	tempDir, err := os.MkdirTemp("", "webapp-bench-*")
	if err != nil {
		b.Fatalf("failed to create temp bench dir: %v", err)
	}

	cm, err := cache.NewDiskManager(tempDir)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		b.Fatalf("failed to create disk manager: %v", err)
	}

	// Create a revision directory structure
	revDir := cm.RevisionDir("prod", "my-store", "rev-abc1234")
	if err := os.MkdirAll(filepath.Join(revDir, "assets"), 0755); err != nil {
		_ = os.RemoveAll(tempDir)
		b.Fatalf("failed to create revision dir: %v", err)
	}

	// 1. index.html (SPA entry)
	indexHTML := []byte(`<!DOCTYPE html><html><head><title>App</title></head><body><div id="root"></div></body></html>`)
	if err := os.WriteFile(filepath.Join(revDir, "index.html"), indexHTML, 0644); err != nil {
		_ = os.RemoveAll(tempDir)
		b.Fatalf("failed to write index.html: %v", err)
	}

	// 2. Large static asset (100KB typical JS bundle)
	jsPayload := make([]byte, 100*1024)
	for i := range jsPayload {
		jsPayload[i] = byte('a' + (i % 26))
	}
	if err := os.WriteFile(filepath.Join(revDir, "assets", "app.deadbeef.js"), jsPayload, 0644); err != nil {
		_ = os.RemoveAll(tempDir)
		b.Fatalf("failed to write asset: %v", err)
	}

	// 3. Small static asset (5KB CSS stylesheet)
	cssPayload := make([]byte, 5*1024)
	for i := range cssPayload {
		cssPayload[i] = byte('a' + (i % 26))
	}
	if err := os.WriteFile(filepath.Join(revDir, "assets", "style.12345.css"), cssPayload, 0644); err != nil {
		_ = os.RemoveAll(tempDir)
		b.Fatalf("failed to write css asset: %v", err)
	}

	rt := NewRouter(cm)
	rt.SetWebsiteRoutes([]string{"store.example.com"}, RouteTarget{
		Namespace:     "prod",
		WebsiteName:   "my-store",
		RevisionName:  "rev-abc1234",
		InjectionMode: "both",
		ConfigPath:    "/_config.js",
		Env: map[string]string{
			"API_URL":      "https://api.example.com",
			"ENVIRONMENT":  "production",
			"REGION":       "us-east-1",
			"ANALYTICS_ID": "UA-12345678-9",
		},
	})

	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}
	return cm, rt, cleanup
}

// BenchmarkStaticAssetServing benchmarks serving an immutable 100KB JS bundle.
func BenchmarkStaticAssetServing(b *testing.B) {
	_, rt, cleanup := setupBenchmarkEnv(b)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "http://store.example.com/assets/app.deadbeef.js", nil)
	rec := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec.Body.Reset()
		rt.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("expected status 200, got %d", rec.Code)
		}
	}
}

// BenchmarkSmallAssetServing benchmarks serving a small 5KB CSS stylesheet.
func BenchmarkSmallAssetServing(b *testing.B) {
	_, rt, cleanup := setupBenchmarkEnv(b)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "http://store.example.com/assets/style.12345.css", nil)
	rec := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec.Body.Reset()
		rt.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("expected status 200, got %d", rec.Code)
		}
	}
}

// BenchmarkDynamicConfigEndpoint benchmarks generating and serving /_config.js.
func BenchmarkDynamicConfigEndpoint(b *testing.B) {
	_, rt, cleanup := setupBenchmarkEnv(b)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "http://store.example.com/_config.js", nil)
	rec := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec.Body.Reset()
		rt.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("expected status 200, got %d", rec.Code)
		}
	}
}

// BenchmarkSPAFallbackWithInlineInjection benchmarks serving unknown routes falling back to index.html with inline env injection.
func BenchmarkSPAFallbackWithInlineInjection(b *testing.B) {
	_, rt, cleanup := setupBenchmarkEnv(b)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "http://store.example.com/products/item-987/view", nil)
	rec := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec.Body.Reset()
		rt.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("expected status 200, got %d", rec.Code)
		}
	}
}

// BenchmarkParallelStatic benchmarks high-concurrency throughput across available cores.
func BenchmarkParallelStatic(b *testing.B) {
	_, rt, cleanup := setupBenchmarkEnv(b)
	defer cleanup()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		req := httptest.NewRequest(http.MethodGet, "http://store.example.com/assets/app.deadbeef.js", nil)
		rec := httptest.NewRecorder()
		for pb.Next() {
			rec.Body.Reset()
			rt.ServeHTTP(rec, req)
		}
	})
}

// BenchmarkParallelSmallAsset benchmarks high-concurrency throughput for small 5KB CSS assets.
func BenchmarkParallelSmallAsset(b *testing.B) {
	_, rt, cleanup := setupBenchmarkEnv(b)
	defer cleanup()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		req := httptest.NewRequest(http.MethodGet, "http://store.example.com/assets/style.12345.css", nil)
		rec := httptest.NewRecorder()
		for pb.Next() {
			rec.Body.Reset()
			rt.ServeHTTP(rec, req)
		}
	})
}

// BenchmarkParallelConfig benchmarks high-concurrency throughput for /_config.js.
func BenchmarkParallelConfig(b *testing.B) {
	_, rt, cleanup := setupBenchmarkEnv(b)
	defer cleanup()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		req := httptest.NewRequest(http.MethodGet, "http://store.example.com/_config.js", nil)
		rec := httptest.NewRecorder()
		for pb.Next() {
			rec.Body.Reset()
			rt.ServeHTTP(rec, req)
		}
	})
}

// BenchmarkParallelSPAFallback benchmarks high-concurrency throughput for SPA route fallback with inline script injection.
func BenchmarkParallelSPAFallback(b *testing.B) {
	_, rt, cleanup := setupBenchmarkEnv(b)
	defer cleanup()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		req := httptest.NewRequest(http.MethodGet, "http://store.example.com/checkout/step-2", nil)
		rec := httptest.NewRecorder()
		for pb.Next() {
			rec.Body.Reset()
			rt.ServeHTTP(rec, req)
		}
	})
}

// TestServerMemoryFootprintUnderLoad measures actual heap and OS memory footprint before and after 50,000 requests.
func TestServerMemoryFootprintUnderLoad(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "webapp-mem-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cm, err := cache.NewDiskManager(tempDir)
	if err != nil {
		t.Fatalf("failed to create disk manager: %v", err)
	}

	revDir := cm.RevisionDir("prod", "my-store", "rev-1")
	_ = os.MkdirAll(filepath.Join(revDir, "assets"), 0755)
	_ = os.WriteFile(filepath.Join(revDir, "index.html"), []byte("<html><body>Hello</body></html>"), 0644)
	_ = os.WriteFile(filepath.Join(revDir, "assets", "app.js"), []byte("console.log('app');"), 0644)

	rt := NewRouter(cm)
	rt.SetWebsiteRoutes([]string{"store.example.com"}, RouteTarget{
		Namespace:     "prod",
		WebsiteName:   "my-store",
		RevisionName:  "rev-1",
		InjectionMode: "both",
		ConfigPath:    "/_config.js",
		Env:           map[string]string{"KEY": "VALUE"},
	})

	var memStart runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memStart)

	// Send 50,000 requests across various endpoints
	paths := []string{"/assets/app.js", "/_config.js", "/checkout", "/profile"}
	for i := 0; i < 50000; i++ {
		path := paths[i%len(paths)]
		req := httptest.NewRequest(http.MethodGet, "http://store.example.com"+path, nil)
		rec := httptest.NewRecorder()
		rt.ServeHTTP(rec, req)
	}

	var memEnd runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memEnd)

	t.Logf("Memory stats:")
	t.Logf("  Start HeapAlloc: %.2f KB", float64(memStart.HeapAlloc)/1024)
	t.Logf("  End   HeapAlloc: %.2f KB", float64(memEnd.HeapAlloc)/1024)
	t.Logf("  Heap InUse:      %.2f KB", float64(memEnd.HeapInuse)/1024)
	t.Logf("  Total Sys:       %.2f MB", float64(memEnd.Sys)/(1024*1024))
}
