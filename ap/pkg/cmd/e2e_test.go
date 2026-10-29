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

package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildE2eCommand(t *testing.T) {
	rootOpt := &RootOptions{}
	cmd := BuildE2eCommand(rootOpt)
	if cmd == nil {
		t.Fatal("BuildE2eCommand returned nil")
	}
	if cmd.Use != "e2e [path...]" {
		t.Errorf("Unexpected use string: %q", cmd.Use)
	}
}

func TestRunE2e_DryRun(t *testing.T) {
	tempDir := t.TempDir()
	apDir := filepath.Join(tempDir, ".ap")
	if err := os.MkdirAll(apDir, 0755); err != nil {
		t.Fatalf("failed to create .ap dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(apDir, "ap.yaml"), []byte("version: latest\n"), 0644); err != nil {
		t.Fatalf("failed to write ap.yaml: %v", err)
	}

	rootOpt := &RootOptions{
		RepoRoot: tempDir,
		DryRun:   true,
	}
	opt := E2eOptions{
		RootOptions: rootOpt,
	}

	if err := RunE2e(t.Context(), opt); err != nil {
		t.Fatalf("RunE2e with dry-run failed: %v", err)
	}
}
