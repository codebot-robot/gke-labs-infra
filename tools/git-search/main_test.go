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

package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRunSearch_FetchFailure(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)
	t.Setenv("HOME", tmpDir)

	repoURL := "https://example.invalid/repo.git"
	repoHash := fmt.Sprintf("%x", sha256.Sum256([]byte(repoURL)))

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	repoCacheRoot := filepath.Join(cacheDir, "git-search", "repos")
	barePath := filepath.Join(repoCacheRoot, repoHash)

	// Create an empty bare git repository so it takes the fetch path
	if err := os.MkdirAll(barePath, 0755); err != nil {
		t.Fatal(err)
	}
	initCmd := exec.Command("git", "init", "--bare", barePath)
	if err := initCmd.Run(); err != nil {
		t.Fatalf("failed to init bare git repo: %v", err)
	}

	opt := &options{
		repo: repoURL,
		ref:  "nonexistent-branch",
	}

	err = runSearch(t.Context(), opt, "some-pattern")
	if err == nil {
		t.Fatal("expected error from failed fetch, got nil")
	}
}
