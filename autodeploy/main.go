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

package main

import (
	"context"
	"flag"
	"os"
	"time"

	"github.com/gke-labs/gke-labs-infra/autodeploy/pkg/controller"
	"github.com/gke-labs/gke-labs-infra/autodeploy/pkg/executor"
	"github.com/gke-labs/gke-labs-infra/autodeploy/pkg/git"
	"github.com/gke-labs/gke-labs-infra/autodeploy/pkg/strategy"
	"k8s.io/klog/v2"
)

func main() {
	klog.InitFlags(nil)
	defer klog.Flush()

	var repoURL string
	var pollInterval time.Duration
	var buildkitAddr string
	var registryAddr string

	flag.StringVar(&repoURL, "repo", "", "URL of the git repository to monitor")
	flag.DurationVar(&pollInterval, "interval", 1*time.Minute, "Polling interval")
	flag.StringVar(&buildkitAddr, "buildkit", "", "Address of the BuildKit endpoint")
	flag.StringVar(&registryAddr, "registry", "", "Address of the self-hosted image registry")
	flag.Parse()

	if repoURL == "" {
		klog.Error("repo flag is required")
		os.Exit(1)
	}

	klog.Infof("Starting autodeploy for repo: %s", repoURL)

	ctrl := &controller.Controller{
		Monitor:  git.NewMonitor(repoURL),
		Strategy: &strategy.AlwaysDeploy{}, // TODO: Make configurable
		Runner:   &executor.APRunner{},     // TODO: Pass buildkit and registry config
	}

	ctx := context.Background()

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(pollInterval):
			if err := ctrl.Reconcile(ctx, repoURL); err != nil {
				klog.Errorf("Reconciliation failed: %v", err)
			}
		}
	}
}
