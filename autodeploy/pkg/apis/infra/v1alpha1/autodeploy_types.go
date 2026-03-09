// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1alpha1

//go:generate go run sigs.k8s.io/controller-tools/cmd/controller-gen object:headerFile="../../../../../.ap/headers.yaml" paths="./..."

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// AutoDeploySpec defines the desired state of AutoDeploy
type AutoDeploySpec struct {
	// Repo is the URL of the git repository to monitor.
	Repo string `json:"repo"`

	// Branch is the branch to monitor. If empty, it defaults to main.
	// +optional
	Branch string `json:"branch,omitempty"`

	// Tag is a pattern for tags to monitor.
	// +optional
	Tag string `json:"tag,omitempty"`

	// Directory is the directory within the repository to deploy. If empty, it defaults to the root.
	// +optional
	Directory string `json:"directory,omitempty"`

	// Interval is the polling interval. If empty, it defaults to 1m.
	// +optional
	Interval string `json:"interval,omitempty"`
}

// AutoDeployStatus defines the observed state of AutoDeploy
type AutoDeployStatus struct {
	// LastDeployedCommit is the hash of the last successfully deployed commit.
	LastDeployedCommit string `json:"lastDeployedCommit,omitempty"`

	// Conditions represent the latest available observations of an object's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// AutoDeploy is the Schema for the autodeploys API
type AutoDeploy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AutoDeploySpec   `json:"spec,omitempty"`
	Status AutoDeployStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AutoDeployList contains a list of AutoDeploy
type AutoDeployList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AutoDeploy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AutoDeploy{}, &AutoDeployList{})
}

var (
	// GroupVersion is group version used to register these objects
	GroupVersion = schema.GroupVersion{Group: "infra.labs.gke.io", Version: "v1alpha1"}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)
