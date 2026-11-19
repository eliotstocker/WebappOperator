package server

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	webappv1alpha1 "github.com/eliotstocker/webapp-operator/api/v1alpha1"
)

type headerPair struct {
	key   string
	value string
}

// CompiledSecurity holds precomputed HTTP security headers for a Website route
// to avoid per-request allocations during static file serving.
type CompiledSecurity struct {
	staticHeaders []headerPair
	cspHeaderName string
	cspValue      string
}

// CompileSecurity resolves presets, structured directives, and overrides into
// a precomputed header set. Returns nil if spec is nil.
func CompileSecurity(spec *webappv1alpha1.SecuritySpec) *CompiledSecurity {
	if spec == nil {
		return nil
	}

	var (
		contentTypeOptions bool
		frameOptions       string
		referrerPolicy     string
		coop               string
		corp               string
		coep               string
		xssProtection      bool

		hstsEnabled           bool
		hstsMaxAge            int32 = 31536000
		hstsIncludeSubDomains bool
		hstsPreload           bool

		perms = make(map[string][]string)

		cspEnabled    bool
		cspReportOnly bool
		cspRaw        string
		cspDirs       *webappv1alpha1.CSPDirectives
	)

	switch spec.Preset {
	case webappv1alpha1.SecurityPresetStandard:
		contentTypeOptions = true
		frameOptions = "DENY"
		referrerPolicy = "strict-origin-when-cross-origin"
		coop = "same-origin-allow-popups"
		xssProtection = true
		perms["camera"] = []string{}
		perms["microphone"] = []string{}
		perms["geolocation"] = []string{}
		perms["payment"] = []string{}
		perms["usb"] = []string{}
	case webappv1alpha1.SecurityPresetStrict:
		contentTypeOptions = true
		frameOptions = "DENY"
		referrerPolicy = "strict-origin-when-cross-origin"
		coop = "same-origin"
		corp = "same-origin"
		coep = "credentialless"
		xssProtection = true
		hstsEnabled = true
		perms["camera"] = []string{}
		perms["microphone"] = []string{}
		perms["geolocation"] = []string{}
		perms["payment"] = []string{}
		perms["usb"] = []string{}
		perms["fullscreen"] = []string{}
		perms["display-capture"] = []string{}
		cspEnabled = true
		cspDirs = defaultStrictCSPDirectives()
	}

	// Apply explicit single-value overrides
	if spec.ContentTypeOptions != nil {
		contentTypeOptions = *spec.ContentTypeOptions
	}
	if spec.FrameOptions != "" {
		frameOptions = resolveDisabled(spec.FrameOptions)
	}
	if spec.ReferrerPolicy != "" {
		referrerPolicy = resolveDisabled(spec.ReferrerPolicy)
	}
	if spec.COOP != "" {
		coop = resolveDisabled(spec.COOP)
	}
	if spec.CORP != "" {
		corp = resolveDisabled(spec.CORP)
	}
	if spec.COEP != "" {
		coep = resolveDisabled(spec.COEP)
	}
	if spec.XSSProtection != nil {
		xssProtection = *spec.XSSProtection
	}

	// Apply HSTS overrides
	if spec.HSTS != nil {
		if spec.HSTS.Enabled != nil {
			hstsEnabled = *spec.HSTS.Enabled
		} else {
			hstsEnabled = true
		}
		if spec.HSTS.MaxAge != nil {
			hstsMaxAge = *spec.HSTS.MaxAge
		}
		hstsIncludeSubDomains = spec.HSTS.IncludeSubDomains
		hstsPreload = spec.HSTS.Preload
	}

	// Apply Permissions-Policy overrides
	if spec.PermissionsPolicy != nil {
		mergePermissionsPolicy(perms, spec.PermissionsPolicy)
	}

	// Apply CSP overrides
	if spec.CSP != nil {
		cspReportOnly = spec.CSP.ReportOnly
		if spec.CSP.Enabled != nil {
			cspEnabled = *spec.CSP.Enabled
		} else if spec.CSP.Raw != "" || spec.CSP.Directives != nil {
			cspEnabled = true
		}
		if spec.CSP.Raw != "" {
			cspRaw = spec.CSP.Raw
		} else if spec.CSP.Directives != nil {
			cspDirs = spec.CSP.Directives
		}
	}

	headers := make(map[string]string)

	if contentTypeOptions {
		headers["X-Content-Type-Options"] = "nosniff"
	}
	if frameOptions != "" {
		headers["X-Frame-Options"] = frameOptions
	}
	if referrerPolicy != "" {
		headers["Referrer-Policy"] = referrerPolicy
	}
	if coop != "" {
		headers["Cross-Origin-Opener-Policy"] = coop
	}
	if corp != "" {
		headers["Cross-Origin-Resource-Policy"] = corp
	}
	if coep != "" {
		headers["Cross-Origin-Embedder-Policy"] = coep
	}
	if xssProtection {
		headers["X-XSS-Protection"] = "0"
	}
	if hstsEnabled {
		headers["Strict-Transport-Security"] = formatHSTS(hstsMaxAge, hstsIncludeSubDomains, hstsPreload)
	}
	if len(perms) > 0 {
		headers["Permissions-Policy"] = formatPermissionsPolicy(perms)
	}

	var cspHeaderName, cspValue string
	if cspEnabled {
		if cspReportOnly {
			cspHeaderName = "Content-Security-Policy-Report-Only"
		} else {
			cspHeaderName = "Content-Security-Policy"
		}
		if cspRaw != "" {
			cspValue = cspRaw
		} else if cspDirs != nil {
			cspValue = formatCSPDirectives(cspDirs)
		}
	}

	// Custom headers act as an escape hatch and can override computed headers.
	for k, v := range spec.CustomHeaders {
		canon := http.CanonicalHeaderKey(k)
		if canon == "Content-Security-Policy" || canon == "Content-Security-Policy-Report-Only" {
			cspHeaderName = canon
			cspValue = v
			continue
		}
		headers[canon] = v
	}

	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	static := make([]headerPair, 0, len(keys))
	for _, k := range keys {
		static = append(static, headerPair{key: k, value: headers[k]})
	}

	return &CompiledSecurity{
		staticHeaders: static,
		cspHeaderName: cspHeaderName,
		cspValue:      cspValue,
	}
}

// Apply writes the compiled security headers onto the HTTP response.
// When inlineScriptHash is non-empty (from inline env injection), it is appended to
// the script-src CSP directive so the injected <script id="__ENV__"> remains permitted.
func (cs *CompiledSecurity) Apply(w http.ResponseWriter, inlineScriptHash string) {
	if cs == nil {
		return
	}
	h := w.Header()
	for _, pair := range cs.staticHeaders {
		h.Set(pair.key, pair.value)
	}
	if cs.cspHeaderName != "" && cs.cspValue != "" {
		val := cs.cspValue
		if inlineScriptHash != "" {
			val = appendCSPScriptHash(val, inlineScriptHash)
		}
		h.Set(cs.cspHeaderName, val)
	}
}

func resolveDisabled(val string) string {
	if strings.EqualFold(val, "disabled") {
		return ""
	}
	return val
}

func formatHSTS(maxAge int32, includeSubDomains, preload bool) string {
	parts := []string{fmt.Sprintf("max-age=%d", maxAge)}
	if includeSubDomains {
		parts = append(parts, "includeSubDomains")
	}
	if preload {
		parts = append(parts, "preload")
	}
	return strings.Join(parts, "; ")
}

func mergePermissionsPolicy(perms map[string][]string, spec *webappv1alpha1.PermissionsPolicySpec) {
	if spec.Camera != nil {
		perms["camera"] = spec.Camera
	}
	if spec.Microphone != nil {
		perms["microphone"] = spec.Microphone
	}
	if spec.Geolocation != nil {
		perms["geolocation"] = spec.Geolocation
	}
	if spec.Payment != nil {
		perms["payment"] = spec.Payment
	}
	if spec.USB != nil {
		perms["usb"] = spec.USB
	}
	if spec.Fullscreen != nil {
		perms["fullscreen"] = spec.Fullscreen
	}
	if spec.DisplayCapture != nil {
		perms["display-capture"] = spec.DisplayCapture
	}
	for k, v := range spec.Extra {
		if v == nil {
			perms[k] = []string{}
		} else {
			perms[k] = v
		}
	}
}

func formatPermissionsPolicy(perms map[string][]string) string {
	keys := make([]string, 0, len(perms))
	for k := range perms {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, formatPermissionsFeature(k, perms[k]))
	}
	return strings.Join(parts, ", ")
}

func formatPermissionsFeature(name string, origins []string) string {
	if len(origins) == 0 || (len(origins) == 1 && origins[0] == "none") {
		return name + "=()"
	}
	if len(origins) == 1 && origins[0] == "*" {
		return name + "=*"
	}
	formatted := make([]string, 0, len(origins))
	for _, o := range origins {
		if o == "self" || o == "*" || o == "src" {
			formatted = append(formatted, o)
		} else {
			formatted = append(formatted, `"`+o+`"`)
		}
	}
	return fmt.Sprintf("%s=(%s)", name, strings.Join(formatted, " "))
}

func defaultStrictCSPDirectives() *webappv1alpha1.CSPDirectives {
	return &webappv1alpha1.CSPDirectives{
		DefaultSrc:     []string{"'self'"},
		ScriptSrc:      []string{"'self'"},
		StyleSrc:       []string{"'self'", "'unsafe-inline'"},
		ImgSrc:         []string{"'self'", "data:", "https:"},
		ObjectSrc:      []string{"'none'"},
		BaseURI:        []string{"'self'"},
		FrameAncestors: []string{"'none'"},
	}
}

func formatCSPDirectives(d *webappv1alpha1.CSPDirectives) string {
	type dirEntry struct {
		name string
		vals []string
	}
	ordered := []dirEntry{
		{"default-src", d.DefaultSrc},
		{"script-src", d.ScriptSrc},
		{"style-src", d.StyleSrc},
		{"img-src", d.ImgSrc},
		{"connect-src", d.ConnectSrc},
		{"font-src", d.FontSrc},
		{"object-src", d.ObjectSrc},
		{"media-src", d.MediaSrc},
		{"frame-src", d.FrameSrc},
		{"frame-ancestors", d.FrameAncestors},
		{"base-uri", d.BaseURI},
		{"form-action", d.FormAction},
	}

	var parts []string
	for _, e := range ordered {
		if len(e.vals) > 0 {
			parts = append(parts, e.name+" "+strings.Join(e.vals, " "))
		}
	}

	if len(d.Extra) > 0 {
		extraKeys := make([]string, 0, len(d.Extra))
		for k := range d.Extra {
			extraKeys = append(extraKeys, k)
		}
		sort.Strings(extraKeys)
		for _, k := range extraKeys {
			if len(d.Extra[k]) > 0 {
				parts = append(parts, k+" "+strings.Join(d.Extra[k], " "))
			}
		}
	}

	if d.UpgradeInsecureRequests {
		parts = append(parts, "upgrade-insecure-requests")
	}
	if d.ReportURI != "" {
		parts = append(parts, "report-uri "+d.ReportURI)
	}
	return strings.Join(parts, "; ")
}

func appendCSPScriptHash(csp, hash string) string {
	directives := strings.Split(csp, ";")
	for i, rawDir := range directives {
		dir := strings.TrimSpace(rawDir)
		if strings.HasPrefix(dir, "script-src ") || dir == "script-src" {
			if strings.Contains(dir, "'unsafe-inline'") || strings.Contains(dir, hash) {
				return csp
			}
			directives[i] = dir + " " + hash
			for j := range directives {
				if j != i {
					directives[j] = strings.TrimSpace(directives[j])
				}
			}
			return strings.Join(directives, "; ")
		}
	}
	return csp
}
