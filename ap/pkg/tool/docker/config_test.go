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

package docker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	cfg, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Fatalf("expected nil config for missing file, got %+v", cfg)
	}
}

func TestLoadConfig_Valid(t *testing.T) {
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

	cfg, err := LoadConfig(tmpDir)
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

func TestLoadConfig_DisallowAlias(t *testing.T) {
	tmpDir := t.TempDir()
	apDir := filepath.Join(tmpDir, ".ap")
	if err := os.MkdirAll(apDir, 0755); err != nil {
		t.Fatal(err)
	}

	content := `
docker: "29.9.2"
docker-buildx: "0.38.1"
`
	if err := os.WriteFile(filepath.Join(apDir, "docker.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Strict unmarshal must reject the unapproved key docker-buildx
	_, err := LoadConfig(tmpDir)
	if err == nil {
		t.Fatal("expected error parsing docker.yaml with unknown field docker-buildx, got nil")
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	tmpDir := t.TempDir()
	apDir := filepath.Join(tmpDir, ".ap")
	if err := os.MkdirAll(apDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(apDir, "docker.yaml"), []byte("{}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	expectedDocker := LatestDocker().Version
	expectedBuildx := LatestBuildx().Version
	if cfg.Docker != expectedDocker {
		t.Errorf("expected default docker %q, got %q", expectedDocker, cfg.Docker)
	}
	if cfg.Buildx != expectedBuildx {
		t.Errorf("expected default buildx %q, got %q", expectedBuildx, cfg.Buildx)
	}
}
