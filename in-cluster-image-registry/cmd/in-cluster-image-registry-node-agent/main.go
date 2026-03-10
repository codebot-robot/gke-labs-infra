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
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

const (
	configPath   = "/etc/containerd/config.toml"
	certsDPath   = "/etc/containerd/certs.d"
	registryHost = "images.local"
	namespace    = "in-cluster-image-registry-system"
	serviceName  = "in-cluster-image-registry"
	beginMarker  = "# BEGIN IN-CLUSTER-IMAGE-REGISTRY CONFIGURATION"
	endMarker    = "# END IN-CLUSTER-IMAGE-REGISTRY CONFIGURATION"
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

	ctx := context.Background()
	for {
		err := reconcile(ctx, clientset)
		if err != nil {
			klog.Errorf("Reconcile failed: %v", err)
		}
		time.Sleep(30 * time.Second)
	}
}

func reconcile(ctx context.Context, clientset *kubernetes.Clientset) error {
	svc, err := clientset.CoreV1().Services(namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get service %s/%s: %v", namespace, serviceName, err)
	}

	clusterIP := svc.Spec.ClusterIP
	if clusterIP == "" || clusterIP == "None" {
		return fmt.Errorf("service %s/%s has no ClusterIP", namespace, serviceName)
	}

	klog.Infof("Found service %s ClusterIP: %s", serviceName, clusterIP)

	hostsDir := fmt.Sprintf("%s/%s", certsDPath, registryHost)
	hostsPath := fmt.Sprintf("%s/hosts.toml", hostsDir)
	if err := updateHostsConfig(hostsDir, hostsPath, clusterIP); err != nil {
		return fmt.Errorf("failed to update hosts config: %v", err)
	}

	return cleanupOldConfig(configPath)
}

func updateHostsConfig(dir, path, ip string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %v", dir, err)
	}

	desiredContent := fmt.Sprintf(`server = "http://%s"

[host."http://%s"]
  capabilities = ["pull", "resolve"]
  skip_verify = true
`, registryHost, ip)

	currentContent, err := os.ReadFile(path)
	if err == nil && string(currentContent) == desiredContent {
		return nil
	}

	klog.Infof("Updating %s", path)
	return os.WriteFile(path, []byte(desiredContent), 0644)
}

func cleanupOldConfig(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read %s: %v", path, err)
	}

	strContent := string(content)
	startIndex := strings.Index(strContent, beginMarker)
	endIndex := strings.Index(strContent, endMarker)

	if startIndex != -1 && endIndex != -1 && startIndex < endIndex {
		klog.Infof("Removing old configuration from %s", path)
		newStr := strContent[:startIndex] + strContent[endIndex+len(endMarker):]
		// Remove trailing newlines if we left any
		newStr = strings.TrimRight(newStr, "\n") + "\n"
		
		err = os.WriteFile(path, []byte(newStr), 0644)
		if err != nil {
			return fmt.Errorf("failed to write %s: %v", path, err)
		}
	}

	return nil
}
