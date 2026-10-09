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

package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManager_Load_CorruptCache(t *testing.T) {
	tmpDir := t.TempDir()
	// Write invalid JSON to metadata.json and gofmt.json
	if err := os.WriteFile(filepath.Join(tmpDir, "metadata.json"), []byte("invalid json"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "gofmt.json"), []byte("{corrupt"), 0644); err != nil {
		t.Fatal(err)
	}

	m := &Manager{
		dir: tmpDir,
		caches: &Caches{
			Metadata: make(map[string]*FileMetadata),
			Gofmt:    make(map[string]bool),
		},
	}

	err := m.load()
	if err == nil {
		t.Fatal("expected error from load() on corrupt files, got nil")
	}
}

func TestManager_Load_NotExist(t *testing.T) {
	tmpDir := t.TempDir()
	m := &Manager{
		dir: tmpDir,
		caches: &Caches{
			Metadata: make(map[string]*FileMetadata),
			Gofmt:    make(map[string]bool),
		},
	}

	err := m.load()
	if err != nil {
		t.Fatalf("expected nil error when cache files do not exist, got: %v", err)
	}
}
