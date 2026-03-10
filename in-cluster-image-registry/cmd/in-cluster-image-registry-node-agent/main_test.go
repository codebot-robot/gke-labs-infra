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
	"strings"
	"testing"
)

func TestUpdateHostsConfig(t *testing.T) {
	tmpDir := t.TempDir()
	hostsDir := tmpDir + "/images.local"
	hostsPath := hostsDir + "/hosts.toml"
	ip := "10.96.0.10"

	err := updateHostsConfig(hostsDir, hostsPath, ip)
	if err != nil {
		t.Fatalf("updateHostsConfig failed: %v", err)
	}

	content, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("failed to read hosts.toml: %v", err)
	}

	expected := `server = "http://images.local"

[host."http://10.96.0.10"]
  capabilities = ["pull", "resolve"]
  skip_verify = true
`
	if string(content) != expected {
		t.Errorf("unexpected content:\ngot:\n%s\nwant:\n%s", string(content), expected)
	}

	// Update with same IP, should not change anything (just verify it doesn't fail)
	err = updateHostsConfig(hostsDir, hostsPath, ip)
	if err != nil {
		t.Fatalf("second updateHostsConfig failed: %v", err)
	}

	// Update with new IP
	newIP := "10.96.0.11"
	err = updateHostsConfig(hostsDir, hostsPath, newIP)
	if err != nil {
		t.Fatalf("third updateHostsConfig failed: %v", err)
	}

	content, err = os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("failed to read hosts.toml after update: %v", err)
	}
	if !strings.Contains(string(content), `[host."http://10.96.0.11"]`) {
		t.Errorf("expected updated IP in content: %s", string(content))
	}
}

func TestCleanupOldConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.toml"

	initialContent := `version = 2
[plugins]
  [plugins."io.containerd.grpc.v1.cri"]
# BEGIN IN-CLUSTER-IMAGE-REGISTRY CONFIGURATION
[plugins."io.containerd.grpc.v1.cri".registry.mirrors."images.local"]
  endpoint = ["http://10.96.0.10"]
# END IN-CLUSTER-IMAGE-REGISTRY CONFIGURATION
    [plugins."io.containerd.grpc.v1.cri".registry]
      config_path = "/etc/containerd/certs.d"
`
	err := os.WriteFile(configPath, []byte(initialContent), 0644)
	if err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	err = cleanupOldConfig(configPath)
	if err != nil {
		t.Fatalf("cleanupOldConfig failed: %v", err)
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config after cleanup: %v", err)
	}

	if strings.Contains(string(content), beginMarker) {
		t.Error("expected begin marker to be removed")
	}
	if strings.Contains(string(content), "mirrors.\"images.local\"") {
		t.Error("expected old config to be removed")
	}
	if !strings.Contains(string(content), "config_path = \"/etc/containerd/certs.d\"") {
		t.Error("expected other config to be preserved")
	}
}
