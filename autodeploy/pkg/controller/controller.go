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

package controller

import (
	"context"
	"fmt"

	"github.com/gke-labs/gke-labs-infra/autodeploy/pkg/executor"
	"github.com/gke-labs/gke-labs-infra/autodeploy/pkg/git"
	"github.com/gke-labs/gke-labs-infra/autodeploy/pkg/strategy"
	"k8s.io/klog/v2"
)

// Controller manages the autodeploy reconciliation loop.
type Controller struct {
	Monitor  *git.Monitor
	Strategy strategy.Strategy
	Runner   *executor.APRunner
}

// Reconcile checks for updates and triggers deployments if necessary.
func (c *Controller) Reconcile(ctx context.Context, repoURL string) error {
	klog.Infof("Reconciling %s", repoURL)

	// TODO: Check for new commits
	commit, err := c.Monitor.GetLatestCommit(ctx, "main")
	if err != nil {
		return err
	}

	if commit == "" {
		klog.Info("No commits found or not implemented yet")
		return nil
	}

	// TODO: Track last deployed commit to avoid redeploying the same thing
	shouldDeploy := c.Strategy.ShouldDeploy(commit, "main", nil)

	if shouldDeploy {
		klog.Infof("Triggering deployment for commit %s", commit)
		// TODO: Clone repo to a temporary directory
		// tmpDir, err := os.MkdirTemp("", "autodeploy-*")
		// ...
		// return c.Runner.DeployFlow(ctx, tmpDir)
	}

	fmt.Printf("Checked %s, latest commit: %s\\n", repoURL, commit)
	return nil
}
