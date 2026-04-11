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
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestResolveTargets(t *testing.T) {
	tempDir := t.TempDir()

	// Create repo structure
	// repo/
	//   .git/
	//   .ap/
	//   foo/
	//     .ap/
	//     bar/
	//       .ap/
	//   baz/

	repoRoot := filepath.Join(tempDir, "repo")
	os.MkdirAll(filepath.Join(repoRoot, ".git"), 0755)
	os.MkdirAll(filepath.Join(repoRoot, ".ap"), 0755)
	os.MkdirAll(filepath.Join(repoRoot, "foo", ".ap"), 0755)
	os.MkdirAll(filepath.Join(repoRoot, "foo", "bar", ".ap"), 0755)
	os.MkdirAll(filepath.Join(repoRoot, "baz"), 0755)

	tests := []struct {
		name          string
		args          []string
		currentAPRoot string
		expected      []string
	}{
		{
			name:          "no args (defaults to .)",
			args:          []string{},
			currentAPRoot: repoRoot,
			expected:      []string{repoRoot},
		},
		{
			name:          "dot",
			args:          []string{"."},
			currentAPRoot: filepath.Join(repoRoot, "foo"),
			expected:      []string{filepath.Join(repoRoot, "foo")},
		},
		{
			name:          "go syntax recursive from root",
			args:          []string{"./..."},
			currentAPRoot: repoRoot,
			expected: []string{
				repoRoot,
				filepath.Join(repoRoot, "foo"),
				filepath.Join(repoRoot, "foo", "bar"),
			},
		},
		{
			name:          "go syntax recursive from subfolder",
			args:          []string{"./..."},
			currentAPRoot: filepath.Join(repoRoot, "foo"),
			expected: []string{
				filepath.Join(repoRoot, "foo"),
				filepath.Join(repoRoot, "foo", "bar"),
			},
		},
		{
			name:          "bazel syntax recursive",
			args:          []string{"//..."},
			currentAPRoot: filepath.Join(repoRoot, "foo", "bar"),
			expected: []string{
				repoRoot,
				filepath.Join(repoRoot, "foo"),
				filepath.Join(repoRoot, "foo", "bar"),
			},
		},
		{
			name:          "bazel syntax absolute",
			args:          []string{"//foo"},
			currentAPRoot: repoRoot,
			expected: []string{
				filepath.Join(repoRoot, "foo"),
			},
		},
		{
			name:          "go syntax relative",
			args:          []string{"bar"},
			currentAPRoot: filepath.Join(repoRoot, "foo"),
			expected: []string{
				filepath.Join(repoRoot, "foo", "bar"),
			},
		},
		{
			name:          "bazel syntax absolute recursive",
			args:          []string{"//foo/..."},
			currentAPRoot: repoRoot,
			expected: []string{
				filepath.Join(repoRoot, "foo"),
				filepath.Join(repoRoot, "foo", "bar"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveTargets(repoRoot, tt.currentAPRoot, tt.args)
			if err != nil {
				t.Fatalf("ResolveTargets failed: %v", err)
			}

			// we sort to compare
			gotMap := make(map[string]bool)
			for _, g := range got {
				gotMap[g] = true
			}
			expMap := make(map[string]bool)
			for _, e := range tt.expected {
				expMap[e] = true
			}

			if !reflect.DeepEqual(gotMap, expMap) {
				t.Errorf("ResolveTargets() = %v, want %v", got, tt.expected)
			}
		})
	}
}
