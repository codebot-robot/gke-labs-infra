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
	"path/filepath"
	"strings"

	golang "github.com/gke-labs/gke-labs-infra/ap/pkg/go"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/images"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/tasks"
	"github.com/spf13/cobra"
)

// LsOptions holds the configuration for the "ls" command.
type LsOptions struct {
	*RootOptions
}

// BuildLsCommand constructs the cobra command for "ls".
func BuildLsCommand(rootOpt *RootOptions) *cobra.Command {
	opt := LsOptions{
		RootOptions: rootOpt,
	}

	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List AP roots",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return RunLs(cmd.Context(), opt)
		},
	}

	return cmd
}

// RunLs executes the business logic for the "ls" command.
func RunLs(ctx context.Context, opt LsOptions) error {
	if err := requireRepoRoot(opt.RootOptions); err != nil {
		return err
	}

	for _, apRoot := range opt.APRoots {
		rel, err := filepath.Rel(opt.RepoRoot, apRoot)
		name := rel
		if err != nil {
			name = apRoot
		} else if rel == "." {
			name = "(repo root)"
		}
		fmt.Printf("%s\n", name)

		// Discover tasks
		allTasks, err := discoverTasks(apRoot)
		if err != nil {
			fmt.Printf("  Error discovering tasks: %v\n", err)
			continue
		}

		for _, task := range allTasks {
			printTask(task, 1)
		}
	}

	return nil
}

func discoverTasks(root string) ([]tasks.Task, error) {
	var all []tasks.Task

	// Build tasks
	buildTasks, err := images.BuildTasks(root, false, "")
	if err == nil && buildTasks != nil {
		all = append(all, buildTasks)
	}

	// Test tasks
	goTestTasks, err := golang.TestTasks(root)
	if err == nil && goTestTasks != nil {
		all = append(all, goTestTasks)
	}

	testScripts, err := tasks.FindTaskScripts(root, tasks.WithPrefix("test-"), tasks.WithExcludePrefix("test-e2e"))
	if err == nil && len(testScripts) > 0 {
		all = append(all, &tasks.Group{Name: "test-scripts", Tasks: testScripts})
	}

	// E2e tasks
	e2eScripts, err := tasks.FindTaskScripts(root, tasks.WithPrefix("test-e2e"))
	if err == nil && len(e2eScripts) > 0 {
		all = append(all, &tasks.Group{Name: "e2e-scripts", Tasks: e2eScripts})
	}

	// Build scripts
	buildScripts, err := tasks.FindTaskScripts(root, tasks.WithPrefix("build-"))
	if err == nil && len(buildScripts) > 0 {
		all = append(all, &tasks.Group{Name: "build-scripts", Tasks: buildScripts})
	}

	return all, nil
}

func printTask(t tasks.Task, indent int) {
	fmt.Printf("%s- %s\n", strings.Repeat("  ", indent), t.GetName())
	for _, child := range t.GetChildren() {
		printTask(child, indent+1)
	}
}
