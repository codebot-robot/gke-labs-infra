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

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProtoConfig_NonExistent(t *testing.T) {
	tempDir := t.TempDir()
	cfg, err := LoadProtoConfig(tempDir)
	if err != nil {
		t.Fatalf("unexpected error for non-existent proto.yaml: %v", err)
	}
	if cfg != nil {
		t.Fatalf("expected nil config, got %+v", cfg)
	}
}

func TestLoadProtoConfig_Empty(t *testing.T) {
	tempDir := t.TempDir()
	apDir := filepath.Join(tempDir, ".ap")
	if err := os.MkdirAll(apDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(apDir, "proto.yaml"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadProtoConfig(tempDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Protoc != DefaultProtocVersion {
		t.Errorf("expected protoc %q, got %q", DefaultProtocVersion, cfg.Protoc)
	}
	if len(cfg.Plugins) != 0 {
		t.Errorf("expected empty plugins map, got %v", cfg.Plugins)
	}
}

func TestLoadProtoConfig_Full(t *testing.T) {
	tempDir := t.TempDir()
	apDir := filepath.Join(tempDir, ".ap")
	if err := os.MkdirAll(apDir, 0755); err != nil {
		t.Fatal(err)
	}

	content := `
protoc: "36.2"
plugins:
  protoc-gen-go: 1.36.12
  protoc-gen-go-grpc: v1.6.2
`
	if err := os.WriteFile(filepath.Join(apDir, "proto.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadProtoConfig(tempDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Protoc != "36.2" {
		t.Errorf("expected protoc 36.2, got %q", cfg.Protoc)
	}
	if cfg.Plugins["protoc-gen-go"] != "v1.36.12" {
		t.Errorf("expected v-prefixed version v1.36.12, got %q", cfg.Plugins["protoc-gen-go"])
	}
	if cfg.Plugins["protoc-gen-go-grpc"] != "v1.6.2" {
		t.Errorf("expected v1.6.2, got %q", cfg.Plugins["protoc-gen-go-grpc"])
	}
}

func TestLoadProtoConfig_UnknownKey(t *testing.T) {
	tempDir := t.TempDir()
	apDir := filepath.Join(tempDir, ".ap")
	if err := os.MkdirAll(apDir, 0755); err != nil {
		t.Fatal(err)
	}

	content := `
protoc: "36.2"
generate:
  inputs:
    - proto/*.proto
`
	if err := os.WriteFile(filepath.Join(apDir, "proto.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadProtoConfig(tempDir)
	if err == nil {
		t.Fatal("expected error for unknown key generate, got nil")
	}
}
