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
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToolProtoc_DryRun(t *testing.T) {
	tempCache := t.TempDir()
	t.Setenv("AP_CACHE_DIR", tempCache)

	tempDir := t.TempDir()
	apDir := filepath.Join(tempDir, ".ap")
	if err := os.MkdirAll(apDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(apDir, "proto.yaml"), []byte("protoc: \"36.2\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Pre-populate cache so it doesn't need to download in dry run test
	version := "36.2"
	binDir := filepath.Join(tempCache, "tools", "protoc", version, "bin")
	includeDir := filepath.Join(tempCache, "tools", "protoc", version, "include")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(includeDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "protoc"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("AP_ROOT", tempDir)

	rootCmd := BuildRootCommand()
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)

	// Use --dry-run
	rootCmd.SetArgs([]string{"tool", "protoc", "--dry-run", "--", "--version"})

	err := rootCmd.ExecuteContext(t.Context())
	output := buf.String()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output, "[dry-run]") {
		t.Errorf("expected output to contain [dry-run], got: %s", output)
	}
	if !strings.Contains(output, "--version") {
		t.Errorf("expected output to contain --version, got: %s", output)
	}
	if !strings.Contains(output, "36.2") {
		t.Errorf("expected output to reference 36.2, got: %s", output)
	}
}

func TestToolProtoc_RealExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real execution in short mode")
	}

	tempCache := t.TempDir()
	t.Setenv("AP_CACHE_DIR", tempCache)

	rootCmd := BuildRootCommand()
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"tool", "protoc", "--", "--version"})

	err := rootCmd.ExecuteContext(t.Context())
	output := buf.String()

	if err != nil {
		t.Fatalf("unexpected error running ap tool protoc -- --version: %v", err)
	}
	if !strings.Contains(output, "libprotoc") && !strings.Contains(output, "protoc") {
		t.Errorf("expected protoc version output, got: %s", output)
	}
}
