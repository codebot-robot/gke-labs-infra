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

package git

import (
	"context"
	"fmt"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
)

// Monitor watches a git repository for changes.
type Monitor struct {
	RepoURL string
	// TODO: Add fields for auth, branches to watch, etc.
}

// NewMonitor creates a new Git monitor.
func NewMonitor(repoURL string) *Monitor {
	return &Monitor{
		RepoURL: repoURL,
	}
}

// GetLatestCommit returns the latest commit hash for the given branch.
func (m *Monitor) GetLatestCommit(ctx context.Context, branch string) (string, error) {
	rem := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		Name: "origin",
		URLs: []string{m.RepoURL},
	})

	refs, err := rem.ListContext(ctx, &git.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list remote refs: %w", err)
	}

	branchRef := fmt.Sprintf("refs/heads/%s", branch)
	for _, ref := range refs {
		if ref.Name().String() == branchRef {
			return ref.Hash().String(), nil
		}
	}

	return "", fmt.Errorf("branch %s not found", branch)
}

// Clone clones the repository to the given directory.
func (m *Monitor) Clone(ctx context.Context, branch string, dir string) error {
	_, err := git.PlainCloneContext(ctx, dir, false, &git.CloneOptions{
		URL:           m.RepoURL,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch:  true,
		Depth:         1,
	})
	if err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}
	return nil
}
