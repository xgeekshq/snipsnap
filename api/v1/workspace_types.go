package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type WorkspacePhase string

const (
	WorkspacePhaseIdle      WorkspacePhase = "Idle"
	WorkspacePhaseSwitching WorkspacePhase = "Switching"
	WorkspacePhaseLoading   WorkspacePhase = "Loading"
	WorkspacePhaseReady     WorkspacePhase = "Ready"
)

// WorkspaceSpec defines the desired state of Workspace.
type WorkspaceSpec struct {
	// +optional
	ActiveModel string `json:"activeModel,omitempty"`
}

// WorkspaceStatus defines the observed state of Workspace.
type WorkspaceStatus struct {
	// +optional
	LoadedModel string `json:"loadedModel,omitempty"`

	// +optional
	// +kubebuilder:validation:Enum=Idle;Switching;Loading;Ready
	Phase WorkspacePhase `json:"phase,omitempty"`

	// +optional
	InferenceAddress string `json:"inferenceAddress,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Active Model",type=string,JSONPath=`.spec.activeModel`
// +kubebuilder:printcolumn:name="Loaded Model",type=string,JSONPath=`.status.loadedModel`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// Workspace is the Schema for the workspaces API.
type Workspace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkspaceSpec   `json:"spec,omitempty"`
	Status WorkspaceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WorkspaceList contains a list of Workspace.
type WorkspaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Workspace `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Workspace{}, &WorkspaceList{})
}
