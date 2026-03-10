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
	"strings"
	"testing"
)

func TestUpdateTOML(t *testing.T) {
	initialTOML := []byte(`# This is a comment that should be preserved
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

	strContent := string(newContent)
	if !strings.Contains(strContent, "# This is a comment that should be preserved") {
		t.Error("expected comment to be preserved")
	}

	if !strings.Contains(strContent, beginMarker) {
		t.Error("expected begin marker")
	}

	if !strings.Contains(strContent, `endpoint = ["http://10.96.0.10"]`) {
		t.Error("expected correct endpoint")
	}

	if !strings.Contains(strContent, `insecure_skip_verify = true`) {
		t.Error("expected insecure_skip_verify to be true")
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
	newContent3, changed3, err := updateTOML(newContent, newIP)
	if err != nil {
		t.Fatalf("third updateTOML failed: %v", err)
	}
	if !changed3 {
		t.Fatal("expected changed to be true on IP change")
	}

	if !strings.Contains(string(newContent3), `endpoint = ["http://10.96.0.11"]`) {
		t.Error("expected updated endpoint")
	}
}
