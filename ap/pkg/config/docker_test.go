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

func TestLoadDockerConfig_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	cfg, err := LoadDockerConfig(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Fatalf("expected nil config for missing file, got %+v", cfg)
	}
}

func TestLoadDockerConfig_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	apDir := filepath.Join(tmpDir, ".ap")
	if err := os.MkdirAll(apDir, 0755); err != nil {
		t.Fatal(err)
	}

	content := `
docker: "28.0.1"
buildx: "v0.21.1"
`
	if err := os.WriteFile(filepath.Join(apDir, "docker.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadDockerConfig(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Docker != "28.0.1" {
		t.Errorf("expected docker 28.0.1, got %q", cfg.Docker)
	}
	if cfg.Buildx != "v0.21.1" {
		t.Errorf("expected buildx v0.21.1, got %q", cfg.Buildx)
	}
}

func TestLoadDockerConfig_Alias(t *testing.T) {
	tmpDir := t.TempDir()
	apDir := filepath.Join(tmpDir, ".ap")
	if err := os.MkdirAll(apDir, 0755); err != nil {
		t.Fatal(err)
	}

	content := `
docker: "29.8.2"
docker-buildx: "v0.38.1"
`
	if err := os.WriteFile(filepath.Join(apDir, "docker.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadDockerConfig(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Docker != "29.8.2" {
		t.Errorf("expected docker 29.8.2, got %q", cfg.Docker)
	}
	if cfg.Buildx != "v0.38.1" {
		t.Errorf("expected buildx v0.38.1 via alias, got %q", cfg.Buildx)
	}
}

func TestLoadDockerConfig_Defaults(t *testing.T) {
	tmpDir := t.TempDir()
	apDir := filepath.Join(tmpDir, ".ap")
	if err := os.MkdirAll(apDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(apDir, "docker.yaml"), []byte("{}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadDockerConfig(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Docker != DefaultDockerVersion {
		t.Errorf("expected default docker %q, got %q", DefaultDockerVersion, cfg.Docker)
	}
	if cfg.Buildx != DefaultBuildxVersion {
		t.Errorf("expected default buildx %q, got %q", DefaultBuildxVersion, cfg.Buildx)
	}
}
