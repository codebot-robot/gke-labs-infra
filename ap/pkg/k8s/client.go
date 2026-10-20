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
	"context"
	"fmt"
	"sync"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

// NewRESTConfig creates a Kubernetes *rest.Config using the standard clientcmd loading rules
// (honoring KUBECONFIG, ~/.kube/config, current context, and exec credential plugins)
// with fallback to in-cluster config when running inside a Kubernetes pod.
func NewRESTConfig() (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	cfg, err := kubeConfig.ClientConfig()
	if err != nil {
		if IsInCluster() {
			inClusterCfg, inClusterErr := rest.InClusterConfig()
			if inClusterErr != nil {
				return nil, fmt.Errorf("failed to load in-cluster config: %w (kubeconfig error: %v)", inClusterErr, err)
			}
			return inClusterCfg, nil
		}
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}
	return cfg, nil
}

var (
	defaultClientMu sync.Mutex
	cachedDynamic   dynamic.Interface
	cachedMapper    meta.RESTMapper
)

// GetDefaultClientAndMapper returns a cached or newly initialized dynamic client and RESTMapper.
func GetDefaultClientAndMapper() (dynamic.Interface, meta.RESTMapper, error) {
	defaultClientMu.Lock()
	defer defaultClientMu.Unlock()

	if cachedDynamic != nil && cachedMapper != nil {
		return cachedDynamic, cachedMapper, nil
	}

	restConfig, err := NewRESTConfig()
	if err != nil {
		return nil, nil, err
	}

	dynClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	discoClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create discovery client: %w", err)
	}

	cachedDisco := memory.NewMemCacheClient(discoClient)
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(cachedDisco)

	cachedDynamic = dynClient
	cachedMapper = mapper
	return cachedDynamic, cachedMapper, nil
}

// SetDefaultClientAndMapper sets the cached dynamic client and RESTMapper (primarily for testing).
func SetDefaultClientAndMapper(dynClient dynamic.Interface, mapper meta.RESTMapper) {
	defaultClientMu.Lock()
	defer defaultClientMu.Unlock()
	cachedDynamic = dynClient
	cachedMapper = mapper
}

// ResetDefaultClientAndMapper clears the cached dynamic client and RESTMapper.
func ResetDefaultClientAndMapper() {
	defaultClientMu.Lock()
	defer defaultClientMu.Unlock()
	cachedDynamic = nil
	cachedMapper = nil
}

// CheckNamespaceExists verifies that the target namespace exists in the cluster.
func CheckNamespaceExists(ctx context.Context, dynamicClient dynamic.Interface, namespace string) error {
	nsGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	_, err := dynamicClient.Resource(nsGVR).Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("namespace %q does not exist", namespace)
		}
		return fmt.Errorf("failed to check namespace %q: %w", namespace, err)
	}
	return nil
}
