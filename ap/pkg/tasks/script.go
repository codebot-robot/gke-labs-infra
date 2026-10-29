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
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/fileutils"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/sourcetree"
	"k8s.io/klog/v2"
)

// ensureIsolatedAp copies the current running ap binary into a dedicated cache bin directory
// (e.g. <cacheRoot>/bin/ap) so that other binaries in the same directory as ap are not dragged into PATH.
func ensureIsolatedAp() (string, error) {
	cacheRoot, err := fileutils.CacheDir()
	if err != nil {
		return "", err
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

	apBinDir := filepath.Join(cacheRoot, "bin")
	targetPath := filepath.Join(apBinDir, binName)

	if execPath == targetPath {
		return apBinDir, nil
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
		if dstFi.Size() == srcFi.Size() && dstFi.ModTime().Equal(srcFi.ModTime()) {
			return apBinDir, nil
		}
	}

	if err := fileutils.CopyFile(execPath, targetPath, 0755); err != nil {
		return "", fmt.Errorf("failed to copy ap to isolated bin directory: %w", err)
	}

	return apBinDir, nil
}

// ScriptEnv returns an environment slice with an isolated directory containing only the ap binary
// prepended to PATH so child scripts can invoke ap directly without dragging other binaries into PATH.
func ScriptEnv() ([]string, error) {
	apBinDir, err := ensureIsolatedAp()
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
	env, err := ScriptEnv()
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
