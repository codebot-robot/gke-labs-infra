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
	"strings"
	"testing"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/baseline"
)

func TestBuildCloseWriterCommand(t *testing.T) {
	cmd := BuildCloseWriterCommand()
	if cmd == nil {
		t.Fatalf("expected command to be built")
	}
	flags := []string{"mode", "baseline", "write-baseline", "type", "skip-tests", "skip-generated", "goos"}
	for _, f := range flags {
		if cmd.Flags().Lookup(f) == nil {
			t.Errorf("missing flag --%s", f)
		}
	}
}

func TestRunCloseWriter_EndToEnd(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Create go.mod
	goModContent := "module example.com/cmdtest\n\ngo 1.27\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goModContent), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Create .ap directory
	apDir := filepath.Join(tmpDir, ".ap")
	if err := os.MkdirAll(apDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(apDir, "ap.yaml"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. Create main.go with one closewriter violation
	mainGoContent := `package main

import (
	"log"
	"os"
)

func ProcessFile(path string) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Printf("close error: %v", err)
		}
	}()
	_, _ = f.WriteString("content")
}

func main() {
	ProcessFile("/tmp/test")
}
`
	mainGoPath := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(mainGoPath, []byte(mainGoContent), 0644); err != nil {
		t.Fatal(err)
	}

	t.Chdir(tmpDir)

	baselineFile := filepath.Join(apDir, "closewriter-baseline.txt")

	// 4. Run closewriter with WriteBaseline=true
	optWrite := CloseWriterOptions{
		RootOptions: &RootOptions{
			RepoRoot: tmpDir,
			APRoot:   tmpDir,
		},
		Baseline:      baselineFile,
		WriteBaseline: true,
		SkipTests:     true,
	}
	if err := RunCloseWriter(t.Context(), nil, optWrite, nil); err != nil {
		t.Fatalf("RunCloseWriter with WriteBaseline failed: %v", err)
	}

	// 5. Verify baseline file was created and contains the entry
	data, err := os.ReadFile(baselineFile)
	if err != nil {
		t.Fatalf("expected baseline file to exist: %v", err)
	}
	entries, err := baseline.ParseBaseline(data)
	if err != nil {
		t.Fatalf("ParseBaseline failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry in baseline, got %d", len(entries))
	}
	if entries[0].File != "main.go" || entries[0].EnclosingFunc != "ProcessFile" || entries[0].Key != "*os.File.Close" {
		t.Errorf("unexpected entry: %+v", entries[0])
	}

	// 6. Run closewriter with baseline and mode: error -> should PASS
	optCheck := CloseWriterOptions{
		RootOptions: &RootOptions{
			RepoRoot: tmpDir,
			APRoot:   tmpDir,
		},
		Baseline:  baselineFile,
		Mode:      "error",
		SkipTests: true,
	}
	if err := RunCloseWriter(t.Context(), nil, optCheck, nil); err != nil {
		t.Fatalf("expected closewriter with baseline to pass: %v", err)
	}

	// 7. Add a second closewriter violation to main.go
	mainGoWithNewViolation := `package main

import (
	"log"
	"os"
)

func ProcessFile(path string) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Printf("close error: %v", err)
		}
	}()
	_, _ = f.WriteString("content")
}

func AnotherProcess(path string) {
	f2, err := os.Create(path)
	if err != nil {
		return
	}
	defer func() {
		if err := f2.Close(); err != nil {
			log.Printf("close error: %v", err)
		}
	}()
	_, _ = f2.WriteString("more")
}

func main() {
	ProcessFile("/tmp/test")
	AnotherProcess("/tmp/test2")
}
`
	if err := os.WriteFile(mainGoPath, []byte(mainGoWithNewViolation), 0644); err != nil {
		t.Fatal(err)
	}

	// 8. Run closewriter with baseline and mode: error -> should FAIL on the new violation
	err = RunCloseWriter(t.Context(), nil, optCheck, nil)
	if err == nil {
		t.Fatalf("expected closewriter to fail on new violation, but it passed")
	}
	if !strings.Contains(err.Error(), "1 new unhandled close errors found") {
		t.Errorf("unexpected error message: %v", err)
	}

	// 9. Run with mode: warn -> should NOT fail
	optWarn := CloseWriterOptions{
		RootOptions: &RootOptions{
			RepoRoot: tmpDir,
			APRoot:   tmpDir,
		},
		Baseline:  baselineFile,
		Mode:      "warn",
		SkipTests: true,
	}
	if err := RunCloseWriter(t.Context(), nil, optWarn, nil); err != nil {
		t.Fatalf("expected mode warn to not fail: %v", err)
	}

	// 10. Fix the first violation so it is now fixed, but leave baseline file intact
	mainGoFixedFirst := `package main

import (
	"os"
)

func ProcessFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if _, err := f.WriteString("content"); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func main() {
	_ = ProcessFile("/tmp/test")
}
`
	if err := os.WriteFile(mainGoPath, []byte(mainGoFixedFirst), 0644); err != nil {
		t.Fatal(err)
	}

	// Run check: first entry in baseline is now fixed -> stale entry reported, but 0 new violations -> PASS!
	if err := RunCloseWriter(t.Context(), nil, optCheck, nil); err != nil {
		t.Fatalf("expected closewriter to pass when only stale baseline exists: %v", err)
	}
}
