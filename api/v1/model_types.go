/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	OLlamaEngine = "OLlama"
	VLLMEngine   = "VLLM"
)

// ModelSpec defines the desired state of Model.
type ModelSpec struct {
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url"`

	// +kubebuilder:validation:Enum=OLlama;VLLM
	Engine string `json:"engine"`

	// +optional
	Cache ModelCache `json:"cache,omitempty"`

	// +optional
	Image string `json:"image,omitempty"`

	// +optional
	Args []string `json:"args,omitempty"`

	// +optional
	Env map[string]string `json:"env,omitempty"`

	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// ModelCache configures persistent volume caching for model weights.
type ModelCache struct {
	Enabled bool `json:"enabled"`

	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`

	// +optional
	// +kubebuilder:default="50Gi"
	StorageSize string `json:"storageSize,omitempty"`

	// +optional
	ExistingPVCName string `json:"existingPVCName,omitempty"`
}

// ModelStatus defines the observed state of Model.
type ModelStatus struct {
	Ready      bool `json:"ready"`
	CacheReady bool `json:"cacheReady"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Engine",type=string,JSONPath=`.spec.engine`
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.spec.url`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Cached",type=boolean,JSONPath=`.status.cacheReady`

// Model is the Schema for the models API.
type Model struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ModelSpec   `json:"spec"`
	Status ModelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ModelList contains a list of Model.
type ModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Model `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Model{}, &ModelList{})
}
