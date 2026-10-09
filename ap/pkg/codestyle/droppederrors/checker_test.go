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
	"os"
	"path/filepath"
	"testing"
)

func createTestModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	goMod := "module example.com/testmod\n\ngo 1.27\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}
	for relPath, content := range files {
		fullPath := filepath.Join(dir, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestChecker_DetectDroppedErrors(t *testing.T) {
	code := `package main

import "errors"

func mayFail() error {
	return errors.New("fail")
}

func multi() (int, error) {
	return 1, nil
}

type Worker struct{}

func (w *Worker) DoWork() error {
	return nil
}

func main() {
	_ = mayFail()       // dropped: assigned to blank
	mayFail()           // dropped: unassigned
	val, _ := multi()   // dropped: error assigned to blank
	_ = val
	w := &Worker{}
	_ = w.DoWork()      // dropped: method error assigned to blank

	// Handled error
	if err := mayFail(); err != nil {
		// handled
	}
}
`
	dir := createTestModule(t, map[string]string{
		"main.go": code,
	})

	opts := Options{
		Dir:       dir,
		APRoot:    dir,
		SkipTests: true,
	}

	res, err := Run(t.Context(), opts)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(res.Findings) != 4 {
		t.Fatalf("expected 4 findings, got %d: %+v", len(res.Findings), res.Findings)
	}

	// Verify enclosing function is "main" for all of them
	for _, f := range res.Findings {
		if f.EnclosingFunc != "main" {
			t.Errorf("expected enclosing func main, got %q", f.EnclosingFunc)
		}
	}
}

func TestChecker_SkipTests(t *testing.T) {
	mainCode := `package main

import "errors"

func fail() error { return errors.New("err") }
func main() {
	_ = fail()
}
`
	testCode := `package main

import "testing"

func TestMain(t *testing.T) {
	_ = fail()
}
`
	dir := createTestModule(t, map[string]string{
		"main.go":      mainCode,
		"main_test.go": testCode,
	})

	// With SkipTests: true
	resSkip, err := Run(t.Context(), Options{
		Dir:       dir,
		APRoot:    dir,
		SkipTests: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resSkip.Findings) != 1 {
		t.Fatalf("expected 1 finding with SkipTests:true, got %d", len(resSkip.Findings))
	}

	// With SkipTests: false
	resNoSkip, err := Run(t.Context(), Options{
		Dir:       dir,
		APRoot:    dir,
		SkipTests: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resNoSkip.Findings) != 2 {
		t.Fatalf("expected 2 findings with SkipTests:false, got %d", len(resNoSkip.Findings))
	}
}

func TestChecker_Exclusions(t *testing.T) {
	code := `package main

import "errors"

func ignoredFunc() error { return errors.New("ignored") }
func otherFunc() error { return errors.New("other") }

func main() {
	_ = ignoredFunc()
	_ = otherFunc()
}
`
	dir := createTestModule(t, map[string]string{
		"main.go": code,
	})

	opts := Options{
		Dir:       dir,
		APRoot:    dir,
		SkipTests: true,
		Exclude:   []string{"example.com/testmod.ignoredFunc"},
	}

	res, err := Run(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Findings) != 1 {
		t.Fatalf("expected 1 finding (otherFunc), got %d: %+v", len(res.Findings), res.Findings)
	}
}

func TestChecker_FprintlnAndDefaultExclusions(t *testing.T) {
	code := `package main

import (
	"bytes"
	"fmt"
	"os"
)

func main() {
	var buf bytes.Buffer
	_, _ = buf.Write([]byte("hello")) // default exclusion

	_, _ = fmt.Fprintln(os.Stdout, "stdout") // CLI output
}
`
	dir := createTestModule(t, map[string]string{
		"main.go": code,
	})

	// Without fmt.Fprintln in exclude: buf.Write is excluded by default, fmt.Fprintln is reported
	res1, err := Run(t.Context(), Options{
		Dir:       dir,
		APRoot:    dir,
		SkipTests: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res1.Findings) != 1 {
		t.Fatalf("expected 1 finding (fmt.Fprintln), got %d: %+v", len(res1.Findings), res1.Findings)
	}

	// With fmt.Fprintln in exclude: both are excluded, 0 findings
	res2, err := Run(t.Context(), Options{
		Dir:       dir,
		APRoot:    dir,
		SkipTests: true,
		Exclude:   []string{"fmt.Fprintln"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.Findings) != 0 {
		t.Fatalf("expected 0 findings with fmt.Fprintln excluded, got %d: %+v", len(res2.Findings), res2.Findings)
	}
}

func TestChecker_BaselineRatchet(t *testing.T) {
	codeV1 := `package main

import "errors"

func f1() error { return errors.New("1") }
func f2() error { return errors.New("2") }

func main() {
	_ = f1()
	_ = f2()
}
`
	dir := createTestModule(t, map[string]string{
		"main.go": codeV1,
	})

	baselinePath := filepath.Join(dir, ".ap", "droppederrors-baseline.txt")

	// 1. Write baseline
	resWrite, err := Run(t.Context(), Options{
		Dir:           dir,
		APRoot:        dir,
		BaselinePath:  baselinePath,
		WriteBaseline: true,
		SkipTests:     true,
	})
	if err != nil {
		t.Fatalf("write baseline failed: %v", err)
	}
	if len(resWrite.Findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(resWrite.Findings))
	}
	if _, err := os.Stat(baselinePath); err != nil {
		t.Fatalf("baseline file not created: %v", err)
	}

	// 2. Check with baseline in mode error -> should PASS (0 new findings)
	resCheck, err := Run(t.Context(), Options{
		Dir:          dir,
		APRoot:       dir,
		BaselinePath: baselinePath,
		Mode:         "error",
		SkipTests:    true,
	})
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	if len(resCheck.NewFindings) != 0 {
		t.Fatalf("expected 0 new findings, got %d", len(resCheck.NewFindings))
	}
	if len(resCheck.StaleEntries) != 0 {
		t.Fatalf("expected 0 stale entries, got %d", len(resCheck.StaleEntries))
	}

	// 3. Add a new dropped error: f3()
	codeV2 := `package main

import "errors"

func f1() error { return errors.New("1") }
func f2() error { return errors.New("2") }
func f3() error { return errors.New("3") }

func main() {
	_ = f1()
	_ = f2()
	_ = f3()
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(codeV2), 0644); err != nil {
		t.Fatal(err)
	}

	resV2, err := Run(t.Context(), Options{
		Dir:          dir,
		APRoot:       dir,
		BaselinePath: baselinePath,
		Mode:         "error",
		SkipTests:    true,
	})
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	if len(resV2.NewFindings) != 1 {
		t.Fatalf("expected 1 new finding, got %d", len(resV2.NewFindings))
	}

	// 4. Fix f2() without updating baseline -> should produce stale baseline entry
	codeV3 := `package main

import "errors"

func f1() error { return errors.New("1") }
func f2() error { return errors.New("2") }

func main() {
	_ = f1()
	if err := f2(); err != nil {
		// handled
	}
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(codeV3), 0644); err != nil {
		t.Fatal(err)
	}

	resV3, err := Run(t.Context(), Options{
		Dir:          dir,
		APRoot:       dir,
		BaselinePath: baselinePath,
		Mode:         "error",
		SkipTests:    true,
	})
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	if len(resV3.NewFindings) != 0 {
		t.Fatalf("expected 0 new findings, got %d", len(resV3.NewFindings))
	}
	if len(resV3.StaleEntries) != 1 {
		t.Fatalf("expected 1 stale entry, got %d", len(resV3.StaleEntries))
	}
}

func TestChecker_TypeConversion(t *testing.T) {
	code := `package main

type MyErr string

func (m MyErr) Error() string { return string(m) }

func main() {
	_ = MyErr("boom") // type conversion, not a function call
	_ = error(MyErr("boom"))
}
`
	dir := createTestModule(t, map[string]string{
		"main.go": code,
	})

	res, err := Run(t.Context(), Options{
		Dir:       dir,
		APRoot:    dir,
		SkipTests: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) != 0 {
		t.Fatalf("expected 0 findings for type conversions, got %d: %+v", len(res.Findings), res.Findings)
	}
}

func TestChecker_EmbeddedInterfaceExclusion(t *testing.T) {
	code := `package main

import "hash/crc32"

func main() {
	h := crc32.NewIEEE()
	h.Write([]byte("data")) // hash.Hash32 embeds hash.Hash which has Write in default exclusions
}
`
	dir := createTestModule(t, map[string]string{
		"main.go": code,
	})

	res, err := Run(t.Context(), Options{
		Dir:       dir,
		APRoot:    dir,
		SkipTests: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) != 0 {
		t.Fatalf("expected 0 findings for embedded hash.Hash.Write, got %d: %+v", len(res.Findings), res.Findings)
	}
}

func TestChecker_SkipGenerated(t *testing.T) {
	code := `// Code generated by test. DO NOT EDIT.
package main

import "errors"

func fail() error { return errors.New("err") }
func main() {
	_ = fail()
}
`
	dir := createTestModule(t, map[string]string{
		"gen.go": code,
	})

	// 1. SkipGenerated: false (default) -> reported
	res1, err := Run(t.Context(), Options{
		Dir:           dir,
		APRoot:        dir,
		SkipTests:     true,
		SkipGenerated: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res1.Findings) != 1 {
		t.Fatalf("expected 1 finding when SkipGenerated is false, got %d", len(res1.Findings))
	}

	// 2. SkipGenerated: true -> skipped
	res2, err := Run(t.Context(), Options{
		Dir:           dir,
		APRoot:        dir,
		SkipTests:     true,
		SkipGenerated: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.Findings) != 0 {
		t.Fatalf("expected 0 findings when SkipGenerated is true, got %d", len(res2.Findings))
	}
}

func TestChecker_RangeOverFuncIterator(t *testing.T) {
	code := `package main

import "errors"

func iterSeq2(yield func(int, error) bool) {
	yield(1, errors.New("err"))
}

func main() {
	for v := range iterSeq2 { // error not bound
		_ = v
	}
	for v, _ := range iterSeq2 { // error bound to blank
		_ = v
	}
	for v, err := range iterSeq2 { // error handled
		_ = v
		if err != nil {}
	}
}
`
	dir := createTestModule(t, map[string]string{
		"main.go": code,
	})

	res, err := Run(t.Context(), Options{
		Dir:       dir,
		APRoot:    dir,
		SkipTests: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) != 2 {
		t.Fatalf("expected 2 iterator findings, got %d: %+v", len(res.Findings), res.Findings)
	}
}

func TestChecker_Recover(t *testing.T) {
	code := `package main

func main() {
	_ = recover()
}
`
	dir := createTestModule(t, map[string]string{
		"main.go": code,
	})

	res, err := Run(t.Context(), Options{
		Dir:       dir,
		APRoot:    dir,
		SkipTests: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) != 1 {
		t.Fatalf("expected 1 finding for _ = recover(), got %d: %+v", len(res.Findings), res.Findings)
	}
}

func TestChecker_ErrorOnlyCheckedForSuccess(t *testing.T) {
	code := `package main

import "errors"

func mayFail() error { return errors.New("err") }
func multi() (int, error) { return 0, errors.New("err") }

func main() {
	// Case 1: err := mayFail(); if err == nil {}
	err := mayFail()
	if err == nil {
		println("success")
	}

	// Case 2: if err := mayFail(); err == nil {}
	if err2 := mayFail(); err2 == nil {
		println("success 2")
	}

	// Case 3: if v, err3 := multi(); err3 == nil {}
	if _, err3 := multi(); err3 == nil {
		println("success 3")
	}

	// Handled: has else
	err4 := mayFail()
	if err4 == nil {
		println("ok")
	} else {
		println("err", err4.Error())
	}

	// Handled: checked for != nil
	err5 := mayFail()
	if err5 != nil {
		return
	}
}
`
	dir := createTestModule(t, map[string]string{
		"main.go": code,
	})

	res, err := Run(t.Context(), Options{
		Dir:       dir,
		APRoot:    dir,
		SkipTests: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var successChecks []Finding
	for _, f := range res.Findings {
		if f.Category == CategoryErrorOnlyCheckedForSuccess {
			successChecks = append(successChecks, f)
		}
	}
	if len(successChecks) != 3 {
		t.Fatalf("expected 3 success-only checks, got %d: %+v", len(successChecks), successChecks)
	}
}

func TestChecker_PackageLevelClosure(t *testing.T) {
	code := `package main

import "errors"

func fail() error { return errors.New("fail") }

var myWorker = func() {
	_ = fail()
}
`
	dir := createTestModule(t, map[string]string{
		"main.go": code,
	})

	res, err := Run(t.Context(), Options{
		Dir:       dir,
		APRoot:    dir,
		SkipTests: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d: %+v", len(res.Findings), res.Findings)
	}
	if res.Findings[0].EnclosingFunc != "myWorker" {
		t.Errorf("expected enclosing func 'myWorker', got %q", res.Findings[0].EnclosingFunc)
	}
}

func TestChecker_UseDefaultExcludesFalse(t *testing.T) {
	code := `package main

import "bytes"

func main() {
	var buf bytes.Buffer
	_, _ = buf.Write([]byte("hello"))
}
`
	dir := createTestModule(t, map[string]string{
		"main.go": code,
	})

	falseVal := false
	// With UseDefaultExcludes: false -> buf.Write is NOT excluded
	res, err := Run(t.Context(), Options{
		Dir:                dir,
		APRoot:             dir,
		SkipTests:          true,
		UseDefaultExcludes: &falseVal,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) != 1 {
		t.Fatalf("expected 1 finding with UseDefaultExcludes false, got %d: %+v", len(res.Findings), res.Findings)
	}
}
