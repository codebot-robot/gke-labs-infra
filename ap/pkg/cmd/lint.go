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

	"github.com/gke-labs/gke-labs-infra/ap/pkg/tasks"
	"github.com/spf13/cobra"
)

// LintOptions holds the configuration for the "lint" command.
type LintOptions struct {
	*RootOptions
	WriteBaseline bool
	Check         string
	Baseline      string
}

// BuildLintCommand constructs the cobra command for "lint".
func BuildLintCommand(rootOpt *RootOptions) *cobra.Command {
	opt := LintOptions{
		RootOptions: rootOpt,
	}

	cmd := &cobra.Command{
		Use:   "lint [path...]",
		Short: "Run linting tasks (vet, govulncheck, prlinter)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := opt.Resolve(args); err != nil {
				return err
			}
			return RunLint(cmd.Context(), opt)
		},
	}

	cmd.Flags().BoolVar(&opt.WriteBaseline, "write-baseline", false, "Write baseline of current findings")
	cmd.Flags().StringVar(&opt.Check, "check", "", "Check to regenerate baseline for (e.g. closewriter, droppederrors)")
	cmd.Flags().StringVar(&opt.Baseline, "baseline", "", "Path to baseline file")

	cmd.AddCommand(BuildUnusedCommand())
	cmd.AddCommand(BuildTestContextCommand())
	cmd.AddCommand(BuildReplaceEmptyInterfaceWithAnyCommand())
	cmd.AddCommand(BuildDroppedErrorsCommand(rootOpt))
	cmd.AddCommand(BuildTestOnlyImportsCommand())
	cmd.AddCommand(BuildCloseWriterCommand())

	return cmd
}

// RunLint executes the business logic for the "lint" command.
func RunLint(ctx context.Context, opt LintOptions) error {
	if opt.WriteBaseline {
		if opt.Check == "" {
			return fmt.Errorf("--write-baseline requires --check=<name> (e.g. --check=closewriter, --check=droppederrors)")
		}
		switch opt.Check {
		case "closewriter":
			return RunCloseWriter(ctx, nil, CloseWriterOptions{
				RootOptions:   opt.RootOptions,
				Baseline:      opt.Baseline,
				WriteBaseline: true,
			}, nil)
		case "droppederrors":
			return RunDroppedErrors(ctx, nil, DroppedErrorsOptions{
				RootOptions:   opt.RootOptions,
				Baseline:      opt.Baseline,
				WriteBaseline: true,
			}, nil)
		case "unused":
			return RunUnused(ctx, nil, UnusedOptions{
				RootOptions:   opt.RootOptions,
				Baseline:      opt.Baseline,
				WriteBaseline: true,
			}, nil)
		case "testonlyimports":
			return RunTestOnlyImports(ctx, nil, TestOnlyImportsOptions{
				RootOptions:   opt.RootOptions,
				Baseline:      opt.Baseline,
				WriteBaseline: true,
			}, nil)
		case "testcontext":
			return RunTestContext(ctx, nil, TestContextOptions{
				RootOptions:   opt.RootOptions,
				Baseline:      opt.Baseline,
				WriteBaseline: true,
			}, nil)
		case "replace-empty-interface-with-any", "any":
			return RunReplaceEmptyInterfaceWithAny(ctx, nil, ReplaceEmptyInterfaceWithAnyOptions{
				RootOptions:   opt.RootOptions,
				Baseline:      opt.Baseline,
				WriteBaseline: true,
			}, nil)
		default:
			return fmt.Errorf("unknown check %q for --write-baseline", opt.Check)
		}
	}

	if err := requireRepoRoot(opt.RootOptions); err != nil {
		return err
	}

	r, err := opt.GetRepo()
	if err != nil {
		return err
	}

	scopes, err := DiscoverScopes(r, opt.RepoRoot, opt.APRoots)
	if err != nil {
		return err
	}

	var allTasks []tasks.Task
	for _, scope := range scopes {
		if len(scope.LintTasks) > 0 {
			group := &tasks.Group{
				Name:            fmt.Sprintf("lint-%s", filepath.Base(scope.Dir)),
				Tasks:           scope.LintTasks,
				ContinueOnError: true,
			}
			allTasks = append(allTasks, group)
		}
	}

	return tasks.Run(ctx, &tasks.APScope{RepoRoot: opt.RepoRoot, Dir: opt.RepoRoot, Repo: r}, allTasks, tasks.RunOptions{DryRun: opt.DryRun, ContinueOnError: true})
}
