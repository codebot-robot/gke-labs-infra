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

package k8s

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestMapScopeResolver(t *testing.T) {
	resolver := MapScopeResolver{
		GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}:                               true,
		GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}:                                    true,
		GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole"}:         false,
		GroupVersionKind{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"}: false,
	}

	tests := []struct {
		gvk  GroupVersionKind
		want bool
	}{
		{GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, true},
		{GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}, true},
		{GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole"}, false},
		{GroupVersionKind{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"}, false},
		// Partial GVK (version omitted) matches fallback
		{GroupVersionKind{Group: "apps", Kind: "Deployment"}, true},
	}

	for _, tt := range tests {
		got, err := resolver.IsNamespaced(tt.gvk)
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", tt.gvk, err)
		}
		if got != tt.want {
			t.Errorf("IsNamespaced(%s) = %v, want %v", tt.gvk, got, tt.want)
		}
	}

	// Unknown GVK
	_, err := resolver.IsNamespaced(GroupVersionKind{Group: "custom.io", Version: "v1", Kind: "Unknown"})
	if err == nil {
		t.Errorf("expected error for unknown GVK, got nil")
	}
}

func TestDiscoveryScopeResolverWithMapper(t *testing.T) {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Group: "apps", Version: "v1"},
		{Group: "rbac.authorization.k8s.io", Version: "v1"},
	})
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole"}, meta.RESTScopeRoot)

	resolver := NewScopeResolverFromMapper(mapper)

	ns, err := resolver.IsNamespaced(GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ns {
		t.Errorf("expected Deployment to be namespaced")
	}

	clusterScoped, err := resolver.IsNamespaced(GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clusterScoped {
		t.Errorf("expected ClusterRole to be cluster-scoped")
	}

	// Unknown
	_, err = resolver.IsNamespaced(GroupVersionKind{Group: "unknown", Version: "v1", Kind: "Unknown"})
	if err == nil {
		t.Errorf("expected error for unknown type")
	}
}

func TestValidateNamespaceName(t *testing.T) {
	valid := []string{
		"default",
		"kube-system",
		"my-namespace-1",
		"a",
		"0",
		"corp-gke",
	}
	for _, ns := range valid {
		if err := ValidateNamespaceName(ns); err != nil {
			t.Errorf("ValidateNamespaceName(%q) unexpected error: %v", ns, err)
		}
	}

	invalid := []string{
		"Bad_Name",
		"-lead",
		"trail-",
		"a..b",
		"Uppercase",
		"space in name",
		"this-namespace-name-is-entirely-too-long-because-it-exceeds-sixty-three-characters",
	}
	for _, ns := range invalid {
		if err := ValidateNamespaceName(ns); err == nil {
			t.Errorf("ValidateNamespaceName(%q) expected error, got nil", ns)
		}
	}
}
