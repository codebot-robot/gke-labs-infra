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
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/repo"
)

// ResolveTargets resolves directory specifiers to a list of AP root directories.
// Supported syntaxes:
//
//	.     - Current AP root
//	./... - Current AP root and all child AP roots
//	//... - Repository root AP root and all child AP roots
//	//foo - The AP root at repoRoot/foo
func ResolveTargets(r *repo.Repo, repoRoot, currentAPRoot string, args []string) ([]string, error) {
	if len(args) == 0 {
		args = []string{"."}
	}

	var roots []string
	seen := make(map[string]bool)

	for _, arg := range args {
		var resolved []string
		var err error

		switch {
		case arg == ".":
			if currentAPRoot != "" {
				resolved = []string{currentAPRoot}
			}
		case arg == "./...":
			if currentAPRoot != "" {
				resolved, err = FindAllAPRoots(r, currentAPRoot)
				if err != nil {
					return nil, fmt.Errorf("failed to find AP roots in %s: %w", currentAPRoot, err)
				}
				if len(resolved) == 0 {
					resolved = []string{currentAPRoot}
				}
				// If the currentAPRoot itself is an AP root, FindAllAPRoots will include it
				// because FindAllAPRoots includes the root if it has a .ap directory.
			}
		case arg == "//...":
			if repoRoot != "" {
				resolved, err = FindAllAPRoots(r, repoRoot)
				if err != nil {
					return nil, fmt.Errorf("failed to find AP roots in %s: %w", repoRoot, err)
				}
				if len(resolved) == 0 {
					resolved = []string{repoRoot}
				}
			}
		default:
			path := ResolveTargetPath(repoRoot, currentAPRoot, arg)
			if strings.HasSuffix(path, "/...") {
				basePath := strings.TrimSuffix(path, "/...")
				resolved, err = FindAllAPRoots(r, basePath)
				if err != nil {
					return nil, fmt.Errorf("failed to find AP roots in %s: %w", basePath, err)
				}
				if len(resolved) == 0 {
					resolved = []string{basePath}
				}
			} else {
				resolved = []string{path}
			}
		}

		for _, r := range resolved {
			// Clean the path to avoid duplicate entries like /foo/bar and /foo/bar/
			cleanPath := filepath.Clean(r)
			if !seen[cleanPath] {
				roots = append(roots, cleanPath)
				seen[cleanPath] = true
			}
		}
	}

	return roots, nil
}

// ResolveTargetPath converts a target argument (such as "//foo", "./bar", "baz", or absolute path)
// into an absolute filesystem path, stripping any ":label" suffix.
func ResolveTargetPath(repoRoot, currentAPRoot, arg string) string {
	if strings.HasPrefix(arg, "//") {
		pathPart := strings.TrimPrefix(arg, "//")
		if idx := strings.Index(pathPart, ":"); idx != -1 {
			pathPart = pathPart[:idx]
		}
		if pathPart == "" {
			pathPart = "."
		}
		return filepath.Clean(filepath.Join(repoRoot, pathPart))
	}
	if filepath.IsAbs(arg) {
		return filepath.Clean(arg)
	}
	pathPart := arg
	if idx := strings.Index(pathPart, ":"); idx != -1 {
		pathPart = pathPart[:idx]
	}
	base := currentAPRoot
	if base == "" {
		base = repoRoot
	}
	return filepath.Clean(filepath.Join(base, pathPart))
}
