// Package injection handles dynamic environment variable injection for client SPAs.
package injection

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// GenerateConfigJS serializes an environment map into a JavaScript snippet
// that sets window.__ENV__.
func GenerateConfigJS(env map[string]string) ([]byte, error) {
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

	return buf.Bytes(), nil
}

// InjectInlineScript injects a <script id="__ENV__"> tag immediately before the closing </head> tag.
// If </head> is not present, it appends the script at the start of <body> or the end of the content.
func InjectInlineScript(html []byte, env map[string]string) ([]byte, error) {
	js, err := GenerateConfigJS(env)
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
