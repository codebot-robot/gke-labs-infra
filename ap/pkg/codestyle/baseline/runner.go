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

package baseline

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gke-labs/gke-labs-infra/ap/pkg/codestyle/fileset"
	"github.com/gke-labs/gke-labs-infra/ap/pkg/fileutils"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"
)

// Options holds configuration for running an analyzer with baseline ratchet.
type Options struct {
	CheckName     string
	RepoRoot      string
	APRoot        string
	Dir           string
	Packages      []string
	Mode          string // "ignore", "warn", "error" (default "error")
	BaselinePath  string
	WriteBaseline bool
	SkipTests     bool
	SkipGenerated bool
	SkipGlobs     []string
	GOOS          []string
}

// Result holds the findings and baseline matching results of an analyzer run.
type Result struct {
	CheckName       string
	Findings        []Finding
	NewFindings     []Finding
	BaselineEntries []Entry
	StaleEntries    []Entry
	CheckedFiles    map[string]bool
	BaselinePath    string
}

// Run executes the given analyzer across target GOOS targets with baseline ratchet matching.
func Run(ctx context.Context, analyzer *analysis.Analyzer, opts Options) (*Result, error) {
	if opts.Dir == "" {
		if opts.APRoot != "" {
			opts.Dir = opts.APRoot
		} else if opts.RepoRoot != "" {
			opts.Dir = opts.RepoRoot
		} else {
			var err error
			opts.Dir, err = os.Getwd()
			if err != nil {
				return nil, fmt.Errorf("failed to get current working directory: %w", err)
			}
		}
	}
	if opts.APRoot == "" {
		opts.APRoot = opts.Dir
	}
	if opts.RepoRoot == "" {
		opts.RepoRoot = opts.APRoot
	}
	if len(opts.Packages) == 0 {
		opts.Packages = []string{"./..."}
	}
	if opts.Mode == "" {
		opts.Mode = "error"
	}

	baseline := opts.BaselinePath
	if baseline == "" {
		defaultBaseline := filepath.Join(opts.APRoot, ".ap", opts.CheckName+"-baseline.txt")
		exists, err := fileutils.FileExists(defaultBaseline)
		if err != nil {
			return nil, err
		}
		if exists {
			baseline = defaultBaseline
		} else if opts.RepoRoot != "" {
			repoBaseline := filepath.Join(opts.RepoRoot, ".ap", opts.CheckName+"-baseline.txt")
			exists, err := fileutils.FileExists(repoBaseline)
			if err != nil {
				return nil, err
			}
			if exists {
				baseline = repoBaseline
			}
		}
		if baseline == "" && opts.WriteBaseline {
			baseline = defaultBaseline
		}
	}
	if baseline != "" && !filepath.IsAbs(baseline) {
		baseline = filepath.Join(opts.APRoot, baseline)
	}

	if opts.Mode == "ignore" && !opts.WriteBaseline {
		return &Result{CheckName: opts.CheckName, BaselinePath: baseline}, nil
	}

	goosList := opts.GOOS
	if len(goosList) == 0 {
		host := runtime.GOOS
		if host == "linux" {
			goosList = []string{"linux"}
		} else {
			goosList = []string{host, "linux"}
		}
	}

	allCheckedFiles := make(map[string]bool)
	findingsByKey := make(map[string]Finding)
	var allFindings []Finding

	for _, targetGOOS := range goosList {
		cfg := &packages.Config{
			Context: ctx,
			Mode:    packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedTypes | packages.NeedTypesSizes | packages.NeedSyntax | packages.NeedTypesInfo,
			Tests:   !opts.SkipTests,
			Dir:     opts.Dir,
		}
		if targetGOOS != "" {
			cfg.Env = append(os.Environ(), "GOOS="+targetGOOS)
		}

		pkgs, err := packages.Load(cfg, opts.Packages...)
		if err != nil {
			return nil, fmt.Errorf("failed to load packages in %s (GOOS=%s): %w", opts.Dir, targetGOOS, err)
		}

		for _, pkg := range pkgs {
			if len(pkg.Errors) > 0 {
				return nil, fmt.Errorf("package load/type error in %s: %s", pkg.PkgPath, pkg.Errors[0].Msg)
			}
		}

		policy := fileset.NewPolicy(opts.SkipGenerated, opts.SkipTests, opts.SkipGlobs, opts.RepoRoot)

		for _, pkg := range pkgs {
			for _, file := range pkg.Syntax {
				pos := pkg.Fset.Position(file.Pos())
				if policy.ShouldSkip(pos.Filename, file) {
					continue
				}
				recordCheckedFile(pos.Filename, opts.APRoot, opts.RepoRoot, opts.Dir, allCheckedFiles)
			}
		}

		graph, err := checker.Analyze([]*analysis.Analyzer{analyzer}, pkgs, nil)
		if err != nil {
			return nil, fmt.Errorf("analysis failed in %s: %w", opts.Dir, err)
		}

		for _, root := range graph.Roots {
			for _, diag := range root.Diagnostics {
				pos := root.Package.Fset.Position(diag.Pos)
				astFile := findASTFile(root.Package.Syntax, diag.Pos)
				if policy.ShouldSkip(pos.Filename, astFile) {
					continue
				}

				encFunc := FindEnclosingFunc(astFile, root.Package.Fset, diag.Pos)
				cat, key := ParseCategoryKey(diag.Category)
				if key == "" {
					key = diag.Message
				}
				if cat == "" {
					cat = opts.CheckName
				}

				relFile := relPath(opts.APRoot, pos.Filename)
				repoRel := relPath(opts.RepoRoot, pos.Filename)
				modRel := relPath(opts.Dir, pos.Filename)

				f := Finding{
					Pos:           pos,
					Category:      cat,
					Key:           key,
					Callee:        key,
					EnclosingFunc: encFunc,
					RelFile:       relFile,
					RepoRel:       repoRel,
					ModRel:        modRel,
					Message:       diag.Message,
					Check:         opts.CheckName,
				}

				k := fmt.Sprintf("%s:%d:%s:%s", pos.Filename, pos.Offset, cat, key)
				if _, exists := findingsByKey[k]; !exists {
					findingsByKey[k] = f
					allFindings = append(allFindings, f)
				}
			}
		}
	}

	baselineRoot := opts.RepoRoot
	if baselineRoot == "" {
		baselineRoot = opts.APRoot
	}
	if baseline != "" {
		bDir := filepath.Dir(baseline)
		if filepath.Base(bDir) == ".ap" {
			baselineRoot = filepath.Dir(bDir)
		}
	}

	res := &Result{
		CheckName:    opts.CheckName,
		Findings:     allFindings,
		CheckedFiles: allCheckedFiles,
		BaselinePath: baseline,
	}

	if opts.WriteBaseline {
		var finalEntries []Entry
		for _, f := range allFindings {
			relFile := relPath(baselineRoot, f.Pos.Filename)
			finalEntries = append(finalEntries, Entry{
				File:          relFile,
				EnclosingFunc: f.EnclosingFunc,
				Category:      f.Category,
				Key:           f.Key,
				Callee:        f.Key,
			})
		}
		if baseline != "" {
			if err := WriteBaseline(baseline, opts.CheckName, finalEntries); err != nil {
				return nil, fmt.Errorf("failed to write baseline to %s: %w", baseline, err)
			}
		}
		res.BaselineEntries = finalEntries
		return res, nil
	}

	var baselineEntries []Entry
	if baseline != "" {
		var err error
		baselineEntries, err = LoadBaseline(baseline)
		if err != nil {
			return nil, err
		}
	}
	res.BaselineEntries = baselineEntries

	newFindings, staleEntries := MatchFindings(allFindings, baselineEntries, allCheckedFiles, baselineRoot, opts.CheckName)
	res.NewFindings = newFindings
	res.StaleEntries = staleEntries
	return res, nil
}

func findASTFile(files []*ast.File, pos token.Pos) *ast.File {
	for _, f := range files {
		if f.Pos() <= pos && pos <= f.End() {
			return f
		}
	}
	return nil
}

func recordCheckedFile(path, apRoot, repoRoot, dir string, checkedFiles map[string]bool) {
	checkedFiles[filepath.ToSlash(path)] = true
	if apRoot != "" {
		checkedFiles[relPath(apRoot, path)] = true
	}
	if repoRoot != "" {
		checkedFiles[relPath(repoRoot, path)] = true
	}
	if dir != "" {
		checkedFiles[relPath(dir, path)] = true
	}
}

func relPath(base, target string) string {
	if base == "" {
		return filepath.ToSlash(target)
	}
	rel, err := filepath.Rel(base, target)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(target)
	}
	return filepath.ToSlash(rel)
}
