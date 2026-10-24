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
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"k8s.io/klog/v2"
)

// FileExists reports whether the named file or directory exists.
// It returns false, nil only when errors.Is(err, fs.ErrNotExist).
// All other errors are returned.
func FileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// CacheDir returns the root cache directory for ap tools.
func CacheDir() (string, error) {
	if env := os.Getenv("AP_CACHE_DIR"); env != "" {
		return env, nil
	}
	userCache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine user cache dir: %w", err)
	}
	return filepath.Join(userCache, "ap"), nil
}

// writeAtomic creates a temporary file in filepath.Dir(dst) with pattern ".<base>-*.tmp",
// runs writeFn, sets permissions, closes the file, and renames it to dst.
// On any error, the temporary file is removed.
func writeAtomic(dst string, perm os.FileMode, writeFn func(w *os.File) error) error {
	dstDir := filepath.Dir(dst)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}

	pattern := fmt.Sprintf(".%s-*.tmp", filepath.Base(dst))
	tmpFile, err := os.CreateTemp(dstDir, pattern)
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	var closed bool
	defer func() {
		if !closed {
			if closeErr := tmpFile.Close(); closeErr != nil {
				klog.V(2).Infof("failed to close temporary file %s: %v", tmpName, closeErr)
			}
		}
		if rmErr := os.Remove(tmpName); rmErr != nil && !os.IsNotExist(rmErr) {
			klog.V(2).Infof("failed to remove temporary file %s: %v", tmpName, rmErr)
		}
	}()

	if err := writeFn(tmpFile); err != nil {
		return err
	}

	if err := tmpFile.Chmod(perm); err != nil {
		return err
	}

	closed = true
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close output file %s: %w", tmpName, err)
	}

	if err := os.Rename(tmpName, dst); err != nil {
		return err
	}
	return nil
}

// WriteFileAtomic writes data to path atomically: write to a temp file in the target's
// directory, set the target mode, close (error checked), then rename over the target.
// Any failure removes the temporary file.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	return writeAtomic(path, perm, func(w *os.File) error {
		_, err := w.Write(data)
		return err
	})
}

// CopyFile copies a file from src to dst atomically via a temporary file, preserving permissions and mtime.
func CopyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := in.Close(); closeErr != nil {
			klog.V(2).Infof("failed to close input file %s: %v", src, closeErr)
		}
	}()

	srcFi, err := in.Stat()
	if err != nil {
		return err
	}

	if err := writeAtomic(dst, perm, func(w *os.File) error {
		_, err := io.Copy(w, in)
		return err
	}); err != nil {
		return err
	}

	if chtimesErr := os.Chtimes(dst, srcFi.ModTime(), srcFi.ModTime()); chtimesErr != nil {
		klog.V(2).Infof("failed to set mtime on %s: %v", dst, chtimesErr)
	}
	return nil
}

// PrependPath returns a copy of env with dir prepended to the PATH environment variable.
func PrependPath(env []string, dir string) []string {
	if dir == "" {
		return env
	}
	pathKey := "PATH"
	var existingPath string
	found := false
	var result []string
	for _, entry := range env {
		if len(entry) >= 5 && strings.EqualFold(entry[:5], "PATH=") {
			pathKey = entry[:4]
			existingPath = entry[5:]
			found = true
			continue
		}
		result = append(result, entry)
	}

	newPath := dir
	if found && existingPath != "" {
		newPath = dir + string(os.PathListSeparator) + existingPath
	}
	result = append(result, pathKey+"="+newPath)
	return result
}

// IsExecutable reports whether the file at path exists and is an executable file.
func IsExecutable(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return fi.Mode().Perm()&0111 != 0
}
