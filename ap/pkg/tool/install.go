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

package tool

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/fileutils"
)

// InstallOptions holds options for Install.
type InstallOptions struct {
	RepoRoot   string
	TargetDir  string
	Executable string
	Stdout     io.Writer
}

// DefaultInstallDir returns the default directory for installing tool symlinks (AP_CACHE_DIR/bin).
func DefaultInstallDir() (string, error) {
	cacheRoot, err := fileutils.CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheRoot, "bin"), nil
}

// RepoInstallDir returns the repository-local directory for installing tool symlinks (${repoRoot}/.build/bin).
func RepoInstallDir(repoRoot string) string {
	return filepath.Join(repoRoot, ".build", "bin")
}

// Install creates or refreshes symlinks for all registered tools pointing at the ap executable.
func Install(opt InstallOptions) ([]string, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("ap tool install is not supported on Windows")
	}

	targetDir := opt.TargetDir
	if targetDir == "" {
		repoRoot := opt.RepoRoot
		if repoRoot == "" {
			repoRoot = FindRepoRoot()
		}
		if repoRoot != "" {
			targetDir = RepoInstallDir(repoRoot)
		} else {
			var err error
			targetDir, err = DefaultInstallDir()
			if err != nil {
				return nil, err
			}
		}
	}

	targetDir, err := filepath.Abs(targetDir)
	if err != nil {
		return nil, fmt.Errorf("failed to determine absolute path for %s: %w", targetDir, err)
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", targetDir, err)
	}

	execPath := opt.Executable
	if execPath == "" {
		execPath = os.Getenv("AP_EXECUTABLE")
	}
	if execPath == "" {
		var err error
		execPath, err = os.Executable()
		if err != nil {
			return nil, fmt.Errorf("failed to determine executable path: %w", err)
		}
	}

	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve symlink for executable %s: %w", execPath, err)
	}
	execPath, err = filepath.Abs(execPath)
	if err != nil {
		return nil, fmt.Errorf("failed to determine absolute path for executable: %w", err)
	}

	binName := "ap"
	if runtime.GOOS == "windows" {
		binName = "ap.exe"
	}
	targetApPath := filepath.Join(targetDir, binName)

	if execPath != targetApPath {
		shouldCopy := true
		srcFi, err := os.Stat(execPath)
		if err != nil {
			return nil, fmt.Errorf("failed to stat executable %s: %w", execPath, err)
		}
		dstFi, err := os.Stat(targetApPath)
		if err == nil {
			if os.SameFile(srcFi, dstFi) || (dstFi.Size() == srcFi.Size() && dstFi.ModTime().Equal(srcFi.ModTime())) {
				shouldCopy = false
			}
		}
		if shouldCopy {
			if err := fileutils.CopyFile(execPath, targetApPath, 0755); err != nil {
				return nil, fmt.Errorf("failed to copy ap to %s: %w", targetApPath, err)
			}
		}
	}

	stdout := opt.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}

	var created []string
	for _, t := range All() {
		linkPath := filepath.Join(targetDir, t.Name)

		if err := os.Remove(linkPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to remove existing link %s: %w", linkPath, err)
		}

		if err := os.Symlink(targetApPath, linkPath); err != nil {
			return nil, fmt.Errorf("failed to create symlink %s -> %s: %w", linkPath, targetApPath, err)
		}

		if _, err := fmt.Fprintf(stdout, "%s -> %s\n", linkPath, targetApPath); err != nil {
			return nil, fmt.Errorf("failed to write link output for %s: %w", linkPath, err)
		}
		created = append(created, linkPath)
	}

	return created, nil
}
