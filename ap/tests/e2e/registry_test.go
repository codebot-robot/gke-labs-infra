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

package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/e2e"
)

func TestInClusterRegistry(t *testing.T) {
	if os.Getenv("RUN_E2E") == "" {
		t.Skip("RUN_E2E not set, skipping e2e test")
	}

	h := e2e.NewHarness(t, e2e.Options{ClusterName: "ap-registry-e2e"})
	h.SetupRegistry()

	repoRoot := h.FindRepoRoot()
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("failed to change to repo root: %v", err)
	}

	// Set up environment for ap
	os.Setenv("IMAGE_PREFIX", "images.local")
	os.Setenv("IMAGE_TAG", "e2e")

	// 1. Build and push image
	// We'll build the examples-helloworld image
	h.RunCmd("go", "run", "./ap", "build", "--root=autodeploy/examples/helloworld", "--push")

	// 2. Deploy using the image
	// We'll create a simple manifest that uses the image
	// examples-helloworld:latest is a placeholder that ap deploy will replace.
	manifest := `
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  namespace: default
spec:
  containers:
  - name: test
    image: examples-helloworld:latest
    command: ["/bin/sh", "-c", "echo hello"]
`

	// Create a temporary directory for the test manifests
	tmpDir, err := os.MkdirTemp("", "ap-e2e-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	k8sDir := filepath.Join(tmpDir, "k8s")
	if err := os.MkdirAll(k8sDir, 0755); err != nil {
		t.Fatalf("failed to create k8s dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(k8sDir, "pod.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatalf("failed to write pod manifest: %v", err)
	}

	// Create a dummy .ap/ap.yaml so ap recognizes it as a root
	if err := os.MkdirAll(filepath.Join(tmpDir, ".ap"), 0755); err != nil {
		t.Fatalf("failed to create .ap dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".ap", "ap.yaml"), []byte("name: test-app"), 0644); err != nil {
		t.Fatalf("failed to write ap.yaml: %v", err)
	}

	// Run ap deploy
	h.RunCmd("go", "run", "./ap", "deploy", "--root="+tmpDir)

	// 3. Verify the pod was created and has the correct image
	image := h.RunCmd("kubectl", "get", "pod", "test-pod", "-n", "default", "-o", "jsonpath={.spec.containers[0].image}")
	expectedImage := "images.local/examples-helloworld:e2e"
	if image != expectedImage {
		t.Errorf("expected image %q, got %q", expectedImage, image)
	}
}
