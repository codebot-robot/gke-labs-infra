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
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/restmapper"
)

// GroupVersionKind represents the group, version, and kind of a Kubernetes object.
type GroupVersionKind = schema.GroupVersionKind

// GroupKind represents the group and kind of a Kubernetes object.
type GroupKind = schema.GroupKind

// ScopeResolver determines whether a Kubernetes GroupVersionKind is namespaced or cluster-scoped.
type ScopeResolver interface {
	IsNamespaced(gvk GroupVersionKind) (bool, error)
}

// DiscoveryScopeResolver implements ScopeResolver using client-go RESTMapper.
type DiscoveryScopeResolver struct {
	mapper meta.RESTMapper
}

// NewDiscoveryScopeResolver creates a ScopeResolver using discovery from client-go.
func NewDiscoveryScopeResolver(discoveryClient discovery.DiscoveryInterface) (*DiscoveryScopeResolver, error) {
	groupResources, err := restmapper.GetAPIGroupResources(discoveryClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get API group resources: %w", err)
	}
	mapper := restmapper.NewDiscoveryRESTMapper(groupResources)
	return &DiscoveryScopeResolver{mapper: mapper}, nil
}

// NewScopeResolverFromMapper creates a ScopeResolver from an existing RESTMapper.
func NewScopeResolverFromMapper(mapper meta.RESTMapper) *DiscoveryScopeResolver {
	return &DiscoveryScopeResolver{mapper: mapper}
}

// IsNamespaced implements ScopeResolver.
func (r *DiscoveryScopeResolver) IsNamespaced(gvk GroupVersionKind) (bool, error) {
	var mapping *meta.RESTMapping
	var err error

	if gvk.Version != "" {
		mapping, err = r.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	} else {
		mapping, err = r.mapper.RESTMapping(gvk.GroupKind())
	}

	if err != nil {
		if resetter, ok := r.mapper.(interface{ Reset() }); ok {
			resetter.Reset()
			if gvk.Version != "" {
				mapping, err = r.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
			} else {
				mapping, err = r.mapper.RESTMapping(gvk.GroupKind())
			}
		}
	}
	if err != nil {
		return false, fmt.Errorf("failed to resolve REST mapping for %s: %w", gvk, err)
	}
	return mapping.Scope.Name() == meta.RESTScopeNameNamespace, nil
}

// MapScopeResolver implements ScopeResolver using a simple map for testing or static resolution.
type MapScopeResolver map[GroupVersionKind]bool

// IsNamespaced implements ScopeResolver.
func (m MapScopeResolver) IsNamespaced(gvk GroupVersionKind) (bool, error) {
	if ns, ok := m[gvk]; ok {
		return ns, nil
	}
	// Fallback to match by group and kind regardless of version
	for k, ns := range m {
		if (gvk.Group == "" || k.Group == gvk.Group) && k.Kind == gvk.Kind {
			return ns, nil
		}
	}
	return false, fmt.Errorf("unknown GVK in MapScopeResolver: %s", gvk)
}

// ValidateNamespaceName validates that a namespace name is valid according to Kubernetes DNS-1123 label rules:
// - At most 63 characters
// - Lowercase alphanumeric characters ('a'-'z', '0'-'9') or '-'
// - Must start and end with an alphanumeric character ('a'-'z', '0'-'9')
func ValidateNamespaceName(ns string) error {
	if ns == "" {
		return nil
	}
	if len(ns) > 63 {
		return fmt.Errorf("namespace %q is invalid: must be no more than 63 characters", ns)
	}
	for i := 0; i < len(ns); i++ {
		c := ns[i]
		isAlphaNum := (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
		if i == 0 || i == len(ns)-1 {
			if !isAlphaNum {
				return fmt.Errorf("namespace %q is invalid: must start and end with an alphanumeric character", ns)
			}
		} else {
			if !isAlphaNum && c != '-' {
				return fmt.Errorf("namespace %q is invalid: must consist of lower case alphanumeric characters or '-'", ns)
			}
		}
	}
	return nil
}
