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

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/pelletier/go-toml/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

const (
	configPath   = "/etc/containerd/config.toml"
	registryHost = "images.local"
	namespace    = "in-cluster-image-registry-system"
	serviceName  = "images"
)

func main() {
	config, err := rest.InClusterConfig()
	if err != nil {
		klog.Fatalf("Error getting in-cluster config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Error creating clientset: %v", err)
	}

	for {
		err := reconcile(clientset)
		if err != nil {
			klog.Errorf("Reconcile failed: %v", err)
		}
		time.Sleep(30 * time.Second)
	}
}

func reconcile(clientset *kubernetes.Clientset) error {
	svc, err := clientset.CoreV1().Services(namespace).Get(context.TODO(), serviceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get service %s/%s: %v", namespace, serviceName, err)
	}

	clusterIP := svc.Spec.ClusterIP
	if clusterIP == "" || clusterIP == "None" {
		return fmt.Errorf("service %s/%s has no ClusterIP", namespace, serviceName)
	}

	klog.Infof("Found service %s ClusterIP: %s", serviceName, clusterIP)

	return updateConfig(clusterIP)
}

func updateConfig(ip string) error {
	content, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// If it doesn't exist, we might be in a weird environment, or we should create it.
			// But usually it exists on K8s nodes.
			return fmt.Errorf("config file %s does not exist", configPath)
		}
		return fmt.Errorf("failed to read %s: %v", configPath, err)
	}

	newContent, changed, err := updateTOML(content, ip)
	if err != nil {
		return err
	}

	if changed {
		klog.Infof("Updating %s", configPath)
		err = os.WriteFile(configPath, newContent, 0644)
		if err != nil {
			return fmt.Errorf("failed to write %s: %v", configPath, err)
		}

		klog.Infof("Successfully updated %s. Note: containerd may need to be restarted to pick up changes.", configPath)
	}

	return nil
}

func updateTOML(content []byte, ip string) ([]byte, bool, error) {
	var cfg map[string]interface{}
	err := toml.Unmarshal(content, &cfg)
	if err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal TOML: %v", err)
	}

	changed := false

	// Ensure plugins."io.containerd.grpc.v1.cri".registry.mirrors."images.local"
	// and plugins."io.containerd.grpc.v1.cri".registry.configs."images.local".tls

	plugins, ok := cfg["plugins"].(map[string]interface{})
	if !ok {
		plugins = make(map[string]interface{})
		cfg["plugins"] = plugins
		changed = true
	}

	cri, ok := plugins["io.containerd.grpc.v1.cri"].(map[string]interface{})
	if !ok {
		cri = make(map[string]interface{})
		plugins["io.containerd.grpc.v1.cri"] = cri
		changed = true
	}

	registry, ok := cri["registry"].(map[string]interface{})
	if !ok {
		registry = make(map[string]interface{})
		cri["registry"] = registry
		changed = true
	}

	mirrors, ok := registry["mirrors"].(map[string]interface{})
	if !ok {
		mirrors = make(map[string]interface{})
		registry["mirrors"] = mirrors
		changed = true
	}

	mirror, ok := mirrors[registryHost].(map[string]interface{})
	if !ok {
		mirror = make(map[string]interface{})
		mirrors[registryHost] = mirror
		changed = true
	}

	desiredEndpoint := fmt.Sprintf("http://%s", ip)
	currentEndpointsRaw, ok := mirror["endpoint"].([]interface{})
	currentEndpoints := []string{}
	if ok {
		for _, e := range currentEndpointsRaw {
			if s, ok := e.(string); ok {
				currentEndpoints = append(currentEndpoints, s)
			}
		}
	} else if existing, ok := mirror["endpoint"].([]string); ok {
		currentEndpoints = existing
	}

	if len(currentEndpoints) != 1 || currentEndpoints[0] != desiredEndpoint {
		mirror["endpoint"] = []string{desiredEndpoint}
		changed = true
	}

	configs, ok := registry["configs"].(map[string]interface{})
	if !ok {
		configs = make(map[string]interface{})
		registry["configs"] = configs
		changed = true
	}

	registryConfig, ok := configs[registryHost].(map[string]interface{})
	if !ok {
		registryConfig = make(map[string]interface{})
		configs[registryHost] = registryConfig
		changed = true
	}

	tls, ok := registryConfig["tls"].(map[string]interface{})
	if !ok {
		tls = make(map[string]interface{})
		registryConfig["tls"] = tls
		changed = true
	}

	if tls["insecure_skip_verify"] != true {
		tls["insecure_skip_verify"] = true
		changed = true
	}

	if changed {
		newContent, err := toml.Marshal(cfg)
		if err != nil {
			return nil, false, fmt.Errorf("failed to marshal TOML: %v", err)
		}
		return newContent, true, nil
	}

	return content, false, nil
}
