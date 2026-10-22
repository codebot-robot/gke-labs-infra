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

package baseline_test

import (
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/any"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/baseline"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/closewriter"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/droppederrors"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/testcontext"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/testonlyimports"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/unused"
)

func TestParseBaseline(t *testing.T) {
	input := `# Some comment
pkg/foo/bar.go:MyFunc:(*os.File).Close

# Another comment with spaces
pkg/baz/qux.go  (*Server).Serve   fmt.Fprintln
`
	entries, err := baseline.ParseBaseline([]byte(input))
	if err != nil {
		t.Fatalf("ParseBaseline failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].File != "pkg/foo/bar.go" || entries[0].EnclosingFunc != "MyFunc" || entries[0].Key != "(*os.File).Close" {
		t.Errorf("entry 0 mismatch: %+v", entries[0])
	}
	if entries[1].File != "pkg/baz/qux.go" || entries[1].EnclosingFunc != "(*Server).Serve" || entries[1].Key != "fmt.Fprintln" {
		t.Errorf("entry 1 mismatch: %+v", entries[1])
	}
}

func TestParseBaseline_Category(t *testing.T) {
	input := `
pkg/foo/bar.go:MyFunc:error-only-checked-for-success:myFunc
pkg/baz/qux.go:MyFunc:myOtherFunc
`
	entries, err := baseline.ParseBaseline([]byte(input))
	if err != nil {
		t.Fatalf("ParseBaseline failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Category != "error-only-checked-for-success" || entries[0].Key != "myFunc" {
		t.Errorf("entry 0 mismatch: %+v", entries[0])
	}
	if entries[1].Category != "dropped-error" || entries[1].Key != "myOtherFunc" {
		t.Errorf("entry 1 mismatch: %+v", entries[1])
	}
}

func TestFormatBaseline(t *testing.T) {
	entries := []baseline.Entry{
		{File: "b/b.go", EnclosingFunc: "B", Key: "close"},
		{File: "a/a.go", EnclosingFunc: "Z", Key: "write"},
		{File: "a/a.go", EnclosingFunc: "A", Key: "write"},
	}
	formatted := baseline.FormatBaseline("testcheck", entries)
	parsed, err := baseline.ParseBaseline([]byte(formatted))
	if err != nil {
		t.Fatalf("ParseBaseline on formatted output failed: %v", err)
	}
	if len(parsed) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(parsed))
	}
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

	bEntries := []baseline.Entry{
		{File: "pkg/foo.go", EnclosingFunc: "Run", Key: "bar"},
		{File: "pkg/foo.go", EnclosingFunc: "Run", Key: "bar"},
	}

	checkedFiles := map[string]bool{
		"pkg/foo.go": true,
	}

	findings := []baseline.Finding{
		{
			Pos:           token.Position{Filename: fooPath, Line: 10, Column: 5},
			RelFile:       "pkg/foo.go",
			EnclosingFunc: "Run",
			Key:           "bar",
			Callee:        "bar",
		},
	}

	newFindings, staleEntries := baseline.MatchFindings(findings, bEntries, checkedFiles, tmpDir, "droppederrors")
	if len(newFindings) != 0 {
		t.Errorf("expected 0 new findings, got %d", len(newFindings))
	}
	if len(staleEntries) != 1 {
		t.Fatalf("expected 1 stale entry, got %d", len(staleEntries))
	}
	if staleEntries[0].File != "pkg/foo.go" || staleEntries[0].EnclosingFunc != "Run" {
		t.Errorf("unexpected stale entry: %+v", staleEntries[0])
	}

	findings = append(findings,
		baseline.Finding{
			Pos:           token.Position{Filename: fooPath, Line: 20, Column: 5},
			RelFile:       "pkg/foo.go",
			EnclosingFunc: "Run",
			Key:           "bar",
			Callee:        "bar",
		},
		baseline.Finding{
			Pos:           token.Position{Filename: fooPath, Line: 30, Column: 5},
			RelFile:       "pkg/foo.go",
			EnclosingFunc: "Run",
			Key:           "bar",
			Callee:        "bar",
		},
	)

	newFindings, staleEntries = baseline.MatchFindings(findings, bEntries, checkedFiles, tmpDir, "droppederrors")
	if len(newFindings) != 1 {
		t.Errorf("expected 1 new finding, got %d", len(newFindings))
	}
	if len(staleEntries) != 0 {
		t.Errorf("expected 0 stale entries, got %d", len(staleEntries))
	}
}

func TestMatchFindings_UncheckedModuleNotStale(t *testing.T) {
	tmpDir := t.TempDir()
	modBFile := filepath.Join(tmpDir, "modb", "b.go")
	if err := os.MkdirAll(filepath.Dir(modBFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modBFile, []byte("package b"), 0644); err != nil {
		t.Fatal(err)
	}

	bEntries := []baseline.Entry{
		{File: "moda/a.go", EnclosingFunc: "Run", Key: "bar"},
		{File: "modb/b.go", EnclosingFunc: "Run", Key: "bar"},
	}

	checkedFiles := map[string]bool{
		"moda/a.go": true,
	}

	findings := []baseline.Finding{
		{
			Pos:           token.Position{Filename: filepath.Join(tmpDir, "moda", "a.go"), Line: 10, Column: 5},
			RelFile:       "moda/a.go",
			EnclosingFunc: "Run",
			Key:           "bar",
			Callee:        "bar",
		},
	}

	newFindings, staleEntries := baseline.MatchFindings(findings, bEntries, checkedFiles, tmpDir, "droppederrors")
	if len(newFindings) != 0 {
		t.Errorf("expected 0 new findings, got %d", len(newFindings))
	}
	if len(staleEntries) != 0 {
		t.Errorf("expected 0 stale entries, got %d: %+v", len(staleEntries), staleEntries)
	}
}

func TestMatchFindings_DeletedFileIsStale(t *testing.T) {
	tmpDir := t.TempDir()
	bEntries := []baseline.Entry{
		{File: "deleted/file.go", EnclosingFunc: "Run", Key: "bar"},
	}

	checkedFiles := map[string]bool{}

	newFindings, staleEntries := baseline.MatchFindings(nil, bEntries, checkedFiles, tmpDir, "droppederrors")
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

func TestStableKey_CloseWriter(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module example.com/test\n\ngo 1.27\n"), 0644)

	src1 := `package main

import (
	"log"
	"os"
)

func ProcessFile(name string) {
	f, err := os.Create(name)
	if err != nil {
		return
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Printf("close error: %v", err)
		}
	}()
	_, _ = f.WriteString("hello")
}
`
	srcPath := filepath.Join(tmpDir, "main.go")
	os.WriteFile(srcPath, []byte(src1), 0644)

	res1, err := baseline.Run(t.Context(), closewriter.Analyzer, baseline.Options{
		Dir:       tmpDir,
		CheckName: "closewriter",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res1.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(res1.Findings))
	}

	key1 := res1.Findings[0].Key
	enc1 := res1.Findings[0].EnclosingFunc
	cat1 := res1.Findings[0].Category
	expectedKey := "*os.File.Close"
	if key1 != expectedKey {
		t.Errorf("expected key %s, got %s", expectedKey, key1)
	}
	if enc1 != "ProcessFile" {
		t.Errorf("expected enclosing func ProcessFile, got %s", enc1)
	}

	// Unrelated edits above and below
	src2 := `package main

// Unrelated comment at the top
import (
	"fmt"
	"log"
	"os"
)

func UnrelatedAbove() {
	fmt.Println("above")
}

func ProcessFile(name string) {
	// Unrelated comment inside
	f, err := os.Create(name)
	if err != nil {
		return
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Printf("close error: %v", err)
		}
	}()
	_, _ = f.WriteString("hello")
}

func UnrelatedBelow() {
	fmt.Println("below")
}
`
	os.WriteFile(srcPath, []byte(src2), 0644)

	res2, err := baseline.Run(t.Context(), closewriter.Analyzer, baseline.Options{
		Dir:       tmpDir,
		CheckName: "closewriter",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.Findings) != 1 {
		t.Fatalf("expected 1 finding after edits, got %d", len(res2.Findings))
	}

	key2 := res2.Findings[0].Key
	enc2 := res2.Findings[0].EnclosingFunc
	cat2 := res2.Findings[0].Category
	if key1 != key2 {
		t.Errorf("key changed after unrelated edits: %s vs %s", key1, key2)
	}
	if enc1 != enc2 {
		t.Errorf("enclosing func changed after unrelated edits: %s vs %s", enc1, enc2)
	}
	if cat1 != cat2 {
		t.Errorf("category changed after unrelated edits: %s vs %s", cat1, cat2)
	}

	// Verify matching with baseline entry from run 1
	bEntries := []baseline.Entry{{
		File:          "main.go",
		EnclosingFunc: enc1,
		Category:      cat1,
		Key:           key1,
	}}
	newFindings, stale := baseline.MatchFindings(res2.Findings, bEntries, res2.CheckedFiles, tmpDir, "closewriter")
	if len(newFindings) != 0 {
		t.Errorf("expected 0 new findings, got %d", len(newFindings))
	}
	if len(stale) != 0 {
		t.Errorf("expected 0 stale entries, got %d", len(stale))
	}
}

func TestStableKey_DroppedErrors(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module example.com/test\n\ngo 1.27\n"), 0644)

	src1 := `package main

import (
	"errors"
)

func fail() error { return errors.New("err") }

func RunApp() {
	_ = fail()
}
`
	srcPath := filepath.Join(tmpDir, "main.go")
	os.WriteFile(srcPath, []byte(src1), 0644)

	res1, err := baseline.Run(t.Context(), droppederrors.Analyzer, baseline.Options{
		Dir:       tmpDir,
		CheckName: "droppederrors",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res1.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(res1.Findings))
	}

	key1 := res1.Findings[0].Key
	enc1 := res1.Findings[0].EnclosingFunc
	cat1 := res1.Findings[0].Category
	if enc1 != "RunApp" {
		t.Errorf("expected enclosing func RunApp, got %s", enc1)
	}
	if key1 != "example.com/test.fail" {
		t.Errorf("expected key example.com/test.fail, got %s", key1)
	}

	// Unrelated edits
	src2 := `package main

// Unrelated top comment
import (
	"errors"
	"fmt"
)

func HelperAbove() {
	fmt.Println("helper")
}

func fail() error { return errors.New("err") }

func RunApp() {
	// Inside comment
	_ = fail()
}

func HelperBelow() {}
`
	os.WriteFile(srcPath, []byte(src2), 0644)

	res2, err := baseline.Run(t.Context(), droppederrors.Analyzer, baseline.Options{
		Dir:       tmpDir,
		CheckName: "droppederrors",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.Findings) != 1 {
		t.Fatalf("expected 1 finding after edits, got %d", len(res2.Findings))
	}

	key2 := res2.Findings[0].Key
	enc2 := res2.Findings[0].EnclosingFunc
	cat2 := res2.Findings[0].Category
	if key1 != key2 || enc1 != enc2 || cat1 != cat2 {
		t.Errorf("mismatch: (%s, %s, %s) vs (%s, %s, %s)", enc1, cat1, key1, enc2, cat2, key2)
	}

	bEntries := []baseline.Entry{{
		File:          "main.go",
		EnclosingFunc: enc1,
		Category:      cat1,
		Key:           key1,
	}}
	newFindings, stale := baseline.MatchFindings(res2.Findings, bEntries, res2.CheckedFiles, tmpDir, "droppederrors")
	if len(newFindings) != 0 || len(stale) != 0 {
		t.Errorf("expected match: new=%d, stale=%d", len(newFindings), len(stale))
	}
}

func TestStableKey_Unused(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module example.com/test\n\ngo 1.27\n"), 0644)

	src1 := `package main

func unusedHelper() {}

func main() {}
`
	srcPath := filepath.Join(tmpDir, "main.go")
	os.WriteFile(srcPath, []byte(src1), 0644)

	res1, err := baseline.Run(t.Context(), unused.Analyzer, baseline.Options{
		Dir:       tmpDir,
		CheckName: "unused",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res1.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(res1.Findings))
	}

	key1 := res1.Findings[0].Key
	enc1 := res1.Findings[0].EnclosingFunc
	cat1 := res1.Findings[0].Category
	if key1 != "unusedHelper" {
		t.Errorf("expected key unusedHelper, got %s", key1)
	}
	if enc1 != "unusedHelper" {
		t.Errorf("expected enclosing func unusedHelper, got %s", enc1)
	}

	// Unrelated edits
	src2 := `package main

// Comment
import "fmt"

func extraFunc() {
	fmt.Println("extra")
}

func unusedHelper() {
	// Inside comment
}

func main() {
	extraFunc()
}
`
	os.WriteFile(srcPath, []byte(src2), 0644)

	res2, err := baseline.Run(t.Context(), unused.Analyzer, baseline.Options{
		Dir:       tmpDir,
		CheckName: "unused",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.Findings) != 1 {
		t.Fatalf("expected 1 finding after edits, got %d", len(res2.Findings))
	}

	key2 := res2.Findings[0].Key
	enc2 := res2.Findings[0].EnclosingFunc
	cat2 := res2.Findings[0].Category
	if key1 != key2 || enc1 != enc2 || cat1 != cat2 {
		t.Errorf("mismatch: (%s, %s, %s) vs (%s, %s, %s)", enc1, cat1, key1, enc2, cat2, key2)
	}

	bEntries := []baseline.Entry{{
		File:          "main.go",
		EnclosingFunc: enc1,
		Category:      cat1,
		Key:           key1,
	}}
	newFindings, stale := baseline.MatchFindings(res2.Findings, bEntries, res2.CheckedFiles, tmpDir, "unused")
	if len(newFindings) != 0 || len(stale) != 0 {
		t.Errorf("expected match: new=%d, stale=%d", len(newFindings), len(stale))
	}
}

func TestStableKey_TestOnlyImports(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module example.com/test\n\ngo 1.27\n\nrequire github.com/google/go-cmp v0.0.0\nreplace github.com/google/go-cmp => ./mockcmp\n"), 0644)

	cmpDir := filepath.Join(tmpDir, "mockcmp", "cmp")
	if err := os.MkdirAll(cmpDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(tmpDir, "mockcmp", "go.mod"), []byte("module github.com/google/go-cmp\n\ngo 1.27\n"), 0644)
	os.WriteFile(filepath.Join(cmpDir, "cmp.go"), []byte("package cmp\n\nfunc Diff(x, y any) string { return \"\" }\n"), 0644)

	src1 := `package main

import (
	_ "github.com/google/go-cmp/cmp"
)

func main() {}
`
	srcPath := filepath.Join(tmpDir, "main.go")
	os.WriteFile(srcPath, []byte(src1), 0644)

	res1, err := baseline.Run(t.Context(), testonlyimports.Analyzer, baseline.Options{
		Dir:       tmpDir,
		CheckName: "testonlyimports",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res1.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(res1.Findings))
	}

	key1 := res1.Findings[0].Key
	enc1 := res1.Findings[0].EnclosingFunc
	cat1 := res1.Findings[0].Category
	expectedKey := "github.com/google/go-cmp/cmp"
	if key1 != expectedKey {
		t.Errorf("expected key %s, got %s", expectedKey, key1)
	}

	// Unrelated edits
	src2 := `package main

// Unrelated comments
import (
	"fmt"

	_ "github.com/google/go-cmp/cmp"
)

func helper() {
	fmt.Println("helper")
}

func main() {
	helper()
}
`
	os.WriteFile(srcPath, []byte(src2), 0644)

	res2, err := baseline.Run(t.Context(), testonlyimports.Analyzer, baseline.Options{
		Dir:       tmpDir,
		CheckName: "testonlyimports",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.Findings) != 1 {
		t.Fatalf("expected 1 finding after edits, got %d", len(res2.Findings))
	}

	key2 := res2.Findings[0].Key
	enc2 := res2.Findings[0].EnclosingFunc
	cat2 := res2.Findings[0].Category
	if key1 != key2 || enc1 != enc2 || cat1 != cat2 {
		t.Errorf("mismatch: (%s, %s, %s) vs (%s, %s, %s)", enc1, cat1, key1, enc2, cat2, key2)
	}

	bEntries := []baseline.Entry{{
		File:          "main.go",
		EnclosingFunc: enc1,
		Category:      cat1,
		Key:           key1,
	}}
	newFindings, stale := baseline.MatchFindings(res2.Findings, bEntries, res2.CheckedFiles, tmpDir, "testonlyimports")
	if len(newFindings) != 0 || len(stale) != 0 {
		t.Errorf("expected match: new=%d, stale=%d", len(newFindings), len(stale))
	}
}

func TestStableKey_TestContext(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module example.com/test\n\ngo 1.27\n"), 0644)

	src1 := `package main

import (
	"context"
	"testing"
)

func TestSample(t *testing.T) {
	_ = context.Background()
}
`
	srcPath := filepath.Join(tmpDir, "main_test.go")
	os.WriteFile(srcPath, []byte(src1), 0644)

	res1, err := baseline.Run(t.Context(), testcontext.Analyzer, baseline.Options{
		Dir:       tmpDir,
		CheckName: "testcontext",
		SkipTests: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res1.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(res1.Findings))
	}

	key1 := res1.Findings[0].Key
	enc1 := res1.Findings[0].EnclosingFunc
	cat1 := res1.Findings[0].Category
	expectedKey := "context.Background"
	if key1 != expectedKey {
		t.Errorf("expected key %s, got %s", expectedKey, key1)
	}
	if enc1 != "TestSample" {
		t.Errorf("expected enclosing func TestSample, got %s", enc1)
	}

	// Unrelated edits
	src2 := `package main

// Unrelated comments
import (
	"context"
	"fmt"
	"testing"
)

func UnrelatedTestHelper() {
	fmt.Println("helper")
}

func TestSample(t *testing.T) {
	// Inside comment
	_ = context.Background()
}

func AnotherHelper() {}
`
	os.WriteFile(srcPath, []byte(src2), 0644)

	res2, err := baseline.Run(t.Context(), testcontext.Analyzer, baseline.Options{
		Dir:       tmpDir,
		CheckName: "testcontext",
		SkipTests: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.Findings) != 1 {
		t.Fatalf("expected 1 finding after edits, got %d", len(res2.Findings))
	}

	key2 := res2.Findings[0].Key
	enc2 := res2.Findings[0].EnclosingFunc
	cat2 := res2.Findings[0].Category
	if key1 != key2 || enc1 != enc2 || cat1 != cat2 {
		t.Errorf("mismatch: (%s, %s, %s) vs (%s, %s, %s)", enc1, cat1, key1, enc2, cat2, key2)
	}

	bEntries := []baseline.Entry{{
		File:          "main_test.go",
		EnclosingFunc: enc1,
		Category:      cat1,
		Key:           key1,
	}}
	newFindings, stale := baseline.MatchFindings(res2.Findings, bEntries, res2.CheckedFiles, tmpDir, "testcontext")
	if len(newFindings) != 0 || len(stale) != 0 {
		t.Errorf("expected match: new=%d, stale=%d", len(newFindings), len(stale))
	}
}

func TestStableKey_Any(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module example.com/test\n\ngo 1.27\n"), 0644)

	src1 := `package main

func Process(x interface{}) {}

func main() {}
`
	srcPath := filepath.Join(tmpDir, "main.go")
	os.WriteFile(srcPath, []byte(src1), 0644)

	res1, err := baseline.Run(t.Context(), any.Analyzer, baseline.Options{
		Dir:       tmpDir,
		CheckName: "replaceEmptyInterfaceWithAny",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res1.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(res1.Findings))
	}

	key1 := res1.Findings[0].Key
	enc1 := res1.Findings[0].EnclosingFunc
	cat1 := res1.Findings[0].Category
	expectedKey := "interface{}"
	if key1 != expectedKey {
		t.Errorf("expected key %s, got %s", expectedKey, key1)
	}

	// Unrelated edits
	src2 := `package main

// Comments
import "fmt"

func other() {
	fmt.Println("other")
}

func Process(x interface{}) {
	// Inside comment
}

func main() {
	other()
}
`
	os.WriteFile(srcPath, []byte(src2), 0644)

	res2, err := baseline.Run(t.Context(), any.Analyzer, baseline.Options{
		Dir:       tmpDir,
		CheckName: "replaceEmptyInterfaceWithAny",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.Findings) != 1 {
		t.Fatalf("expected 1 finding after edits, got %d", len(res2.Findings))
	}

	key2 := res2.Findings[0].Key
	enc2 := res2.Findings[0].EnclosingFunc
	cat2 := res2.Findings[0].Category
	if key1 != key2 || enc1 != enc2 || cat1 != cat2 {
		t.Errorf("mismatch: (%s, %s, %s) vs (%s, %s, %s)", enc1, cat1, key1, enc2, cat2, key2)
	}

	bEntries := []baseline.Entry{{
		File:          "main.go",
		EnclosingFunc: enc1,
		Category:      cat1,
		Key:           key1,
	}}
	newFindings, stale := baseline.MatchFindings(res2.Findings, bEntries, res2.CheckedFiles, tmpDir, "replaceEmptyInterfaceWithAny")
	if len(newFindings) != 0 || len(stale) != 0 {
		t.Errorf("expected match: new=%d, stale=%d", len(newFindings), len(stale))
	}
}
