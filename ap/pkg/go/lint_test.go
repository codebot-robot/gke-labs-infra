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

package golang

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/config"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/repo"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/tasks"
)

func TestHasGoFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hasgofiles-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name     string
		setup    func(root string) error
		expected bool
	}{
		{
			name: "empty directory",
			setup: func(root string) error {
				return nil
			},
			expected: false,
		},
		{
			name: "direct go file",
			setup: func(root string) error {
				return os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0644)
			},
			expected: true,
		},
		{
			name: "go file in subdirectory",
			setup: func(root string) error {
				dir := filepath.Join(root, "pkg")
				if err := os.Mkdir(dir, 0755); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(dir, "foo.go"), []byte("package pkg"), 0644)
			},
			expected: true,
		},
		{
			name: "go file only in submodule",
			setup: func(root string) error {
				dir := filepath.Join(root, "submod")
				if err := os.Mkdir(dir, 0755); err != nil {
					return err
				}
				if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module submod"), 0644); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(dir, "foo.go"), []byte("package foo"), 0644)
			},
			expected: false,
		},
		{
			name: "go file in root and in submodule",
			setup: func(root string) error {
				if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0644); err != nil {
					return err
				}
				dir := filepath.Join(root, "submod")
				if err := os.Mkdir(dir, 0755); err != nil {
					return err
				}
				if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module submod"), 0644); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(dir, "foo.go"), []byte("package foo"), 0644)
			},
			expected: true,
		},
		{
			name: "non-go files only",
			setup: func(root string) error {
				return os.WriteFile(filepath.Join(root, "README.md"), []byte("# My Project"), 0644)
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := filepath.Join(tmpDir, tt.name)
			if err := os.Mkdir(root, 0755); err != nil {
				t.Fatalf("Failed to create test root: %v", err)
			}
			if err := tt.setup(root); err != nil {
				t.Fatalf("Failed to setup test: %v", err)
			}

			r, err := repo.Open(root)
			if err != nil {
				t.Fatalf("repo.Open failed: %v", err)
			}
			got, err := hasGoFiles(r, root)
			if err != nil {
				t.Errorf("hasGoFiles() error = %v", err)
				return
			}
			if got != tt.expected {
				t.Errorf("hasGoFiles() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestLintTasks_DroppedErrors(t *testing.T) {
	setupModule := func(t *testing.T, apYAML string) (string, *repo.Repo) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.27\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644); err != nil {
			t.Fatal(err)
		}
		var skips []string
		if apYAML != "" {
			apDir := filepath.Join(dir, ".ap")
			if err := os.MkdirAll(apDir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(apDir, "go.yaml"), []byte(apYAML), 0644); err != nil {
				t.Fatal(err)
			}
			cfg, err := config.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			skips = cfg.Skip
		}
		r, err := repo.Open(dir, repo.WithSkip(skips...))
		if err != nil {
			t.Fatal(err)
		}
		return dir, r
	}

	findDroppedTask := func(taskGroup tasks.Task) *DroppedErrorsCheckTask {
		group, ok := taskGroup.(*tasks.Group)
		if !ok {
			return nil
		}
		for _, t := range group.Tasks {
			if modGroup, ok := t.(*tasks.Group); ok {
				for _, sub := range modGroup.Tasks {
					if dt, ok := sub.(*DroppedErrorsCheckTask); ok {
						return dt
					}
				}
			}
		}
		return nil
	}

	t.Run("default_error", func(t *testing.T) {
		dir, r := setupModule(t, "")
		taskGroup, err := LintTasks(r, dir)
		if err != nil {
			t.Fatal(err)
		}
		dt := findDroppedTask(taskGroup)
		if dt == nil {
			t.Fatalf("expected DroppedErrorsCheckTask to be included by default")
		}
		if dt.IsError != true {
			t.Errorf("expected IsError to be true in default error mode")
		}
	})

	t.Run("mode_warn", func(t *testing.T) {
		dir, r := setupModule(t, "lint:\n  droppedErrors:\n    mode: warn\n")
		taskGroup, err := LintTasks(r, dir)
		if err != nil {
			t.Fatal(err)
		}
		dt := findDroppedTask(taskGroup)
		if dt == nil {
			t.Fatalf("expected DroppedErrorsCheckTask to be included")
		}
		if dt.IsError != false {
			t.Errorf("expected IsError to be false in mode warn")
		}
	})

	t.Run("mode_error", func(t *testing.T) {
		dir, r := setupModule(t, "lint:\n  droppedErrors:\n    mode: error\n")
		taskGroup, err := LintTasks(r, dir)
		if err != nil {
			t.Fatal(err)
		}
		dt := findDroppedTask(taskGroup)
		if dt == nil {
			t.Fatalf("expected DroppedErrorsCheckTask to be included")
		}
		if dt.IsError != true {
			t.Errorf("expected IsError to be true in mode error")
		}
	})

	t.Run("mode_ignore", func(t *testing.T) {
		dir, r := setupModule(t, "lint:\n  droppedErrors:\n    mode: ignore\n")
		taskGroup, err := LintTasks(r, dir)
		if err != nil {
			t.Fatal(err)
		}
		dt := findDroppedTask(taskGroup)
		if dt != nil {
			t.Fatalf("expected DroppedErrorsCheckTask to be excluded in mode ignore")
		}
	})
}

func TestLintTasks_SkipGenerated(t *testing.T) {
	setupModule := func(t *testing.T, apYAML string) (string, *repo.Repo) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.27\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644); err != nil {
			t.Fatal(err)
		}
		var skips []string
		if apYAML != "" {
			apDir := filepath.Join(dir, ".ap")
			if err := os.MkdirAll(apDir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(apDir, "go.yaml"), []byte(apYAML), 0644); err != nil {
				t.Fatal(err)
			}
			cfg, err := config.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			skips = cfg.Skip
		}
		r, err := repo.Open(dir, repo.WithSkip(skips...))
		if err != nil {
			t.Fatal(err)
		}
		return dir, r
	}

	findTasks := func(taskGroup tasks.Task) (*DroppedErrorsCheckTask, *AnalyzersTask) {
		group, ok := taskGroup.(*tasks.Group)
		if !ok {
			return nil, nil
		}
		var dt *DroppedErrorsCheckTask
		var at *AnalyzersTask
		for _, t := range group.Tasks {
			if modGroup, ok := t.(*tasks.Group); ok {
				for _, sub := range modGroup.Tasks {
					if d, ok := sub.(*DroppedErrorsCheckTask); ok {
						dt = d
					}
					if a, ok := sub.(*AnalyzersTask); ok {
						at = a
					}
				}
			}
		}
		return dt, at
	}

	t.Run("skipGenerated_true", func(t *testing.T) {
		dir, r := setupModule(t, "lint:\n  skipGenerated: true\n")
		taskGroup, err := LintTasks(r, dir)
		if err != nil {
			t.Fatal(err)
		}
		dt, at := findTasks(taskGroup)
		if dt == nil || at == nil {
			t.Fatalf("expected tasks to be found")
		}
		if !dt.SkipGenerated {
			t.Errorf("expected DroppedErrorsCheckTask.SkipGenerated to be true")
		}
		if !at.SkipGenerated("testcontext") {
			t.Errorf("expected testcontext SkipGenerated to be true")
		}
	})

	t.Run("default_skipGenerated", func(t *testing.T) {
		dir, r := setupModule(t, "")
		taskGroup, err := LintTasks(r, dir)
		if err != nil {
			t.Fatal(err)
		}
		dt, at := findTasks(taskGroup)
		if dt == nil || at == nil {
			t.Fatalf("expected tasks to be found")
		}
		if dt.SkipGenerated {
			t.Errorf("expected DroppedErrorsCheckTask.SkipGenerated to be false by default")
		}
		if at.SkipGenerated("testcontext") {
			t.Errorf("expected testcontext SkipGenerated to be false by default")
		}
	})
}

func findAnalyzersTask(taskGroup tasks.Task) *AnalyzersTask {
	group, ok := taskGroup.(*tasks.Group)
	if !ok {
		return nil
	}
	for _, t := range group.Tasks {
		if modGroup, ok := t.(*tasks.Group); ok {
			for _, sub := range modGroup.Tasks {
				if at, ok := sub.(*AnalyzersTask); ok {
					return at
				}
			}
		}
	}
	return nil
}

func TestLintTasks_PerCheckOverrides(t *testing.T) {
	setupModule := func(t *testing.T, apYAML string) (string, *repo.Repo) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.27\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644); err != nil {
			t.Fatal(err)
		}
		var skips []string
		if apYAML != "" {
			apDir := filepath.Join(dir, ".ap")
			if err := os.MkdirAll(apDir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(apDir, "go.yaml"), []byte(apYAML), 0644); err != nil {
				t.Fatal(err)
			}
			cfg, err := config.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			skips = cfg.Skip
		}
		r, err := repo.Open(dir, repo.WithSkip(skips...))
		if err != nil {
			t.Fatal(err)
		}
		return dir, r
	}

	t.Run("unused_skipGenerated_false_takes_effect", func(t *testing.T) {
		dir, r := setupModule(t, `
lint:
  unused:
    skipGenerated: false
    skipTests: true
`)
		taskGroup, err := LintTasks(r, dir)
		if err != nil {
			t.Fatal(err)
		}
		at := findAnalyzersTask(taskGroup)
		if at == nil {
			t.Fatalf("expected AnalyzersTask to be found")
		}
		if at.SkipGenerated("unused") {
			t.Errorf("expected unused SkipGenerated to be false when configured")
		}
		if !at.SkipTests("unused") {
			t.Errorf("expected unused SkipTests to be true when configured")
		}
	})

	t.Run("testcontext_and_any_overrides_take_effect", func(t *testing.T) {
		dir, r := setupModule(t, `
lint:
  testcontext:
    skipGenerated: true
    skipTests: true
  replaceEmptyInterfaceWithAny:
    skipGenerated: false
    skipTests: true
`)
		taskGroup, err := LintTasks(r, dir)
		if err != nil {
			t.Fatal(err)
		}
		at := findAnalyzersTask(taskGroup)
		if at == nil {
			t.Fatalf("expected AnalyzersTask to be found")
		}
		if !at.SkipGenerated("testcontext") {
			t.Errorf("expected testcontext SkipGenerated to be true")
		}
		if !at.SkipTests("testcontext") {
			t.Errorf("expected testcontext SkipTests to be true")
		}
		if at.SkipGenerated("replaceEmptyInterfaceWithAny") {
			t.Errorf("expected replaceEmptyInterfaceWithAny SkipGenerated to be false")
		}
		if !at.SkipTests("replaceEmptyInterfaceWithAny") {
			t.Errorf("expected replaceEmptyInterfaceWithAny SkipTests to be true")
		}
	})
}

func TestLintTasks_TestOnlyImports(t *testing.T) {
	setupModule := func(t *testing.T, apYAML string) (string, *repo.Repo) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.27\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644); err != nil {
			t.Fatal(err)
		}
		var skips []string
		if apYAML != "" {
			apDir := filepath.Join(dir, ".ap")
			if err := os.MkdirAll(apDir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(apDir, "go.yaml"), []byte(apYAML), 0644); err != nil {
				t.Fatal(err)
			}
			cfg, err := config.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			skips = cfg.Skip
		}
		r, err := repo.Open(dir, repo.WithSkip(skips...))
		if err != nil {
			t.Fatal(err)
		}
		return dir, r
	}

	t.Run("default_enabled_with_error_mode", func(t *testing.T) {
		dir, r := setupModule(t, "")
		taskGroup, err := LintTasks(r, dir)
		if err != nil {
			t.Fatal(err)
		}
		task := findAnalyzersTask(taskGroup)
		if task == nil {
			t.Fatalf("expected AnalyzersTask to be created by default")
		}
		if task.Enabled["testonlyimports"] != codestyle.ModeError {
			t.Errorf("expected ModeError by default, got %v", task.Enabled["testonlyimports"])
		}
		pkgs := task.Packages("testonlyimports")
		if len(pkgs) != 0 {
			t.Errorf("expected empty packages by default (analyzer defaults used), got %d", len(pkgs))
		}
		if task.SkipGenerated("testonlyimports") {
			t.Errorf("expected SkipGenerated to be false by default")
		}
		if task.SkipTests("testonlyimports") {
			t.Errorf("expected SkipTests to be false by default")
		}
	})

	t.Run("configured_warn_mode_and_custom_packages", func(t *testing.T) {
		dir, r := setupModule(t, `
lint:
  testOnlyImports:
    mode: warn
    packages:
      - github.com/google/go-cmp
    skipGenerated: true
    skipTests: true
`)
		taskGroup, err := LintTasks(r, dir)
		if err != nil {
			t.Fatal(err)
		}
		task := findAnalyzersTask(taskGroup)
		if task == nil {
			t.Fatalf("expected AnalyzersTask to be created")
		}
		if task.Enabled["testonlyimports"] != codestyle.ModeWarn {
			t.Errorf("expected ModeWarn in warn mode, got %v", task.Enabled["testonlyimports"])
		}
		pkgs := task.Packages("testonlyimports")
		if len(pkgs) != 1 || pkgs[0] != "github.com/google/go-cmp" {
			t.Errorf("unexpected packages: %v", pkgs)
		}
		if !task.SkipGenerated("testonlyimports") {
			t.Errorf("expected SkipGenerated to be true")
		}
		if !task.SkipTests("testonlyimports") {
			t.Errorf("expected SkipTests to be true")
		}
	})

	t.Run("mode_ignore_disables_task", func(t *testing.T) {
		dir, r := setupModule(t, `
lint:
  testOnlyImports:
    mode: ignore
`)
		taskGroup, err := LintTasks(r, dir)
		if err != nil {
			t.Fatal(err)
		}
		task := findAnalyzersTask(taskGroup)
		if task == nil {
			t.Fatalf("expected AnalyzersTask to be created for other analyzers")
		}
		if _, ok := task.Enabled["testonlyimports"]; ok {
			t.Fatalf("expected testonlyimports to not be enabled when mode is ignore")
		}
	})
}

func TestLintTasks_CloseWriter(t *testing.T) {
	setupModule := func(t *testing.T, yamlContent string) (string, *repo.Repo) {
		t.Helper()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.27\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
			t.Fatal(err)
		}
		var skips []string
		if yamlContent != "" {
			apDir := filepath.Join(dir, ".ap")
			if err := os.Mkdir(apDir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(apDir, "go.yaml"), []byte(yamlContent), 0644); err != nil {
				t.Fatal(err)
			}
			cfg, err := config.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			skips = cfg.Skip
		}
		r, err := repo.Open(dir, repo.WithSkip(skips...))
		if err != nil {
			t.Fatal(err)
		}
		return dir, r
	}

	t.Run("default_enabled_with_error_mode", func(t *testing.T) {
		dir, r := setupModule(t, "")
		taskGroup, err := LintTasks(r, dir)
		if err != nil {
			t.Fatal(err)
		}
		task := findAnalyzersTask(taskGroup)
		if task == nil {
			t.Fatalf("expected AnalyzersTask to be created by default")
		}
		if task.Enabled["closewriter"] != codestyle.ModeError {
			t.Errorf("expected ModeError by default, got %v", task.Enabled["closewriter"])
		}
		types := task.Types("closewriter")
		if len(types) != 0 {
			t.Errorf("expected empty types by default (analyzer defaults used), got %d", len(types))
		}
		if task.SkipGenerated("closewriter") {
			t.Errorf("expected SkipGenerated to be false by default")
		}
		if task.SkipTests("closewriter") {
			t.Errorf("expected SkipTests to be false by default")
		}
	})

	t.Run("configured_types_and_warn_mode", func(t *testing.T) {
		dir, r := setupModule(t, `
lint:
  closeWriter:
    mode: warn
    types:
      - "*bufio.Writer"
    skipGenerated: true
    skipTests: true
`)
		taskGroup, err := LintTasks(r, dir)
		if err != nil {
			t.Fatal(err)
		}
		task := findAnalyzersTask(taskGroup)
		if task == nil {
			t.Fatalf("expected AnalyzersTask to be created")
		}
		if task.Enabled["closewriter"] != codestyle.ModeWarn {
			t.Errorf("expected ModeWarn in warn mode, got %v", task.Enabled["closewriter"])
		}
		types := task.Types("closewriter")
		if len(types) != 1 || types[0] != "*bufio.Writer" {
			t.Errorf("unexpected types: %v", types)
		}
		if !task.SkipGenerated("closewriter") {
			t.Errorf("expected SkipGenerated to be true")
		}
		if !task.SkipTests("closewriter") {
			t.Errorf("expected SkipTests to be true")
		}
	})

	t.Run("mode_ignore_disables_task", func(t *testing.T) {
		dir, r := setupModule(t, `
lint:
  closeWriter:
    mode: ignore
`)
		taskGroup, err := LintTasks(r, dir)
		if err != nil {
			t.Fatal(err)
		}
		task := findAnalyzersTask(taskGroup)
		if task == nil {
			t.Fatalf("expected AnalyzersTask to be created for other analyzers")
		}
		if _, ok := task.Enabled["closewriter"]; ok {
			t.Fatalf("expected closewriter to not be enabled when mode is ignore")
		}
	})
}

func TestLintTasks_ContinueOnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.27\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := repo.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	taskGroup, err := LintTasks(r, dir)
	if err != nil {
		t.Fatal(err)
	}

	outerGroup, ok := taskGroup.(*tasks.Group)
	if !ok {
		t.Fatalf("expected outer group to be *tasks.Group, got %T", taskGroup)
	}
	if !outerGroup.ContinueOnError {
		t.Errorf("expected outer go-lints group to have ContinueOnError: true")
	}

	if len(outerGroup.Tasks) != 1 {
		t.Fatalf("expected 1 module group, got %d", len(outerGroup.Tasks))
	}

	modGroup, ok := outerGroup.Tasks[0].(*tasks.Group)
	if !ok {
		t.Fatalf("expected module task to be *tasks.Group, got %T", outerGroup.Tasks[0])
	}
	if !modGroup.ContinueOnError {
		t.Errorf("expected module group %s to have ContinueOnError: true", modGroup.Name)
	}
}

func TestLintTasks_GovulncheckAndDroppedErrorsBothReport(t *testing.T) {
	errVuln := errors.New("govulncheck failed in /workspace/test: exit status 1")
	errDropped := errors.New("droppederrors check failed in /workspace/test: exit status 1")

	var ranVet, ranVuln, ranUnused, ranTC, ranAny, ranDropped bool

	modGroup := &tasks.Group{
		Name:            "go-lint-test",
		ContinueOnError: true,
		Tasks: []tasks.Task{
			&tasks.DummyTask{
				Name: "go-vet",
				RunFn: func(ctx context.Context, scope *tasks.APScope) error {
					ranVet = true
					return nil
				},
			},
			&tasks.DummyTask{
				Name: "govulncheck",
				RunFn: func(ctx context.Context, scope *tasks.APScope) error {
					ranVuln = true
					return errVuln
				},
			},
			&tasks.DummyTask{
				Name: "unused-check",
				RunFn: func(ctx context.Context, scope *tasks.APScope) error {
					ranUnused = true
					return nil
				},
			},
			&tasks.DummyTask{
				Name: "testcontext-check",
				RunFn: func(ctx context.Context, scope *tasks.APScope) error {
					ranTC = true
					return nil
				},
			},
			&tasks.DummyTask{
				Name: "replace-empty-interface-with-any-check",
				RunFn: func(ctx context.Context, scope *tasks.APScope) error {
					ranAny = true
					return nil
				},
			},
			&tasks.DummyTask{
				Name: "droppederrors-check",
				RunFn: func(ctx context.Context, scope *tasks.APScope) error {
					ranDropped = true
					return errDropped
				},
			},
		},
	}

	outerGroup := &tasks.Group{
		Name:            "go-lints",
		ContinueOnError: true,
		Tasks:           []tasks.Task{modGroup},
	}

	ctx := context.Background()
	scope := &tasks.APScope{}
	err := outerGroup.Run(ctx, scope)
	if err == nil {
		t.Fatalf("expected error from outerGroup.Run(), got nil")
	}

	// Verify both failing checks ran and didn't hide each other
	if !ranVuln {
		t.Errorf("expected govulncheck to run")
	}
	if !ranDropped {
		t.Errorf("expected droppederrors to run despite govulncheck failing")
	}
	if !ranVet || !ranUnused || !ranTC || !ranAny {
		t.Errorf("expected all checks to run")
	}

	// Verify both errors are in the returned joined error
	if !errors.Is(err, errVuln) {
		t.Errorf("expected joined error to contain govulncheck error")
	}
	if !errors.Is(err, errDropped) {
		t.Errorf("expected joined error to contain droppederrors error")
	}
}

func TestLintTasks_PackageLoadError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.27\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Invalid Go file with syntax error that causes package load to fail.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\ninvalid syntax here\n"), 0644); err != nil {
		t.Fatal(err)
	}
	r, err := repo.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	taskGroup, err := LintTasks(r, dir)
	if err != nil {
		t.Fatal(err)
	}

	task := findAnalyzersTask(taskGroup)
	if task == nil {
		t.Fatalf("expected AnalyzersTask to be created")
	}

	ctx := t.Context()
	err = task.Run(ctx, &tasks.APScope{RepoRoot: dir, Dir: dir, Repo: r})
	if err == nil {
		t.Fatalf("expected task.Run to fail on package load error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to load packages") {
		t.Errorf("expected error to mention failed to load packages, got: %v", err)
	}
}
