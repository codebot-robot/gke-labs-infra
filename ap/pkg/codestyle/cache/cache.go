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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/fileutils"
	"k8s.io/klog/v2"
)

type Caches struct {
	Metadata map[string]*FileMetadata `json:"metadata"`
}

type Manager struct {
	dir    string
	caches *Caches
	done   *DoneIndex
	mu     sync.Mutex
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
		dir: dir,
		caches: &Caches{
			Metadata: make(map[string]*FileMetadata),
		},
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
	removeLegacyFile(filepath.Join(m.dir, "done.json"))
	removeLegacyFile(filepath.Join(m.dir, "gofmt.json"))

	metaPath := filepath.Join(m.dir, "metadata.json")
	if data, err := os.ReadFile(metaPath); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			klog.Warningf("failed to read metadata cache %s: %v", metaPath, err)
			errs = append(errs, fmt.Errorf("failed to read metadata cache %s: %w", metaPath, err))
		}
	} else if err := json.Unmarshal(data, &m.caches.Metadata); err != nil {
		klog.Warningf("failed to parse metadata cache %s: %v", metaPath, err)
		errs = append(errs, fmt.Errorf("failed to parse metadata cache %s: %w", metaPath, err))
	}

	doneBinPath := filepath.Join(m.dir, "done.bin")
	if data, err := os.ReadFile(doneBinPath); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			klog.Warningf("failed to read done cache %s: %v", doneBinPath, err)
			errs = append(errs, fmt.Errorf("failed to read done cache %s: %w", doneBinPath, err))
		}
	} else if idx, err := LoadDone(data); err != nil {
		klog.Warningf("failed to parse done cache %s: %v", doneBinPath, err)
		errs = append(errs, fmt.Errorf("failed to parse done cache %s: %w", doneBinPath, err))
	} else {
		m.done = idx
	}

	return errors.Join(errs...)
}

func (m *Manager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Clean up legacy JSON caches
	removeLegacyFile(filepath.Join(m.dir, "done.json"))
	removeLegacyFile(filepath.Join(m.dir, "gofmt.json"))

	metaPath := filepath.Join(m.dir, "metadata.json")
	metaData, err := json.MarshalIndent(m.caches.Metadata, "", "  ")
	if err != nil {
		return err
	}
	if err := fileutils.WriteFileAtomic(metaPath, metaData, 0644); err != nil {
		return err
	}

	doneBinPath := filepath.Join(m.dir, "done.bin")
	if err := m.done.Save(doneBinPath); err != nil {
		return err
	}
	return nil
}

// GetOrUpdateMetadata returns the FileMetadata with Hash populated.
// If the file on disk matches the cached metadata (Size, Mtime, Inode), the cached Hash is used.
// Otherwise, the file is read and hashed, and the cache is updated.
func (m *Manager) GetOrUpdateMetadata(path string) (*FileMetadata, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get current stat
	current, err := GetMetadata(path)
	if err != nil {
		return nil, err
	}

	cached, ok := m.caches.Metadata[path]
	if ok && cached.Size == current.Size && cached.Mtime == current.Mtime && cached.Inode == current.Inode {
		return cached, nil
	}

	// Hash the file
	hash, err := hashFile(path)
	if err != nil {
		return nil, err
	}
	current.Hash = hash
	m.caches.Metadata[path] = current
	return current, nil
}

func (m *Manager) IsDone(step, hash string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.done == nil {
		return false
	}
	return m.done.IsDone(step, hash)
}

func (m *Manager) MarkDone(step, hash string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.done == nil {
		m.done = NewDoneIndex()
	}
	if err := m.done.MarkDone(step, hash); err != nil {
		klog.V(2).Infof("failed to mark done for step %s: %v", step, err)
	}
}

func hashFile(path string) (retHash string, retErr error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := f.Close(); err != nil && retErr == nil {
			retErr = err
		}
	}()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
