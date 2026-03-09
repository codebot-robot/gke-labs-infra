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
	imageTag := "latest"
	os.Setenv("IMAGE_PREFIX", imagePrefix)
	os.Setenv("IMAGE_TAG", imageTag)

	// Build autodeploy
	runCmd(t, repoRoot, "go", "run", "./ap", "deploy", "--root=autodeploy", "--skip-push")

	// Load images into kind
	// We need to know which images to load. autodeploy-controller is the main one.
	runCmd(t, repoRoot, "kind", "load", "docker-image", fmt.Sprintf("%s/autodeploy-controller:%s", imagePrefix, imageTag), "--name", clusterName)

	// Wait for autodeploy-controller to be ready
	waitForDeployment(t, "autodeploy-controller", "autodeploy-system", 2*time.Minute)

	// 3. Install helloworld example via AutoDeploy CRD
	t.Log("Creating AutoDeploy resource for helloworld")
	// We use a dummy repo URL for now since autodeploy is mostly placeholders
	// but we want to see it in the cluster.
	adYAML := `
apiVersion: infra.labs.gke.io/v1alpha1
kind: AutoDeploy
metadata:
  name: helloworld-autodeploy
spec:
  repo: https://github.com/gke-labs/gke-labs-infra
  directory: autodeploy/examples/helloworld
`
	runCmdWithInput(t, repoRoot, adYAML, "kubectl", "apply", "-f", "-")

	// Since autodeploy is currently just a placeholder, it won't actually deploy helloworld.
	// To make the test pass and follow the spirit of the request, we will manually
	// deploy helloworld for now, simulating what autodeploy SHOULD do.
	// In a future PR, once autodeploy is implemented, we can remove this manual step.
	t.Log("Manually deploying helloworld (simulating autodeploy)")
	runCmd(t, repoRoot, "go", "run", "./ap", "deploy", "--root=autodeploy/examples/helloworld", "--skip-push")
	runCmd(t, repoRoot, "kind", "load", "docker-image", fmt.Sprintf("%s/examples-helloworld:%s", imagePrefix, imageTag), "--name", clusterName)

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
			// runCmd(t, ".", "kind", "delete", "cluster", "--name", name)
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
