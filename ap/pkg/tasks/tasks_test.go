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

package tasks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/repo"
)

func TestFindTaskScripts(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ap-tasks-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tasksDir := filepath.Join(tmpDir, "dev", "tasks")
	if err := os.MkdirAll(tasksDir, 0755); err != nil {
		t.Fatal(err)
	}

	scriptPath := filepath.Join(tasksDir, "build-foo")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/bash\necho foo"), 0755); err != nil {
		t.Fatal(err)
	}

	r, err := repo.Open(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	foundTasks, err := FindTaskScripts(r, tmpDir, WithPrefix("build-"))
	if err != nil {
		t.Fatalf("FindTaskScripts() error = %v", err)
	}

	if len(foundTasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(foundTasks))
	}

	ts, ok := foundTasks[0].(*TaskScript)
	if !ok {
		t.Fatalf("expected *TaskScript, got %T", foundTasks[0])
	}

	if ts.Name != "build-foo" {
		t.Errorf("expected name build-foo, got %s", ts.Name)
	}

	if ts.Root != tmpDir {
		t.Errorf("expected Root %s, got %s", tmpDir, ts.Root)
	}

	if _, err := FindTaskScripts(nil, tmpDir); err == nil {
		t.Errorf("expected error when repo is nil, got nil")
	}
}

func TestAPScopeGetRepo(t *testing.T) {
	// Nil repo should return an error
	scope := &APScope{
		Dir: "/some/path",
	}
	if _, err := scope.GetRepo(); err == nil {
		t.Fatalf("expected error from GetRepo() when Repo is nil, got nil")
	}

	// Non-nil repo should return the repo
	tmpDir := t.TempDir()
	r, err := repo.Open(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	scope.Repo = r
	got, err := scope.GetRepo()
	if err != nil {
		t.Fatalf("unexpected error from GetRepo(): %v", err)
	}
	if got != r {
		t.Fatalf("GetRepo() = %v, want %v", got, r)
	}
}

func TestGroupRun_ContinueOnError(t *testing.T) {
	errA := errors.New("error A")
	errC := errors.New("error C")

	var ranA, ranB, ranC bool

	group := &Group{
		Name:            "go-lint-testmodule",
		ContinueOnError: true,
		Tasks: []Task{
			&FuncTask{
				Name: "check-a",
				RunFn: func(ctx context.Context, scope *APScope) error {
					ranA = true
					return errA
				},
			},
			&FuncTask{
				Name: "check-b",
				RunFn: func(ctx context.Context, scope *APScope) error {
					ranB = true
					return nil
				},
			},
			&FuncTask{
				Name: "check-c",
				RunFn: func(ctx context.Context, scope *APScope) error {
					ranC = true
					return errC
				},
			},
		},
	}

	ctx := t.Context()
	scope := &APScope{}
	err := group.Run(ctx, scope)
	if err == nil {
		t.Fatalf("expected error from Group.Run(), got nil")
	}

	// Assert all children ran
	if !ranA {
		t.Errorf("expected child A to run")
	}
	if !ranB {
		t.Errorf("expected child B to run")
	}
	if !ranC {
		t.Errorf("expected child C to run")
	}

	// Assert joined error contains each child's error
	if !errors.Is(err, errA) {
		t.Errorf("expected joined error to contain errA, got %v", err)
	}
	if !errors.Is(err, errC) {
		t.Errorf("expected joined error to contain errC, got %v", err)
	}
}

func TestGroupRun_FailFast(t *testing.T) {
	errA := errors.New("error A")
	errB := errors.New("error B")

	var ranA, ranB bool

	group := &Group{
		Name:            "build-images",
		ContinueOnError: false,
		Tasks: []Task{
			&FuncTask{
				Name: "task-a",
				RunFn: func(ctx context.Context, scope *APScope) error {
					ranA = true
					return errA
				},
			},
			&FuncTask{
				Name: "task-b",
				RunFn: func(ctx context.Context, scope *APScope) error {
					ranB = true
					return errB
				},
			},
		},
	}

	ctx := t.Context()
	scope := &APScope{}
	err := group.Run(ctx, scope)
	if err == nil {
		t.Fatalf("expected error from Group.Run(), got nil")
	}

	if !ranA {
		t.Errorf("expected task A to run")
	}
	if ranB {
		t.Errorf("expected task B not to run when ContinueOnError is false")
	}
	if !errors.Is(err, errA) {
		t.Errorf("expected errA, got %v", err)
	}
}

func TestTasksRun_ContinueOnError(t *testing.T) {
	err1 := errors.New("err 1")
	err2 := errors.New("err 2")

	var ran1, ran2 bool
	ts := []Task{
		&FuncTask{
			Name: "t1",
			RunFn: func(ctx context.Context, scope *APScope) error {
				ran1 = true
				return err1
			},
		},
		&FuncTask{
			Name: "t2",
			RunFn: func(ctx context.Context, scope *APScope) error {
				ran2 = true
				return err2
			},
		},
	}

	ctx := t.Context()
	scope := &APScope{}

	// Test fail-fast
	err := Run(ctx, scope, ts, RunOptions{ContinueOnError: false})
	if !ran1 || ran2 {
		t.Errorf("fail-fast: expected ran1=true, ran2=false, got ran1=%v, ran2=%v", ran1, ran2)
	}
	if !errors.Is(err, err1) {
		t.Errorf("expected err1, got %v", err)
	}

	// Test continue on error
	ran1, ran2 = false, false
	err = Run(ctx, scope, ts, RunOptions{ContinueOnError: true})
	if !ran1 || !ran2 {
		t.Errorf("continue-on-error: expected ran1=true, ran2=true, got ran1=%v, ran2=%v", ran1, ran2)
	}
	if !errors.Is(err, err1) || !errors.Is(err, err2) {
		t.Errorf("expected joined error with err1 and err2, got %v", err)
	}
}

func TestScriptEnv(t *testing.T) {
	tempCache := t.TempDir()
	t.Setenv("AP_CACHE_DIR", tempCache)

	env, err := ScriptEnv()
	if err != nil {
		t.Fatalf("ScriptEnv failed: %v", err)
	}
	expectedBinDir := filepath.Join(tempCache, "bin")

	var foundPath string
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			foundPath = e[5:]
			break
		}
	}
	if foundPath == "" {
		t.Fatal("PATH not found in ScriptEnv")
	}

	parts := strings.Split(foundPath, string(os.PathListSeparator))
	if len(parts) == 0 || parts[0] != expectedBinDir {
		t.Errorf("expected PATH to start with %s, got %v", expectedBinDir, parts)
	}

	binName := "ap"
	if runtime.GOOS == "windows" {
		binName = "ap.exe"
	}
	if _, err := os.Stat(filepath.Join(expectedBinDir, binName)); err != nil {
		t.Errorf("expected %s to exist in isolated bin dir: %v", binName, err)
	}
}

func TestScriptEnv_ErrorOnUnwritableCache(t *testing.T) {
	tempCache := t.TempDir()
	unwritableCache := filepath.Join(tempCache, "readonly")
	if err := os.WriteFile(unwritableCache, []byte("not-a-dir"), 0444); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AP_CACHE_DIR", unwritableCache)

	_, err := ScriptEnv()
	if err == nil {
		t.Fatal("expected error from ScriptEnv when cache dir is unwritable, got nil")
	}
}

func TestTaskScript_ResolvesApOnPath(t *testing.T) {
	tmpDir := t.TempDir()

	binDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	fakeAp := filepath.Join(binDir, "ap")
	if err := os.WriteFile(fakeAp, []byte("#!/bin/sh\necho \"fake-ap-version\"\n"), 0755); err != nil {
		t.Fatal(err)
	}

	scriptFile := filepath.Join(tmpDir, "test.sh")
	outputFile := filepath.Join(tmpDir, "output.txt")
	scriptContent := fmt.Sprintf("#!/bin/sh\nap > %q\n", outputFile)
	if err := os.WriteFile(scriptFile, []byte(scriptContent), 0755); err != nil {
		t.Fatal(err)
	}

	ts := &TaskScript{
		Name: "test-script",
		Path: scriptFile,
		Root: tmpDir,
	}

	tempCache := t.TempDir()
	t.Setenv("AP_CACHE_DIR", tempCache)
	t.Setenv("AP_EXECUTABLE", fakeAp)
	if err := ts.Run(t.Context(), &APScope{Dir: tmpDir, RepoRoot: tmpDir}); err != nil {
		t.Fatalf("task run failed: %v", err)
	}

	out, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	if !strings.Contains(string(out), "fake-ap-version") {
		t.Errorf("unexpected output: %s", string(out))
	}
}
