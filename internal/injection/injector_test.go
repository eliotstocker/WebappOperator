package injection

import (
	"strings"
	"testing"
)

func TestGenerateConfigJS(t *testing.T) {
	// 1. Map with values, version, and polling enabled with default /_version
	env := map[string]string{
		"API_URL": "https://api.example.com",
		"DEBUG":   "false",
	}

	js, err := GenerateConfigJS(env, "rev-v1.0", 30, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	str := string(js)
	if !strings.Contains(str, "window.__ENV__ = Object.freeze(") {
		t.Errorf("missing window.__ENV__ assignment: %s", str)
	}
	if !strings.Contains(str, `"API_URL": "https://api.example.com"`) {
		t.Errorf("missing API_URL in output: %s", str)
	}
	if !strings.Contains(str, `window.__APP_VERSION__ = "rev-v1.0";`) {
		t.Errorf("missing window.__APP_VERSION__: %s", str)
	}
	if !strings.Contains(str, "var pollIntervalMs = 30000;") {
		t.Errorf("expected 30000ms polling interval: %s", str)
	}
	if !strings.Contains(str, "webapp:update") {
		t.Errorf("missing webapp:update custom event dispatch in script: %s", str)
	}
	if !strings.Contains(str, `fetch("/_version"`) {
		t.Errorf("missing /_version fetch: %s", str)
	}

	// 1b. Custom version endpoint path
	jsCustomPath, err := GenerateConfigJS(env, "rev-v1.0", 30, "/api/v1/version")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(jsCustomPath), `fetch("/api/v1/version"`) {
		t.Errorf("expected custom fetch path /api/v1/version, got: %s", string(jsCustomPath))
	}

	// 2. Polling disabled (pollIntervalSeconds = 0)
	jsDisabled, err := GenerateConfigJS(env, "rev-v1.0", 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	strDisabled := string(jsDisabled)
	if !strings.Contains(strDisabled, `window.__APP_VERSION__ = "rev-v1.0";`) {
		t.Errorf("missing window.__APP_VERSION__ when polling disabled: %s", strDisabled)
	}
	if strings.Contains(strDisabled, "pollIntervalMs") {
		t.Errorf("expected no polling watcher when pollIntervalSeconds=0: %s", strDisabled)
	}

	// 3. Nil map, empty version
	jsNil, err := GenerateConfigJS(nil, "", 0, "")
	if err != nil {
		t.Fatalf("unexpected error on nil env: %v", err)
	}
	if !strings.Contains(string(jsNil), "{}") {
		t.Errorf("expected empty object for nil map, got: %s", string(jsNil))
	}
	if strings.Contains(string(jsNil), "__APP_VERSION__") {
		t.Errorf("did not expect __APP_VERSION__ when version empty: %s", string(jsNil))
	}
}

func TestInjectInlineScript(t *testing.T) {
	env := map[string]string{"KEY": "VALUE"}

	// 1. With </head> and custom version path
	htmlHead := []byte(`<!DOCTYPE html><html><head><title>App</title></head><body><div id="root"></div></body></html>`)
	injected, err := InjectInlineScript(htmlHead, env, "rev-456", 15, "/custom-ver")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(injected), `<script id="__ENV__">`) {
		t.Errorf("missing script tag: %s", string(injected))
	}
	if !strings.Contains(string(injected), `window.__APP_VERSION__ = "rev-456";`) {
		t.Errorf("missing app version in inline script: %s", string(injected))
	}
	if !strings.Contains(string(injected), "var pollIntervalMs = 15000;") {
		t.Errorf("missing 15000ms poll interval: %s", string(injected))
	}
	if !strings.Contains(string(injected), `fetch("/custom-ver"`) {
		t.Errorf("missing /custom-ver in inline script: %s", string(injected))
	}
	headIdx := strings.Index(string(injected), "</head>")
	scriptIdx := strings.Index(string(injected), `<script id="__ENV__">`)
	if scriptIdx >= headIdx {
		t.Errorf("script tag should appear before </head>")
	}

	// 2. Fallback when head tag does not exist (prepends script)
	htmlBare := []byte(`<div>raw fragment</div>`)
	injectedBare, err := InjectInlineScript(htmlBare, env, "rev-456", 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(string(injectedBare), `<script id="__ENV__">`) {
		t.Errorf("expected prepended script for bare HTML: %s", string(injectedBare))
	}
}

func TestCleanPath(t *testing.T) {
	if CleanPath("") != "/_config.js" {
		t.Errorf("expected default /_config.js for empty path")
	}
	if CleanPath("env.js") != "/env.js" {
		t.Errorf("expected /env.js for relative path")
	}
	if CleanPath("/custom.js") != "/custom.js" {
		t.Errorf("expected /custom.js for absolute path")
	}
}

func TestCleanVersionPath(t *testing.T) {
	if CleanVersionPath("") != "/_version" {
		t.Errorf("expected default /_version for empty path")
	}
	if CleanVersionPath("version.json") != "/version.json" {
		t.Errorf("expected /version.json for relative path")
	}
	if CleanVersionPath("/api/version") != "/api/version" {
		t.Errorf("expected /api/version for absolute path")
	}
}
