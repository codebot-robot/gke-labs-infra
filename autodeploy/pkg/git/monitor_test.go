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

package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"time"
)

func TestMonitor(t *testing.T) {
	ctx := t.Context()

	// 1. Setup a local git repository
	repoPath := t.TempDir()
	repo, err := git.PlainInit(repoPath, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}

	testFile := filepath.Join(repoPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	if _, err := w.Add("test.txt"); err != nil {
		t.Fatalf("failed to add file: %v", err)
	}

	commitHash, err := w.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	// 2. Test Monitor.GetLatestCommit
	monitor := NewMonitor(repoPath)
	latest, err := monitor.GetLatestCommit(ctx, "master")
	if err != nil {
		t.Fatalf("failed to get latest commit: %v", err)
	}

	if latest != commitHash.String() {
		t.Errorf("expected %s, got %s", commitHash.String(), latest)
	}

	// 3. Test Monitor.Clone
	clonePath := t.TempDir()
	if err := monitor.Clone(ctx, "master", clonePath); err != nil {
		t.Fatalf("failed to clone: %v", err)
	}

	// Verify cloned content
	clonedFile := filepath.Join(clonePath, "test.txt")
	content, err := os.ReadFile(clonedFile)
	if err != nil {
		t.Fatalf("failed to read cloned file: %v", err)
	}

	if string(content) != "hello" {
		t.Errorf("expected 'hello', got '%s'", string(content))
	}
}
