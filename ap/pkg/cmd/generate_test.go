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

func TestBuildGenerateCommand(t *testing.T) {
	rootOpt := &RootOptions{}
	cmd := BuildGenerateCommand(rootOpt)

	if cmd.Use != "generate [path...]" {
		t.Errorf("unexpected Use: %q", cmd.Use)
	}

	verifyFlag := cmd.Flags().Lookup("verify")
	if verifyFlag == nil {
		t.Fatalf("expected --verify flag to exist")
	}
	if verifyFlag.DefValue != "false" {
		t.Errorf("expected --verify default to be false, got %q", verifyFlag.DefValue)
	}
}

func TestBuildVerifyGenerateCommand(t *testing.T) {
	rootOpt := &RootOptions{}
	cmd := BuildVerifyGenerateCommand(rootOpt)

	if cmd.Use != "verify-generate [path...]" {
		t.Errorf("unexpected Use: %q", cmd.Use)
	}
}

func TestVerifyGenerateDryRun(t *testing.T) {
	tempDir := t.TempDir()
	apDir := filepath.Join(tempDir, ".ap")
	if err := os.MkdirAll(apDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(apDir, "ap.yaml"), []byte("version: latest\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(apDir, "headers.yaml"), []byte("license: apache-2.0\ncopyrightHolder: Google LLC\n"), 0644); err != nil {
		t.Fatal(err)
	}

	rootOpt := &RootOptions{
		RepoRoot: tempDir,
		APRoot:   tempDir,
		DryRun:   true,
	}

	opt := GenerateOptions{
		RootOptions: rootOpt,
		Verify:      true,
	}

	if err := opt.Resolve(nil); err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if err := RunGenerate(t.Context(), opt); err != nil {
		t.Fatalf("RunGenerate in verify dry-run mode failed: %v", err)
	}
}
