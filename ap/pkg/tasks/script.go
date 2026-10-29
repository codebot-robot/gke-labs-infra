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
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/fileutils"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/sourcetree"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/tool"
	"k8s.io/klog/v2"
)

var ensureScriptBinMu sync.Mutex

func ensureIsolatedApInDir(binDir string) (string, error) {
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", binDir, err)
	}

	execPath := os.Getenv("AP_EXECUTABLE")
	if execPath == "" {
		var err error
		execPath, err = os.Executable()
		if err != nil {
			return "", fmt.Errorf("failed to determine executable path: %w", err)
		}
	}

	binName := "ap"
	if runtime.GOOS == "windows" {
		binName = "ap.exe"
	}

	targetPath := filepath.Join(binDir, binName)

	if execPath == targetPath {
		return binDir, nil
	}

	srcFi, err := os.Stat(execPath)
	if err != nil {
		return "", err
	}

	dstFi, statErr := os.Stat(targetPath)
	if statErr != nil {
		if !os.IsNotExist(statErr) {
			klog.V(2).Infof("failed to stat %s: %v", targetPath, statErr)
		}
	} else {
		if os.SameFile(srcFi, dstFi) || (dstFi.Size() == srcFi.Size() && dstFi.ModTime().Equal(srcFi.ModTime())) {
			return binDir, nil
		}
	}

	if err := fileutils.CopyFile(execPath, targetPath, 0755); err != nil {
		return "", fmt.Errorf("failed to copy ap to isolated bin directory: %w", err)
	}

	return binDir, nil
}

// EnsureScriptBin ensures the ap binary and tool symlinks exist in the target bin directory
// (defaults to ${repoRoot}/.build/bin if repoRoot is provided, or <cacheRoot>/bin otherwise)
// and returns the bin directory path.
func EnsureScriptBin(repoRoot string) (string, error) {
	ensureScriptBinMu.Lock()
	defer ensureScriptBinMu.Unlock()

	var binDir string
	if repoRoot != "" {
		binDir = filepath.Join(repoRoot, ".build", "bin")
	} else {
		cacheRoot, err := fileutils.CacheDir()
		if err != nil {
			return "", err
		}
		binDir = filepath.Join(cacheRoot, "bin")
	}

	if runtime.GOOS == "windows" {
		return ensureIsolatedApInDir(binDir)
	}

	_, err := tool.Install(tool.InstallOptions{
		RepoRoot:  repoRoot,
		TargetDir: binDir,
		Stdout:    io.Discard,
	})
	if err != nil {
		return "", fmt.Errorf("failed to install tools in %s: %w", binDir, err)
	}
	return binDir, nil
}

// ScriptEnvForRepo returns an environment slice with an isolated directory containing ap and tool symlinks
// prepended to PATH so child scripts can invoke ap and tools directly.
// If repoRoot is non-empty, ${repoRoot}/.build/bin is used; otherwise AP_CACHE_DIR/bin is used.
func ScriptEnvForRepo(repoRoot string) ([]string, error) {
	apBinDir, err := EnsureScriptBin(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to isolate ap binary for PATH: %w", err)
	}

	var env []string
	foundPath := false
	for _, e := range os.Environ() {
		if strings.HasPrefix(strings.ToUpper(e), "PATH=") {
			foundPath = true
			env = append(env, "PATH="+apBinDir+string(os.PathListSeparator)+e[5:])
		} else {
			env = append(env, e)
		}
	}
	if !foundPath {
		env = append(env, "PATH="+apBinDir)
	}
	return env, nil
}

// ScriptEnv returns an environment slice with an isolated directory containing only the ap binary
// prepended to PATH so child scripts can invoke ap directly without dragging other binaries into PATH.
func ScriptEnv() ([]string, error) {
	repoRoot := os.Getenv("REPO_ROOT")
	return ScriptEnvForRepo(repoRoot)
}

// TaskScript represents a discoverable task script.
type TaskScript struct {
	Name string
	Path string
	Root string
}

func (t *TaskScript) Run(ctx context.Context, scope *APScope) error {
	klog.Infof("Running task: %s", t.Name)
	cmd := exec.CommandContext(ctx, t.Path)
	cmd.Dir = t.Root
	repoRoot := ""
	if scope != nil && scope.RepoRoot != "" {
		repoRoot = scope.RepoRoot
	} else if t.Root != "" {
		repoRoot = t.Root
	}
	env, err := ScriptEnvForRepo(repoRoot)
	if err != nil {
		return fmt.Errorf("failed to prepare script environment for %s: %w", t.Name, err)
	}
	cmd.Env = env
	cmd.Stdout = scope.GetStdout()
	cmd.Stderr = scope.GetStderr()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("task %s failed: %w", t.Name, err)
	}
	return nil
}

func (t *TaskScript) GetName() string {
	return t.Name
}

func (t *TaskScript) GetChildren() []Task {
	return nil
}

type FindOptions struct {
	Prefix        string
	ExcludePrefix string
}

type FindOption func(*FindOptions)

func WithPrefix(prefix string) FindOption {
	return func(o *FindOptions) {
		o.Prefix = prefix
	}
}

func WithExcludePrefix(prefix string) FindOption {
	return func(o *FindOptions) {
		o.ExcludePrefix = prefix
	}
}

// FindTaskScripts looks for executable scripts in dev/tasks that match the prefix.
func FindTaskScripts(r *sourcetree.SourceTree, root string, opts ...FindOption) ([]Task, error) {
	if r == nil {
		return nil, fmt.Errorf("sourcetree is nil")
	}
	options := FindOptions{}
	for _, o := range opts {
		o(&options)
	}

	tasksDir := filepath.Join(root, "dev", "tasks")
	var tasks []Task
	for _, f := range r.FilesInDir(tasksDir) {
		name := f.Name
		if options.Prefix != "" && !strings.HasPrefix(name, options.Prefix) {
			continue
		}
		if options.ExcludePrefix != "" && strings.HasPrefix(name, options.ExcludePrefix) {
			continue
		}
		tasks = append(tasks, &TaskScript{
			Name: name,
			Path: f.Path,
			Root: root,
		})
	}

	// Sort by name for deterministic order
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].GetName() < tasks[j].GetName()
	})

	return tasks, nil
}
