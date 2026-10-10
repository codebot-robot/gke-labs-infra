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
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupMockKubectl(t *testing.T, scriptContent string) {
	t.Helper()
	binDir := t.TempDir()
	kubectlPath := filepath.Join(binDir, "kubectl")
	if err := os.WriteFile(kubectlPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to write mock kubectl: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestCollectArtifacts_NoArtifactsDir(t *testing.T) {
	t.Setenv("ARTIFACTS", "")
	h := NewHarness(t, "test-cluster")
	h.TrackNamespace("default")
	if err := h.CollectArtifacts("test-case"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCollectArtifacts_KubectlFails_NoEmptyFilesWritten(t *testing.T) {
	artifactsDir := t.TempDir()
	t.Setenv("ARTIFACTS", artifactsDir)
	script := `#!/bin/sh
echo "kubectl error" >&2
exit 1
`
	setupMockKubectl(t, script)
	h := NewHarness(t, "test-cluster")
	h.TrackNamespace("test-ns")
	if err := h.CollectArtifacts("test-case"); err == nil {
		t.Fatal("expected error from CollectArtifacts when kubectl commands fail")
	}

	// Ensure no empty files were written
	nsDir := filepath.Join(artifactsDir, "tests", "test-case", "objects", "test-ns")
	if _, err := os.Stat(filepath.Join(nsDir, "pods.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected pods.txt not to exist, got err: %v", err)
	}
	if _, err := os.Stat(filepath.Join(nsDir, "pods.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected pods.yaml not to exist, got err: %v", err)
	}
	clusterDir := filepath.Join(artifactsDir, "tests", "test-case", "objects", "_cluster")
	if _, err := os.Stat(filepath.Join(clusterDir, "nodes.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected nodes.txt not to exist, got err: %v", err)
	}
}

func TestCollectArtifacts_Success(t *testing.T) {
	artifactsDir := t.TempDir()
	t.Setenv("ARTIFACTS", artifactsDir)
	script := `#!/bin/sh
case "$*" in
  *"get pods -n test-ns -o yaml"*)
    printf "apiVersion: v1\nkind: PodList\n"
    ;;
  *"get pods -n test-ns -o jsonpath={.items[*].metadata.name}"*)
    printf "pod-1\n"
    ;;
  *"get pods -n test-ns"*)
    printf "NAME READY STATUS\n"
    ;;
  *"logs pod-1 -n test-ns"*)
    printf "container logs\n"
    ;;
  *"get nodes -o yaml"*)
    printf "apiVersion: v1\nkind: NodeList\n"
    ;;
  *"get nodes"*)
    printf "NAME STATUS ROLES\n"
    ;;
  *)
    printf "ok\n"
    ;;
esac
`
	setupMockKubectl(t, script)
	h := NewHarness(t, "test-cluster")
	h.TrackNamespace("test-ns")
	if err := h.CollectArtifacts("test-case"); err != nil {
		t.Fatalf("unexpected error from CollectArtifacts: %v", err)
	}

	nsDir := filepath.Join(artifactsDir, "tests", "test-case", "objects", "test-ns")
	podsContent, err := os.ReadFile(filepath.Join(nsDir, "pods.txt"))
	if err != nil {
		t.Fatalf("failed to read pods.txt: %v", err)
	}
	if !strings.Contains(string(podsContent), "NAME READY STATUS") {
		t.Errorf("unexpected pods.txt content: %s", podsContent)
	}

	logsDir := filepath.Join(artifactsDir, "tests", "test-case", "logs", "test-ns")
	logContent, err := os.ReadFile(filepath.Join(logsDir, "pod-1.log"))
	if err != nil {
		t.Fatalf("failed to read pod-1.log: %v", err)
	}
	if !strings.Contains(string(logContent), "container logs") {
		t.Errorf("unexpected log content: %s", logContent)
	}
}

func TestMustWriteFile(t *testing.T) {
	dir := t.TempDir()
	h := NewHarness(t, "test-cluster")
	target := filepath.Join(dir, "sub", "test.txt")
	h.MustWriteFile(target, []byte("hello"))

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("got %q, want %q", string(data), "hello")
	}
}

func TestDeleteMethods(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		script := `#!/bin/sh
exit 0
`
		setupMockKubectl(t, script)
		h := NewHarness(t, "test-cluster")
		if err := h.DeleteDeployment("dep", "ns"); err != nil {
			t.Errorf("unexpected error from DeleteDeployment: %v", err)
		}
		if err := h.DeleteStatefulSet("sts", "ns"); err != nil {
			t.Errorf("unexpected error from DeleteStatefulSet: %v", err)
		}
		if err := h.DeleteDaemonSet("ds", "ns"); err != nil {
			t.Errorf("unexpected error from DeleteDaemonSet: %v", err)
		}
		if err := h.DeleteService("svc", "ns"); err != nil {
			t.Errorf("unexpected error from DeleteService: %v", err)
		}
		if err := h.DeletePod("pod", "ns"); err != nil {
			t.Errorf("unexpected error from DeletePod: %v", err)
		}
		if err := h.DeleteJob("job", "ns"); err != nil {
			t.Errorf("unexpected error from DeleteJob: %v", err)
		}
	})

	t.Run("failure returns error", func(t *testing.T) {
		script := `#!/bin/sh
echo "error: unauthorized" >&2
exit 1
`
		setupMockKubectl(t, script)
		h := NewHarness(t, "test-cluster")
		if err := h.DeleteDeployment("dep", "ns"); err == nil || !strings.Contains(err.Error(), "unauthorized") {
			t.Errorf("expected unauthorized error from DeleteDeployment, got: %v", err)
		}
		if err := h.DeleteStatefulSet("sts", "ns"); err == nil || !strings.Contains(err.Error(), "unauthorized") {
			t.Errorf("expected unauthorized error from DeleteStatefulSet, got: %v", err)
		}
		if err := h.DeleteDaemonSet("ds", "ns"); err == nil || !strings.Contains(err.Error(), "unauthorized") {
			t.Errorf("expected unauthorized error from DeleteDaemonSet, got: %v", err)
		}
		if err := h.DeleteService("svc", "ns"); err == nil || !strings.Contains(err.Error(), "unauthorized") {
			t.Errorf("expected unauthorized error from DeleteService, got: %v", err)
		}
		if err := h.DeletePod("pod", "ns"); err == nil || !strings.Contains(err.Error(), "unauthorized") {
			t.Errorf("expected unauthorized error from DeletePod, got: %v", err)
		}
		if err := h.DeleteJob("job", "ns"); err == nil || !strings.Contains(err.Error(), "unauthorized") {
			t.Errorf("expected unauthorized error from DeleteJob, got: %v", err)
		}
	})
}

func TestWaitForPodReady(t *testing.T) {
	t.Run("ready immediately", func(t *testing.T) {
		script := `#!/bin/sh
case "$*" in
  *"status.phase"*)
    echo "Running"
    ;;
  *"status.containerStatuses[*].ready"*)
    echo "true"
    ;;
esac
`
		setupMockKubectl(t, script)
		h := NewHarness(t, "test-cluster")
		h.PollInterval = 1 * time.Millisecond
		if err := h.WaitForPodReady("pod-1", "ns", 1*time.Second); err != nil {
			t.Fatalf("expected pod to be ready, got error: %v", err)
		}
	})

	t.Run("persistent failure fails fast", func(t *testing.T) {
		script := `#!/bin/sh
echo "connection refused" >&2
exit 1
`
		setupMockKubectl(t, script)
		h := NewHarness(t, "test-cluster")
		h.PollInterval = 1 * time.Millisecond
		start := time.Now()
		err := h.WaitForPodReady("pod-1", "ns", 10*time.Second)
		elapsed := time.Since(start)
		if err == nil {
			t.Fatal("expected error from persistent kubectl failure, got nil")
		}
		if !strings.Contains(err.Error(), "kubectl failed persistently") {
			t.Errorf("expected 'kubectl failed persistently' error, got: %v", err)
		}
		if elapsed > 3*time.Second {
			t.Errorf("expected to fail fast, took %v", elapsed)
		}
	})

	t.Run("not found times out without failing fast", func(t *testing.T) {
		script := `#!/bin/sh
echo 'Error from server (NotFound): pods "pod-1" not found' >&2
exit 1
`
		setupMockKubectl(t, script)
		h := NewHarness(t, "test-cluster")
		h.PollInterval = 5 * time.Millisecond
		err := h.WaitForPodReady("pod-1", "ns", 50*time.Millisecond)
		if err == nil {
			t.Fatal("expected timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "timed out waiting for pod") {
			t.Errorf("expected timeout error for not-found pod, got: %v", err)
		}
	})
}
