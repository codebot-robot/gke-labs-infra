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

	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/any"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/baseline"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/config"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/fileutils"
	"github.com/spf13/cobra"
)

// ReplaceEmptyInterfaceWithAnyOptions holds CLI options for the replace-empty-interface-with-any command.
type ReplaceEmptyInterfaceWithAnyOptions struct {
	*RootOptions
	Mode          string
	Baseline      string
	WriteBaseline bool
	SkipTests     bool
	SkipGenerated bool
	RepoRoot      string
	SkipGlobs     []string
	GOOS          []string
}

// BuildReplaceEmptyInterfaceWithAnyCommand constructs the cobra command for "replace-empty-interface-with-any".
func BuildReplaceEmptyInterfaceWithAnyCommand() *cobra.Command {
	opt := ReplaceEmptyInterfaceWithAnyOptions{
		SkipGenerated: true,
	}

	cmd := &cobra.Command{
		Use:     "replace-empty-interface-with-any [packages...]",
		Short:   "Check for use of interface{} where any could be used",
		Aliases: []string{"any"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunReplaceEmptyInterfaceWithAny(cmd.Context(), cmd, opt, args)
		},
	}

	cmd.Flags().StringVar(&opt.Mode, "mode", "", "Mode: ignore | warn | error (default: error)")
	cmd.Flags().StringVar(&opt.Baseline, "baseline", "", "Path to baseline file")
	cmd.Flags().BoolVar(&opt.WriteBaseline, "write-baseline", false, "Write baseline of current findings")
	cmd.Flags().BoolVar(&opt.SkipTests, "skip-tests", false, "Skip _test.go files")
	cmd.Flags().BoolVar(&opt.SkipTests, "replaceEmptyInterfaceWithAny.skip-tests", false, "Skip _test.go files")
	cmd.Flags().BoolVar(&opt.SkipGenerated, "skip-generated", true, "Skip generated files")
	cmd.Flags().BoolVar(&opt.SkipGenerated, "replaceEmptyInterfaceWithAny.skip-generated", true, "Skip generated files")
	cmd.Flags().StringVar(&opt.RepoRoot, "replaceEmptyInterfaceWithAny.repo-root", "", "Repository root")
	cmd.Flags().StringSliceVar(&opt.SkipGlobs, "replaceEmptyInterfaceWithAny.skip-glob", nil, "Skip file glob patterns")
	cmd.Flags().StringSliceVar(&opt.GOOS, "goos", nil, "Target GOOS list to check")

	return cmd
}

// RunReplaceEmptyInterfaceWithAny executes the replace-empty-interface-with-any check with baseline support.
func RunReplaceEmptyInterfaceWithAny(ctx context.Context, cmd *cobra.Command, opt ReplaceEmptyInterfaceWithAnyOptions, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current working directory: %w", err)
	}

	repoRoot := ""
	apRoot := ""
	if opt.RootOptions != nil {
		repoRoot = opt.RootOptions.RepoRoot
		apRoot = opt.RootOptions.APRoot
	}
	if opt.RepoRoot != "" {
		repoRoot = opt.RepoRoot
	}
	if repoRoot == "" || apRoot == "" {
		rRoot, aRoot, err := findRoots()
		if err != nil {
			if repoRoot == "" {
				repoRoot = cwd
			}
			if apRoot == "" {
				apRoot = cwd
			}
		} else {
			if repoRoot == "" {
				repoRoot = rRoot
			}
			if apRoot == "" {
				apRoot = aRoot
			}
		}
	}
	if apRoot == "" {
		apRoot = cwd
	}
	if repoRoot == "" {
		repoRoot = apRoot
	}

	var cfg *config.Config
	if apRoot != "" {
		c, err := config.Load(apRoot)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		cfg = c
	}
	if cfg == nil && repoRoot != "" {
		c, err := config.Load(repoRoot)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		cfg = c
	}

	mode := opt.Mode
	if mode == "" {
		if cfg != nil && cfg.Lint != nil && cfg.Lint.ReplaceEmptyInterfaceWithAny != nil && cfg.Lint.ReplaceEmptyInterfaceWithAny.Mode != "" {
			mode = cfg.Lint.ReplaceEmptyInterfaceWithAny.Mode
		} else {
			mode = "error"
		}
	}
	if mode != "error" && mode != "warn" && mode != "ignore" {
		return fmt.Errorf("invalid replace-empty-interface-with-any mode %q: must be one of 'error', 'warn', 'ignore'", mode)
	}

	baselinePath := opt.Baseline
	if baselinePath == "" && cfg != nil {
		baselinePath = cfg.ReplaceEmptyInterfaceWithAnyBaseline()
	}
	if baselinePath == "" {
		defaultBaseline := filepath.Join(apRoot, ".ap", "any-baseline.txt")
		exists, err := fileutils.FileExists(defaultBaseline)
		if err != nil {
			return err
		}
		if exists {
			baselinePath = defaultBaseline
		} else if repoRoot != "" {
			repoBaseline := filepath.Join(repoRoot, ".ap", "any-baseline.txt")
			exists, err := fileutils.FileExists(repoBaseline)
			if err != nil {
				return err
			}
			if exists {
				baselinePath = repoBaseline
			}
		}
		if baselinePath == "" && opt.WriteBaseline {
			baselinePath = defaultBaseline
		}
	}
	if baselinePath != "" && !filepath.IsAbs(baselinePath) {
		baselinePath = filepath.Join(apRoot, baselinePath)
	}

	skipTests := opt.SkipTests
	if cmd != nil && !cmd.Flags().Changed("skip-tests") && !cmd.Flags().Changed("replaceEmptyInterfaceWithAny.skip-tests") && cfg != nil {
		skipTests = cfg.ResolveSkipTests(cfg.ReplaceEmptyInterfaceWithAnySkipTests(), false)
	}

	skipGenerated := opt.SkipGenerated
	if cmd != nil && !cmd.Flags().Changed("skip-generated") && !cmd.Flags().Changed("replaceEmptyInterfaceWithAny.skip-generated") && cfg != nil {
		skipGenerated = cfg.ResolveSkipGenerated(cfg.ReplaceEmptyInterfaceWithAnySkipGenerated(), true)
	}

	packages := args
	if len(packages) == 0 {
		packages = []string{"./..."}
	}

	var skipGlobs []string
	if cfg != nil {
		skipGlobs = append(skipGlobs, cfg.Skip...)
	}
	skipGlobs = append(skipGlobs, opt.SkipGlobs...)

	checkOpts := baseline.Options{
		CheckName:     "replaceEmptyInterfaceWithAny",
		RepoRoot:      repoRoot,
		APRoot:        apRoot,
		Dir:           cwd,
		Packages:      packages,
		Mode:          mode,
		BaselinePath:  baselinePath,
		WriteBaseline: opt.WriteBaseline,
		SkipTests:     skipTests,
		SkipGenerated: skipGenerated,
		SkipGlobs:     skipGlobs,
		GOOS:          opt.GOOS,
	}

	res, err := baseline.Run(ctx, any.Analyzer, checkOpts)
	if err != nil {
		return err
	}

	if opt.WriteBaseline {
		fmt.Printf("Wrote %d baseline entries to %s\n", len(res.BaselineEntries), baselinePath)
		return nil
	}

	for _, stale := range res.StaleEntries {
		fmt.Printf("stale baseline entry: %s (remove from baseline)\n", stale)
	}

	if mode == "warn" {
		findingsToReport := res.NewFindings
		if len(res.BaselineEntries) == 0 {
			findingsToReport = res.Findings
		}
		for _, f := range findingsToReport {
			fmt.Printf("%s:%d:%d: [warn] %s\n", f.Pos.Filename, f.Pos.Line, f.Pos.Column, f.Message)
		}
		return nil
	}

	if mode == "error" {
		if len(res.NewFindings) > 0 {
			for _, f := range res.NewFindings {
				fmt.Printf("%s:%d:%d: %s\n", f.Pos.Filename, f.Pos.Line, f.Pos.Column, f.Message)
			}
			return fmt.Errorf("replace-empty-interface-with-any: %d new findings found", len(res.NewFindings))
		}
		return nil
	}

	return nil
}
