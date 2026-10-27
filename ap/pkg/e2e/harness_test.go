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
)

func TestHarness_KindCommandPointsIntoCache(t *testing.T) {
	tempCache := t.TempDir()
	t.Setenv("AP_CACHE_DIR", tempCache)

	// Pre-populate cache so test runs without network requests
	version := "v0.33.0"
	binDir := filepath.Join(tempCache, "tools", "kind", version, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	fakeBin := filepath.Join(binDir, "kind")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	h := &Harness{t: t}
	cmdPath := h.KindCommand()
	if !strings.HasPrefix(cmdPath, tempCache) {
		t.Errorf("expected kind command path %q to point into cache dir %q", cmdPath, tempCache)
	}
	expectedSuffix := filepath.Join("tools", "kind", version, "bin", "kind")
	if !strings.HasSuffix(cmdPath, expectedSuffix) {
		t.Errorf("expected kind command path %q to end with %q", cmdPath, expectedSuffix)
	}
}

func TestLoadImages_Delegation(t *testing.T) {
	err := LoadImages(t.Context(), "", "ap-cluster", []string{"test-image:latest"})
	if err == nil {
		t.Fatal("expected error when kindBin is empty, got nil")
	}
	if !strings.Contains(err.Error(), "kindBin is required") {
		t.Fatalf("expected error to contain 'kindBin is required', got: %v", err)
	}
}

func TestNewHarness(t *testing.T) {
	if os.Getenv("RUN_E2E") == "" {
		t.Skip("Skipping E2E test; set RUN_E2E=1 to run")
	}

	h := NewHarness(t, Options{
		ClusterName: "ap-e2e-test",
	})

	if h.Namespace() == "" {
		t.Fatal("Namespace should not be empty")
	}

	h.KubectlApplyContent(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
data:
  key: value
`)

	h.runCmd("kubectl", "get", "configmap", "test-cm", "-n", h.Namespace())
}
