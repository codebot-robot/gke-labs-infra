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
)

const testKubeconfig = `apiVersion: v1
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

func writeTestKubeconfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")
	if err := os.WriteFile(path, []byte(testKubeconfig), 0o600); err != nil {
		t.Fatalf("writing test kubeconfig: %v", err)
	}
	return path
}

func TestLoadRESTConfig_ExplicitKubeconfig(t *testing.T) {
	path := writeTestKubeconfig(t)

	cfg, err := loadRESTConfig(path)
	if err != nil {
		t.Fatalf("loadRESTConfig(%q) returned unexpected error: %v", path, err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil rest.Config")
	}
	if cfg.Host != "https://127.0.0.1:6443" {
		t.Errorf("cfg.Host = %q, want https://127.0.0.1:6443", cfg.Host)
	}
	if cfg.BearerToken != "fake-token" {
		t.Errorf("cfg.BearerToken = %q, want fake-token", cfg.BearerToken)
	}
}

func TestLoadRESTConfig_ExplicitKubeconfig_NotFound(t *testing.T) {
	nonexistent := filepath.Join(t.TempDir(), "nonexistent-kubeconfig")
	cfg, err := loadRESTConfig(nonexistent)
	if err == nil {
		t.Fatalf("loadRESTConfig(%q) succeeded, expected error", nonexistent)
	}
	if cfg != nil {
		t.Errorf("expected nil rest.Config, got %+v", cfg)
	}
}

func TestLoadRESTConfig_FallbackWhenNotInCluster(t *testing.T) {
	path := writeTestKubeconfig(t)

	// Ensure in-cluster detection fails with rest.ErrNotInCluster.
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
	t.Setenv("KUBECONFIG", path)

	cfg, err := loadRESTConfig("")
	if err != nil {
		t.Fatalf("loadRESTConfig(\"\") returned unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil rest.Config")
	}
	if cfg.Host != "https://127.0.0.1:6443" {
		t.Errorf("cfg.Host = %q, want https://127.0.0.1:6443", cfg.Host)
	}
	if cfg.BearerToken != "fake-token" {
		t.Errorf("cfg.BearerToken = %q, want fake-token", cfg.BearerToken)
	}
}
