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

package scaffold

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInit_Default(t *testing.T) {
	dir := t.TempDir()

	err := Init(InitOptions{
		Dir:             dir,
		CopyrightHolder: "My Org",
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Verify ap.yaml
	b, err := os.ReadFile(filepath.Join(dir, ".ap", "ap.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "version: latest\n" {
		t.Errorf("unexpected ap.yaml content: %q", string(b))
	}

	// Verify go.yaml
	b, err = os.ReadFile(filepath.Join(dir, ".ap", "go.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	expectedGo := "gofmt:\n  enabled: true\ngovet:\n  enabled: true\n"
	if string(b) != expectedGo {
		t.Errorf("unexpected go.yaml content: %q", string(b))
	}

	// Verify headers.yaml
	b, err = os.ReadFile(filepath.Join(dir, ".ap", "headers.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	expectedHeaders := "license: apache-2.0\ncopyrightHolder: My Org\n"
	if string(b) != expectedHeaders {
		t.Errorf("unexpected headers.yaml content: %q", string(b))
	}
}

func TestInit_LicenseNone(t *testing.T) {
	dir := t.TempDir()

	err := Init(InitOptions{
		Dir:     dir,
		License: "none",
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(dir, ".ap", "headers.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "license: none\n" {
		t.Errorf("unexpected headers.yaml: %q", string(b))
	}
}

func TestInit_AlreadyExists(t *testing.T) {
	dir := t.TempDir()

	if err := Init(InitOptions{Dir: dir}); err != nil {
		t.Fatal(err)
	}

	// Second run without Force should error
	err := Init(InitOptions{Dir: dir})
	if err == nil {
		t.Fatal("expected error on re-init without force")
	}

	// Run with Force should succeed
	err = Init(InitOptions{Dir: dir, Force: true, CopyrightHolder: "New Org"})
	if err != nil {
		t.Fatalf("expected success with force: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, ".ap", "headers.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "license: apache-2.0\ncopyrightHolder: New Org\n" {
		t.Errorf("unexpected headers.yaml after force: %q", string(b))
	}
}

func TestInit_InvalidLicense(t *testing.T) {
	dir := t.TempDir()

	err := Init(InitOptions{Dir: dir, License: "mit"})
	if err == nil {
		t.Fatal("expected error for unsupported license")
	}
}
