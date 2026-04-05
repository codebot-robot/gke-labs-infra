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

package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/images"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/tasks"
	"github.com/spf13/cobra"
)

// BuildOptions holds the configuration for the "build" command.
type BuildOptions struct {
	*RootOptions
	Push         bool
	BuildkitHost string
}

// BuildBuildCommand constructs the cobra command for "build".
func BuildBuildCommand(rootOpt *RootOptions) *cobra.Command {
	opt := BuildOptions{
		RootOptions: rootOpt,
	}

	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build artifacts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return RunBuild(cmd.Context(), opt)
		},
	}

	cmd.Flags().BoolVar(&opt.Push, "push", false, "Push images to registry")
	cmd.Flags().StringVar(&opt.BuildkitHost, "buildkit-host", os.Getenv("BUILDKIT_HOST"), "Buildkit host to use (e.g. k8s://namespace/service)")

	return cmd
}

// RunBuild executes the business logic for the "build" command.
func RunBuild(ctx context.Context, opt BuildOptions) error {
	if err := requireRepoRoot(opt.RootOptions); err != nil {
		return err
	}

	var allTasks []tasks.Task
	for _, apRoot := range opt.APRoots {
		group := &tasks.Group{
			Name: fmt.Sprintf("build-%s", filepath.Base(apRoot)),
		}

		buildkitHost := opt.BuildkitHost
		if buildkitHost == "k8s" {
			buildkitHost = "k8s://autodeploy-system/buildkit"
		}

		imageTasks, err := images.BuildTasks(apRoot, opt.Push, buildkitHost)
		if err != nil {
			return err
		}
		group.Tasks = append(group.Tasks, imageTasks)

		// Run build-* scripts
		buildScripts, err := tasks.FindTaskScripts(apRoot, tasks.WithPrefix("build-"))
		if err != nil {
			return fmt.Errorf("failed to discover build tasks in %s: %w", apRoot, err)
		}
		group.Tasks = append(group.Tasks, buildScripts...)

		allTasks = append(allTasks, group)
	}

	return tasks.Run(ctx, opt.RepoRoot, allTasks, tasks.RunOptions{DryRun: opt.DryRun})
}
