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
	"runtime"
	"strings"
	"testing"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/tool/kind"
)

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

func TestHarness_KindPointsToCache(t *testing.T) {
	tempCache := t.TempDir()
	t.Setenv("AP_CACHE_DIR", tempCache)

	rel := kind.Latest()
	if rel == nil {
		t.Fatal("expected at least one kind release in table")
	}
	tag := rel.Version
	binDir := filepath.Join(tempCache, "tools", "kind", tag, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	binName := "kind"
	if runtime.GOOS == "windows" {
		binName = "kind.exe"
	}
	binPath := filepath.Join(binDir, binName)
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}

	h := &Harness{t: t}
	h.setupKind()

	if h.KindCommand() != binPath {
		t.Errorf("expected KindCommand %q, got %q", binPath, h.KindCommand())
	}
	if !strings.HasPrefix(h.KindCommand(), tempCache) {
		t.Errorf("expected command path %q to point into cache %q", h.KindCommand(), tempCache)
	}
}
