// Package injection handles dynamic environment variable injection for client SPAs.
package injection

import (
	"bytes"
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
	js, err := GenerateConfigJS(env, version, pollIntervalSeconds, versionPath)
	if err != nil {
		return nil, err
	}

	scriptTag := fmt.Sprintf("<script id=\"__ENV__\">\n%s</script>\n", string(js))

	// Insert before closing </head> tag if present
	lower := bytes.ToLower(html)
	if idx := bytes.Index(lower, []byte("</head>")); idx != -1 {
		res := make([]byte, 0, len(html)+len(scriptTag))
		res = append(res, html[:idx]...)
		res = append(res, scriptTag...)
		res = append(res, html[idx:]...)
		return res, nil
	}

	// Fallback: prepend script tag
	res := make([]byte, 0, len(html)+len(scriptTag))
	res = append(res, scriptTag...)
	res = append(res, html...)
	return res, nil
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
