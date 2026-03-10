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

	return updateConfig(ctx, clusterIP)
}

func updateConfig(ctx context.Context, ip string) error {
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
	desiredBlock := fmt.Sprintf(`%s
[plugins."io.containerd.grpc.v1.cri".registry.mirrors."%s"]
  endpoint = ["http://%s"]

[plugins."io.containerd.grpc.v1.cri".registry.configs."%s".tls]
  insecure_skip_verify = true
%s`, beginMarker, registryHost, ip, registryHost, endMarker)

	strContent := string(content)
	startIndex := strings.Index(strContent, beginMarker)
	endIndex := strings.Index(strContent, endMarker)

	if startIndex != -1 && endIndex != -1 && startIndex < endIndex {
		// Existing block found, check if it matches
		currentBlock := strContent[startIndex : endIndex+len(endMarker)]
		if currentBlock == desiredBlock {
			return content, false, nil
		}
		// Replace it
		newStr := strContent[:startIndex] + desiredBlock + strContent[endIndex+len(endMarker):]
		return []byte(newStr), true, nil
	}

	// Not found, append it
	newStr := strContent
	if len(newStr) > 0 && newStr[len(newStr)-1] != '\n' {
		newStr += "\n"
	}
	newStr += "\n" + desiredBlock + "\n"
	return []byte(newStr), true, nil
}
