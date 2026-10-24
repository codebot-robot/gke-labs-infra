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
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/fileutils"
	"k8s.io/klog/v2"
)

type Manager struct {
	dir  string
	done *DoneIndex
	mu   sync.Mutex
}

func NewManager() (*Manager, error) {
	cacheDir, err := fileutils.CacheDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(cacheDir, "codestyle")
	return NewManagerWithDir(dir)
}

func NewManagerWithDir(dir string) (*Manager, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	m := &Manager{
		dir:  dir,
		done: NewDoneIndex(),
	}
	if err := m.load(); err != nil {
		klog.Warningf("falling back to empty codestyle cache: %v", err)
	}
	return m, nil
}

func removeLegacyFile(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		klog.V(2).Infof("failed to remove legacy cache file %s: %v", path, err)
	}
}

func (m *Manager) load() error {
	var errs []error

	// Ignore and delete legacy JSON caches
	removeLegacyFile(filepath.Join(m.dir, "metadata.json"))
	removeLegacyFile(filepath.Join(m.dir, "done.json"))
	removeLegacyFile(filepath.Join(m.dir, "gofmt.json"))

	doneBinPath := filepath.Join(m.dir, "done.bin")
	if data, err := os.ReadFile(doneBinPath); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			klog.V(2).Infof("failed to read done cache %s: %v, starting with empty cache", doneBinPath, err)
		}
		m.done = NewDoneIndex()
	} else if idx, err := LoadDone(data); err != nil {
		klog.V(2).Infof("invalid done cache %s: %v, starting with empty cache", doneBinPath, err)
		m.done = NewDoneIndex()
	} else {
		m.done = idx
	}

	return errors.Join(errs...)
}

func (m *Manager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Clean up legacy JSON caches
	removeLegacyFile(filepath.Join(m.dir, "metadata.json"))
	removeLegacyFile(filepath.Join(m.dir, "done.json"))
	removeLegacyFile(filepath.Join(m.dir, "gofmt.json"))

	doneBinPath := filepath.Join(m.dir, "done.bin")
	if err := m.done.Save(doneBinPath); err != nil {
		return err
	}
	return nil
}

func (m *Manager) IsDone(step Step, hash string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.done == nil {
		return false
	}
	return m.done.IsDone(step, hash)
}

func (m *Manager) MarkDone(step Step, hash string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.done == nil {
		m.done = NewDoneIndex()
	}
	if err := m.done.MarkDone(step, hash); err != nil {
		klog.V(2).Infof("failed to mark done for step %d: %v", step, err)
	}
}
