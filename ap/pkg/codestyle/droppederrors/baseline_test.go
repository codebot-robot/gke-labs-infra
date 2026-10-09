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

package droppederrors

import (
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

func TestParseBaseline(t *testing.T) {
	input := `# Some comment
pkg/foo/bar.go:MyFunc:(*os.File).Close

# Another comment with spaces
pkg/baz/qux.go  (*Server).Serve   fmt.Fprintln
`
	entries, err := ParseBaseline([]byte(input))
	if err != nil {
		t.Fatalf("ParseBaseline failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].File != "pkg/foo/bar.go" || entries[0].EnclosingFunc != "MyFunc" || entries[0].Callee != "(*os.File).Close" {
		t.Errorf("entry 0 mismatch: %+v", entries[0])
	}
	if entries[1].File != "pkg/baz/qux.go" || entries[1].EnclosingFunc != "(*Server).Serve" || entries[1].Callee != "fmt.Fprintln" {
		t.Errorf("entry 1 mismatch: %+v", entries[1])
	}
}

func TestParseBaseline_Category(t *testing.T) {
	input := `
pkg/foo/bar.go:MyFunc:error-only-checked-for-success:myFunc
pkg/baz/qux.go:MyFunc:myOtherFunc
`
	entries, err := ParseBaseline([]byte(input))
	if err != nil {
		t.Fatalf("ParseBaseline failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Category != CategoryErrorOnlyCheckedForSuccess || entries[0].Callee != "myFunc" {
		t.Errorf("entry 0 mismatch: %+v", entries[0])
	}
	if entries[1].Category != CategoryDroppedError || entries[1].Callee != "myOtherFunc" {
		t.Errorf("entry 1 mismatch: %+v", entries[1])
	}
}

func TestFormatBaseline(t *testing.T) {
	entries := []BaselineEntry{
		{File: "b/b.go", EnclosingFunc: "B", Callee: "close"},
		{File: "a/a.go", EnclosingFunc: "Z", Callee: "write"},
		{File: "a/a.go", EnclosingFunc: "A", Callee: "write"},
	}
	formatted := FormatBaseline(entries)
	parsed, err := ParseBaseline([]byte(formatted))
	if err != nil {
		t.Fatalf("ParseBaseline on formatted output failed: %v", err)
	}
	if len(parsed) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(parsed))
	}
	// Verify sorted order: a/a.go:A:write, a/a.go:Z:write, b/b.go:B:close
	if parsed[0].File != "a/a.go" || parsed[0].EnclosingFunc != "A" {
		t.Errorf("expected a/a.go:A first, got %+v", parsed[0])
	}
	if parsed[1].File != "a/a.go" || parsed[1].EnclosingFunc != "Z" {
		t.Errorf("expected a/a.go:Z second, got %+v", parsed[1])
	}
	if parsed[2].File != "b/b.go" || parsed[2].EnclosingFunc != "B" {
		t.Errorf("expected b/b.go:B third, got %+v", parsed[2])
	}
}

func TestMatchFindings_Multiset(t *testing.T) {
	tmpDir := t.TempDir()
	fooPath := filepath.Join(tmpDir, "pkg", "foo.go")
	if err := os.MkdirAll(filepath.Dir(fooPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fooPath, []byte("package foo"), 0644); err != nil {
		t.Fatal(err)
	}

	baseline := []BaselineEntry{
		{File: "pkg/foo.go", EnclosingFunc: "Run", Callee: "bar"},
		{File: "pkg/foo.go", EnclosingFunc: "Run", Callee: "bar"},
	}

	checkedFiles := map[string]bool{
		"pkg/foo.go": true,
	}

	// 1 finding: 1 matches baseline, 1 baseline entry becomes stale
	findings := []Finding{
		{
			Pos:           token.Position{Filename: fooPath, Line: 10, Column: 5},
			RelFile:       "pkg/foo.go",
			EnclosingFunc: "Run",
			Callee:        "bar",
		},
	}

	newFindings, staleEntries := MatchFindings(findings, baseline, checkedFiles, tmpDir)
	if len(newFindings) != 0 {
		t.Errorf("expected 0 new findings, got %d", len(newFindings))
	}
	if len(staleEntries) != 1 {
		t.Fatalf("expected 1 stale entry, got %d", len(staleEntries))
	}
	if staleEntries[0].File != "pkg/foo.go" || staleEntries[0].EnclosingFunc != "Run" {
		t.Errorf("unexpected stale entry: %+v", staleEntries[0])
	}

	// 3 findings: 2 match baseline, 1 is new finding, 0 stale entries
	findings = append(findings,
		Finding{
			Pos:           token.Position{Filename: fooPath, Line: 20, Column: 5},
			RelFile:       "pkg/foo.go",
			EnclosingFunc: "Run",
			Callee:        "bar",
		},
		Finding{
			Pos:           token.Position{Filename: fooPath, Line: 30, Column: 5},
			RelFile:       "pkg/foo.go",
			EnclosingFunc: "Run",
			Callee:        "bar",
		},
	)

	newFindings, staleEntries = MatchFindings(findings, baseline, checkedFiles, tmpDir)
	if len(newFindings) != 1 {
		t.Errorf("expected 1 new finding, got %d", len(newFindings))
	}
	if len(staleEntries) != 0 {
		t.Errorf("expected 0 stale entries, got %d", len(staleEntries))
	}
}

func TestMatchFindings_UncheckedModuleNotStale(t *testing.T) {
	tmpDir := t.TempDir()
	// File in module B exists
	modBFile := filepath.Join(tmpDir, "modb", "b.go")
	if err := os.MkdirAll(filepath.Dir(modBFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modBFile, []byte("package b"), 0644); err != nil {
		t.Fatal(err)
	}

	baseline := []BaselineEntry{
		{File: "moda/a.go", EnclosingFunc: "Run", Callee: "bar"},
		{File: "modb/b.go", EnclosingFunc: "Run", Callee: "bar"}, // Not checked in this run
	}

	// We only checked moda/a.go
	checkedFiles := map[string]bool{
		"moda/a.go": true,
	}

	findings := []Finding{
		{
			Pos:           token.Position{Filename: filepath.Join(tmpDir, "moda", "a.go"), Line: 10, Column: 5},
			RelFile:       "moda/a.go",
			EnclosingFunc: "Run",
			Callee:        "bar",
		},
	}

	newFindings, staleEntries := MatchFindings(findings, baseline, checkedFiles, tmpDir)
	if len(newFindings) != 0 {
		t.Errorf("expected 0 new findings, got %d", len(newFindings))
	}
	// modb/b.go was not checked, but exists on disk -> should NOT be considered stale!
	if len(staleEntries) != 0 {
		t.Errorf("expected 0 stale entries, got %d: %+v", len(staleEntries), staleEntries)
	}
}

func TestMatchFindings_DeletedFileIsStale(t *testing.T) {
	tmpDir := t.TempDir()
	baseline := []BaselineEntry{
		{File: "deleted/file.go", EnclosingFunc: "Run", Callee: "bar"},
	}

	// deleted/file.go does not exist on disk
	checkedFiles := map[string]bool{}

	newFindings, staleEntries := MatchFindings(nil, baseline, checkedFiles, tmpDir)
	if len(newFindings) != 0 {
		t.Errorf("expected 0 new findings, got %d", len(newFindings))
	}
	if len(staleEntries) != 1 {
		t.Fatalf("expected 1 stale entry for deleted file, got %d", len(staleEntries))
	}
	if staleEntries[0].File != "deleted/file.go" {
		t.Errorf("unexpected stale entry: %+v", staleEntries[0])
	}
}
