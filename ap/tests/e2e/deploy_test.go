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
	"strings"
	"testing"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/e2e"
)

func TestDeploy_DigestRollout(t *testing.T) {
	if os.Getenv("RUN_E2E") == "" {
		t.Skip("RUN_E2E not set, skipping e2e test")
	}

	h := e2e.NewHarness(t, e2e.Options{ClusterName: "ap-deploy-digest-e2e"})

	repoRoot := h.FindRepoRoot()
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("failed to change to repo root: %v", err)
	}

	tmpDir, err := os.MkdirTemp("", "ap-deploy-digest-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create minimal scratch repository structure
	if err := os.MkdirAll(filepath.Join(tmpDir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, ".ap"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".ap", "ap.yaml"), []byte("imagePrefix: example.com/test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "images", "my-app"), 0755); err != nil {
		t.Fatal(err)
	}
	dockerfile1 := "FROM alpine:latest\nRUN echo v1 > /version.txt\nCMD [\"/bin/sh\", \"-c\", \"sleep 3600\"]\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "images", "my-app", "Dockerfile"), []byte(dockerfile1), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(tmpDir, "k8s"), 0755); err != nil {
		t.Fatal(err)
	}
	deploymentManifest := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: my-app
  template:
    metadata:
      labels:
        app: my-app
    spec:
      containers:
      - name: my-app
        image: my-app
        imagePullPolicy: IfNotPresent
        command: ["/bin/sh", "-c", "sleep 3600"]
`
	if err := os.WriteFile(filepath.Join(tmpDir, "k8s", "deployment.yaml"), []byte(deploymentManifest), 0644); err != nil {
		t.Fatal(err)
	}

	// 1. Deploy initial version with default tag (:latest)
	os.Unsetenv("IMAGE_TAG")
	os.Setenv("IMAGE_PREFIX", "example.com/test")

	h.RunCmd("go", "run", "./ap", "deploy", "--root="+tmpDir)
	h.RunCmd("kubectl", "rollout", "status", "deployment/my-app", "-n", "default", "--timeout=60s")

	rs1 := strings.TrimSpace(h.RunCmd("kubectl", "get", "pods", "-l", "app=my-app", "-n", "default", "-o", "jsonpath={.items[0].metadata.ownerReferences[0].name}"))
	appliedImage1 := strings.TrimSpace(h.RunCmd("kubectl", "get", "deployment", "my-app", "-n", "default", "-o", "jsonpath={.spec.template.spec.containers[0].image}"))

	if !strings.Contains(appliedImage1, "@sha256:") {
		t.Fatalf("expected applied image to contain @sha256: digest, got %q", appliedImage1)
	}
	digest1 := appliedImage1[strings.LastIndex(appliedImage1, "@")+1:]

	podImageID1 := strings.TrimSpace(h.RunCmd("kubectl", "get", "pods", "-l", "app=my-app", "-n", "default", "-o", "jsonpath={.items[0].status.containerStatuses[0].imageID}"))
	if !strings.Contains(podImageID1, digest1) {
		t.Errorf("expected pod imageID %q to match digest %q", podImageID1, digest1)
	}

	// 2. Change Dockerfile and redeploy with default tag (:latest)
	dockerfile2 := "FROM alpine:latest\nRUN echo v2 > /version.txt\nCMD [\"/bin/sh\", \"-c\", \"sleep 3600\"]\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "images", "my-app", "Dockerfile"), []byte(dockerfile2), 0644); err != nil {
		t.Fatal(err)
	}

	h.RunCmd("go", "run", "./ap", "deploy", "--root="+tmpDir)
	h.RunCmd("kubectl", "rollout", "status", "deployment/my-app", "-n", "default", "--timeout=60s")

	rs2 := strings.TrimSpace(h.RunCmd("kubectl", "get", "pods", "-l", "app=my-app", "-n", "default", "-o", "jsonpath={.items[0].metadata.ownerReferences[0].name}"))
	if rs2 == rs1 {
		t.Errorf("expected a new ReplicaSet after redeploy with changed Dockerfile, but both were %q", rs1)
	}

	appliedImage2 := strings.TrimSpace(h.RunCmd("kubectl", "get", "deployment", "my-app", "-n", "default", "-o", "jsonpath={.spec.template.spec.containers[0].image}"))
	if appliedImage2 == appliedImage1 {
		t.Errorf("expected applied image to change after redeploy, got %q", appliedImage2)
	}
	if !strings.Contains(appliedImage2, "@sha256:") {
		t.Fatalf("expected applied image to contain @sha256: digest, got %q", appliedImage2)
	}
	digest2 := appliedImage2[strings.LastIndex(appliedImage2, "@")+1:]
	if digest2 == digest1 {
		t.Errorf("expected different digest for changed image, got %q", digest2)
	}

	podImageID2 := strings.TrimSpace(h.RunCmd("kubectl", "get", "pods", "-l", "app=my-app", "-n", "default", "-o", "jsonpath={.items[0].status.containerStatuses[0].imageID}"))
	if !strings.Contains(podImageID2, digest2) {
		t.Errorf("expected pod imageID %q to match new digest %q", podImageID2, digest2)
	}

	// 3. Redeploy with an explicit unique tag (no digest should be appended)
	os.Setenv("IMAGE_TAG", "v1.0")
	h.RunCmd("go", "run", "./ap", "deploy", "--root="+tmpDir)
	h.RunCmd("kubectl", "rollout", "status", "deployment/my-app", "-n", "default", "--timeout=60s")

	appliedImage3 := strings.TrimSpace(h.RunCmd("kubectl", "get", "deployment", "my-app", "-n", "default", "-o", "jsonpath={.spec.template.spec.containers[0].image}"))
	expectedImage3 := "example.com/test/my-app:v1.0"
	if appliedImage3 != expectedImage3 {
		t.Errorf("expected applied image %q with explicit IMAGE_TAG, got %q", expectedImage3, appliedImage3)
	}
}
