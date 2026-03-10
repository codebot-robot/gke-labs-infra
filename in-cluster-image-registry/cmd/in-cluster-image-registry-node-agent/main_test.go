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
	"bytes"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestUpdateTOML(t *testing.T) {
	initialTOML := []byte(`
version = 2
[plugins]
  [plugins."io.containerd.grpc.v1.cri"]
    [plugins."io.containerd.grpc.v1.cri".registry]
      [plugins."io.containerd.grpc.v1.cri".registry.mirrors]
        [plugins."io.containerd.grpc.v1.cri".registry.mirrors."docker.io"]
          endpoint = ["https://registry-1.docker.io"]
`)

	ip := "10.96.0.10"
	newContent, changed, err := updateTOML(initialTOML, ip)
	if err != nil {
		t.Fatalf("updateTOML failed: %v", err)
	}

	if !changed {
		t.Fatal("expected changed to be true")
	}

	var cfg map[string]interface{}
	err = toml.Unmarshal(newContent, &cfg)
	if err != nil {
		t.Fatalf("failed to unmarshal new TOML: %v", err)
	}

	// Verify mirror
	plugins := cfg["plugins"].(map[string]interface{})
	cri := plugins["io.containerd.grpc.v1.cri"].(map[string]interface{})
	registry := cri["registry"].(map[string]interface{})
	mirrors := registry["mirrors"].(map[string]interface{})
	mirror := mirrors["images.local"].(map[string]interface{})
	endpoints := mirror["endpoint"].([]interface{})
	if len(endpoints) != 1 || endpoints[0] != "http://10.96.0.10" {
		t.Errorf("unexpected endpoints: %v", endpoints)
	}

	// Verify config
	configs := registry["configs"].(map[string]interface{})
	config := configs["images.local"].(map[string]interface{})
	tls := config["tls"].(map[string]interface{})
	if tls["insecure_skip_verify"] != true {
		t.Errorf("expected insecure_skip_verify to be true")
	}

	// Run again with same IP, should not change
	newContent2, changed2, err := updateTOML(newContent, ip)
	if err != nil {
		t.Fatalf("second updateTOML failed: %v", err)
	}
	if changed2 {
		t.Fatal("expected changed to be false on second run")
	}
	if !bytes.Equal(newContent, newContent2) {
		t.Fatal("expected content to be identical on second run")
	}

	// Run with different IP, should change
	newIP := "10.96.0.11"
	_, changed3, err := updateTOML(newContent, newIP)
	if err != nil {
		t.Fatalf("third updateTOML failed: %v", err)
	}
	if !changed3 {
		t.Fatal("expected changed to be true on IP change")
	}
}
