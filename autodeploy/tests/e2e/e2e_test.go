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

//go:build e2e

package e2e

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAutodeploy(t *testing.T) {
	if os.Getenv("RUN_E2E") == "" {
		t.Skip("RUN_E2E not set, skipping e2e test")
	}

	repoRoot := findRepoRoot(t)
	clusterName := "autodeploy-e2e"

	// 1. Create kind cluster
	setupKindCluster(t, clusterName)

	// 2. Build and install autodeploy
	imagePrefix := "e2e"
	imageTag := time.Now().Format("20060102T150405")
	os.Setenv("IMAGE_PREFIX", imagePrefix)
	os.Setenv("IMAGE_TAG", imageTag)

	// Build all components
	runCmd(t, repoRoot, "go", "run", "./ap", "build")
	runCmd(t, repoRoot, "go", "run", "./ap", "deploy", "--root=autodeploy", "--skip-push")
	runCmd(t, repoRoot, "go", "run", "./ap", "deploy", "--root=in-cluster-image-registry", "--skip-push")

	// Load images into kind
	imagesToLoad := []string{
		"autodeploy-controller",
		"in-cluster-image-registry-node-agent",
	}
	for _, img := range imagesToLoad {
		runCmd(t, repoRoot, "kind", "load", "docker-image", fmt.Sprintf("%s/%s:%s", imagePrefix, img, imageTag), "--name", clusterName)
	}

	// Wait for components to be ready
	waitForDeployment(t, "buildkit", "autodeploy-system", 2*time.Minute)
	waitForDeployment(t, "autodeploy-controller", "autodeploy-system", 2*time.Minute)
	waitForStatefulSet(t, "in-cluster-image-registry", "in-cluster-image-registry-system", 2*time.Minute)
	waitForDaemonSet(t, "node-agent", "in-cluster-image-registry-system", 2*time.Minute)

	// 3. Install helloworld example via Package CRD
	t.Log("Creating Package resource for helloworld")
	// We use the actual repo URL since autodeploy will clone it.
	pkgYAML := `
apiVersion: infra.labs.gke.io/v1alpha1
kind: Package
metadata:
  name: helloworld-package
  namespace: default
spec:
  repo: https://github.com/gke-labs/gke-labs-infra
  directory: autodeploy/examples/helloworld
`
	runCmdWithInput(t, repoRoot, pkgYAML, "kubectl", "apply", "-f", "-")

	// 4. Verify helloworld is deployed
	waitForDeployment(t, "helloworld", "default", 2*time.Minute)
}

func runCmdWithInput(t *testing.T, dir string, input string, name string, args ...string) {
	t.Helper()
	t.Logf("Running command with input: %s %s", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(input)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("failed to find repo root")
		}
		dir = parent
	}
}

func setupKindCluster(t *testing.T, name string) {
	t.Helper()
	// Check if cluster exists
	out := runCmdCapture(t, ".", "kind", "get", "clusters")
	exists := false
	for _, cluster := range strings.Split(out, "\n") {
		if strings.TrimSpace(cluster) == name {
			exists = true
			break
		}
	}

	if !exists {
		t.Logf("Creating kind cluster %s", name)
		runCmd(t, ".", "kind", "create", "cluster", "--name", name)
	}

	// Ensure we use the correct context
	runCmd(t, ".", "kubectl", "config", "use-context", "kind-"+name)

	// No automatic cleanup of the cluster, usually we want to keep it if it fails for debugging
	// or let the CI environment handle it. But the pattern says robust cleanup.
	if os.Getenv("SKIP_CLEANUP") == "" {
		t.Cleanup(func() {
			t.Logf("Deleting kind cluster %s", name)
			runCmd(t, ".", "kind", "delete", "cluster", "--name", name)
		})
	}
}

func waitForDeployment(t *testing.T, name, namespace string, timeout time.Duration) {
	t.Helper()
	t.Logf("Waiting for deployment %s in namespace %s", name, namespace)
	start := time.Now()
	for {
		if time.Since(start) > timeout {
			t.Fatalf("timeout waiting for deployment %s", name)
		}

		cmd := exec.Command("kubectl", "get", "deployment", name, "-n", namespace, "-o", "jsonpath={.status.readyReplicas}")
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if err := cmd.Run(); err == nil {
			ready := strings.TrimSpace(stdout.String())
			if ready != "" && ready != "0" {
				t.Logf("Deployment %s is ready", name)
				return
			}
		}

		time.Sleep(5 * time.Second)
	}
}

func waitForStatefulSet(t *testing.T, name, namespace string, timeout time.Duration) {
	t.Helper()
	t.Logf("Waiting for statefulset %s in namespace %s", name, namespace)
	start := time.Now()
	for {
		if time.Since(start) > timeout {
			t.Fatalf("timeout waiting for statefulset %s", name)
		}

		cmd := exec.Command("kubectl", "get", "statefulset", name, "-n", namespace, "-o", "jsonpath={.status.readyReplicas}")
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if err := cmd.Run(); err == nil {
			ready := strings.TrimSpace(stdout.String())
			if ready != "" && ready != "0" {
				t.Logf("StatefulSet %s is ready", name)
				return
			}
		}

		time.Sleep(5 * time.Second)
	}
}

func waitForDaemonSet(t *testing.T, name, namespace string, timeout time.Duration) {
	t.Helper()
	t.Logf("Waiting for daemonset %s in namespace %s", name, namespace)
	start := time.Now()
	for {
		if time.Since(start) > timeout {
			t.Fatalf("timeout waiting for daemonset %s", name)
		}

		cmd := exec.Command("kubectl", "get", "daemonset", name, "-n", namespace, "-o", "jsonpath={.status.numberReady}")
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if err := cmd.Run(); err == nil {
			ready := strings.TrimSpace(stdout.String())
			if ready != "" && ready != "0" {
				t.Logf("DaemonSet %s is ready", name)
				return
			}
		}

		time.Sleep(5 * time.Second)
	}
}

func runCmd(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	t.Logf("Running command: %s %s", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
}

func runCmdCapture(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
	return stdout.String()
}
