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
	"os"
	"path/filepath"
	"testing"
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

	foundTasks, err := FindTaskScripts(tmpDir, WithPrefix("build-"))
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
}
