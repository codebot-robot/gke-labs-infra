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

	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/baseline"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/closewriter"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/config"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/fileutils"
	"github.com/spf13/cobra"
)

// CloseWriterOptions holds CLI options for the closewriter command.
type CloseWriterOptions struct {
	*RootOptions
	Mode          string
	Baseline      string
	WriteBaseline bool
	Types         []string
	SkipTests     bool
	SkipGenerated bool
	RepoRoot      string
	SkipGlobs     []string
	GOOS          []string
}

// BuildCloseWriterCommand constructs the cobra command for "closewriter".
func BuildCloseWriterCommand() *cobra.Command {
	opt := CloseWriterOptions{}

	cmd := &cobra.Command{
		Use:   "closewriter [packages...]",
		Short: "Check for unhandled Close() errors on writers",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunCloseWriter(cmd.Context(), cmd, opt, args)
		},
	}

	cmd.Flags().StringVar(&opt.Mode, "mode", "", "Mode: ignore | warn | error (default: error)")
	cmd.Flags().StringVar(&opt.Baseline, "baseline", "", "Path to baseline file")
	cmd.Flags().BoolVar(&opt.WriteBaseline, "write-baseline", false, "Write baseline of current findings")
	cmd.Flags().StringSliceVar(&opt.Types, "type", nil, "Additional writer types to check")
	cmd.Flags().StringSliceVar(&opt.Types, "closewriter.type", nil, "Additional writer types to check")
	cmd.Flags().BoolVar(&opt.SkipTests, "skip-tests", false, "Skip _test.go files")
	cmd.Flags().BoolVar(&opt.SkipTests, "closewriter.skip-tests", false, "Skip _test.go files")
	cmd.Flags().BoolVar(&opt.SkipGenerated, "skip-generated", false, "Skip generated files")
	cmd.Flags().BoolVar(&opt.SkipGenerated, "closewriter.skip-generated", false, "Skip generated files")
	cmd.Flags().StringVar(&opt.RepoRoot, "closewriter.repo-root", "", "Repository root")
	cmd.Flags().StringSliceVar(&opt.SkipGlobs, "closewriter.skip-glob", nil, "Skip file glob patterns")
	cmd.Flags().StringSliceVar(&opt.GOOS, "goos", nil, "Target GOOS list to check")

	return cmd
}

// RunCloseWriter executes the closewriter check with baseline support.
func RunCloseWriter(ctx context.Context, cmd *cobra.Command, opt CloseWriterOptions, args []string) error {
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
		if cfg != nil && cfg.CloseWriter() != nil && cfg.CloseWriter().Mode != "" {
			mode = cfg.CloseWriter().Mode
		} else {
			mode = "error"
		}
	}
	if mode != "error" && mode != "warn" && mode != "ignore" {
		return fmt.Errorf("invalid closewriter mode %q: must be one of 'error', 'warn', 'ignore'", mode)
	}

	baselinePath := opt.Baseline
	if baselinePath == "" && cfg != nil {
		baselinePath = cfg.CloseWriterBaseline()
	}
	if baselinePath == "" {
		defaultBaseline := filepath.Join(apRoot, ".ap", "closewriter-baseline.txt")
		exists, err := fileutils.FileExists(defaultBaseline)
		if err != nil {
			return err
		}
		if exists {
			baselinePath = defaultBaseline
		} else if repoRoot != "" {
			repoBaseline := filepath.Join(repoRoot, ".ap", "closewriter-baseline.txt")
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
	if cmd != nil && !cmd.Flags().Changed("skip-tests") && !cmd.Flags().Changed("closewriter.skip-tests") && cfg != nil && cfg.CloseWriter() != nil {
		skipTests = cfg.ResolveCommonSkipTests(cfg.CloseWriter().Common(), false)
	}

	skipGenerated := opt.SkipGenerated
	if cmd != nil && !cmd.Flags().Changed("skip-generated") && !cmd.Flags().Changed("closewriter.skip-generated") && cfg != nil && cfg.CloseWriter() != nil {
		skipGenerated = cfg.ResolveCommonSkipGenerated(cfg.CloseWriter().Common(), false)
	}

	var typesList []string
	if cfg != nil && cfg.CloseWriter() != nil {
		typesList = append(typesList, cfg.CloseWriter().TypesOrDefault()...)
	}
	typesList = append(typesList, opt.Types...)

	packages := args
	if len(packages) == 0 {
		packages = []string{"./..."}
	}

	var skipGlobs []string
	if cfg != nil {
		skipGlobs = append(skipGlobs, cfg.Skip...)
	}
	skipGlobs = append(skipGlobs, opt.SkipGlobs...)

	dir := cwd

	checkOpts := baseline.Options{
		CheckName:     "closewriter",
		RepoRoot:      repoRoot,
		APRoot:        apRoot,
		Dir:           dir,
		Packages:      packages,
		Mode:          mode,
		BaselinePath:  baselinePath,
		WriteBaseline: opt.WriteBaseline,
		SkipTests:     skipTests,
		SkipGenerated: skipGenerated,
		SkipGlobs:     skipGlobs,
		GOOS:          opt.GOOS,
	}

	res, err := baseline.Run(ctx, closewriter.Analyzer, checkOpts)
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
			return fmt.Errorf("closewriter: %d new unhandled close errors found", len(res.NewFindings))
		}
		return nil
	}

	return nil
}
