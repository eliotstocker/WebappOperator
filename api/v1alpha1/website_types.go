package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InjectionMode defines how environment variables are injected into client apps
// +kubebuilder:validation:Enum=endpoint;inline;both
type InjectionMode string

const (
	InjectionModeEndpoint InjectionMode = "endpoint"
	InjectionModeInline   InjectionMode = "inline"
	InjectionModeBoth     InjectionMode = "both"
)

// InjectionSpec defines configuration for runtime environment variable injection
type InjectionSpec struct {
	// Mode determines if env is served via dynamic JS endpoint, injected inline into index.html, or both.
	// Default is "endpoint".
	// +kubebuilder:default=endpoint
	// +optional
	Mode InjectionMode `json:"mode,omitempty"`

	// Path is the URL path for the dynamic config endpoint. Default is "/_config.js".
	// +kubebuilder:default="/_config.js"
	// +optional
	Path string `json:"path,omitempty"`

	// VersionPath is the URL path for the active version endpoint. Default is "/_version".
	// +kubebuilder:default="/_version"
	// +optional
	VersionPath string `json:"versionPath,omitempty"`

	// VersionPolling controls whether client applications automatically poll for new deployed versions.
	// Defaults to true.
	// +kubebuilder:default=true
	// +optional
	VersionPolling *bool `json:"versionPolling,omitempty"`

	// PollIntervalSeconds configures how often (in seconds) the client app checks for new deployed versions.
	// Defaults to 30.
	// +kubebuilder:default=30
	// +optional
	PollIntervalSeconds *int32 `json:"pollIntervalSeconds,omitempty"`
}

// GetVersionPath returns the configured version endpoint path, defaulting to "/_version".
func (s *InjectionSpec) GetVersionPath() string {
	if s.VersionPath != "" {
		return s.VersionPath
	}
	return "/_version"
}

// IsVersionPollingEnabled reports whether version polling is active.
// It defaults to true unless explicitly disabled via VersionPolling: false or PollIntervalSeconds <= 0.
func (s *InjectionSpec) IsVersionPollingEnabled() bool {
	if s.VersionPolling != nil && !*s.VersionPolling {
		return false
	}
	if s.PollIntervalSeconds != nil && *s.PollIntervalSeconds <= 0 {
		return false
	}
	return true
}

// GetPollIntervalSeconds returns the effective polling interval in seconds, or 0 if disabled.
func (s *InjectionSpec) GetPollIntervalSeconds() int32 {
	if !s.IsVersionPollingEnabled() {
		return 0
	}
	if s.PollIntervalSeconds != nil && *s.PollIntervalSeconds > 0 {
		return *s.PollIntervalSeconds
	}
	return 30
}

// IngressSpec defines optional automated Ingress reconciliation
type IngressSpec struct {
	// Enabled determines whether the operator creates and manages a Kubernetes Ingress resource.
	// Default is false.
	Enabled bool `json:"enabled"`

	// ClassName is the IngressClass name to associate with the created Ingress.
	// +optional
	ClassName *string `json:"className,omitempty"`

	// Annotations to attach to the generated Ingress resource.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// TLS configuration for the generated Ingress resource.
	// +optional
	TLS []networkingv1.IngressTLS `json:"tls,omitempty"`
}

// WebsitePhase defines the current state of a Website
type WebsitePhase string

const (
	WebsitePhasePending    WebsitePhase = "Pending"
	WebsitePhasePrewarming WebsitePhase = "Prewarming"
	WebsitePhaseReady      WebsitePhase = "Ready"
	WebsitePhaseDegraded   WebsitePhase = "Degraded"
)

// SecurityPreset defines a baseline set of HTTP security headers for web applications.
// +kubebuilder:validation:Enum=none;standard;strict
type SecurityPreset string

const (
	SecurityPresetNone     SecurityPreset = "none"
	SecurityPresetStandard SecurityPreset = "standard"
	SecurityPresetStrict   SecurityPreset = "strict"
)

// HSTSSpec configures the Strict-Transport-Security header.
type HSTSSpec struct {
	// Enabled controls whether the Strict-Transport-Security header is emitted.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// MaxAge sets the max-age directive in seconds. Defaults to 31536000 (1 year).
	// +kubebuilder:default=31536000
	// +optional
	MaxAge *int32 `json:"maxAge,omitempty"`

	// IncludeSubDomains appends the includeSubDomains directive.
	// +optional
	IncludeSubDomains bool `json:"includeSubDomains,omitempty"`

	// Preload appends the preload directive. Only enable once all subdomains support HTTPS.
	// +optional
	Preload bool `json:"preload,omitempty"`
}

// PermissionsPolicySpec configures browser feature permissions as structured directives.
// Each slice accepts origins ("self", "*", "https://example.com") or an empty slice [] / ["none"] to disable.
type PermissionsPolicySpec struct {
	// Camera controls access to camera devices.
	// +optional
	Camera []string `json:"camera,omitempty"`

	// Microphone controls access to microphone devices.
	// +optional
	Microphone []string `json:"microphone,omitempty"`

	// Geolocation controls access to the Geolocation API.
	// +optional
	Geolocation []string `json:"geolocation,omitempty"`

	// Payment controls access to the Payment Request API.
	// +optional
	Payment []string `json:"payment,omitempty"`

	// USB controls access to the WebUSB API.
	// +optional
	USB []string `json:"usb,omitempty"`

	// Fullscreen controls access to requestFullscreen().
	// +optional
	Fullscreen []string `json:"fullscreen,omitempty"`

	// DisplayCapture controls access to getDisplayMedia().
	// +optional
	DisplayCapture []string `json:"displayCapture,omitempty"`

	// Extra allows configuring additional Permissions-Policy feature directives by name.
	// +optional
	Extra map[string][]string `json:"extra,omitempty"`
}

// CSPDirectives defines structured Content-Security-Policy directives.
type CSPDirectives struct {
	// DefaultSrc sets the default-src fallback directive.
	// +optional
	DefaultSrc []string `json:"defaultSrc,omitempty"`

	// ScriptSrc sets the script-src directive.
	// +optional
	ScriptSrc []string `json:"scriptSrc,omitempty"`

	// StyleSrc sets the style-src directive.
	// +optional
	StyleSrc []string `json:"styleSrc,omitempty"`

	// ImgSrc sets the img-src directive.
	// +optional
	ImgSrc []string `json:"imgSrc,omitempty"`

	// ConnectSrc sets the connect-src directive.
	// +optional
	ConnectSrc []string `json:"connectSrc,omitempty"`

	// FontSrc sets the font-src directive.
	// +optional
	FontSrc []string `json:"fontSrc,omitempty"`

	// ObjectSrc sets the object-src directive.
	// +optional
	ObjectSrc []string `json:"objectSrc,omitempty"`

	// MediaSrc sets the media-src directive.
	// +optional
	MediaSrc []string `json:"mediaSrc,omitempty"`

	// FrameSrc sets the frame-src directive.
	// +optional
	FrameSrc []string `json:"frameSrc,omitempty"`

	// FrameAncestors sets the frame-ancestors directive.
	// +optional
	FrameAncestors []string `json:"frameAncestors,omitempty"`

	// BaseURI sets the base-uri directive.
	// +optional
	BaseURI []string `json:"baseUri,omitempty"`

	// FormAction sets the form-action directive.
	// +optional
	FormAction []string `json:"formAction,omitempty"`

	// UpgradeInsecureRequests appends the upgrade-insecure-requests directive.
	// +optional
	UpgradeInsecureRequests bool `json:"upgradeInsecureRequests,omitempty"`

	// ReportURI sets the report-uri directive for CSP violation reports.
	// +optional
	ReportURI string `json:"reportUri,omitempty"`

	// Extra allows specifying additional CSP directives by kebab-case directive name.
	// +optional
	Extra map[string][]string `json:"extra,omitempty"`
}

// CSPSpec configures Content-Security-Policy emission.
type CSPSpec struct {
	// Enabled controls whether Content-Security-Policy is emitted.
	// Defaults to true when Directives or Raw is set, or under the "strict" preset.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// ReportOnly emits Content-Security-Policy-Report-Only instead of enforcing.
	// +optional
	ReportOnly bool `json:"reportOnly,omitempty"`

	// Raw specifies a literal Content-Security-Policy header value, overriding Directives.
	// +optional
	Raw string `json:"raw,omitempty"`

	// Directives configures structured Content-Security-Policy directives.
	// +optional
	Directives *CSPDirectives `json:"directives,omitempty"`
}

// SecuritySpec defines HTTP security headers applied to responses for this website.
type SecuritySpec struct {
	// Preset selects a baseline header profile ("none", "standard", "strict").
	// +optional
	Preset SecurityPreset `json:"preset,omitempty"`

	// ContentTypeOptions emits "X-Content-Type-Options: nosniff" when true.
	// +optional
	ContentTypeOptions *bool `json:"contentTypeOptions,omitempty"`

	// FrameOptions sets X-Frame-Options ("DENY", "SAMEORIGIN", or "disabled").
	// +kubebuilder:validation:Enum=DENY;SAMEORIGIN;disabled
	// +optional
	FrameOptions string `json:"frameOptions,omitempty"`

	// ReferrerPolicy sets the Referrer-Policy header (or "disabled").
	// +optional
	ReferrerPolicy string `json:"referrerPolicy,omitempty"`

	// COOP sets Cross-Origin-Opener-Policy ("same-origin", "same-origin-allow-popups", "unsafe-none", or "disabled").
	// +kubebuilder:validation:Enum=same-origin;same-origin-allow-popups;unsafe-none;disabled
	// +optional
	COOP string `json:"coop,omitempty"`

	// CORP sets Cross-Origin-Resource-Policy ("same-origin", "same-site", "cross-origin", or "disabled").
	// +kubebuilder:validation:Enum=same-origin;same-site;cross-origin;disabled
	// +optional
	CORP string `json:"corp,omitempty"`

	// COEP sets Cross-Origin-Embedder-Policy ("unsafe-none", "credentialless", "require-corp", or "disabled").
	// +kubebuilder:validation:Enum=unsafe-none;credentialless;require-corp;disabled
	// +optional
	COEP string `json:"coep,omitempty"`

	// XSSProtection emits "X-XSS-Protection: 0" when true to disable legacy buggy XSS auditors.
	// +optional
	XSSProtection *bool `json:"xssProtection,omitempty"`

	// HSTS configures Strict-Transport-Security.
	// +optional
	HSTS *HSTSSpec `json:"hsts,omitempty"`

	// PermissionsPolicy configures the Permissions-Policy header using structured feature keys.
	// +optional
	PermissionsPolicy *PermissionsPolicySpec `json:"permissionsPolicy,omitempty"`

	// CSP configures Content-Security-Policy.
	// +optional
	CSP *CSPSpec `json:"csp,omitempty"`

	// CustomHeaders sets or overrides arbitrary response headers.
	// +optional
	CustomHeaders map[string]string `json:"customHeaders,omitempty"`
}

// WebsiteSpec defines the desired state of Website
type WebsiteSpec struct {
	// Image specifies the OCI artifact containing static website assets.
	Image string `json:"image"`

	// Hostnames routed to this website.
	Hostnames []string `json:"hostnames"`

	// Env is a key-value map injected into the frontend at runtime.
	// +optional
	Env map[string]string `json:"env,omitempty"`

	// Injection settings for runtime environment variables.
	// +optional
	Injection InjectionSpec `json:"injection,omitempty"`

	// ImagePullSecrets references secrets used to pull private OCI artifacts.
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// RevisionHistoryLimit is the number of old WebsiteRevisions to retain. Default is 3.
	// +kubebuilder:default=3
	// +optional
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`

	// Ingress configuration for optional automated Ingress reconciliation.
	// +optional
	Ingress *IngressSpec `json:"ingress,omitempty"`

	// Security configures HTTP security headers for responses served by this website.
	// +optional
	Security *SecuritySpec `json:"security,omitempty"`
}

// WebsiteStatus defines the observed state of Website
type WebsiteStatus struct {
	// ActiveRevision is the name of the WebsiteRevision currently serving traffic.
	// +optional
	ActiveRevision string `json:"activeRevision,omitempty"`

	// ActiveImage is the OCI artifact reference currently serving traffic.
	// +optional
	ActiveImage string `json:"activeImage,omitempty"`

	// Phase is the high-level health state of the website.
	// +optional
	Phase WebsitePhase `json:"phase,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the website state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=web;sites
// +kubebuilder:printcolumn:name="Active Revision",type=string,JSONPath=`.status.activeRevision`
// +kubebuilder:printcolumn:name="Active Image",type=string,JSONPath=`.status.activeImage`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Website is the Schema for the websites API
type Website struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WebsiteSpec   `json:"spec,omitempty"`
	Status WebsiteStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WebsiteList contains a list of Website
type WebsiteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Website `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Website{}, &WebsiteList{})
}
