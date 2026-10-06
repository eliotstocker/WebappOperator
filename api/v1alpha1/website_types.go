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
