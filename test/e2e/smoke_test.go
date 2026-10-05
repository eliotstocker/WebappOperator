package e2e

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"

	"github.com/eliotstocker/webapp-operator/internal/cache"
	"github.com/eliotstocker/webapp-operator/internal/oci"
	"github.com/eliotstocker/webapp-operator/internal/server"
)

// Helper to build a synthetic OCI image containing a set of files
func buildImageWithFiles(files []struct{ Name, Body string }) (v1.Image, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	for _, f := range files {
		hdr := &tar.Header{
			Name: f.Name,
			Mode: 0644,
			Size: int64(len(f.Body)),
		}
		if strings.HasSuffix(f.Name, "/") {
			hdr.Typeflag = tar.TypeDir
			hdr.Mode = 0755
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}
		if !strings.HasSuffix(f.Name, "/") {
			if _, err := tw.Write([]byte(f.Body)); err != nil {
				return nil, err
			}
		}
	}
	_ = tw.Close()

	layer, err := tarball.LayerFromReader(&buf)
	if err != nil {
		return nil, err
	}
	return mutate.AppendLayers(empty.Image, layer)
}

func TestEndToEndSmokeWorkflow(t *testing.T) {
	// 1. Start an in-memory OCI registry
	regServer := httptest.NewServer(registry.New())
	defer regServer.Close()

	regHost := strings.TrimPrefix(regServer.URL, "http://")
	imageRefStr := regHost + "/org/demo-spa:v1.0.0"

	// 2. Build and push sample SPA OCI image to the in-memory registry
	img, err := buildImageWithFiles([]struct{ Name, Body string }{
		{"index.html", "<!DOCTYPE html><html><head><title>Smoke App</title></head><body><div id=\"root\">App Loaded</div></body></html>"},
		{"assets/", ""},
		{"assets/main.js", "console.log('SPA started');"},
		{"assets/style.css", "body { font-family: sans-serif; }"},
	})
	if err != nil {
		t.Fatalf("failed to build test image: %v", err)
	}

	ref, err := name.ParseReference(imageRefStr)
	if err != nil {
		t.Fatalf("failed to parse image ref: %v", err)
	}

	if err := remote.Write(ref, img, remote.WithTransport(http.DefaultTransport)); err != nil {
		t.Fatalf("failed to push image to in-memory registry: %v", err)
	}

	// 3. Initialize cache manager & puller
	tempDir := t.TempDir()
	cacheMgr, err := cache.NewDiskManager(tempDir)
	if err != nil {
		t.Fatalf("failed to init cache manager: %v", err)
	}

	puller := oci.NewPuller()

	ns := "production"
	webName := "demo-spa"
	revName := "demo-spa-abc12345"

	// 4. Emulate controller pulling OCI image and caching it
	ctx := context.Background()
	unpackedDir, err := cacheMgr.EnsureRevision(ctx, ns, webName, revName, func(destDir string) error {
		res, err := puller.PullAndExtract(ctx, imageRefStr, destDir, nil)
		if err != nil {
			return err
		}
		if res.Digest == "" {
			t.Errorf("expected non-empty digest")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("EnsureRevision failed: %v", err)
	}

	if !cacheMgr.IsCached(ns, webName, revName) {
		t.Fatalf("expected revision to be cached at %s", unpackedDir)
	}

	// 5. Initialize HTTP router and register website route
	router := server.NewRouter(cacheMgr)
	router.SetWebsiteRoutes([]string{"demo.example.com"}, server.RouteTarget{
		Namespace:     ns,
		WebsiteName:   webName,
		RevisionName:  revName,
		Env:           map[string]string{"API_ENDPOINT": "https://api.production.internal", "FEATURE_X": "enabled"},
		InjectionMode: "both",
		ConfigPath:    "/_config.js",
	})

	serverMux := http.NewServeMux()
	serverMux.Handle("/", router)
	httpServer := httptest.NewServer(serverMux)
	defer httpServer.Close()

	client := httpServer.Client()

	doGet := func(path string) (*http.Response, string) {
		req, err := http.NewRequest(http.MethodGet, httpServer.URL+path, nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		req.Host = "demo.example.com"
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request to %s failed: %v", path, err)
		}
		defer resp.Body.Close()
		var b bytes.Buffer
		_, _ = b.ReadFrom(resp.Body)
		return resp, b.String()
	}

	// Scenario A: Dynamic Config JS Endpoint
	resp, body := doGet("/_config.js")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for /_config.js, got %d", resp.StatusCode)
	}
	if !strings.Contains(body, "https://api.production.internal") {
		t.Errorf("expected API_ENDPOINT in config.js, got: %s", body)
	}

	// Scenario B: Root index.html with inline injection
	resp, body = doGet("/")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for /, got %d", resp.StatusCode)
	}
	if !strings.Contains(body, "<script id=\"__ENV__\">") {
		t.Errorf("expected inline script in index.html, got: %s", body)
	}

	// Scenario C: Immutable asset serving
	resp, body = doGet("/assets/main.js")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for /assets/main.js, got %d", resp.StatusCode)
	}
	if !strings.Contains(body, "console.log('SPA started');") {
		t.Errorf("unexpected asset content: %s", body)
	}

	// Scenario D: SPA Client-side route fallback
	resp, body = doGet("/dashboard/settings")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for SPA fallback, got %d", resp.StatusCode)
	}
	if !strings.Contains(body, "App Loaded") {
		t.Errorf("expected index.html body on fallback")
	}
}

func TestEndToEndMultiHostAndChunkRetention(t *testing.T) {
	// 1. In-memory registry
	regServer := httptest.NewServer(registry.New())
	defer regServer.Close()
	regHost := strings.TrimPrefix(regServer.URL, "http://")

	// 2. Push Revision 1 (v1.0.0 with chunk-v1.js)
	v1RefStr := fmt.Sprintf("%s/org/store:v1.0.0", regHost)
	imgV1, _ := buildImageWithFiles([]struct{ Name, Body string }{
		{"index.html", "<html><head></head><body>Store V1</body></html>"},
		{"assets/chunk-v1.js", "console.log('v1 chunk');"},
	})
	v1Ref, _ := name.ParseReference(v1RefStr)
	_ = remote.Write(v1Ref, imgV1, remote.WithTransport(http.DefaultTransport))

	// 3. Push Revision 2 (v2.0.0 with chunk-v2.js)
	v2RefStr := fmt.Sprintf("%s/org/store:v2.0.0", regHost)
	imgV2, _ := buildImageWithFiles([]struct{ Name, Body string }{
		{"index.html", "<html><head></head><body>Store V2</body></html>"},
		{"assets/chunk-v2.js", "console.log('v2 chunk');"},
	})
	v2Ref, _ := name.ParseReference(v2RefStr)
	_ = remote.Write(v2Ref, imgV2, remote.WithTransport(http.DefaultTransport))

	// 4. Initialize cache & pull both revisions
	tempDir := t.TempDir()
	cacheMgr, _ := cache.NewDiskManager(tempDir)
	puller := oci.NewPuller()
	ctx := context.Background()

	_, _ = cacheMgr.EnsureRevision(ctx, "prod", "store", "rev-1", func(dest string) error {
		_, err := puller.PullAndExtract(ctx, v1RefStr, dest, nil)
		return err
	})
	_, _ = cacheMgr.EnsureRevision(ctx, "prod", "store", "rev-2", func(dest string) error {
		_, err := puller.PullAndExtract(ctx, v2RefStr, dest, nil)
		return err
	})

	// 5. Setup Router with 2 isolated hosts:
	// - "store.example.com" -> active rev-2
	// - "admin.example.com" -> active rev-1
	router := server.NewRouter(cacheMgr)
	router.SetWebsiteRoutes([]string{"store.example.com"}, server.RouteTarget{
		Namespace:     "prod",
		WebsiteName:   "store",
		RevisionName:  "rev-2",
		Env:           map[string]string{"STORE_ID": "store-us"},
		InjectionMode: "endpoint",
		ConfigPath:    "/_config.js",
	})
	router.SetWebsiteRoutes([]string{"admin.example.com"}, server.RouteTarget{
		Namespace:     "prod",
		WebsiteName:   "store",
		RevisionName:  "rev-1",
		Env:           map[string]string{"STORE_ID": "admin-internal"},
		InjectionMode: "endpoint",
		ConfigPath:    "/_config.js",
	})

	httpServer := httptest.NewServer(router)
	defer httpServer.Close()
	client := httpServer.Client()

	sendReq := func(host, path string) (int, string) {
		req, _ := http.NewRequest(http.MethodGet, httpServer.URL+path, nil)
		req.Host = host
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		var b bytes.Buffer
		_, _ = b.ReadFrom(resp.Body)
		return resp.StatusCode, b.String()
	}

	// Verify Store (rev-2)
	code, body := sendReq("store.example.com", "/")
	if code != 200 || !strings.Contains(body, "Store V2") {
		t.Errorf("expected Store V2, got %s", body)
	}
	code, body = sendReq("store.example.com", "/_config.js")
	if !strings.Contains(body, "store-us") {
		t.Errorf("expected store-us env var")
	}
	code, body = sendReq("store.example.com", "/assets/chunk-v2.js")
	if code != 200 || !strings.Contains(body, "v2 chunk") {
		t.Errorf("expected 200 for v2 chunk")
	}

	// Verify Admin (rev-1) isolated on same cluster
	code, body = sendReq("admin.example.com", "/")
	if code != 200 || !strings.Contains(body, "Store V1") {
		t.Errorf("expected Store V1 for admin, got %s", body)
	}
	code, body = sendReq("admin.example.com", "/_config.js")
	if !strings.Contains(body, "admin-internal") {
		t.Errorf("expected admin-internal env var")
	}
	code, body = sendReq("admin.example.com", "/assets/chunk-v1.js")
	if code != 200 || !strings.Contains(body, "v1 chunk") {
		t.Errorf("expected 200 for v1 chunk")
	}
}
