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

package config

import (
	"sort"
	"sync"
)

var (
	linterMu       sync.RWMutex
	linterRegistry = make(map[string]func() LinterConfig)
)

// RegisterLinter registers a constructor for a linter's config type.
func RegisterLinter(name string, newConfig func() LinterConfig) {
	linterMu.Lock()
	defer linterMu.Unlock()
	if newConfig == nil {
		newConfig = func() LinterConfig { return &CommonLinterConfig{} }
	}
	linterRegistry[name] = newConfig
}

// RegisteredLinters returns a sorted list of registered linter names.
func RegisteredLinters() []string {
	linterMu.RLock()
	defer linterMu.RUnlock()
	names := make([]string, 0, len(linterRegistry))
	for name := range linterRegistry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// RegisteredLinterConstructor returns the constructor for the named linter, or nil if not registered.
func RegisteredLinterConstructor(name string) func() LinterConfig {
	linterMu.RLock()
	defer linterMu.RUnlock()
	return linterRegistry[name]
}
