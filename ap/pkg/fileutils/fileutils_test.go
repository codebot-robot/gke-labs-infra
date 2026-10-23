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

package fileutils

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestFileExists(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Existing directory
	exists, err := FileExists(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error for existing directory: %v", err)
	}
	if !exists {
		t.Errorf("expected true for existing directory, got false")
	}

	// 2. Existing file
	filePath := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	exists, err = FileExists(filePath)
	if err != nil {
		t.Fatalf("unexpected error for existing file: %v", err)
	}
	if !exists {
		t.Errorf("expected true for existing file, got false")
	}

	// 3. Non-existent path
	missingPath := filepath.Join(tmpDir, "nonexistent.txt")
	exists, err = FileExists(missingPath)
	if err != nil {
		t.Fatalf("unexpected error for non-existent path: %v", err)
	}
	if exists {
		t.Errorf("expected false for non-existent path, got true")
	}

	// 4. Permission error (when running as non-root)
	if os.Geteuid() != 0 {
		restrictedDir := filepath.Join(tmpDir, "restricted")
		if err := os.Mkdir(restrictedDir, 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.Chmod(restrictedDir, 0000); err != nil {
			t.Fatalf("failed to chmod dir: %v", err)
		}
		defer os.Chmod(restrictedDir, 0755)

		inaccessiblePath := filepath.Join(restrictedDir, "child.txt")
		exists, err = FileExists(inaccessiblePath)
		if err == nil {
			t.Errorf("expected error for inaccessible path, got nil (exists=%v)", exists)
		} else if errors.Is(err, fs.ErrNotExist) {
			t.Errorf("expected permission error, but error matched fs.ErrNotExist: %v", err)
		}
	}
}

func TestWriteFileAtomic(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "sub", "test.sh")

	// 1. Initial atomic write
	initialData := []byte("#!/bin/bash\necho initial\n")
	if err := WriteFileAtomic(targetPath, initialData, 0755); err != nil {
		t.Fatalf("WriteFileAtomic failed: %v", err)
	}

	read, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if string(read) != string(initialData) {
		t.Errorf("expected %q, got %q", string(initialData), string(read))
	}
	fi, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if fi.Mode().Perm() != 0755 {
		t.Errorf("expected mode 0755, got %v", fi.Mode().Perm())
	}

	// 2. Open file descriptor before overwrite
	f, err := os.Open(targetPath)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	defer f.Close()

	// 3. Overwrite with new content
	newData := []byte("#!/bin/bash\necho new\n")
	if err := WriteFileAtomic(targetPath, newData, 0755); err != nil {
		t.Fatalf("WriteFileAtomic overwrite failed: %v", err)
	}

	// The target file now has new data
	afterRead, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read updated file: %v", err)
	}
	if string(afterRead) != string(newData) {
		t.Errorf("expected %q, got %q", string(newData), string(afterRead))
	}

	// Open file descriptor still sees initial data (inode preserved for open fd)
	buf := make([]byte, len(initialData))
	n, err := f.Read(buf)
	if err != nil {
		t.Fatalf("failed to read from open fd: %v", err)
	}
	if string(buf[:n]) != string(initialData) {
		t.Errorf("expected open fd to see original data %q, got %q", string(initialData), string(buf[:n]))
	}
}
