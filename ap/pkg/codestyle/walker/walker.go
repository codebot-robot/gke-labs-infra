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

package walker

import (
	"os"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/repo"
)

// Filter is a function that returns true if the file should be included.
type Filter func(path string, info os.FileInfo) bool

// Walk walks the directory tree rooted at root and returns a list of files using the shared repo tree.
// It skips paths matched by the ignore list.
// If filter is provided, it only returns files for which filter returns true.
func Walk(r *repo.Repo, root string, ignore *IgnoreList, filter Filter) ([]string, error) {
	return r.Walk(root, ignore, filter)
}

// ExpandPaths expands a list of target paths (which can be files or directories)
// into a list of individual file paths, skipping files/dirs matched by the ignore list.
// If paths is empty, it defaults to repoRoot.
func ExpandPaths(r *repo.Repo, repoRoot string, paths []string, ignore *IgnoreList) ([]string, error) {
	return r.ExpandPaths(repoRoot, paths, ignore)
}
