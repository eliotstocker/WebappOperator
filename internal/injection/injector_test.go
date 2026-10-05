package injection

import (
	"strings"
	"testing"
)

func TestGenerateConfigJS(t *testing.T) {
	// 1. Map with values
	env := map[string]string{
		"API_URL": "https://api.example.com",
		"DEBUG":   "false",
	}

	js, err := GenerateConfigJS(env)
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

	// 2. Nil map
	jsNil, err := GenerateConfigJS(nil)
	if err != nil {
		t.Fatalf("unexpected error on nil env: %v", err)
	}
	if !strings.Contains(string(jsNil), "{}") {
		t.Errorf("expected empty object for nil map, got: %s", string(jsNil))
	}
}

func TestInjectInlineScript(t *testing.T) {
	env := map[string]string{"KEY": "VALUE"}

	// 1. With </head>
	htmlHead := []byte(`<!DOCTYPE html><html><head><title>App</title></head><body><div id="root"></div></body></html>`)
	injected, err := InjectInlineScript(htmlHead, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(injected), `<script id="__ENV__">`) {
		t.Errorf("missing script tag: %s", string(injected))
	}
	headIdx := strings.Index(string(injected), "</head>")
	scriptIdx := strings.Index(string(injected), `<script id="__ENV__">`)
	if scriptIdx >= headIdx {
		t.Errorf("script tag should appear before </head>")
	}

	// 2. Fallback when head tag does not exist (prepends script)
	htmlBare := []byte(`<div>raw fragment</div>`)
	injectedBare, err := InjectInlineScript(htmlBare, env)
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
