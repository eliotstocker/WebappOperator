package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RevisionPhase defines the lifecycle phase of a WebsiteRevision
type RevisionPhase string

const (
	RevisionPhasePending    RevisionPhase = "Pending"
	RevisionPhasePrewarming RevisionPhase = "Prewarming"
	RevisionPhaseReady      RevisionPhase = "Ready"
	RevisionPhaseActive     RevisionPhase = "Active"
	RevisionPhaseRetired    RevisionPhase = "Retired"
	RevisionPhaseFailed     RevisionPhase = "Failed"
)

// WebsiteRevisionSpec defines the immutable specification of a website release
type WebsiteRevisionSpec struct {
	// WebsiteName is the name of the parent Website CRD.
	WebsiteName string `json:"websiteName"`

	// Image is the OCI artifact reference pulled for this revision.
	Image string `json:"image"`

	// Digest is the resolved OCI content digest (e.g. sha256:abc...).
	// +optional
	Digest string `json:"digest,omitempty"`

	// Env is the static snapshot of environment variables for this revision.
	// +optional
	Env map[string]string `json:"env,omitempty"`

	// Injection settings for runtime environment variables.
	// +optional
	Injection InjectionSpec `json:"injection,omitempty"`

	// ImagePullSecrets references secrets used to pull this OCI artifact.
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
}

// WebsiteRevisionStatus defines the observed state of WebsiteRevision
type WebsiteRevisionStatus struct {
	// Phase is the current lifecycle phase (Pending -> Prewarming -> Ready -> Active -> Retired).
	// +optional
	Phase RevisionPhase `json:"phase,omitempty"`

	// ReadyPods lists gateway pod names that have successfully pulled and unpacked this revision locally.
	// +optional
	ReadyPods []string `json:"readyPods,omitempty"`

	// UnpackedSize is the total unpacked byte size of the static assets on disk.
	// +optional
	UnpackedSize int64 `json:"unpackedSize,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest observations of the revision state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=webrev;revs
// +kubebuilder:printcolumn:name="Website",type=string,JSONPath=`.spec.websiteName`
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready Pods",type=string,JSONPath=`.status.readyPods`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// WebsiteRevision is the Schema for the websiterevisions API
type WebsiteRevision struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WebsiteRevisionSpec   `json:"spec,omitempty"`
	Status WebsiteRevisionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WebsiteRevisionList contains a list of WebsiteRevision
type WebsiteRevisionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WebsiteRevision `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WebsiteRevision{}, &WebsiteRevisionList{})
}
