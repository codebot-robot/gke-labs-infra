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
	"os"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestOptionsInitDefaults(t *testing.T) {
	var opt Options
	opt.InitDefaults()

	if opt.RuntimeClassName != "microvm" {
		t.Errorf("RuntimeClassName = %q, want %q", opt.RuntimeClassName, "microvm")
	}
	if opt.CPURequest != "4" {
		t.Errorf("CPURequest = %q, want %q", opt.CPURequest, "4")
	}
	if opt.CPULimit != "4" {
		t.Errorf("CPULimit = %q, want %q", opt.CPULimit, "4")
	}
	if opt.MemoryRequest != "2Gi" {
		t.Errorf("MemoryRequest = %q, want %q", opt.MemoryRequest, "2Gi")
	}
	if opt.MemoryLimit != "4Gi" {
		t.Errorf("MemoryLimit = %q, want %q", opt.MemoryLimit, "4Gi")
	}
}

func TestBuildPoolOptions(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		var opt Options
		opt.InitDefaults()
		poolOpts, err := buildPoolOptions(opt)
		if err != nil {
			t.Fatalf("buildPoolOptions failed: %v", err)
		}
		if poolOpts.Runner.RuntimeClassName != "microvm" {
			t.Errorf("RuntimeClassName = %q, want %q", poolOpts.Runner.RuntimeClassName, "microvm")
		}
		cpuReq := poolOpts.Runner.Resources.Requests[corev1.ResourceCPU]
		if cpuReq.Cmp(resource.MustParse("4")) != 0 {
			t.Errorf("CPURequest = %v, want 4", cpuReq)
		}
		cpuLim := poolOpts.Runner.Resources.Limits[corev1.ResourceCPU]
		if cpuLim.Cmp(resource.MustParse("4")) != 0 {
			t.Errorf("CPULimit = %v, want 4", cpuLim)
		}
	})

	t.Run("opt-out runtime class", func(t *testing.T) {
		var opt Options
		opt.InitDefaults()
		opt.RuntimeClassName = ""
		poolOpts, err := buildPoolOptions(opt)
		if err != nil {
			t.Fatalf("buildPoolOptions failed: %v", err)
		}
		if poolOpts.Runner.RuntimeClassName != "" {
			t.Errorf("RuntimeClassName = %q, want empty", poolOpts.Runner.RuntimeClassName)
		}
	})
}

func TestLoadRESTConfig(t *testing.T) {
	t.Run("explicit kubeconfig file", func(t *testing.T) {
		kubeconfigContent := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
    token: fake-token
`
		tmpDir := t.TempDir()
		kcPath := filepath.Join(tmpDir, "kubeconfig")
		if err := os.WriteFile(kcPath, []byte(kubeconfigContent), 0o600); err != nil {
			t.Fatalf("failed to write kubeconfig file: %v", err)
		}

		cfg, err := loadRESTConfig(kcPath)
		if err != nil {
			t.Fatalf("loadRESTConfig(%q) returned error: %v", kcPath, err)
		}
		if cfg.Host != "https://127.0.0.1:6443" {
			t.Errorf("cfg.Host = %q, want https://127.0.0.1:6443", cfg.Host)
		}
		if cfg.BearerToken != "fake-token" {
			t.Errorf("cfg.BearerToken = %q, want fake-token", cfg.BearerToken)
		}
	})

	t.Run("nonexistent kubeconfig file", func(t *testing.T) {
		_, err := loadRESTConfig("/nonexistent/kubeconfig/path")
		if err == nil {
			t.Errorf("loadRESTConfig with nonexistent path expected error, got nil")
		}
	})

	t.Run("fallback when not in cluster", func(t *testing.T) {
		// When kubeconfig is empty and running outside a cluster (without
		// KUBERNETES_SERVICE_HOST), loadRESTConfig should fall back to default
		// loading rules without erroring on ErrNotInCluster.
		t.Setenv("KUBERNETES_SERVICE_HOST", "")
		t.Setenv("KUBERNETES_SERVICE_PORT", "")

		// Even if no kubeconfig exists at default location, it should attempt
		// clientcmd loading and not fail with "loading in-cluster config".
		_, _ = loadRESTConfig("")
	})
}
