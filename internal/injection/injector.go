// Package injection handles dynamic environment variable injection for client SPAs.
package injection

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// GenerateConfigJS serializes an environment map and version metadata into a JavaScript snippet
// that sets window.__ENV__, window.__APP_VERSION__, and optionally schedules version update polling.
func GenerateConfigJS(env map[string]string, version string, pollIntervalSeconds int32, versionPath string) ([]byte, error) {
	if env == nil {
		env = make(map[string]string)
	}

	payload, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal env map to JSON: %w", err)
	}

	var buf bytes.Buffer
	buf.WriteString("/* Dynamic runtime configuration injected by webapp-operator */\n")
	buf.WriteString("window.__ENV__ = Object.freeze(")
	buf.Write(payload)
	buf.WriteString(");\n")

	if version != "" {
		versionJSON, _ := json.Marshal(version)
		buf.WriteString(fmt.Sprintf("window.__APP_VERSION__ = %s;\n", string(versionJSON)))
	}

	if pollIntervalSeconds > 0 && version != "" {
		pollMs := int64(pollIntervalSeconds) * 1000
		versionJSON, _ := json.Marshal(version)
		vPath := CleanVersionPath(versionPath)
		vPathJSON, _ := json.Marshal(vPath)
		buf.WriteString(fmt.Sprintf(`(function() {
  if (typeof window === "undefined" || window.__WEBAPP_WATCHER_INIT__) return;
  window.__WEBAPP_WATCHER_INIT__ = true;
  var currentVersion = %s;
  var lastNotifiedVersion = currentVersion;
  var pollIntervalMs = %d;
  function checkVersion() {
    fetch(%s, { cache: "no-store" })
      .then(function(res) {
        if (!res.ok) return null;
        return res.json();
      })
      .then(function(data) {
        if (data && data.version && data.version !== currentVersion && data.version !== lastNotifiedVersion) {
          lastNotifiedVersion = data.version;
          var detail = { currentVersion: currentVersion, newVersion: data.version, image: data.image };
          var evt;
          if (typeof CustomEvent === "function") {
            evt = new CustomEvent("webapp:update", { detail: detail, bubbles: true });
          } else if (typeof document !== "undefined" && document.createEvent) {
            evt = document.createEvent("CustomEvent");
            evt.initCustomEvent("webapp:update", true, false, detail);
          }
          if (evt) {
            window.dispatchEvent(evt);
            if (typeof document !== "undefined" && document.dispatchEvent) {
              document.dispatchEvent(evt);
            }
          }
        }
      })
      .catch(function() {});
  }
  setInterval(checkVersion, pollIntervalMs);
})();
`, string(versionJSON), pollMs, string(vPathJSON)))
	}

	return buf.Bytes(), nil
}

// InjectInlineScript injects a <script id="__ENV__"> tag immediately before the closing </head> tag.
// If </head> is not present, it appends the script at the start of <body> or the end of the content.
func InjectInlineScript(html []byte, env map[string]string, version string, pollIntervalSeconds int32, versionPath string) ([]byte, error) {
	res, _, err := InjectInlineScriptWithHash(html, env, version, pollIntervalSeconds, versionPath)
	return res, err
}

// InjectInlineScriptWithHash injects the inline config script and returns the modified HTML
// along with the CSP sha256 source expression (e.g. "'sha256-...'") for the script body.
func InjectInlineScriptWithHash(html []byte, env map[string]string, version string, pollIntervalSeconds int32, versionPath string) ([]byte, string, error) {
	js, err := GenerateConfigJS(env, version, pollIntervalSeconds, versionPath)
	if err != nil {
		return nil, "", err
	}

	// Browser CSP hashes the exact bytes between <script ...> and </script>, including the leading newline.
	scriptBody := "\n" + string(js)
	sum := sha256.Sum256([]byte(scriptBody))
	cspHash := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"

	scriptTag := fmt.Sprintf("<script id=\"__ENV__\">%s</script>\n", scriptBody)

	// Insert before closing </head> tag if present
	lower := bytes.ToLower(html)
	if idx := bytes.Index(lower, []byte("</head>")); idx != -1 {
		res := make([]byte, 0, len(html)+len(scriptTag))
		res = append(res, html[:idx]...)
		res = append(res, scriptTag...)
		res = append(res, html[idx:]...)
		return res, cspHash, nil
	}

	// Fallback: prepend script tag
	res := make([]byte, 0, len(html)+len(scriptTag))
	res = append(res, scriptTag...)
	res = append(res, html...)
	return res, cspHash, nil
}

// CleanPath normalizes the configured config endpoint path.
func CleanPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/_config.js"
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}

// CleanVersionPath normalizes the configured version endpoint path, defaulting to "/_version".
func CleanVersionPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/_version"
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}
