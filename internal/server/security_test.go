package server

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	webappv1alpha1 "github.com/eliotstocker/webapp-operator/api/v1alpha1"
	"github.com/eliotstocker/webapp-operator/internal/cache"
)

func boolPtr(b bool) *bool { return &b }
func int32Ptr(i int32) *int32 { return &i }

func TestCompileSecurityNil(t *testing.T) {
	if cs := CompileSecurity(nil); cs != nil {
		t.Fatalf("expected nil CompiledSecurity for nil spec")
	}
}

func TestCompileSecurityStandardPreset(t *testing.T) {
	cs := CompileSecurity(&webappv1alpha1.SecuritySpec{
		Preset: webappv1alpha1.SecurityPresetStandard,
	})
	rec := httptest.NewRecorder()
	cs.Apply(rec, "")

	h := rec.Header()
	if got := h.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := h.Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options = %q, want DENY", got)
	}
	if got := h.Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
		t.Errorf("Referrer-Policy = %q", got)
	}
	if got := h.Get("Cross-Origin-Opener-Policy"); got != "same-origin-allow-popups" {
		t.Errorf("COOP = %q", got)
	}
	if got := h.Get("X-XSS-Protection"); got != "0" {
		t.Errorf("X-XSS-Protection = %q, want 0", got)
	}
	if got := h.Get("Permissions-Policy"); got != "camera=(), geolocation=(), microphone=(), payment=(), usb=()" {
		t.Errorf("Permissions-Policy = %q", got)
	}
	if got := h.Get("Strict-Transport-Security"); got != "" {
		t.Errorf("expected no HSTS in standard preset, got %q", got)
	}
	if got := h.Get("Content-Security-Policy"); got != "" {
		t.Errorf("expected no CSP in standard preset, got %q", got)
	}
}

func TestCompileSecurityOverridesAndStructuredFields(t *testing.T) {
	cs := CompileSecurity(&webappv1alpha1.SecuritySpec{
		Preset:       webappv1alpha1.SecurityPresetStandard,
		FrameOptions: "SAMEORIGIN",
		COOP:         "disabled",
		CORP:         "same-site",
		COEP:         "credentialless",
		HSTS: &webappv1alpha1.HSTSSpec{
			Enabled:           boolPtr(true),
			MaxAge:            int32Ptr(63072000),
			IncludeSubDomains: true,
			Preload:           true,
		},
		PermissionsPolicy: &webappv1alpha1.PermissionsPolicySpec{
			Camera:      []string{"self"},
			Geolocation: []string{"self", "https://maps.example.com"},
			Fullscreen:  []string{"*"},
			Extra: map[string][]string{
				"midi": {},
			},
		},
		CSP: &webappv1alpha1.CSPSpec{
			ReportOnly: true,
			Directives: &webappv1alpha1.CSPDirectives{
				DefaultSrc:              []string{"'self'"},
				ScriptSrc:               []string{"'self'"},
				ConnectSrc:              []string{"'self'", "https://api.example.com"},
				UpgradeInsecureRequests: true,
			},
		},
		CustomHeaders: map[string]string{
			"X-DNS-Prefetch-Control": "off",
		},
	})

	rec := httptest.NewRecorder()
	cs.Apply(rec, "")
	h := rec.Header()

	if got := h.Get("X-Frame-Options"); got != "SAMEORIGIN" {
		t.Errorf("X-Frame-Options = %q, want SAMEORIGIN", got)
	}
	if got := h.Get("Cross-Origin-Opener-Policy"); got != "" {
		t.Errorf("expected COOP disabled, got %q", got)
	}
	if got := h.Get("Cross-Origin-Resource-Policy"); got != "same-site" {
		t.Errorf("CORP = %q, want same-site", got)
	}
	if got := h.Get("Cross-Origin-Embedder-Policy"); got != "credentialless" {
		t.Errorf("COEP = %q, want credentialless", got)
	}
	if got := h.Get("Strict-Transport-Security"); got != "max-age=63072000; includeSubDomains; preload" {
		t.Errorf("HSTS = %q", got)
	}
	wantPerms := `camera=(self), fullscreen=*, geolocation=(self "https://maps.example.com"), microphone=(), midi=(), payment=(), usb=()`
	if got := h.Get("Permissions-Policy"); got != wantPerms {
		t.Errorf("Permissions-Policy = %q, want %q", got, wantPerms)
	}
	wantCSP := "default-src 'self'; script-src 'self'; connect-src 'self' https://api.example.com; upgrade-insecure-requests"
	if got := h.Get("Content-Security-Policy-Report-Only"); got != wantCSP {
		t.Errorf("CSP-Report-Only = %q, want %q", got, wantCSP)
	}
	if got := h.Get("X-DNS-Prefetch-Control"); got != "off" {
		t.Errorf("X-DNS-Prefetch-Control = %q, want off", got)
	}
}

func TestRouterSecurityHeadersAndInlineCSPHash(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "webapp-security-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	mgr, err := cache.NewDiskManager(tempDir)
	if err != nil {
		t.Fatalf("failed to init disk manager: %v", err)
	}

	router := NewRouter(mgr)
	ns, web, rev := "default", "secure-spa", "rev-1"
	revDir := mgr.RevisionDir(ns, web, rev)
	if err := os.MkdirAll(filepath.Join(revDir, "assets"), 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	_ = os.WriteFile(filepath.Join(revDir, "index.html"), []byte("<html><head></head><body>OK</body></html>"), 0644)
	_ = os.WriteFile(filepath.Join(revDir, "assets", "app.js"), []byte("console.log(1);"), 0644)

	router.SetWebsiteRoutes([]string{"secure.example.com"}, RouteTarget{
		Namespace:     ns,
		WebsiteName:   web,
		RevisionName:  rev,
		Image:         "ghcr.io/example/spa:v1",
		Env:           map[string]string{"API": "https://api.example.com"},
		InjectionMode: "both",
		Security: &webappv1alpha1.SecuritySpec{
			Preset: webappv1alpha1.SecurityPresetStrict,
		},
	})

	// 1. Static asset receives security headers
	reqAsset := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	reqAsset.Host = "secure.example.com"
	recAsset := httptest.NewRecorder()
	router.ServeHTTP(recAsset, reqAsset)
	if recAsset.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("missing X-Content-Type-Options on static asset")
	}
	if recAsset.Header().Get("Cross-Origin-Resource-Policy") != "same-origin" {
		t.Errorf("missing CORP on static asset")
	}

	// 2. SPA fallback injects inline script and adds matching sha256 hash to script-src
	reqIndex := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	reqIndex.Host = "secure.example.com"
	recIndex := httptest.NewRecorder()
	router.ServeHTTP(recIndex, reqIndex)

	body := recIndex.Body.String()
	start := strings.Index(body, `<script id="__ENV__">`)
	end := strings.Index(body, `</script>`)
	if start == -1 || end == -1 {
		t.Fatalf("expected inline script in HTML body: %s", body)
	}
	scriptContent := body[start+len(`<script id="__ENV__">`) : end]
	sum := sha256.Sum256([]byte(scriptContent))
	expectedHash := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"

	csp := recIndex.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, expectedHash) {
		t.Errorf("expected CSP %q to contain inline script hash %s", csp, expectedHash)
	}
	if recIndex.Header().Get("Strict-Transport-Security") != "max-age=31536000" {
		t.Errorf("expected HSTS header on strict preset, got %q", recIndex.Header().Get("Strict-Transport-Security"))
	}
}
